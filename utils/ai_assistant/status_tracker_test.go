package ai_assistant

import (
	"testing"
	"time"
)

// Helper function to wait for debounce time threshold
func waitForDebounce() {
	time.Sleep(600 * time.Millisecond) // Slightly more than 500ms threshold
}

func TestStatusTracker_EscToInterruptDisappears(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantClaudeCode)

	// Simulate: "(esc to interrupt)" appears - thinking starts
	chunk1 := []byte("✻ Brewing… (esc to interrupt · 5s · ↑ 1.2k tokens)\n")
	state, _, changed := tracker.Process(chunk1)

	if !changed {
		t.Error("Expected state change when esc to interrupt appears")
	}
	if state != AIAssistantStateThinking {
		t.Errorf("Expected Thinking state, got %v", state)
	}

	// Need at least 3 chunks with "esc to interrupt" to confirm working state
	chunk2 := []byte("∴ Thinking… (esc to interrupt · 10s · ↑ 1.5k tokens)\n")
	tracker.Process(chunk2)

	chunk3 := []byte("∴ Thinking… (esc to interrupt · 15s · ↑ 2.1k tokens)\n")
	tracker.Process(chunk3)

	// Now working state is confirmed
	currentState, _ := tracker.State()
	if currentState != AIAssistantStateThinking {
		t.Errorf("Expected Thinking state, got %v", currentState)
	}

	//  Wait for time threshold
	waitForDebounce()

	// Simulate: "(esc to interrupt)" disappears
	// First chunk without esc to interrupt (debounce counter = 1)
	chunk4 := []byte("Some output without esc to interrupt\n")
	state, _, changed = tracker.Process(chunk4)

	// Should NOT trigger yet due to debounce threshold
	if changed {
		t.Error("Should not trigger on first chunk without esc to interrupt (debounce 1/3)")
	}

	// Second chunk without esc to interrupt (debounce counter = 2)
	chunk5 := []byte("More output\n")
	state, _, changed = tracker.Process(chunk5)

	// Still should NOT trigger (threshold is 3)
	if changed {
		t.Error("Should not trigger on second chunk (debounce 2/3)")
	}

	// Third chunk without esc to interrupt (debounce counter = 3) → execution completed!
	chunk6 := []byte("Final output\n")
	state, _, changed = tracker.Process(chunk6)

	if !changed {
		t.Error("Expected state change when esc to interrupt disappears after debounce")
	}
	if state != AIAssistantStateWaitingInput {
		t.Errorf("Expected WaitingInput state after esc disappears, got %v", state)
	}
}

func TestStatusTracker_EscToInterruptMultipleCycles(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantClaudeCode)

	// Cycle 1: Thinking → Complete
	// Need at least 3 chunks with "esc to interrupt" to confirm working
	tracker.Process([]byte("✻ Analyzing… (esc to interrupt)\n"))
	tracker.Process([]byte("✻ Analyzing… (esc to interrupt · 2s)\n"))
	tracker.Process([]byte("✻ Analyzing… (esc to interrupt · 4s)\n"))
	waitForDebounce()
	// Need 3 chunks without esc
	tracker.Process([]byte("Output line\n"))
	tracker.Process([]byte("More output\n"))
	state, _, changed := tracker.Process([]byte("Final output\n"))

	if !changed || state != AIAssistantStateWaitingInput {
		t.Error("First cycle: Expected WaitingInput after esc disappears")
	}

	// Cycle 2: Thinking again → Complete again
	// Need at least 3 chunks with "esc to interrupt" to confirm working
	tracker.Process([]byte("· Planning… (esc to interrupt · 1s)\n"))
	tracker.Process([]byte("· Planning… (esc to interrupt · 2s)\n"))
	tracker.Process([]byte("· Planning… (esc to interrupt · 3s)\n"))
	waitForDebounce()
	// Need 3 chunks without esc
	tracker.Process([]byte("Another output\n"))
	tracker.Process([]byte("More output 2\n"))
	state, _, changed = tracker.Process([]byte("Final output 2\n"))

	if !changed || state != AIAssistantStateWaitingInput {
		t.Error("Second cycle: Expected WaitingInput after esc disappears")
	}
}

