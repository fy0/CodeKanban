package ai_assistant

import (
	"encoding/json"
	"regexp"
	"strings"
)

// AIEvent represents a parsed AI assistant event from JSON output
type AIEvent struct {
	Type       string `json:"type"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Name       string `json:"name"`
	Tool       string `json:"tool"`
	Event      string `json:"event"`
	Message    string `json:"message"`
	StopReason string `json:"stop_reason"`
}

// EventPatterns defines regex patterns for detecting AI assistant states
type EventPatterns struct {
	Thinking        []*regexp.Regexp
	Executing       []*regexp.Regexp
	WaitingApproval []*regexp.Regexp
	Replying        []*regexp.Regexp
	WaitingInput    []*regexp.Regexp
}

var defaultPatterns = EventPatterns{
	Thinking: []*regexp.Regexp{
		// Claude Code fixed formats (most reliable)
		regexp.MustCompile(`∴\s*Thinking`),                                               // ∴ Thinking…
		regexp.MustCompile(`∴\s*Thought\s+for\s+[\d\w]+.*\(ctrl\+o\s+to\s+show\s+thinking\)`), // ∴ Thought for 4s/2m/1m30s (ctrl+o to show thinking)
		regexp.MustCompile(`(?i)\(esc\s+to\s+interrupt`),                                 // Claude Code: (esc to interrupt · 54s · ↓ 2.2k tokens)
		regexp.MustCompile(`(?i)esc\s+to\s+interrupt\)`),                                 // Codex: (5s • esc to interrupt)
		// JSON formats
		regexp.MustCompile(`(?i)"type"\s*:\s*"thinking"`),
		regexp.MustCompile(`(?i)agent[_\s-]?thought`),
		// Legacy patterns
		regexp.MustCompile(`(?i)<thinking>`),
	},
	Executing: []*regexp.Regexp{
		// JSON formats (most reliable)
		regexp.MustCompile(`(?i)"type"\s*:\s*"tool[_\s-]?use"`),
		regexp.MustCompile(`(?i)"kind"\s*:\s*"execute"`),
		// Generic tool execution patterns
		regexp.MustCompile(`(?i)tool[_\s-]?(call|use|execution)`),
		regexp.MustCompile(`(?i)executing\s+(command|tool)`),
		regexp.MustCompile(`(?i)running\s+(command|tool)`),
	},
	WaitingApproval: []*regexp.Regexp{
		// Claude Code format: "proceed? (y/n)"
		regexp.MustCompile(`(?i)proceed\?\s*\([yn]/[yn]\)`),
		// Generic approval patterns
		regexp.MustCompile(`(?i)request[_\s-]?permission`),
		regexp.MustCompile(`(?i)waiting.*approval`),
		regexp.MustCompile(`(?i)approve|confirm.*\?`),
	},
	Replying: []*regexp.Regexp{
		// JSON formats
		regexp.MustCompile(`(?i)"type"\s*:\s*"(assistant[_\s-]?)?message"`),
		regexp.MustCompile(`(?i)agent[_\s-]?message`),
		// Text patterns (less reliable, lower priority)
		regexp.MustCompile(`(?i)replying|responding`),
	},
	WaitingInput: []*regexp.Regexp{
		// JSON completion indicators
		regexp.MustCompile(`(?i)"done"\s*:\s*true`),
		regexp.MustCompile(`(?i)"stop[_\s-]?reason"`),
		// Text completion indicators
		regexp.MustCompile(`(?i)completed|finished`),
		regexp.MustCompile(`(?i)waiting.*input`),
	},
}

// DetectStateFromLine attempts to detect AI assistant state from a single line
// using multiple detection strategies in order of reliability
func DetectStateFromLine(line string) AIAssistantState {
	if line == "" {
		return AIAssistantStateUnknown
	}

	// Strategy 1: Try JSON parsing (most reliable)
	if state := detectFromJSON(line); state != AIAssistantStateUnknown {
		return state
	}

	// Strategy 2: Clean ANSI and apply pattern matching
	cleanedLine := CleanLine(line)
	if cleanedLine == "" {
		return AIAssistantStateUnknown
	}

	// Strategy 3: Pattern-based detection with priority order
	// Check in order of specificity to avoid false positives
	if matchAnyPattern(cleanedLine, defaultPatterns.WaitingApproval) {
		return AIAssistantStateWaitingApproval
	}
	if matchAnyPattern(cleanedLine, defaultPatterns.Executing) {
		return AIAssistantStateExecuting
	}
	if matchAnyPattern(cleanedLine, defaultPatterns.Thinking) {
		return AIAssistantStateThinking
	}
	if matchAnyPattern(cleanedLine, defaultPatterns.Replying) {
		return AIAssistantStateReplying
	}
	if matchAnyPattern(cleanedLine, defaultPatterns.WaitingInput) {
		return AIAssistantStateWaitingInput
	}

	return AIAssistantStateUnknown
}

// detectFromJSON attempts to parse line as JSON and extract state information
func detectFromJSON(line string) AIAssistantState {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "{") {
		return AIAssistantStateUnknown
	}

	var event AIEvent
	if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
		return AIAssistantStateUnknown
	}

	return inferStateFromEvent(event)
}

// inferStateFromEvent maps JSON event data to assistant state
func inferStateFromEvent(event AIEvent) AIAssistantState {
	// Check type field (Claude Code format)
	switch strings.ToLower(event.Type) {
	case "thinking", "thought", "agent_thought", "agent_thought_chunk":
		return AIAssistantStateThinking
	case "tool_use", "tool_call", "tool_update", "toolcall", "toolupdate":
		return AIAssistantStateExecuting
	case "message", "assistant_message", "agent_message", "agent_message_chunk":
		return AIAssistantStateReplying
	case "request_permission", "approval_request":
		return AIAssistantStateWaitingApproval
	case "done", "complete":
		return AIAssistantStateWaitingInput
	}

	// Check kind field
	if strings.ToLower(event.Kind) == "execute" {
		return AIAssistantStateExecuting
	}

	// Check status field
	switch strings.ToLower(event.Status) {
	case "thinking", "analyzing":
		return AIAssistantStateThinking
	case "executing", "running":
		return AIAssistantStateExecuting
	case "waiting", "pending":
		return AIAssistantStateWaitingApproval
	case "done", "completed":
		return AIAssistantStateWaitingInput
	}

	// Check for stop_reason (indicates completion)
	if event.StopReason != "" {
		return AIAssistantStateWaitingInput
	}

	return AIAssistantStateUnknown
}

// matchAnyPattern checks if text matches any of the provided patterns
func matchAnyPattern(text string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

// DetectStateFromBlock processes multiple lines as a block for better context
func DetectStateFromBlock(lines []string) AIAssistantState {
	// Try to find JSON lines first
	for _, line := range lines {
		if state := detectFromJSON(line); state != AIAssistantStateUnknown {
			return state
		}
	}

	// Fall back to pattern matching on cleaned lines
	cleanedBlock := make([]string, 0, len(lines))
	for _, line := range lines {
		if cleaned := CleanLine(line); cleaned != "" {
			cleanedBlock = append(cleanedBlock, cleaned)
		}
	}

	if len(cleanedBlock) == 0 {
		return AIAssistantStateUnknown
	}

	// Join lines for multi-line pattern matching
	fullText := strings.Join(cleanedBlock, " ")

	// Apply patterns with priority
	if matchAnyPattern(fullText, defaultPatterns.WaitingApproval) {
		return AIAssistantStateWaitingApproval
	}
	if matchAnyPattern(fullText, defaultPatterns.Executing) {
		return AIAssistantStateExecuting
	}
	if matchAnyPattern(fullText, defaultPatterns.Thinking) {
		return AIAssistantStateThinking
	}
	if matchAnyPattern(fullText, defaultPatterns.Replying) {
		return AIAssistantStateReplying
	}
	if matchAnyPattern(fullText, defaultPatterns.WaitingInput) {
		return AIAssistantStateWaitingInput
	}

	return AIAssistantStateUnknown
}
