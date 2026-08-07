package types

// AssistantType represents the type of AI assistant
type AssistantType string

const (
	AssistantTypeUnknown    AssistantType = "unknown"
	AssistantTypeClaudeCode AssistantType = "claude-code"
	AssistantTypeCodex      AssistantType = "codex"
	AssistantTypeQwenCode   AssistantType = "qwen-code"
	AssistantTypeGemini     AssistantType = "gemini"
)

// AssistantInfo contains information about a detected AI assistant
type AssistantInfo struct {
	Type        AssistantType
	Name        string
	DisplayName string
	Command     string
	Detected    bool
}

// AIAssistantInfo is the assistant identity used in API responses.
type AIAssistantInfo struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Detected    bool   `json:"detected"`
	Command     string `json:"command,omitempty"`
}

// String returns the string representation of the assistant type
func (t AssistantType) String() string {
	return string(t)
}

// DisplayName returns a human-readable name for the assistant type
func (t AssistantType) DisplayName() string {
	switch t {
	case AssistantTypeClaudeCode:
		return "Claude Code"
	case AssistantTypeCodex:
		return "OpenAI Codex"
	case AssistantTypeQwenCode:
		return "Qwen Code"
	case AssistantTypeGemini:
		return "Google Gemini"
	default:
		return ""
	}
}
