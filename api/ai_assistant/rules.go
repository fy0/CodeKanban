package ai_assistant

import "strings"

// DetectionRule defines a rule for detecting an AI assistant.
type DetectionRule struct {
	Type        AIAssistantType
	Patterns    []string // Command line patterns to match
	Description string
}

// defaultRules contains the built-in detection rules for known AI assistants.
var defaultRules = []DetectionRule{
	{
		Type: AIAssistantClaudeCode,
		Patterns: []string{
			"@anthropic-ai/claude-code",
			"claude-code/cli.js",
			"claude-code/bin/",
		},
		Description: "Detects Anthropic Claude Code CLI",
	},
	{
		Type: AIAssistantCodex,
		Patterns: []string{
			"@openai/codex",
			"codex/bin/codex.js",
			"codex.js",
		},
		Description: "Detects OpenAI Codex CLI",
	},
	{
		Type: AIAssistantQwenCode,
		Patterns: []string{
			"@qwen-code/qwen-code",
			"qwen-code/cli.js",
			"qwen-code/bin/",
		},
		Description: "Detects Qwen Code CLI",
	},
	{
		Type: AIAssistantGemini,
		Patterns: []string{
			"@google/gemini-cli",
			"gemini-cli/dist/index.js",
			"gemini-cli/bin/",
		},
		Description: "Detects Google Gemini CLI",
	},
	{
		Type: AIAssistantCursor,
		Patterns: []string{
			"cursor",
			"cursor.exe",
			"cursor-server",
		},
		Description: "Detects Cursor editor",
	},
	{
		Type: AIAssistantCopilot,
		Patterns: []string{
			"github-copilot",
			"copilot-agent",
			"copilot.vim",
		},
		Description: "Detects GitHub Copilot",
	},
}

// Match checks if the command matches this rule.
func (r *DetectionRule) Match(command string) bool {
	if command == "" {
		return false
	}

	// Normalize command for case-insensitive matching on Windows
	normalizedCmd := strings.ToLower(command)

	for _, pattern := range r.Patterns {
		normalizedPattern := strings.ToLower(pattern)
		if strings.Contains(normalizedCmd, normalizedPattern) {
			return true
		}
	}

	return false
}

// GetDefaultRules returns a copy of the default detection rules.
func GetDefaultRules() []DetectionRule {
	rules := make([]DetectionRule, len(defaultRules))
	copy(rules, defaultRules)
	return rules
}
