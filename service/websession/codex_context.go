package websession

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"

	"code-kanban/model/tables"
)

const (
	codexConfigFileName           = "config.toml"
	codexRuntimeConfigCacheTTL    = 5 * time.Minute
	codexBinaryCapabilityCacheTTL = 5 * time.Second
	codexModelCatalogCacheTTL     = 5 * time.Minute
	codexModelCatalogTimeout      = 3 * time.Second
)

var (
	goalModeMinCodexVersion     = semver.MustParse("0.133.0")
	multiAgentV2MinCodexVersion = semver.MustParse("0.146.0")
)

var codexVersionPattern = regexp.MustCompile(`\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?`)

type codexContextWindowCache struct {
	path      string
	expiresAt time.Time
	config    CodexRuntimeConfig
	loaded    bool
}

type codexBinaryCapabilityCache struct {
	expiresAt time.Time
	config    CodexRuntimeConfig
	loaded    bool
}

type codexModelCatalogCache struct {
	expiresAt time.Time
	models    []CodexModelInfo
	loaded    bool
}

type codexContextWindowResolver struct {
	mu     sync.Mutex
	cache  codexContextWindowCache
	bins   codexBinaryCapabilityCache
	models codexModelCatalogCache
}

type CodexModelInfo struct {
	Model                     string            `json:"model"`
	DisplayName               string            `json:"displayName"`
	DefaultReasoningEffort    ReasoningEffort   `json:"defaultReasoningEffort"`
	SupportedReasoningEfforts []ReasoningEffort `json:"supportedReasoningEfforts"`
}

type AgentPermissionModeCapability struct {
	ID        string `json:"id"`
	Available bool   `json:"available"`
}

type AgentCapability struct {
	Installed                bool                            `json:"installed"`
	Version                  *string                         `json:"version,omitempty"`
	SupportsWebSession       bool                            `json:"supportsWebSession"`
	SupportsTree             bool                            `json:"supportsTree"`
	SupportsImages           bool                            `json:"supportsImages"`
	SupportsCompaction       bool                            `json:"supportsCompaction"`
	SupportsSteer            bool                            `json:"supportsSteer"`
	SupportsFollowUp         bool                            `json:"supportsFollowUp"`
	SupportsGoal             bool                            `json:"supportsGoal"`
	SupportsSubAgentRegistry bool                            `json:"supportsSubAgentRegistry"`
	PermissionModes          []AgentPermissionModeCapability `json:"permissionModes"`
}