func TestStatusTracker_NoFalsePositive(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantClaudeCode)

	// No "esc to interrupt" in first chunk
	chunk1 := []byte("Regular output line\n")
	_, _, changed := tracker.Process(chunk1)

	if changed {
		t.Error("Should not trigger state change without esc to interrupt")
	}

	// Still no "esc to interrupt"
	chunk2 := []byte("Another regular line\n")
	_, _, changed = tracker.Process(chunk2)

	if changed {
		t.Error("Should not trigger false positive completion")
	}
}

func TestStatusTracker_ThoughtForXsFormat(t *testing.T) {
	// Test various time formats
	testCases := []string{
		"∴ Thought for 5s (ctrl+o to show thinking)\n",
		"∴ Thought for 2m (ctrl+o to show thinking)\n",
		"∴ Thought for 1m30s (ctrl+o to show thinking)\n",
	}

	for _, tc := range testCases {
		// Create new tracker for each test case
		tracker := NewStatusTracker()
		tracker.Activate(AIAssistantClaudeCode)

		state, _, changed := tracker.Process([]byte(tc))

		if !changed {
			t.Errorf("Expected state change for: %s", tc)
		}
		if state != AIAssistantStateThinking {
			t.Errorf("Expected Thinking state for: %s, got %v", tc, state)
		}
	}
}

func TestStatusTracker_InterruptedState(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantClaudeCode)

	// Start thinking with long time and todos format
	state, _, changed := tracker.Process([]byte("✻ 提交文书进入审批流程… (esc to interrupt · ctrl+t to show todos · 8m 41s · ↑ 11.8k tokens)\n"))

	if !changed || state != AIAssistantStateThinking {
		t.Error("Expected state change to Thinking")
	}

	// Should not be interrupted yet
	if tracker.WasInterrupted() {
		t.Error("Should not be interrupted yet")
	}

	// User interrupts - should immediately trigger WaitingInput due to "Interrupted" keyword
	chunk := []byte("[Request interrupted by user]\n⎿ Interrupted · What should Claude do instead?\n")
	state, _, changed = tracker.Process(chunk)

	if !changed {
		t.Error("Expected immediate state change when 'Interrupted' keyword is detected")
	}
	if state != AIAssistantStateWaitingInput {
		t.Errorf("Expected WaitingInput after interrupt, got %v", state)
	}

	// CRITICAL: Should be marked as interrupted
	if !tracker.WasInterrupted() {
		t.Error("Expected WasInterrupted() to return true after user interruption")
	}
}

func TestStatusTracker_ChineseActionWithInterrupt(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantClaudeCode)

	// Chinese action with esc to interrupt - need 3 chunks to confirm working
	state, _, changed := tracker.Process([]byte("✻ 提交文书进入审批流程… (esc to interrupt · ctrl+t to show todos · 8m 41s · ↑ 11.8k tokens)\n"))

	if !changed {
		t.Error("Expected state change to Thinking")
	}
	if state != AIAssistantStateThinking {
		t.Errorf("Expected Thinking state, got %v", state)
	}

	// Continue with esc to interrupt to confirm working state
	tracker.Process([]byte("✻ 提交文书进入审批流程… (esc to interrupt · ctrl+t to show todos · 8m 42s · ↑ 11.9k tokens)\n"))
	tracker.Process([]byte("✻ 提交文书进入审批流程… (esc to interrupt · ctrl+t to show todos · 8m 43s · ↑ 12.0k tokens)\n"))
	waitForDebounce()

	// Normal completion (not interrupted) - need 3 chunks
	tracker.Process([]byte("文书已提交成功\n"))
	tracker.Process([]byte("等待用户输入\n"))
	state, _, changed = tracker.Process([]byte("准备接收下一条指令\n"))

	if !changed {
		t.Error("Expected state change to WaitingInput")
	}
	if state != AIAssistantStateWaitingInput {
		t.Errorf("Expected WaitingInput state, got %v", state)
	}
}

