package ai_assistant

import "time"

// AIAssistantType represents the type of AI assistant detected.
type AIAssistantType string

const (
	AIAssistantClaudeCode AIAssistantType = "claude-code"
	AIAssistantCodex      AIAssistantType = "codex"
	AIAssistantQwenCode   AIAssistantType = "qwen-code"
	AIAssistantGemini     AIAssistantType = "gemini"
	AIAssistantCursor     AIAssistantType = "cursor"
	AIAssistantCopilot    AIAssistantType = "copilot"
	AIAssistantUnknown    AIAssistantType = ""
)

// AIAssistantState represents the execution status inferred from stdout events.
type AIAssistantState string

const (
	AIAssistantStateUnknown         AIAssistantState = ""
	AIAssistantStateThinking        AIAssistantState = "thinking"
	AIAssistantStateExecuting       AIAssistantState = "executing"
	AIAssistantStateWaitingApproval AIAssistantState = "waiting_approval"
	AIAssistantStateReplying        AIAssistantState = "replying"
	AIAssistantStateWaitingInput    AIAssistantState = "waiting_input"
)

// StateStats tracks duration statistics for AI assistant states
type StateStats struct {
	ThinkingDuration        time.Duration `json:"thinkingDuration"`
	ExecutingDuration       time.Duration `json:"executingDuration"`
	WaitingApprovalDuration time.Duration `json:"waitingApprovalDuration"`
	WaitingInputDuration    time.Duration `json:"waitingInputDuration"`
	CurrentStateDuration    time.Duration `json:"currentStateDuration"`
}

// AIAssistantInfo contains information about a detected AI assistant.
type AIAssistantInfo struct {
	Type           AIAssistantType  `json:"type"`
	Name           string           `json:"name"`
	DisplayName    string           `json:"displayName"`
	Detected       bool             `json:"detected"`
	Command        string           `json:"command,omitempty"`
	State          AIAssistantState `json:"state,omitempty"`
	StateUpdatedAt time.Time        `json:"stateUpdatedAt,omitempty"`
	Stats          *StateStats      `json:"stats,omitempty"`
}

// String returns the string representation of the assistant type.
func (t AIAssistantType) String() string {
	return string(t)
}

// DisplayName returns a human-readable name for the assistant type.
func (t AIAssistantType) DisplayName() string {
	switch t {
	case AIAssistantClaudeCode:
		return "Claude Code"
	case AIAssistantCodex:
		return "OpenAI Codex"
	case AIAssistantQwenCode:
		return "Qwen Code"
	case AIAssistantGemini:
		return "Google Gemini"
	case AIAssistantCursor:
		return "Cursor"
	case AIAssistantCopilot:
		return "GitHub Copilot"
	default:
		return ""
	}
}

// SupportsProgressTracking reports whether progress detection is implemented for this assistant.
func (t AIAssistantType) SupportsProgressTracking() bool {
	switch t {
	case AIAssistantClaudeCode, AIAssistantCodex, AIAssistantQwenCode:
		return true
	default:
		return false
	}
}