type PiModelInfo struct {
	Provider      string   `json:"provider"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Reasoning     bool     `json:"reasoning"`
	Input         []string `json:"input"`
	ContextWindow int64    `json:"contextWindow"`
	MaxTokens     int64    `json:"maxTokens"`
}

type WebSessionRuntimeConfig struct {
	Agents              map[Agent]AgentCapability `json:"agents"`
	Model               string                    `json:"model,omitempty"`
	ContextWindowTokens int64                     `json:"contextWindowTokens"`
	CompactLimitTokens  int64                     `json:"compactLimitTokens"`
	Source              ContextWindowSource       `json:"source"`
	Models              []CodexModelInfo          `json:"models"`
	PiModels            []PiModelInfo             `json:"piModels"`
	// Legacy top-level fields are retained for one compatibility cycle.
	HasCodex             bool    `json:"hasCodex"`
	HasClaudeCode        bool    `json:"hasClaudeCode"`
	CodexVersion         *string `json:"codexVersion,omitempty"`
	HasPi                bool    `json:"hasPi"`
	PiVersion            *string `json:"piVersion,omitempty"`
	SupportsPiWebSession bool    `json:"supportsPiWebSession"`
	PiRPCCompatible      bool    `json:"piRpcCompatible"`
	PiMinVersion         string  `json:"piMinVersion"`
	PiDiagnostics        string  `json:"piDiagnostics,omitempty"`
	// SupportsWebSession reports whether ordinary Codex web sessions can run.
	SupportsWebSession   bool   `json:"supportsWebSession"`
	WebSessionMinVersion string `json:"webSessionMinCodexVersion"`
	// SupportsMultiAgentV2 gates only the V2 collaboration and rollout features.
	SupportsMultiAgentV2   bool   `json:"supportsMultiAgentV2"`
	MultiAgentV2MinVersion string `json:"multiAgentV2MinCodexVersion"`
	SupportsGoalMode       bool   `json:"supportsGoalMode"`
	GoalModeMinVersion     string `json:"goalModeMinCodexVersion"`
}

// CodexRuntimeConfig remains an alias while callers migrate to the provider-neutral name.
type CodexRuntimeConfig = WebSessionRuntimeConfig

type CodexSkillSource string

const (
	CodexSkillSourceUser    CodexSkillSource = "user"
	CodexSkillSourceSystem  CodexSkillSource = "system"
	CodexSkillSourceBundled CodexSkillSource = "bundled"
)

type CodexSkillSummary struct {
	Name          string           `json:"name"`
	DisplayName   string           `json:"displayName"`
	Description   string           `json:"description"`
	DefaultPrompt string           `json:"defaultPrompt"`
	Source        CodexSkillSource `json:"source"`
}

func (m *Manager) mapSessionSummary(record tables.WebSessionTable) SessionSummary {
	summary := mapSessionRecord(record)
	summary.ActiveCallTimeoutEnabled = m.effectiveActiveCallTimeoutEnabled(record)
	m.decorateSessionSummary(&summary)
	return summary
}

func (m *Manager) decorateSessionSummary(summary *SessionSummary) {
	if summary == nil {
		return
	}
	if normalizeAgent(summary.Agent) == AgentClaude {
		// Claude reports its authoritative context window on result.modelUsage.
		// Preserve that observed value; there is no local Claude model catalog
		// from which to infer an unknown window safely before the first result.
		if summary.ContextWindowTokens != nil &&
			*summary.ContextWindowTokens > 0 &&
			summary.ContextWindowSource == ContextWindowSourceSessionUsage {
			return
		}
		summary.ContextWindowTokens = nil
		summary.ContextWindowSource = ContextWindowSourceUnavailable
		return
	}
	if normalizeAgent(summary.Agent) == AgentPi {
		if summary.ContextWindowTokens != nil &&
			*summary.ContextWindowTokens > 0 &&
			summary.ContextWindowSource == ContextWindowSourceSessionUsage {
			return
		}
		summary.ContextWindowTokens = nil
		summary.ContextWindowSource = ContextWindowSourceUnavailable
		return
	}
	if normalizeAgent(summary.Agent) != AgentCodex {
		summary.ContextWindowTokens = nil
		summary.ContextWindowSource = ContextWindowSourceUnavailable
		return
	}
	if summary.ContextWindowTokens != nil &&
		*summary.ContextWindowTokens > 0 &&
		summary.ContextWindowSource == ContextWindowSourceSessionUsage {
		return
	}
	config := m.GetCodexRuntimeConfig()
	if config.ContextWindowTokens > 0 && sameCodexModel(summary.Model, config.Model) {
		summary.ContextWindowTokens = ptr(config.ContextWindowTokens)
		summary.ContextWindowSource = config.Source
		return
	}
	summary.ContextWindowTokens = nil
	summary.ContextWindowSource = ContextWindowSourceUnavailable
}

func (m *Manager) GetCodexRuntimeConfig() CodexRuntimeConfig {
	defaultConfig := CodexRuntimeConfig{
		Source:                 ContextWindowSourceUnavailable,
		Models:                 []CodexModelInfo{},
		HasCodex:               false,
		HasClaudeCode:          false,
		SupportsWebSession:     false,
		WebSessionMinVersion:   "",
		SupportsMultiAgentV2:   false,
		MultiAgentV2MinVersion: multiAgentV2MinCodexVersion.String(),
		SupportsGoalMode:       false,
		GoalModeMinVersion:     goalModeMinCodexVersion.String(),
	}
	defaultConfig.Agents = runtimeAgentCapabilities(defaultConfig)
	if m == nil {
		return defaultConfig
	}
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		return m.applyCodexRuntimeCapabilities(defaultConfig)
	}

	configPath := filepath.Join(homeDir, ".codex", codexConfigFileName)

	m.codexContextWindow.mu.Lock()
	cached := m.codexContextWindow.cache
	if cached.loaded && cached.path == configPath && time.Now().Before(cached.expiresAt) {
		m.codexContextWindow.mu.Unlock()
		return m.applyCodexRuntimeCapabilities(cached.config)
	}
	m.codexContextWindow.mu.Unlock()

	raw, err := os.ReadFile(configPath)
	config := defaultConfig
	if err == nil {
		config.Model, _ = parseCodexConfigString(string(raw), "model")
		contextWindowTokens, hasContextWindow := parseCodexConfigInt(string(raw), "model_context_window")
		compactLimitTokens, hasCompactLimit := parseCodexConfigInt(string(raw), "model_auto_compact_token_limit")
		if hasContextWindow {
			config.ContextWindowTokens = contextWindowTokens
			config.Source = ContextWindowSourceConfig
		}
		if hasCompactLimit {
			config.CompactLimitTokens = compactLimitTokens
			config.Source = ContextWindowSourceConfig
		} else if hasContextWindow {
			config.CompactLimitTokens = contextWindowTokens
		}
	}

	m.codexContextWindow.mu.Lock()
	m.codexContextWindow.cache = codexContextWindowCache{
		path:      configPath,
		expiresAt: time.Now().Add(codexRuntimeConfigCacheTTL),
		config:    config,
		loaded:    true,
	}
	m.codexContextWindow.mu.Unlock()

	return m.applyCodexRuntimeCapabilities(config)
}

func (m *Manager) applyCodexRuntimeCapabilities(config CodexRuntimeConfig) CodexRuntimeConfig {
	config = m.applyBinaryCapabilities(config)
	if config.Models == nil {
		config.Models = []CodexModelInfo{}
	}
	config.Agents = runtimeAgentCapabilities(config)
	return config
}

func availablePermissionModes(unrestricted, approval, sandbox bool) []AgentPermissionModeCapability {
	return []AgentPermissionModeCapability{
		{ID: "unrestricted", Available: unrestricted},
		{ID: "approval", Available: approval},
		{ID: "sandbox", Available: sandbox},
	}
}

func runtimeAgentCapabilities(config WebSessionRuntimeConfig) map[Agent]AgentCapability {
	return map[Agent]AgentCapability{
		AgentClaude: {
			Installed:                config.HasClaudeCode,
			SupportsWebSession:       config.HasClaudeCode,
			SupportsImages:           true,
			SupportsCompaction:       true,
			SupportsSteer:            true,
			SupportsFollowUp:         true,
			SupportsSubAgentRegistry: false,
			PermissionModes:          availablePermissionModes(true, true, false),
		},
		AgentCodex: {
			Installed:                config.HasCodex,
			Version:                  config.CodexVersion,
			SupportsWebSession:       config.SupportsWebSession,
			SupportsImages:           true,
			SupportsCompaction:       true,
			SupportsSteer:            true,
			SupportsFollowUp:         true,
			SupportsGoal:             config.SupportsGoalMode,
			SupportsSubAgentRegistry: config.SupportsMultiAgentV2,
			PermissionModes:          availablePermissionModes(true, true, true),
		},
		AgentPi: {
			Installed:                config.HasPi,
			Version:                  config.PiVersion,
			SupportsWebSession:       config.SupportsPiWebSession,
			SupportsTree:             config.SupportsPiWebSession,
			SupportsImages:           true,
			SupportsCompaction:       config.SupportsPiWebSession,
			SupportsSteer:            config.SupportsPiWebSession,
			SupportsFollowUp:         config.SupportsPiWebSession,
			SupportsGoal:             false,
			SupportsSubAgentRegistry: false,
			PermissionModes:          availablePermissionModes(true, false, false),
		},
	}
}

func (m *Manager) GetWebSessionRuntimeConfig() WebSessionRuntimeConfig {
	return m.applyPiRuntimeCapabilities(m.GetCodexRuntimeConfig())
}

func (m *Manager) SupportsPiSessionTree() bool {
	if m == nil {
		return false
	}
	return m.GetWebSessionRuntimeConfig().Agents[AgentPi].SupportsTree
}

func (m *Manager) GetWebSessionRuntimeConfigWithModels() WebSessionRuntimeConfig {
	config := m.GetWebSessionRuntimeConfig()
	if config.HasCodex {
		config.Models = m.getCodexModelCatalog()
	}
	return config
}

// GetCodexRuntimeConfigWithModels is kept for callers using the previous API name.
func (m *Manager) GetCodexRuntimeConfigWithModels() CodexRuntimeConfig {
	config := m.GetCodexRuntimeConfig()
	if config.HasCodex {
		config.Models = m.getCodexModelCatalog()
	}
	return config
}

func (m *Manager) applyBinaryCapabilities(config CodexRuntimeConfig) CodexRuntimeConfig {
	config.WebSessionMinVersion = ""
	config.MultiAgentV2MinVersion = multiAgentV2MinCodexVersion.String()
	config.GoalModeMinVersion = goalModeMinCodexVersion.String()
	if m == nil {
		return config
	}
	now := time.Now()
	m.codexContextWindow.mu.Lock()
	cached := m.codexContextWindow.bins
	if cached.loaded && now.Before(cached.expiresAt) {
		result := config
		result.HasCodex = cached.config.HasCodex
		result.HasClaudeCode = cached.config.HasClaudeCode
		result.CodexVersion = cached.config.CodexVersion
		result.SupportsWebSession = cached.config.SupportsWebSession
		result.WebSessionMinVersion = cached.config.WebSessionMinVersion
		result.SupportsMultiAgentV2 = cached.config.SupportsMultiAgentV2
		result.MultiAgentV2MinVersion = cached.config.MultiAgentV2MinVersion
		result.SupportsGoalMode = cached.config.SupportsGoalMode
		result.GoalModeMinVersion = cached.config.GoalModeMinVersion
		m.codexContextWindow.mu.Unlock()
		return result
	}
	m.codexContextWindow.mu.Unlock()

	hasCodex := hasExecutable(m.cfg.CodexPath)
	hasClaude := hasExecutable(m.cfg.ClaudePath)
	codexVersion := (*string)(nil)
	supportsMultiAgentV2 := false
	supportsGoalMode := false
	if hasCodex {
		if version := detectCodexVersion(m.cfg.CodexPath); version != nil {
			copied := *version
			codexVersion = &copied
			supportsMultiAgentV2 = codexVersionAtLeast(copied, multiAgentV2MinCodexVersion)
			supportsGoalMode = codexVersionAtLeast(copied, goalModeMinCodexVersion)
		}
	}

	binaryConfig := CodexRuntimeConfig{
		HasCodex:               hasCodex,
		HasClaudeCode:          hasClaude,
		CodexVersion:           codexVersion,
		SupportsWebSession:     hasCodex,
		WebSessionMinVersion:   "",
		SupportsMultiAgentV2:   supportsMultiAgentV2,
		MultiAgentV2MinVersion: multiAgentV2MinCodexVersion.String(),
		SupportsGoalMode:       supportsGoalMode,
		GoalModeMinVersion:     goalModeMinCodexVersion.String(),
	}

	m.codexContextWindow.mu.Lock()
	m.codexContextWindow.bins = codexBinaryCapabilityCache{
		expiresAt: now.Add(codexBinaryCapabilityCacheTTL),
		config:    binaryConfig,
		loaded:    true,
	}
	m.codexContextWindow.mu.Unlock()

	config.HasCodex = hasCodex
	config.HasClaudeCode = hasClaude
	config.CodexVersion = codexVersion
	config.SupportsWebSession = hasCodex
	config.SupportsMultiAgentV2 = supportsMultiAgentV2
	config.SupportsGoalMode = supportsGoalMode
	return config
}

func (m *Manager) getCodexModelCatalog() []CodexModelInfo {
	if m == nil {
		return []CodexModelInfo{}
	}
	now := time.Now()
	m.codexContextWindow.mu.Lock()
	cached := m.codexContextWindow.models
	if cached.loaded && now.Before(cached.expiresAt) {
		models := append([]CodexModelInfo(nil), cached.models...)
		m.codexContextWindow.mu.Unlock()
		return models
	}
	m.codexContextWindow.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), codexModelCatalogTimeout)
	defer cancel()
	models, err := loadCodexModelCatalog(ctx, m.cfg.CodexPath)
	if err != nil {
		models = []CodexModelInfo{}
	}

	m.codexContextWindow.mu.Lock()
	m.codexContextWindow.models = codexModelCatalogCache{
		expiresAt: now.Add(codexModelCatalogCacheTTL),
		models:    append([]CodexModelInfo(nil), models...),
		loaded:    true,
	}
	m.codexContextWindow.mu.Unlock()
	return models
}

func loadCodexModelCatalog(ctx context.Context, codexPath string) ([]CodexModelInfo, error) {
	client, stderr, err := startCodexAppServer(ctx, codexPath, "")
	if err != nil {
		return nil, err
	}
	go func() {
		_, _ = io.Copy(io.Discard, stderr)
	}()
	defer stopCodexAppServerProbe(client)

	if _, err := client.request(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "codekanban-runtime-config",
			"version": "0.0.0",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	}); err != nil {
		return nil, err
	}

	type reasoningOption struct {
		ReasoningEffort string `json:"reasoningEffort"`
	}
	type modelRecord struct {
		Model                     string            `json:"model"`
		DisplayName               string            `json:"displayName"`
		DefaultReasoningEffort    string            `json:"defaultReasoningEffort"`
		SupportedReasoningEfforts []reasoningOption `json:"supportedReasoningEfforts"`
	}
	type modelListResponse struct {
		Data       []modelRecord `json:"data"`
		NextCursor *string       `json:"nextCursor"`
	}

	models := make([]CodexModelInfo, 0)
	cursor := ""
	for page := 0; page < 20; page++ {
		params := map[string]any{
			"includeHidden": true,
			"limit":         100,
		}
		if cursor != "" {
			params["cursor"] = cursor
		}
		response, err := client.request(ctx, "model/list", params)
		if err != nil {
			return nil, err
		}
		var result modelListResponse
		if err := json.Unmarshal(response.Result, &result); err != nil {
			return nil, err
		}
		for _, record := range result.Data {
			modelName := strings.TrimSpace(record.Model)
			if modelName == "" {
				continue
			}
			efforts := make([]ReasoningEffort, 0, len(record.SupportedReasoningEfforts))
			seen := make(map[ReasoningEffort]struct{}, len(record.SupportedReasoningEfforts))
			for _, option := range record.SupportedReasoningEfforts {
				effort := ReasoningEffort(strings.ToLower(strings.TrimSpace(option.ReasoningEffort)))
				if effort == "" {
					continue
				}
				if _, exists := seen[effort]; exists {
					continue
				}
				seen[effort] = struct{}{}
				efforts = append(efforts, effort)
			}
			models = append(models, CodexModelInfo{
				Model:                     modelName,
				DisplayName:               strings.TrimSpace(record.DisplayName),
				DefaultReasoningEffort:    ReasoningEffort(strings.ToLower(strings.TrimSpace(record.DefaultReasoningEffort))),
				SupportedReasoningEfforts: efforts,
			})
		}
		if result.NextCursor == nil || strings.TrimSpace(*result.NextCursor) == "" {
			break
		}
		cursor = strings.TrimSpace(*result.NextCursor)
	}
	return models, nil
}

func stopCodexAppServerProbe(client *codexAppServerClient) {
	if client == nil || client.cmd == nil {
		return
	}
	_ = client.closeStdin()
	done := make(chan struct{})
	go func() {
		_ = client.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-time.After(250 * time.Millisecond):
		killCmdTree(client.cmd)
		<-done
	}
}

func sameCodexModel(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right)) &&
		strings.TrimSpace(left) != ""
}

func hasExecutable(command string) bool {
	parts := splitCommandParts(command)
	if len(parts) == 0 {
		return false
	}
	_, err := exec.LookPath(parts[0])
	return err == nil
}

func detectCodexVersion(command string) *string {
	parts := splitCommandParts(command)
	if len(parts) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, parts[0], append(parts[1:], "--version")...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}
	match := codexVersionPattern.FindString(string(output))
	if strings.TrimSpace(match) == "" {
		return nil
	}
	version := strings.TrimSpace(match)
	return &version
}

func splitCommandParts(command string) []string {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return nil
	}

	parts := make([]string, 0, 4)
	var current strings.Builder
	var quote byte
	flush := func() {
		if current.Len() == 0 {
			return
		}
		parts = append(parts, current.String())
		current.Reset()
	}
	for i := 0; i < len(trimmed); i++ {
		char := trimmed[i]
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteByte(char)
			}
			continue
		}
		switch char {
		case '"', '\'':
			quote = char
		case ' ', '\t', '\r', '\n':
			flush()
		default:
			current.WriteByte(char)
		}
	}
	if quote != 0 {
		return nil
	}
	flush()
	return parts
}

func codexVersionAtLeast(raw string, min *semver.Version) bool {
	if min == nil {
		return true
	}
	version, err := semver.NewVersion(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return !version.LessThan(min)
}

func parseCodexContextWindow(raw string) (int64, bool) {
	return parseCodexConfigInt(raw, "model_context_window")
}

func parseCodexConfigInt(raw string, keyName string) (int64, bool) {
	currentSection := ""
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(stripTOMLComment(line))
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			currentSection = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			continue
		}
		if currentSection != "" {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) != keyName {
			continue
		}
		parsed, err := strconv.ParseInt(strings.ReplaceAll(strings.TrimSpace(value), "_", ""), 10, 64)
		if err != nil || parsed <= 0 {
			return 0, false
		}
		return parsed, true
	}
	return 0, false
}

func parseCodexConfigString(raw string, keyName string) (string, bool) {
	currentSection := ""
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(stripTOMLComment(line))
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			currentSection = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			continue
		}
		if currentSection != "" {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok || strings.TrimSpace(key) != keyName {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) < 2 {
			return "", false
		}
		switch value[0] {
		case '"':
			parsed, err := strconv.Unquote(value)
			if err != nil || strings.TrimSpace(parsed) == "" {
				return "", false
			}
			return strings.TrimSpace(parsed), true
		case '\'':
			if value[len(value)-1] != '\'' {
				return "", false
			}
			parsed := strings.TrimSpace(value[1 : len(value)-1])
			return parsed, parsed != ""
		default:
			return "", false
		}
	}
	return "", false
}

func stripTOMLComment(line string) string {
	inSingle := false
	inDouble := false
	for i, r := range line {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}
