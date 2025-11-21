package ai_assistant

import (
	"regexp"
	"strings"
)

// Codex specific patterns
var codexPatterns = struct {
	Thinking        []*regexp.Regexp
	Executing       []*regexp.Regexp
	WaitingApproval []*regexp.Regexp
	Replying        []*regexp.Regexp
	WaitingInput    []*regexp.Regexp
	EscToInterrupt  *regexp.Regexp
}{
	Thinking: []*regexp.Regexp{
		// Codex fixed format: "[symbol] [action] ([time] • esc to interrupt)"
		// Examples: "◦ Working (5s • esc to interrupt)"
		//           "• Confirming content (15s • esc to interrupt)"
		// Only "esc to interrupt)" is fixed!
		regexp.MustCompile(`(?i)esc\s+to\s+interrupt\)`), // matches "...esc to interrupt)"
	},
	Executing: []*regexp.Regexp{
		regexp.MustCompile(`(?i)"type"\s*:\s*"tool[_\s-]?use"`),
		regexp.MustCompile(`(?i)"kind"\s*:\s*"execute"`),
		regexp.MustCompile(`(?i)tool[_\s-]?(call|use|execution)`),
	},
	WaitingApproval: []*regexp.Regexp{
		regexp.MustCompile(`(?i)proceed\?\s*\([yn]/[yn]\)`),
		regexp.MustCompile(`(?i)request[_\s-]?permission`),
		regexp.MustCompile(`(?i)approve|confirm.*\?`),
	},
	Replying: []*regexp.Regexp{
		regexp.MustCompile(`(?i)"type"\s*:\s*"(assistant[_\s-]?)?message"`),
		regexp.MustCompile(`(?i)agent[_\s-]?message`),
	},
	WaitingInput: []*regexp.Regexp{
		regexp.MustCompile(`(?i)"done"\s*:\s*true`),
		regexp.MustCompile(`(?i)"stop[_\s-]?reason"`),
		// Codex specific: interrupted state
		regexp.MustCompile(`(?i)■\s*Conversation\s+interrupted`),
		regexp.MustCompile(`(?i)tell\s+the\s+model\s+what\s+to\s+do\s+differently`),
	},
	// Codex format: "(5s • esc to interrupt)" - esc to interrupt at the end
	EscToInterrupt: regexp.MustCompile(`(?i)esc\s+to\s+interrupt\)`),
}

// DetectCodexState detects state from Codex output
func DetectCodexState(line string) AIAssistantState {
	if line == "" {
		return AIAssistantStateUnknown
	}

	// Try JSON parsing first
	if state := detectFromJSON(line); state != AIAssistantStateUnknown {
		return state
	}

	// Clean ANSI and apply Codex specific patterns
	cleanedLine := CleanLine(line)
	if cleanedLine == "" {
		return AIAssistantStateUnknown
	}

	// Check patterns in priority order
	if matchAnyPattern(cleanedLine, codexPatterns.WaitingApproval) {
		return AIAssistantStateWaitingApproval
	}
	if matchAnyPattern(cleanedLine, codexPatterns.Executing) {
		return AIAssistantStateExecuting
	}
	if matchAnyPattern(cleanedLine, codexPatterns.Thinking) {
		return AIAssistantStateThinking
	}
	if matchAnyPattern(cleanedLine, codexPatterns.Replying) {
		return AIAssistantStateReplying
	}
	if matchAnyPattern(cleanedLine, codexPatterns.WaitingInput) {
		return AIAssistantStateWaitingInput
	}

	return AIAssistantStateUnknown
}

// DetectCodexEscToInterrupt checks if line contains Codex's "esc to interrupt)" marker
func DetectCodexEscToInterrupt(line string) bool {
	cleaned := CleanLine(line)
	return codexPatterns.EscToInterrupt.MatchString(cleaned)
}

// HasCodexEscToInterrupt is an alias for better readability
func HasCodexEscToInterrupt(line string) bool {
	return DetectCodexEscToInterrupt(line)
}

// CodexStateDescription returns a human-readable description for Codex states
func CodexStateDescription(state AIAssistantState) string {
	switch state {
	case AIAssistantStateThinking:
		return "Codex is working"
	case AIAssistantStateExecuting:
		return "Codex is executing a tool"
	case AIAssistantStateWaitingApproval:
		return "Codex is waiting for approval"
	case AIAssistantStateReplying:
		return "Codex is replying"
	case AIAssistantStateWaitingInput:
		return "Codex is waiting for input"
	default:
		return "Unknown state"
	}
}

// isCodexLine checks if output line looks like Codex output
func isCodexLine(line string) bool {
	cleaned := strings.ToLower(CleanLine(line))

	// Codex specific marker: "esc to interrupt)" at the end
	// This is different from Claude Code which has "(esc to interrupt" at the start
	return strings.Contains(cleaned, "esc to interrupt)")
}
