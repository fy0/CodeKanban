package ai_assistant

import (
	"testing"
	"time"
)

// Helper function to wait for debounce time threshold
func waitForCodexDebounce() {
	time.Sleep(600 * time.Millisecond) // Slightly more than 500ms threshold
}

func TestDetectCodexState(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected AIAssistantState
	}{
		{
			name:     "Codex Working with ◦ symbol",
			input:    "◦ Working (5s • esc to interrupt)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Codex Working with • symbol",
			input:    "• Working (2s • esc to interrupt)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Codex Confirming content",
			input:    "◦ Confirming content (15s • esc to interrupt)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Codex with minute time",
			input:    "• Analyzing (1m30s • esc to interrupt)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Codex interrupted",
			input:    "■ Conversation interrupted - tell the model what to do differently.",
			expected: AIAssistantStateWaitingInput,
		},
		{
			name:     "Codex feedback hint",
			input:    "Something went wrong? Hit /feedback to report the issue.",
			expected: AIAssistantStateUnknown, // This is just info, not a state change
		},
		{
			name:     "Regular output (no state)",
			input:    "Here is your code output",
			expected: AIAssistantStateUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectCodexState(tt.input)
			if result != tt.expected {
				t.Errorf("DetectCodexState(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDetectCodexEscToInterrupt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "Working with esc to interrupt",
			input:    "◦ Working (5s • esc to interrupt)",
			expected: true,
		},
		{
			name:     "Confirming with esc to interrupt",
			input:    "• Confirming content (15s • esc to interrupt)",
			expected: true,
		},
		{
			name:     "No esc to interrupt",
			input:    "Regular output line",
			expected: false,
		},
		{
			name:     "Claude Code format (different pattern)",
			input:    "(esc to interrupt · 5s · ↓ 1.2k tokens)",
			expected: false, // Claude Code format, not Codex
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectCodexEscToInterrupt(tt.input)
			if result != tt.expected {
				t.Errorf("DetectCodexEscToInterrupt(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStatusTracker_CodexWorkingDisappears(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantCodex)

	// Codex "Working" format appears - need 3 chunks to confirm working
	chunk1 := []byte("◦ Working (5s • esc to interrupt)\n")
	state, _, changed := tracker.Process(chunk1)

	if !changed {
		t.Error("Expected state change when Codex Working appears")
	}
	if state != AIAssistantStateThinking {
		t.Errorf("Expected Thinking state, got %v", state)
	}

	// Continue with esc to interrupt to confirm working state
	chunk2 := []byte("• Confirming content (10s • esc to interrupt)\n")
	tracker.Process(chunk2)

	chunk3 := []byte("• Confirming content (15s • esc to interrupt)\n")
	tracker.Process(chunk3)
	waitForCodexDebounce()

	// Working disappears - first chunk without "esc to interrupt" (debounce counter = 1)
	chunk4 := []byte("Output without esc to interrupt\n")
	state, _, changed = tracker.Process(chunk4)

	// Should NOT trigger yet due to debounce threshold
	if changed {
		t.Error("Should not trigger on first chunk without esc to interrupt (debounce)")
	}

	// Second chunk without "esc to interrupt" (debounce counter = 2)
	chunk5 := []byte("More output\n")
	state, _, changed = tracker.Process(chunk5)

	// Still should NOT trigger (threshold is 3)
	if changed {
		t.Error("Should not trigger on second chunk (debounce threshold = 3)")
	}

	// Third chunk without "esc to interrupt" (debounce counter = 3) → execution completed
	chunk6 := []byte("Final output\n")
	state, _, changed = tracker.Process(chunk6)

	if !changed {
		t.Error("Expected state change when Codex Working disappears after debounce")
	}
	if state != AIAssistantStateWaitingInput {
		t.Errorf("Expected WaitingInput state, got %v", state)
	}
}

func TestStatusTracker_CodexInterrupted(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantCodex)

	// Start working
	tracker.Process([]byte("◦ Working (3s • esc to interrupt)\n"))

	// User interrupts
	chunk := []byte("■ Conversation interrupted - tell the model what to do differently.\n")
	state, _, changed := tracker.Process(chunk)

	// Interrupted should trigger WaitingInput
	if !changed {
		t.Error("Expected state change after Codex interrupt")
	}
	if state != AIAssistantStateWaitingInput {
		t.Errorf("Expected WaitingInput after interrupt, got %v", state)
	}
}

func TestStatusTracker_CodexMultipleCycles(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantCodex)

	// Cycle 1 - need 3 chunks to confirm working
	tracker.Process([]byte("◦ Working (1s • esc to interrupt)\n"))
	tracker.Process([]byte("◦ Working (2s • esc to interrupt)\n"))
	tracker.Process([]byte("◦ Working (3s • esc to interrupt)\n"))
	waitForCodexDebounce()
	// Need 3 chunks without esc to trigger
	tracker.Process([]byte("Output 1\n"))
	tracker.Process([]byte("Output 2\n"))
	state, _, changed := tracker.Process([]byte("Output 3\n"))

	if !changed || state != AIAssistantStateWaitingInput {
		t.Error("Codex cycle 1: Expected WaitingInput")
	}

	// Cycle 2 - need 3 chunks to confirm working
	tracker.Process([]byte("• Confirming (6s • esc to interrupt)\n"))
	tracker.Process([]byte("• Confirming (7s • esc to interrupt)\n"))
	tracker.Process([]byte("• Confirming (8s • esc to interrupt)\n"))
	waitForCodexDebounce()
	// Need 3 chunks without esc to trigger
	tracker.Process([]byte("Output A\n"))
	tracker.Process([]byte("Output B\n"))
	state, _, changed = tracker.Process([]byte("Output C\n"))

	if !changed || state != AIAssistantStateWaitingInput {
		t.Error("Codex cycle 2: Expected WaitingInput")
	}
}

func TestStatusTracker_CodexDebounce(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantCodex)

	// Start working - need 3 chunks to confirm
	tracker.Process([]byte("◦ Working (3s • esc to interrupt)\n"))
	tracker.Process([]byte("◦ Working (4s • esc to interrupt)\n"))
	tracker.Process([]byte("◦ Working (5s • esc to interrupt)\n"))
	waitForCodexDebounce()

	// First chunk without esc - should NOT trigger completion
	state, _, changed := tracker.Process([]byte("Output 1\n"))
	if changed {
		t.Error("Debounce: Should not trigger on first chunk without esc")
	}

	// "esc to interrupt" comes back - reset debounce counter and reconfirm working (need 3)
	tracker.Process([]byte("◦ Working (9s • esc to interrupt)\n"))
	tracker.Process([]byte("◦ Working (10s • esc to interrupt)\n"))
	tracker.Process([]byte("◦ Working (11s • esc to interrupt)\n"))
	waitForCodexDebounce()

	// First chunk without esc again - counter reset, should NOT trigger
	state, _, changed = tracker.Process([]byte("Output 2\n"))
	if changed {
		t.Error("Debounce: Should not trigger after counter reset (chunk 1/3)")
	}

	// Second chunk - still should NOT trigger (threshold is 3)
	state, _, changed = tracker.Process([]byte("Output 3\n"))
	if changed {
		t.Error("Debounce: Should not trigger on second chunk (2/3)")
	}

	// Third consecutive chunk without esc - NOW should trigger
	state, _, changed = tracker.Process([]byte("Output 4\n"))
	if !changed {
		t.Error("Debounce: Should trigger on third consecutive chunk without esc")
	}
	if state != AIAssistantStateWaitingInput {
		t.Errorf("Debounce: Expected WaitingInput, got %v", state)
	}
}

func TestStatusTracker_NoCompletionFromNonWorkingState(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantCodex)

	// Initial state is WaitingInput (not a working state)
	currentState, _ := tracker.State()
	if currentState != AIAssistantStateWaitingInput {
		t.Errorf("Expected initial state WaitingInput, got %v", currentState)
	}

	// Simulate "esc to interrupt" appearing briefly - need 3 chunks to confirm working
	tracker.Process([]byte("◦ Working (3s • esc to interrupt)\n"))
	tracker.Process([]byte("◦ Working (4s • esc to interrupt)\n"))
	tracker.Process([]byte("◦ Working (5s • esc to interrupt)\n"))
	waitForCodexDebounce()

	// Now simulate it disappearing - need 3 chunks
	state, _, changed := tracker.Process([]byte("Regular output\n"))
	if changed {
		t.Error("Should not trigger on first chunk (debounce 1/3)")
	}

	tracker.Process([]byte("More output\n"))
	// Third chunk - debounce threshold met, and we're in Thinking state, so this SHOULD trigger
	state, _, changed = tracker.Process([]byte("Final output\n"))
	if !changed || state != AIAssistantStateWaitingInput {
		t.Error("Should trigger completion from Thinking → WaitingInput")
	}

	// Test edge case: Now we're in WaitingInput state
	// Verify that we stay in WaitingInput without triggering completion events
	currentState, _ = tracker.State()
	if currentState != AIAssistantStateWaitingInput {
		t.Errorf("Should be in WaitingInput state, got %v", currentState)
	}
}

func BenchmarkDetectCodexState(b *testing.B) {
	testLines := []string{
		"◦ Working (5s • esc to interrupt)",
		"• Confirming content (15s • esc to interrupt)",
		"■ Conversation interrupted - tell the model what to do differently.",
		"Regular output line",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectCodexState(testLines[i%len(testLines)])
	}
}
