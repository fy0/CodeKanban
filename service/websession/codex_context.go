package websession

import (
	"context"
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
	defaultCodexContextWindowTokens int64 = 400000
	codexConfigFileName                   = "config.toml"
	codexRuntimeConfigCacheTTL            = 5 * time.Minute
	codexBinaryCapabilityCacheTTL         = 5 * time.Second
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

type codexContextWindowResolver struct {
	mu    sync.Mutex
	cache codexContextWindowCache
	bins  codexBinaryCapabilityCache
}

type CodexRuntimeConfig struct {
	ContextWindowTokens int64               `json:"contextWindowTokens"`
	CompactLimitTokens  int64               `json:"compactLimitTokens"`
	Source              ContextWindowSource `json:"source"`
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
	summary.ContextWindowTokens = ptr(config.ContextWindowTokens)
	summary.ContextWindowSource = config.Source
}

func (m *Manager) GetCodexRuntimeConfig() CodexRuntimeConfig {
	defaultConfig := CodexRuntimeConfig{
		ContextWindowTokens: defaultCodexContextWindowTokens,
		CompactLimitTokens:  defaultCodexContextWindowTokens,
		Source:              ContextWindowSourceDefault,
		HasCodex:            false,
		HasClaudeCode:       false,
		SupportsGoalMode:    false,
		GoalModeMinVersion:  goalModeMinCodexVersion.String(),
	}
	if m == nil {
		return defaultConfig
	}
	defaultConfig = m.applyBinaryCapabilities(defaultConfig)
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		return defaultConfig
	}

	configPath := filepath.Join(homeDir, ".codex", codexConfigFileName)

	m.codexContextWindow.mu.Lock()
	cached := m.codexContextWindow.cache
	if cached.loaded && cached.path == configPath && time.Now().Before(cached.expiresAt) {
		m.codexContextWindow.mu.Unlock()
		return cached.config
	}
	m.codexContextWindow.mu.Unlock()

	raw, err := os.ReadFile(configPath)
	config := defaultConfig
	if err == nil {
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
