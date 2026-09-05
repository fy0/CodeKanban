package websession

import (
	"context"
	"encoding/json"
	"errors"
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
	"go.uber.org/zap"

	"code-kanban/model/tables"
)

const (
	codexConfigFileName           = "config.toml"
	codexRuntimeConfigCacheTTL    = 5 * time.Minute
	codexBinaryCapabilityCacheTTL = 5 * time.Minute
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
	config    WebSessionRuntimeConfig
	loaded    bool
}

type codexBinaryCapabilityCache = runtimeCapabilityCache[WebSessionRuntimeConfig]

type codexModelCatalogCache = runtimeCapabilityCache[[]CodexModelInfo]

type codexContextWindowResolver struct {
	mu     sync.RWMutex
	cache  codexContextWindowCache
	bins   codexBinaryCapabilityCache
	models codexModelCatalogCache
}

type runtimeCapabilityProbeHooks struct {
	codexBinary func() (WebSessionRuntimeConfig, error)
	codexModels func() ([]CodexModelInfo, error)
	pi          func() (piRuntimeProbeResult, error)
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
	Agents                 map[Agent]AgentCapability `json:"agents"`
	CapabilitiesRefreshing bool                      `json:"capabilitiesRefreshing"`
	Model                  string                    `json:"model,omitempty"`
	ContextWindowTokens    int64                     `json:"contextWindowTokens"`
	CompactLimitTokens     int64                     `json:"compactLimitTokens"`
	Source                 ContextWindowSource       `json:"source"`
	Models                 []CodexModelInfo          `json:"models"`
	PiModels               []PiModelInfo             `json:"piModels"`
	HasCodex               bool                      `json:"hasCodex"`
	HasClaudeCode          bool                      `json:"hasClaudeCode"`
	CodexVersion           *string                   `json:"codexVersion,omitempty"`
	HasPi                  bool                      `json:"hasPi"`
	PiVersion              *string                   `json:"piVersion,omitempty"`
	SupportsPiWebSession   bool                      `json:"supportsPiWebSession"`
	PiRPCCompatible        bool                      `json:"piRpcCompatible"`
	PiMinVersion           string                    `json:"piMinVersion"`
	PiDiagnostics          string                    `json:"piDiagnostics,omitempty"`
	// SupportsWebSession reports whether ordinary Codex web sessions can run.
	SupportsWebSession   bool   `json:"supportsWebSession"`
	WebSessionMinVersion string `json:"webSessionMinCodexVersion"`
	// SupportsMultiAgentV2 gates only the V2 collaboration and rollout features.
	SupportsMultiAgentV2   bool   `json:"supportsMultiAgentV2"`
	MultiAgentV2MinVersion string `json:"multiAgentV2MinCodexVersion"`
	SupportsGoalMode       bool   `json:"supportsGoalMode"`
	GoalModeMinVersion     string `json:"goalModeMinCodexVersion"`
}

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
	return m.mapSessionSummaryWithContext(record, m.cachedCodexSessionContextConfig())
}

type codexSessionContextConfig struct {
	model               string
	contextWindowTokens int64
	source              ContextWindowSource
}

func (m *Manager) mapSessionSummaryWithContext(
	record tables.WebSessionTable,
	contextConfig codexSessionContextConfig,
) SessionSummary {
	summary := mapSessionRecord(record)
	summary.ActiveCallTimeoutEnabled = m.effectiveActiveCallTimeoutEnabled(record)
	decorateSessionSummaryWithContext(&summary, contextConfig)
	return summary
}

func (m *Manager) decorateSessionSummary(summary *SessionSummary) {
	decorateSessionSummaryWithContext(summary, m.cachedCodexSessionContextConfig())
}

func decorateSessionSummaryWithContext(summary *SessionSummary, config codexSessionContextConfig) {
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
	if config.contextWindowTokens > 0 && sameCodexModel(summary.Model, config.model) {
		summary.ContextWindowTokens = ptr(config.contextWindowTokens)
		summary.ContextWindowSource = config.source
		return
	}
	summary.ContextWindowTokens = nil
	summary.ContextWindowSource = ContextWindowSourceUnavailable
}

func defaultCodexRuntimeConfig() WebSessionRuntimeConfig {
	config := WebSessionRuntimeConfig{
		Source:                 ContextWindowSourceUnavailable,
		Models:                 []CodexModelInfo{},
		PiModels:               []PiModelInfo{},
		HasCodex:               false,
		HasClaudeCode:          false,
		SupportsWebSession:     false,
		WebSessionMinVersion:   "",
		SupportsMultiAgentV2:   false,
		MultiAgentV2MinVersion: multiAgentV2MinCodexVersion.String(),
		SupportsGoalMode:       false,
		GoalModeMinVersion:     goalModeMinCodexVersion.String(),
	}
	config.Agents = runtimeAgentCapabilities(config)
	return config
}

