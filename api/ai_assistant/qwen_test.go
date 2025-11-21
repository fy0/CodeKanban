package ai_assistant

import (
	"testing"
	"time"
)

// Helper function to wait for debounce time threshold
func waitForQwenDebounce() {
	time.Sleep(600 * time.Millisecond) // Slightly more than 500ms threshold
}

func TestDetectQwenState(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected AIAssistantState
	}{
		{
			name:     "Qwen thinking with spinner ⠋",
			input:    "⠋ Finding a suitable loading screen pun... (esc to cancel, 11s)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Qwen thinking with spinner ⠹",
			input:    "⠹ Figuring out how to make this more witty... (esc to cancel, 10s)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Qwen thinking with spinner ⠙",
			input:    "⠙ Processing your request... (esc to cancel, 5s)",
			expected: AIAssistantStateThinking,
		},
		{
			name:     "Qwen completion with ✦ (just output, no state change)",
			input:    "✦ 在 Windows 系统上没有 sleep 命令，我需要用其他方式等待。让我用 Go 程序来实现：",
			expected: AIAssistantStateUnknown,
		},
		{
			name:     "Qwen completion with ✦ (short message, no state change)",
			input:    "✦ 10秒等待已完成！使用Python的time.sleep()函数成功等待了10秒。",
			expected: AIAssistantStateUnknown,
		},
		{
			name:     "Qwen completion with ✦ at start (no state change)",
			input:    "✦ 创建一个简单的Go程序来等待10秒：",
			expected: AIAssistantStateUnknown,
		},
		{
			name:     "Regular output (no state)",
			input:    "Some regular terminal output",
			expected: AIAssistantStateUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectQwenState(tt.input)
			if result != tt.expected {
				t.Errorf("DetectQwenState(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDetectQwenEscToCancel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "Has esc to cancel",
			input:    "⠋ Finding a suitable loading screen pun... (esc to cancel, 11s)",
			expected: true,
		},
		{
			name:     "Has esc to cancel with different spinner",
			input:    "⠹ Figuring out how to make this more witty... (esc to cancel, 10s)",
			expected: true,
		},
		{
			name:     "No esc to cancel",
			input:    "✦ 10秒等待已完成！",
			expected: false,
		},
		{
			name:     "No esc to cancel in regular output",
			input:    "Regular output line",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectQwenEscToCancel(tt.input)
			if result != tt.expected {
				t.Errorf("DetectQwenEscToCancel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStatusTracker_QwenWorkingDisappears(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantQwenCode)

	// Qwen thinking format appears - need 3 chunks to confirm working
	chunk1 := []byte("⠋ Finding a suitable loading screen pun... (esc to cancel, 11s)\n")
	state, _, changed := tracker.Process(chunk1)

	if !changed {
		t.Error("Expected state change when Qwen thinking appears")
	}
	if state != AIAssistantStateThinking {
		t.Errorf("Expected Thinking state, got %v", state)
	}

	// Continue with esc to cancel to confirm working state
	chunk2 := []byte("⠙ Still thinking... (esc to cancel, 12s)\n")
	tracker.Process(chunk2)

	chunk3 := []byte("⠹ Almost there... (esc to cancel, 13s)\n")
	tracker.Process(chunk3)
	waitForQwenDebounce()

	// Thinking completes - first chunk without "esc to cancel" (debounce counter = 1)
	chunk4 := []byte("✦ 10秒等待已完成！使用Python的time.sleep()函数成功等待了10秒。\n")
	state, _, changed = tracker.Process(chunk4)

	// Should NOT trigger yet due to debounce threshold
	if changed {
		t.Error("Should not trigger on first chunk without esc to cancel (debounce)")
	}

	// Second chunk without "esc to cancel" (debounce counter = 2)
	chunk5 := []byte("More output\n")
	state, _, changed = tracker.Process(chunk5)

	// Still should NOT trigger (threshold is 3)
	if changed {
		t.Error("Should not trigger on second chunk (debounce threshold = 3)")
	}

	// Third chunk without "esc to cancel" (debounce counter = 3) → execution completed
	chunk6 := []byte("Final output\n")
	state, _, changed = tracker.Process(chunk6)

	if !changed {
		t.Error("Expected state change when Qwen thinking disappears after debounce")
	}
	if state != AIAssistantStateWaitingInput {
		t.Errorf("Expected WaitingInput state, got %v", state)
	}
}

func TestStatusTracker_QwenMultipleCycles(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantQwenCode)

	// Cycle 1 - need 3 chunks to confirm working
	tracker.Process([]byte("⠋ Working... (esc to cancel, 1s)\n"))
	tracker.Process([]byte("⠙ Working... (esc to cancel, 2s)\n"))
	tracker.Process([]byte("⠹ Working... (esc to cancel, 3s)\n"))
	waitForQwenDebounce()
	// Need 3 chunks without esc to trigger
	tracker.Process([]byte("✦ Completed first task\n"))
	tracker.Process([]byte("Output 1\n"))
	state, _, changed := tracker.Process([]byte("Output 2\n"))

	if !changed || state != AIAssistantStateWaitingInput {
		t.Error("Qwen cycle 1: Expected WaitingInput")
	}

	// Cycle 2 - need 3 chunks to confirm working
	tracker.Process([]byte("⠋ Working again... (esc to cancel, 6s)\n"))
	tracker.Process([]byte("⠙ Working again... (esc to cancel, 7s)\n"))
	tracker.Process([]byte("⠹ Working again... (esc to cancel, 8s)\n"))
	waitForQwenDebounce()
	// Need 3 chunks without esc to trigger
	tracker.Process([]byte("✦ Completed second task\n"))
	tracker.Process([]byte("Output A\n"))
	state, _, changed = tracker.Process([]byte("Output B\n"))

	if !changed || state != AIAssistantStateWaitingInput {
		t.Error("Qwen cycle 2: Expected WaitingInput")
	}
}

func TestStatusTracker_QwenDebounce(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Activate(AIAssistantQwenCode)

	// Start working - need 3 chunks to confirm
	tracker.Process([]byte("⠋ Working... (esc to cancel, 3s)\n"))
	tracker.Process([]byte("⠙ Working... (esc to cancel, 4s)\n"))
	tracker.Process([]byte("⠹ Working... (esc to cancel, 5s)\n"))
	waitForQwenDebounce()

	// First chunk without esc - should NOT trigger completion
	state, _, changed := tracker.Process([]byte("Output 1\n"))
	if changed {
		t.Error("Debounce: Should not trigger on first chunk without esc")
	}

	// "esc to cancel" comes back - reset debounce counter and reconfirm working (need 3)
	tracker.Process([]byte("⠋ Working... (esc to cancel, 9s)\n"))
	tracker.Process([]byte("⠙ Working... (esc to cancel, 10s)\n"))
	tracker.Process([]byte("⠹ Working... (esc to cancel, 11s)\n"))
	waitForQwenDebounce()

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

func BenchmarkDetectQwenState(b *testing.B) {
	testLines := []string{
		"⠋ Finding a suitable loading screen pun... (esc to cancel, 11s)",
		"⠹ Figuring out how to make this more witty... (esc to cancel, 10s)",
		"✦ 10秒等待已完成！使用Python的time.sleep()函数成功等待了10秒。",
		"Regular output line",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectQwenState(testLines[i%len(testLines)])
	}
}
