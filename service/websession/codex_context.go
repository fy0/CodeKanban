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

var goalModeMinCodexVersion = semver.MustParse("0.133.0")

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

type CodexRuntimeConfig struct {
	Model               string              `json:"model,omitempty"`
	ContextWindowTokens int64               `json:"contextWindowTokens"`
	CompactLimitTokens  int64               `json:"compactLimitTokens"`
	Source              ContextWindowSource `json:"source"`
	Models              []CodexModelInfo    `json:"models"`
	HasCodex            bool                `json:"hasCodex"`
	HasClaudeCode       bool                `json:"hasClaudeCode"`
	CodexVersion        *string             `json:"codexVersion,omitempty"`
	SupportsGoalMode    bool                `json:"supportsGoalMode"`
	GoalModeMinVersion  string              `json:"goalModeMinCodexVersion"`
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
	summary := mapSessionRecord(record)
	summary.ActiveCallTimeoutEnabled = m.effectiveActiveCallTimeoutEnabled(record)
	m.decorateSessionSummary(&summary)
	return summary
}

func (m *Manager) decorateSessionSummary(summary *SessionSummary) {
	if summary == nil {
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
		Source:             ContextWindowSourceUnavailable,
		Models:             []CodexModelInfo{},
		HasCodex:           false,
		HasClaudeCode:      false,
		SupportsGoalMode:   false,
		GoalModeMinVersion: goalModeMinCodexVersion.String(),
	}
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
	return config
}

func (m *Manager) GetCodexRuntimeConfigWithModels() CodexRuntimeConfig {
	config := m.GetCodexRuntimeConfig()
	if config.HasCodex {
		config.Models = m.getCodexModelCatalog()
	}
	return config
}

func (m *Manager) applyBinaryCapabilities(config CodexRuntimeConfig) CodexRuntimeConfig {
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
		result.SupportsGoalMode = cached.config.SupportsGoalMode
		result.GoalModeMinVersion = cached.config.GoalModeMinVersion
		m.codexContextWindow.mu.Unlock()
		return result
	}
	m.codexContextWindow.mu.Unlock()

	hasCodex := hasExecutable(m.cfg.CodexPath)
	hasClaude := hasExecutable(m.cfg.ClaudePath)
	codexVersion := (*string)(nil)
	supportsGoalMode := false
	if hasCodex {
		if version := detectCodexVersion(m.cfg.CodexPath); version != nil {
			copied := *version
			codexVersion = &copied
			supportsGoalMode = codexVersionAtLeast(copied, goalModeMinCodexVersion)
		}
	}

	binaryConfig := CodexRuntimeConfig{
		HasCodex:           hasCodex,
		HasClaudeCode:      hasClaude,
		CodexVersion:       codexVersion,
		SupportsGoalMode:   supportsGoalMode,
		GoalModeMinVersion: goalModeMinCodexVersion.String(),
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
	if trimmed[0] != '"' && trimmed[0] != '\'' {
		return strings.Fields(trimmed)
	}
	quote := trimmed[0]
	end := strings.IndexByte(trimmed[1:], quote)
	if end < 0 {
		return strings.Fields(trimmed)
	}
	commandPart := trimmed[1 : end+1]
	remainder := strings.TrimSpace(trimmed[end+2:])
	if remainder == "" {
		return []string{commandPart}
	}
	return append([]string{commandPart}, strings.Fields(remainder)...)
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