func TestStatusTracker_OnlyWorkingStateTriggersCompletion(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantClaudeCode)

	// Initial state is WaitingInput
	state, _ := tracker.State()
	if state != AIAssistantStateWaitingInput {
		t.Errorf("Expected WaitingInput initially, got %v", state)
	}

	// Process some regular output without "esc to interrupt"
	// This should NOT trigger any completion event
	tracker.Process([]byte("Regular output line 1\n"))
	tracker.Process([]byte("Regular output line 2\n"))
	_, _, changed := tracker.Process([]byte("Regular output line 3\n"))

	if changed {
		t.Error("Regular output should not trigger state change from WaitingInput")
	}

	// Now enter a working state - need 3 chunks to confirm
	state, _, changed = tracker.Process([]byte("✻ Brewing… (esc to interrupt · 5s)\n"))
	if !changed || state != AIAssistantStateThinking {
		t.Error("Should enter Thinking state")
	}
	tracker.Process([]byte("✻ Brewing… (esc to interrupt · 6s)\n"))
	tracker.Process([]byte("✻ Brewing… (esc to interrupt · 7s)\n"))
	waitForDebounce()

	// Exit working state - this SHOULD trigger completion (need 3 chunks)
	tracker.Process([]byte("Output without esc\n"))
	tracker.Process([]byte("More output\n"))
	state, _, changed = tracker.Process([]byte("Final output\n"))

	if !changed || state != AIAssistantStateWaitingInput {
		t.Error("Should trigger completion from Thinking → WaitingInput")
	}

	// Now in WaitingInput again - processing more output should NOT trigger
	tracker.Process([]byte("Post-completion output 1\n"))
	_, _, changed = tracker.Process([]byte("Post-completion output 2\n"))

	if changed {
		t.Error("Should NOT trigger another completion from WaitingInput")
	}
}

func TestStatusTracker_NormalCompletionVsInterruption(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantClaudeCode)

	// Test 1: Normal completion (not interrupted)
	// Enter thinking state - state change happens on first chunk
	state, _, changed := tracker.Process([]byte("✻ Brewing… (esc to interrupt · 5s)\n"))

	if !changed || state != AIAssistantStateThinking {
		t.Error("Should enter Thinking state")
	}

	// Send more chunks to confirm working state (need 3 total)
	tracker.Process([]byte("✻ Brewing… (esc to interrupt · 6s)\n"))
	tracker.Process([]byte("✻ Brewing… (esc to interrupt · 7s)\n"))

	waitForDebounce()

	// Normal completion - esc to interrupt disappears (need 3 chunks)
	tracker.Process([]byte("Output line 1\n"))
	tracker.Process([]byte("Output line 2\n"))
	state, _, changed = tracker.Process([]byte("Output line 3\n"))

	if !changed || state != AIAssistantStateWaitingInput {
		t.Error("Should transition to WaitingInput on normal completion")
	}

	// CRITICAL: Normal completion should NOT be marked as interrupted
	if tracker.WasInterrupted() {
		t.Error("Normal completion should NOT be marked as interrupted")
	}

	// Test 2: User interruption
	// Enter thinking state again - state change happens on first chunk
	state, _, changed = tracker.Process([]byte("✻ Thinking… (esc to interrupt · 5s)\n"))

	if !changed || state != AIAssistantStateThinking {
		t.Error("Should enter Thinking state again")
	}

	// Send more chunks to confirm working state
	tracker.Process([]byte("✻ Thinking… (esc to interrupt · 6s)\n"))
	tracker.Process([]byte("✻ Thinking… (esc to interrupt · 7s)\n"))

	// User presses ESC - interrupted keyword appears
	chunk := []byte("[Request interrupted by user]\n⎿ Interrupted · What should Claude do instead?\n")
	state, _, changed = tracker.Process(chunk)

	if !changed || state != AIAssistantStateWaitingInput {
		t.Error("Should transition to WaitingInput on interruption")
	}

	// CRITICAL: Interruption SHOULD be marked
	if !tracker.WasInterrupted() {
		t.Error("User interruption SHOULD be marked as interrupted")
	}

	// Test 3: Interrupted flag should clear when entering working state again
	state, _, changed = tracker.Process([]byte("✻ Planning… (esc to interrupt · 5s)\n"))

	if !changed || state != AIAssistantStateThinking {
		t.Error("Should enter Thinking state once more")
	}

	// Send more chunks to confirm working state
	tracker.Process([]byte("✻ Planning… (esc to interrupt · 6s)\n"))
	tracker.Process([]byte("✻ Planning… (esc to interrupt · 7s)\n"))

	// CRITICAL: Interrupted flag should be cleared when entering working state
	if tracker.WasInterrupted() {
		t.Error("Interrupted flag should be cleared when entering working state")
	}
}

