package ai_assistant

import (
	"testing"
)

func TestDetectStateFromJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected AIAssistantState
	}{
		{
			name:     "Claude Code thinking event",
			input:    `{"type":"thinking","content":"analyzing the code"}`,
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Tool use event",
			input:    `{"type":"tool_use","name":"bash","status":"running"}`,
			expected: AIAssistantStateExecuting,
		},
		{
			name:     "Tool call with kind",
			input:    `{"kind":"execute","tool":"grep"}`,
			expected: AIAssistantStateExecuting,
		},
		{
			name:     "Assistant message",
			input:    `{"type":"assistant_message","content":"Here is the result"}`,
			expected: AIAssistantStateReplying,
		},
		{
			name:     "Done with stop reason",
			input:    `{"type":"done","stop_reason":"end_turn"}`,
			expected: AIAssistantStateWaitingInput,
		},
		{
			name:     "Request permission",
			input:    `{"type":"request_permission","action":"write_file"}`,
			expected: AIAssistantStateWaitingApproval,
		},
		{
			name:     "Not JSON",
			input:    "This is just regular text",
			expected: AIAssistantStateUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectStateFromLine(tt.input)
			if result != tt.expected {
				t.Errorf("DetectStateFromLine(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDetectStateWithANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected AIAssistantState
	}{
		{
			name:     "Claude Code Deciphering with esc to interrupt",
			input:    "⭐ Deciphering... (esc to interrupt)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Claude Code Analyzing with ANSI colors",
			input:    "\x1b[33m⭐ Analyzing...\x1b[0m (esc to interrupt)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Any action with esc to interrupt",
			input:    "🔍 Searching... (ESC to interrupt)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Planning with esc to interrupt",
			input:    "📝 Planning the implementation (esc to interrupt)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Tool call with ANSI colors",
			input:    "\x1b[32mtool_call\x1b[0m: bash executing",
			expected: AIAssistantStateExecuting,
		},
		{
			name:     "Approval request with ANSI",
			input:    "\x1b[33mProceed? (y/n):\x1b[0m",
			expected: AIAssistantStateWaitingApproval,
		},
		{
			name:     "Carriage return overwrite",
			input:    "Loading...\rExecuting tool_call",
			expected: AIAssistantStateExecuting,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectStateFromLine(tt.input)
			if result != tt.expected {
				t.Errorf("DetectStateFromLine(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Color codes",
			input:    "\x1b[32mGreen text\x1b[0m",
			expected: "Green text",
		},
		{
			name:     "Multiple escapes",
			input:    "\x1b[1m\x1b[32mBold Green\x1b[0m\x1b[0m",
			expected: "Bold Green",
		},
		{
			name:     "Cursor control",
			input:    "\x1b[2K\rCleared line",
			expected: "Cleared line",
		},
		{
			name:     "OSC sequences",
			input:    "\x1b]0;Window Title\x07Text",
			expected: "Text",
		},
		{
			name:     "No ANSI codes",
			input:    "Plain text",
			expected: "Plain text",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("StripANSI(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDetectStateFromBlock(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected AIAssistantState
	}{
		{
			name: "Multi-line with JSON",
			input: []string{
				"Starting analysis...",
				`{"type":"thinking","content":"analyzing"}`,
				"Processing...",
			},
			expected: AIAssistantStateThinking,
		},
		{
			name: "Multi-line text pattern",
			input: []string{
				"The assistant is now",
				"executing the tool call",
				"for your request",
			},
			expected: AIAssistantStateExecuting,
		},
		{
			name: "Mixed ANSI and text with esc to interrupt",
			input: []string{
				"\x1b[32mProgress:\x1b[0m",
				"⭐ Deciphering... (esc to interrupt)",
			},
			expected: AIAssistantStateThinking,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectStateFromBlock(tt.input)
			if result != tt.expected {
				t.Errorf("DetectStateFromBlock(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRealWorldClaudeCodeOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected AIAssistantState
	}{
		{
			name:     "Claude Code Format 1: ∴ Thinking…",
			input:    "∴ Thinking…",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Claude Code Format 2: ∴ Thought for Xs",
			input:    "∴ Thought for 4s (ctrl+o to show thinking)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Claude Code Format 3: (esc to interrupt) with stats",
			input:    "(esc to interrupt · 54s · ↓ 2.2k tokens)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Brewing with ✻ symbol and stats",
			input:    "✻ Brewing… (esc to interrupt · 43s · ↑ 1.4k tokens)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Brewing with · symbol and download stats",
			input:    "· Brewing… (esc to interrupt · 55s · ↓ 1.6k tokens)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Deciphering with ⭐ symbol",
			input:    "⭐ Deciphering... (esc to interrupt)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "∴ Thinking with ANSI colors",
			input:    "\x1b[33m∴ Thinking…\x1b[0m",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "∴ Thought with variable time",
			input:    "∴ Thought for 12s (ctrl+o to show thinking)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Chinese action with long time and todos",
			input:    "✻ 提交文书进入审批流程… (esc to interrupt · ctrl+t to show todos · 8m 41s · ↑ 11.8k tokens)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "With ctrl+t to show todos",
			input:    "· Planning implementation (esc to interrupt · ctrl+t to show todos · 2m 15s)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Long time format with minutes and seconds",
			input:    "∴ Analyzing codebase (esc to interrupt · 15m 30s · ↓ 25.3k tokens)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Bypass permissions line (should be ignored)",
			input:    "►► bypass permissions on (shift+tab to cycle)",
			expected: AIAssistantStateUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectStateFromLine(tt.input)
			if result != tt.expected {
				t.Errorf("DetectStateFromLine(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func BenchmarkDetectStateFromLine(b *testing.B) {
	testLines := []string{
		`{"type":"tool_use","name":"bash"}`,
		"\x1b[32mtool_call: executing\x1b[0m",
		"⭐ Deciphering... (esc to interrupt)",
		"Proceed? (y/n)",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectStateFromLine(testLines[i%len(testLines)])
	}
}

func BenchmarkStripANSI(b *testing.B) {
	input := "\x1b[1m\x1b[32mBold Green Text\x1b[0m with \x1b[33mYellow\x1b[0m"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		StripANSI(input)
	}
}
