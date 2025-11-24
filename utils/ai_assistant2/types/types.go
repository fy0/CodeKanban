package types

import "time"

// AssistantType represents the type of AI assistant
type AssistantType string

const (
	AssistantTypeUnknown    AssistantType = "unknown"
	AssistantTypeClaudeCode AssistantType = "claude-code"
	AssistantTypeCodex      AssistantType = "codex"
	AssistantTypeQwenCode   AssistantType = "qwen-code"
	AssistantTypeGemini     AssistantType = "gemini"
)

// State represents the current state of an AI assistant
type State string

const (
	StateUnknown         State = "unknown"
	StateWorking         State = "working"          // Combines thinking/executing/replying
	StateWaitingApproval State = "waiting_approval" // Waiting for user approval
	StateWaitingInput    State = "waiting_input"    // Waiting for user input
)

// AssistantInfo contains information about a detected AI assistant
type AssistantInfo struct {
	Type        AssistantType
	Name        string
	DisplayName string
	Command     string
	Detected    bool
}

// AIAssistantInfo is the full assistant info including state for API responses
type AIAssistantInfo struct {
	Type           string    `json:"type"`
	Name           string    `json:"name"`
	DisplayName    string    `json:"displayName"`
	Detected       bool      `json:"detected"`
	Command        string    `json:"command,omitempty"`
	State          string    `json:"state,omitempty"`
	StateUpdatedAt time.Time `json:"stateUpdatedAt,omitempty"`
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

// SupportsProgressTracking reports whether progress detection is implemented for this assistant
func (t AssistantType) SupportsProgressTracking() bool {
	switch t {
	case AssistantTypeClaudeCode, AssistantTypeCodex, AssistantTypeQwenCode, AssistantTypeGemini:
		return true
	default:
		return false
	}
}

// StatusDetector is an interface for detecting AI assistant states from terminal output
type StatusDetector interface {
	// DetectStateFromLines analyzes multiple lines and returns the detected state
	// cols is the terminal width, required for structure-based detection
	// timestamp is when these lines were captured
	// currentState is the current detected state (for stability checking)
	// lastDetectedAt is when the current state was last detected (updated every chunk, for stability checking)
	// Returns:
	//   - state: the detected state (may be forced by stability checking)
	//   - actuallyDetected: true if the state was actually detected from display (not forced by stability check)
	DetectStateFromLines(lines []string, cols int, timestamp time.Time, currentState State, lastDetectedAt time.Time) (state State, actuallyDetected bool)
}