func TestStatusTracker_CustomTaskDescriptions(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantClaudeCode)

	// Test case 1: "✻ 浏览器验证最终效果… (esc to interrupt · ctrl+t to show todos · 4m 58s · ↑ 3.8k tokens)"
	// This should be detected as Thinking state because it matches the strict format
	chunk1 := []byte("✻ 浏览器验证最终效果… (esc to interrupt · ctrl+t to show todos · 4m 58s · ↑ 3.8k tokens)\n")
	state, _, changed := tracker.Process(chunk1)

	if !changed {
		t.Error("Expected state change for custom task with esc to interrupt")
	}
	if state != AIAssistantStateThinking {
		t.Errorf("Expected Thinking state for '✻ 浏览器验证最终效果…', got %v", state)
	}

	// Continue to confirm working state (need 3 chunks total)
	chunk2 := []byte("✻ 浏览器验证最终效果… (esc to interrupt · ctrl+t to show todos · 4m 59s · ↑ 3.9k tokens)\n")
	tracker.Process(chunk2)

	chunk3 := []byte("✻ 浏览器验证最终效果… (esc to interrupt · ctrl+t to show todos · 5m 0s · ↑ 4.0k tokens)\n")
	tracker.Process(chunk3)

	// Verify we're still in Thinking state
	currentState, _ := tracker.State()
	if currentState != AIAssistantStateThinking {
		t.Errorf("Expected Thinking state after multiple chunks, got %v", currentState)
	}

	// Test case 2: "✶ Vibing… (esc to interrupt)"
	// Reset tracker for second test
	tracker2 := NewStatusTracker()
	tracker2.Activate(AIAssistantClaudeCode)

	chunk4 := []byte("✶ Vibing… (esc to interrupt)\n")
	state2, _, changed2 := tracker2.Process(chunk4)

	if !changed2 {
		t.Error("Expected state change for '✶ Vibing…' with esc to interrupt")
	}
	if state2 != AIAssistantStateThinking {
		t.Errorf("Expected Thinking state for '✶ Vibing…', got %v", state2)
	}

	// Continue to confirm working state
	chunk5 := []byte("✶ Vibing… (esc to interrupt)\n")
	tracker2.Process(chunk5)

	chunk6 := []byte("✶ Vibing… (esc to interrupt)\n")
	tracker2.Process(chunk6)

	// Verify working state is confirmed
	currentState2, _ := tracker2.State()
	if currentState2 != AIAssistantStateThinking {
		t.Errorf("Expected Thinking state for '✶ Vibing…' after confirmation, got %v", currentState2)
	}

	// Test completion after debounce
	waitForDebounce()

	// Need 3 chunks without esc to trigger completion
	tracker2.Process([]byte("Regular output 1\n"))
	tracker2.Process([]byte("Regular output 2\n"))
	finalState, _, finalChanged := tracker2.Process([]byte("Regular output 3\n"))

	if !finalChanged {
		t.Error("Expected state change to WaitingInput after esc disappears")
	}
	if finalState != AIAssistantStateWaitingInput {
		t.Errorf("Expected WaitingInput after completion, got %v", finalState)
	}
}

