package ai_assistant

import (
	"regexp"
	"strings"
)

// Claude Code specific patterns
var claudeCodePatterns = struct {
	Thinking        []*regexp.Regexp
	Executing       []*regexp.Regexp
	WaitingApproval []*regexp.Regexp
	Replying        []*regexp.Regexp
	WaitingInput    []*regexp.Regexp
	EscToInterrupt  *regexp.Regexp
}{
	Thinking: []*regexp.Regexp{
		// Claude Code fixed formats
		regexp.MustCompile(`∴\s*Thinking`),                                                   // ∴ Thinking…
		regexp.MustCompile(`∴\s*Thought\s+for\s+[\d\w]+.*\(ctrl\+o\s+to\s+show\s+thinking\)`), // ∴ Thought for 4s (ctrl+o to show thinking)
		regexp.MustCompile(`(?i)\(esc\s+to\s+interrupt`),                                      // (esc to interrupt · 54s · ↓ 2.2k tokens)
	},
	Executing: []*regexp.Regexp{
		regexp.MustCompile(`(?i)"type"\s*:\s*"tool[_\s-]?use"`),
		regexp.MustCompile(`(?i)"kind"\s*:\s*"execute"`),
		regexp.MustCompile(`(?i)tool[_\s-]?(call|use|execution)`),
	},
	WaitingApproval: []*regexp.Regexp{
		regexp.MustCompile(`(?i)Do\s+you\s+want\s+to\s+proceed\?`), // Do you want to proceed?
		regexp.MustCompile(`(?i)❯\s*\d+\.\s*Yes`),                  // ❯ 1. Yes
		regexp.MustCompile(`(?i)proceed\?\s*\([yn]/[yn]\)`),
		regexp.MustCompile(`(?i)request[_\s-]?permission`),
	},
	Replying: []*regexp.Regexp{
		regexp.MustCompile(`(?i)"type"\s*:\s*"(assistant[_\s-]?)?message"`),
		regexp.MustCompile(`(?i)agent[_\s-]?message`),
	},
	WaitingInput: []*regexp.Regexp{
		regexp.MustCompile(`(?i)interrupted`),                // ⎿ Interrupted · What should Claude do instead?
		regexp.MustCompile(`(?i)"done"\s*:\s*true`),
		regexp.MustCompile(`(?i)"stop[_\s-]?reason"`),
	},
	EscToInterrupt: regexp.MustCompile(`(?i)\(esc\s+to\s+interrupt`),
}

// DetectClaudeCodeState detects state from Claude Code output
func DetectClaudeCodeState(line string) AIAssistantState {
	if line == "" {
		return AIAssistantStateUnknown
	}

	// Try JSON parsing first
	if state := detectFromJSON(line); state != AIAssistantStateUnknown {
		return state
	}

	// Clean ANSI and apply Claude Code specific patterns
	cleanedLine := CleanLine(line)
	if cleanedLine == "" {
		return AIAssistantStateUnknown
	}

	// Check patterns in priority order
	if matchAnyPattern(cleanedLine, claudeCodePatterns.WaitingApproval) {
		return AIAssistantStateWaitingApproval
	}
	if matchAnyPattern(cleanedLine, claudeCodePatterns.Executing) {
		return AIAssistantStateExecuting
	}
	if matchAnyPattern(cleanedLine, claudeCodePatterns.Thinking) {
		return AIAssistantStateThinking
	}
	if matchAnyPattern(cleanedLine, claudeCodePatterns.Replying) {
		return AIAssistantStateReplying
	}
	if matchAnyPattern(cleanedLine, claudeCodePatterns.WaitingInput) {
		return AIAssistantStateWaitingInput
	}

	return AIAssistantStateUnknown
}

// DetectClaudeCodeEscToInterrupt checks if line contains Claude Code's "(esc to interrupt" marker
func DetectClaudeCodeEscToInterrupt(line string) bool {
	cleaned := CleanLine(line)
	return claudeCodePatterns.EscToInterrupt.MatchString(cleaned)
}

// HasClaudeCodeEscToInterrupt is an alias for better readability
func HasClaudeCodeEscToInterrupt(line string) bool {
	return DetectClaudeCodeEscToInterrupt(line)
}

// ClaudeCodeStateDescription returns a human-readable description for Claude Code states
func ClaudeCodeStateDescription(state AIAssistantState) string {
	switch state {
	case AIAssistantStateThinking:
		return "Claude Code is thinking"
	case AIAssistantStateExecuting:
		return "Claude Code is executing a tool"
	case AIAssistantStateWaitingApproval:
		return "Claude Code is waiting for approval"
	case AIAssistantStateReplying:
		return "Claude Code is replying"
	case AIAssistantStateWaitingInput:
		return "Claude Code is waiting for input"
	default:
		return "Unknown state"
	}
}

// isClaudeCodeLine checks if output line looks like Claude Code output
func isClaudeCodeLine(line string) bool {
	cleaned := strings.ToLower(CleanLine(line))

	// Check for Claude Code specific markers
	markers := []string{
		"∴ thinking",
		"∴ thought for",
		"(esc to interrupt",
		"(ctrl+o to show thinking)",
	}

	for _, marker := range markers {
		if strings.Contains(cleaned, marker) {
			return true
		}
	}

	return false
}
