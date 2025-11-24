package ai_assistant2

import (
	"strings"

	"code-kanban/utils/ai_assistant2/types"
)

// DetectionRule defines how to detect a specific AI assistant from command
type DetectionRule struct {
	Type        types.AssistantType
	Patterns    []string // Command line patterns to match (case-insensitive)
	Description string
}

var defaultRules = []DetectionRule{
	{
		Type: types.AssistantTypeClaudeCode,
		Patterns: []string{
			"@anthropic-ai/claude-code",
			"claude-code/cli.js",
			"claude-code/bin/",
		},
		Description: "Detects Anthropic Claude Code CLI",
	},
	{
		Type: types.AssistantTypeCodex,
		Patterns: []string{
			"@openai/codex",
			"codex/bin/codex.js",
			"codex.js",
		},
		Description: "Detects OpenAI Codex CLI",
	},
	{
		Type: types.AssistantTypeQwenCode,
		Patterns: []string{
			"@qwen-code/qwen-code",
			"qwen-code/cli.js",
			"qwen-code/bin/",
		},
		Description: "Detects Qwen Code CLI",
	},
	{
		Type: types.AssistantTypeGemini,
		Patterns: []string{
			"@google/gemini-cli",
			"gemini-cli/dist/index.js",
			"gemini-cli/bin/",
		},
		Description: "Detects Google Gemini CLI",
	},
}

// Match checks if the command matches this rule
func (r *DetectionRule) Match(command string) bool {
	if command == "" {
		return false
	}

	// Normalize command for case-insensitive matching
	normalizedCmd := strings.ToLower(command)

	for _, pattern := range r.Patterns {
		normalizedPattern := strings.ToLower(pattern)
		if strings.Contains(normalizedCmd, normalizedPattern) {
			return true
		}
	}

	return false
}

// AssistantDetector detects AI assistant type from command
type AssistantDetector struct {
	rules []DetectionRule
}

// NewAssistantDetector creates a new AI assistant detector
func NewAssistantDetector() *AssistantDetector {
	return &AssistantDetector{
		rules: defaultRules,
	}
}

// DetectFromCommand analyzes a command string and returns the AI assistant type
func (d *AssistantDetector) DetectFromCommand(command string) *types.AssistantInfo {
	if command == "" {
		return nil
	}

	for _, rule := range d.rules {
		if rule.Match(command) {
			return &types.AssistantInfo{
				Type:        rule.Type,
				Name:        string(rule.Type),
				DisplayName: rule.Type.DisplayName(),
				Command:     command,
				Detected:    true,
			}
		}
	}

	return nil
}

// IsAIAssistant checks if the command is running an AI assistant
func (d *AssistantDetector) IsAIAssistant(command string) bool {
	return d.DetectFromCommand(command) != nil
}

// GetType returns the AI assistant type from command
func (d *AssistantDetector) GetType(command string) types.AssistantType {
	info := d.DetectFromCommand(command)
	if info != nil {
		return info.Type
	}
	return types.AssistantTypeUnknown
}

// Default detector instance
var defaultDetector = NewAssistantDetector()

// DetectFromCommand uses the default detector to analyze a command
func DetectFromCommand(command string) *types.AssistantInfo {
	return defaultDetector.DetectFromCommand(command)
}

// IsAIAssistant uses the default detector to check if command is an AI assistant
func IsAIAssistant(command string) bool {
	return defaultDetector.IsAIAssistant(command)
}

// GetType uses the default detector to get the assistant type
func GetType(command string) types.AssistantType {
	return defaultDetector.GetType(command)
}
