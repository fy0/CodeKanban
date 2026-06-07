package websession

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"code-kanban/model/tables"
)

const (
	defaultCodexContextWindowTokens int64 = 400000
	codexConfigFileName                   = "config.toml"
	codexRuntimeConfigCacheTTL            = 5 * time.Minute
)

type codexContextWindowCache struct {
	path      string
	expiresAt time.Time
	config    CodexRuntimeConfig
	loaded    bool
}

type codexContextWindowResolver struct {
	mu    sync.Mutex
	cache codexContextWindowCache
}

type CodexRuntimeConfig struct {
	ContextWindowTokens int64               `json:"contextWindowTokens"`
	CompactLimitTokens  int64               `json:"compactLimitTokens"`
	Source              ContextWindowSource `json:"source"`
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
	}
	if m == nil {
		return defaultConfig
	}
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