func TestStatusTracker_AvoidFalsePositives(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantClaudeCode)

	// Test case 1: Text contains "(esc to interrupt)" but not in the correct format
	// Should NOT be detected as working state
	falsePositives := []string{
		"Some random text with (esc to interrupt) in the middle\n",
		"Documentation: Press (esc to interrupt) the operation\n",
		"Error message contains (esc to interrupt) text\n",
		"No symbol before (esc to interrupt)\n",
		"✻ Missing ellipsis (esc to interrupt)\n", // No … before (esc to interrupt)
	}

	for i, fp := range falsePositives {
		_, _, changed := tracker.Process([]byte(fp))
		if changed {
			t.Errorf("Test %d: Should NOT detect false positive: %s", i+1, fp)
		}
	}

	// Test case 2: Verify that correct format IS detected
	correctFormat := "✻ Working on something… (esc to interrupt · 10s)\n"
	state, _, changed := tracker.Process([]byte(correctFormat))

	if !changed {
		t.Error("Should detect correct format")
	}
	if state != AIAssistantStateThinking {
		t.Errorf("Expected Thinking state for correct format, got %v", state)
	}
}

func TestStatusTracker_VariousSymbols(t *testing.T) {
	// Test various symbols that should all be recognized
	symbols := []string{
		"✻ Testing… (esc to interrupt)\n",
		"✶ Testing… (esc to interrupt)\n",
		"∴ Testing… (esc to interrupt)\n",
		"· Testing… (esc to interrupt)\n",
		"○ Testing… (esc to interrupt)\n",
		"◆ Testing… (esc to interrupt)\n",
		"● Testing… (esc to interrupt)\n",
		"★ Testing… (esc to interrupt)\n",
		"☆ Testing… (esc to interrupt)\n",
		"✓ Testing… (esc to interrupt)\n",
		"✔ Testing… (esc to interrupt)\n",
	}

	for i, symbol := range symbols {
		tracker := NewStatusTracker()
		tracker.Activate(AIAssistantClaudeCode)

		state, _, changed := tracker.Process([]byte(symbol))

		if !changed {
			t.Errorf("Symbol test %d failed: %s should be detected", i+1, symbol)
		}
		if state != AIAssistantStateThinking {
			t.Errorf("Symbol test %d: Expected Thinking state, got %v", i+1, state)
		}
	}
}

func TestStatusTracker_CompactingFormat(t *testing.T) {
	testCases := []struct {
		name          string
		input         string
		expectedState AIAssistantState
	}{
		{
			name:          "Compacting conversation with full info",
			input:         "· Compacting conversation… (esc to interrupt · ctrl+t to show todos · 8m 49s · ↓ 9.3k tokens)\n",
			expectedState: AIAssistantStateThinking,
		},
		{
			name:          "Compacting without todos",
			input:         "· Compacting conversation… (esc to interrupt · 8m 49s · ↓ 9.3k tokens)\n",
			expectedState: AIAssistantStateThinking,
		},
		{
			name:          "Compacting minimal",
			input:         "· Compacting… (esc to interrupt)\n",
			expectedState: AIAssistantStateThinking,
		},
		{
			name:          "Other actions with dot symbol",
			input:         "· Processing files… (esc to interrupt · 2s)\n",
			expectedState: AIAssistantStateThinking,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tracker := NewStatusTracker()
			tracker.Activate(AIAssistantClaudeCode)

			// Process the chunk
			state, _, changed := tracker.Process([]byte(tc.input))

			if !changed {
				t.Errorf("Expected state change for: %s", tc.input)
			}

			if state != tc.expectedState {
				t.Errorf("Expected state %s, got %s for input: %s",
					tc.expectedState, state, tc.input)
			}
		})
	}
}

func BenchmarkStatusTracker_Process(b *testing.B) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantClaudeCode)

	chunks := [][]byte{
		[]byte("✻ Brewing… (esc to interrupt · 5s)\n"),
		[]byte("∴ Thinking…\n"),
		[]byte("Regular output\n"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.Process(chunks[i%len(chunks)])
	}
}