func (m *Manager) cachedCodexSessionContextConfig() codexSessionContextConfig {
	if m == nil {
		return codexSessionContextConfig{source: ContextWindowSourceUnavailable}
	}
	m.codexContextWindow.mu.RLock()
	cached := m.codexContextWindow.cache
	m.codexContextWindow.mu.RUnlock()
	if !cached.loaded {
		return codexSessionContextConfig{source: ContextWindowSourceUnavailable}
	}
	return codexSessionContextConfig{
		model:               cached.config.Model,
		contextWindowTokens: cached.config.ContextWindowTokens,
		source:              cached.config.Source,
	}
}

func (m *Manager) loadCodexContextConfig(force bool) WebSessionRuntimeConfig {
	defaultConfig := defaultCodexRuntimeConfig()
	if m == nil {
		return defaultConfig
	}
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		return defaultConfig
	}

	configPath := filepath.Join(homeDir, ".codex", codexConfigFileName)

	m.codexContextWindow.mu.RLock()
	cached := m.codexContextWindow.cache
	m.codexContextWindow.mu.RUnlock()
	if !force && cached.loaded && cached.path == configPath && time.Now().Before(cached.expiresAt) {
		return cached.config
	}

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

	return config
}

func (m *Manager) GetCodexRuntimeConfig() WebSessionRuntimeConfig {
	return m.getCodexRuntimeConfig(false)
}

func (m *Manager) getCodexRuntimeConfig(force bool) WebSessionRuntimeConfig {
	config := m.loadCodexContextConfig(force)
	return m.applyCodexRuntimeCapabilitiesWithRefresh(config, force)
}

func (m *Manager) applyCodexRuntimeCapabilitiesWithRefresh(
	config WebSessionRuntimeConfig,
	force bool,
) WebSessionRuntimeConfig {
	config = m.applyBinaryCapabilities(config, force)
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
	return m.getWebSessionRuntimeConfig(false)
}

func (m *Manager) getWebSessionRuntimeConfig(force bool) WebSessionRuntimeConfig {
	return m.applyPiRuntimeCapabilitiesWithRefresh(m.getCodexRuntimeConfig(force), force)
}

func (m *Manager) SupportsPiSessionTree() bool {
	if m == nil {
		return false
	}
	return m.GetWebSessionRuntimeConfig().Agents[AgentPi].SupportsTree
}

func (m *Manager) GetWebSessionRuntimeConfigWithModels() WebSessionRuntimeConfig {
	return m.getWebSessionRuntimeConfigWithModelsBackground()
}

func (m *Manager) RefreshWebSessionRuntimeConfigWithModels() WebSessionRuntimeConfig {
	return m.getWebSessionRuntimeConfigWithModels(true)
}

func (m *Manager) getWebSessionRuntimeConfigWithModels(force bool) WebSessionRuntimeConfig {
	config := m.getWebSessionRuntimeConfig(force)
	if config.HasCodex {
		config.Models = m.getCodexModelCatalog(force)
	}
	return config
}

func (m *Manager) getWebSessionRuntimeConfigWithModelsBackground() WebSessionRuntimeConfig {
	config := m.loadCodexContextConfig(false)
	var binaryRefreshing bool
	config, binaryRefreshing = m.applyBinaryCapabilitiesBackground(config)
	var piRefreshing bool
	config, piRefreshing = m.applyPiRuntimeCapabilitiesBackground(config)
	modelsRefreshing := false
	if config.HasCodex {
		config.Models, modelsRefreshing = m.getCodexModelCatalogBackground()
	} else if config.Models == nil {
		config.Models = []CodexModelInfo{}
	}
	config.CapabilitiesRefreshing = binaryRefreshing || piRefreshing || modelsRefreshing
	config.Agents = runtimeAgentCapabilities(config)
	return config
}

func (m *Manager) applyBinaryCapabilities(config WebSessionRuntimeConfig, force bool) WebSessionRuntimeConfig {
	config.WebSessionMinVersion = ""
	config.MultiAgentV2MinVersion = multiAgentV2MinCodexVersion.String()
	config.GoalModeMinVersion = goalModeMinCodexVersion.String()
	if m == nil {
		return config
	}
	binaryConfig := m.codexContextWindow.bins.get(
		force,
		runtimeCapabilityCachePolicy{successTTL: codexBinaryCapabilityCacheTTL},
		cloneCodexBinaryConfig,
		m.probeCodexBinaryCapabilities,
	)
	return mergeCodexBinaryCapabilities(config, binaryConfig)
}

