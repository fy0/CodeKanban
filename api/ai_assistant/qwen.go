package ai_assistant

import (
	"regexp"
	"strings"
)

// Qwen specific patterns
var qwenPatterns = struct {
	Thinking        []*regexp.Regexp
	Replying        []*regexp.Regexp
	WaitingInput    []*regexp.Regexp
	EscToCancel     *regexp.Regexp
}{
	Thinking: []*regexp.Regexp{
		// Qwen spinner characters + "esc to cancel" pattern
		// ⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏
		regexp.MustCompile(`[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]\s+.*\(esc to cancel`),
	},
	Replying: []*regexp.Regexp{
		// Qwen completion marker: ✦ at start of line
		regexp.MustCompile(`^\s*✦\s+`),
	},
	WaitingInput: []*regexp.Regexp{
		// After ✦ message completes, it's waiting for input
		regexp.MustCompile(`^\s*✦\s+`),
	},
	EscToCancel: regexp.MustCompile(`\(esc to cancel`),
}

// DetectQwenState detects state from Qwen output
func DetectQwenState(line string) AIAssistantState {
	if line == "" {
		return AIAssistantStateUnknown
	}

	// Clean ANSI and apply Qwen specific patterns
	cleanedLine := CleanLine(line)
	if cleanedLine == "" {
		return AIAssistantStateUnknown
	}

	// Check patterns in priority order
	// Thinking state: spinner + "esc to cancel"
	if matchAnyPattern(cleanedLine, qwenPatterns.Thinking) {
		return AIAssistantStateThinking
	}

	// Note: ✦ is just output formatting, not a state indicator
	// The state transition from Thinking → WaitingInput is handled by the
	// absence of "esc to cancel" after debounce threshold, not by detecting ✦

	return AIAssistantStateUnknown
}

// DetectQwenEscToCancel checks if line contains Qwen's "esc to cancel" marker
func DetectQwenEscToCancel(line string) bool {
	cleaned := CleanLine(line)
	return qwenPatterns.EscToCancel.MatchString(cleaned)
}

// HasQwenEscToCancel is an alias for better readability
func HasQwenEscToCancel(line string) bool {
	return DetectQwenEscToCancel(line)
}

// QwenStateDescription returns a human-readable description for Qwen states
func QwenStateDescription(state AIAssistantState) string {
	switch state {
	case AIAssistantStateThinking:
		return "Qwen is thinking"
	case AIAssistantStateExecuting:
		return "Qwen is executing a command"
	case AIAssistantStateWaitingApproval:
		return "Qwen is waiting for approval"
	case AIAssistantStateReplying:
		return "Qwen is replying"
	case AIAssistantStateWaitingInput:
		return "Qwen is waiting for input"
	default:
		return "Unknown state"
	}
}

// isQwenLine checks if output line looks like Qwen output
func isQwenLine(line string) bool {
	cleaned := strings.ToLower(CleanLine(line))

	// Check for Qwen specific markers
	markers := []string{
		"esc to cancel",
		"✦",
	}

	for _, marker := range markers {
		if strings.Contains(cleaned, marker) {
			return true
		}
	}

	// Check for spinner characters
	spinnerChars := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
	for _, char := range spinnerChars {
		if strings.ContainsRune(cleaned, char) {
			return true
		}
	}

	return false
}