func (m *Manager) applyBinaryCapabilitiesBackground(config WebSessionRuntimeConfig) (WebSessionRuntimeConfig, bool) {
	config.WebSessionMinVersion = ""
	config.MultiAgentV2MinVersion = multiAgentV2MinCodexVersion.String()
	config.GoalModeMinVersion = goalModeMinCodexVersion.String()
	if m == nil {
		return config, false
	}
	binaryConfig, refreshing := m.codexContextWindow.bins.getBackground(
		runtimeCapabilityCachePolicy{successTTL: codexBinaryCapabilityCacheTTL},
		cloneCodexBinaryConfig,
		m.probeCodexBinaryCapabilities,
	)
	return mergeCodexBinaryCapabilities(config, binaryConfig), refreshing
}

func mergeCodexBinaryCapabilities(config, binaryConfig WebSessionRuntimeConfig) WebSessionRuntimeConfig {
	config.HasCodex = binaryConfig.HasCodex
	config.HasClaudeCode = binaryConfig.HasClaudeCode
	config.CodexVersion = binaryConfig.CodexVersion
	config.SupportsWebSession = binaryConfig.SupportsWebSession
	config.WebSessionMinVersion = binaryConfig.WebSessionMinVersion
	config.SupportsMultiAgentV2 = binaryConfig.SupportsMultiAgentV2
	config.MultiAgentV2MinVersion = binaryConfig.MultiAgentV2MinVersion
	config.SupportsGoalMode = binaryConfig.SupportsGoalMode
	config.GoalModeMinVersion = binaryConfig.GoalModeMinVersion
	return config
}

func (m *Manager) probeCodexBinaryCapabilities() (result WebSessionRuntimeConfig, probeErr error) {
	startedAt := time.Now()
	defer func() {
		m.logRuntimeCapabilityProbe(
			"codex_binary",
			startedAt,
			probeErr,
			zap.Bool("codexInstalled", result.HasCodex),
			zap.Bool("claudeInstalled", result.HasClaudeCode),
			zap.Bool("versionDetected", result.CodexVersion != nil),
		)
	}()
	if m.runtimeCapabilityProbes.codexBinary != nil {
		return m.runtimeCapabilityProbes.codexBinary()
	}
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
		} else {
			probeErr = errors.New("failed to detect Codex version")
		}
	}

	return WebSessionRuntimeConfig{
		HasCodex:               hasCodex,
		HasClaudeCode:          hasClaude,
		CodexVersion:           codexVersion,
		SupportsWebSession:     hasCodex,
		WebSessionMinVersion:   "",
		SupportsMultiAgentV2:   supportsMultiAgentV2,
		MultiAgentV2MinVersion: multiAgentV2MinCodexVersion.String(),
		SupportsGoalMode:       supportsGoalMode,
		GoalModeMinVersion:     goalModeMinCodexVersion.String(),
	}, probeErr
}

func cloneCodexBinaryConfig(config WebSessionRuntimeConfig) WebSessionRuntimeConfig {
	cloned := config
	if config.CodexVersion != nil {
		version := *config.CodexVersion
		cloned.CodexVersion = &version
	}
	return cloned
}

func (m *Manager) getCodexModelCatalog(force bool) []CodexModelInfo {
	if m == nil {
		return []CodexModelInfo{}
	}
	return m.codexContextWindow.models.get(
		force,
		runtimeCapabilityCachePolicy{successTTL: codexModelCatalogCacheTTL},
		cloneCodexModelCatalog,
		m.probeCodexModelCatalog,
	)
}

func (m *Manager) getCodexModelCatalogBackground() ([]CodexModelInfo, bool) {
	if m == nil {
		return []CodexModelInfo{}, false
	}
	return m.codexContextWindow.models.getBackground(
		runtimeCapabilityCachePolicy{successTTL: codexModelCatalogCacheTTL},
		cloneCodexModelCatalog,
		m.probeCodexModelCatalog,
	)
}

func (m *Manager) probeCodexModelCatalog() (models []CodexModelInfo, probeErr error) {
	startedAt := time.Now()
	defer func() {
		m.logRuntimeCapabilityProbe(
			"codex_models",
			startedAt,
			probeErr,
			zap.Int("modelCount", len(models)),
		)
	}()
	if m.runtimeCapabilityProbes.codexModels != nil {
		return m.runtimeCapabilityProbes.codexModels()
	}
	ctx, cancel := context.WithTimeout(context.Background(), codexModelCatalogTimeout)
	defer cancel()
	models, err := loadCodexModelCatalog(ctx, m.cfg.CodexPath)
	if err != nil {
		return []CodexModelInfo{}, err
	}
	return models, nil
}

func cloneCodexModelCatalog(models []CodexModelInfo) []CodexModelInfo {
	cloned := make([]CodexModelInfo, len(models))
	for index, model := range models {
		cloned[index] = model
		cloned[index].SupportedReasoningEfforts = append(
			[]ReasoningEffort(nil),
			model.SupportedReasoningEfforts...,
		)
	}
	return cloned
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
