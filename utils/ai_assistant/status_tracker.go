package ai_assistant

import (
	"strings"
	"sync"
	"time"
)

const (
	defaultIdleTimeout  = 5 * time.Second
	maxBufferedLineBytes = 4096

	// Debounce times for different AI assistants
	// Claude Code/Qwen: 500ms (output frequently, quick to detect completion)
	// Codex: 3s (sometimes hangs without output while still working)
	defaultEscToInterruptDebounceTime = 500 * time.Millisecond
	codexEscToInterruptDebounceTime   = 3 * time.Second

	// Thresholds for detecting state transitions
	escPresentThreshold = 3  // Need 3 consecutive chunks WITH "esc to interrupt" to confirm working state
	escAbsentThreshold  = 3  // Default: need 3 consecutive chunks WITHOUT to confirm completion
	codexEscAbsentThreshold = 3 // Codex: need 3 chunks (reduced from 10)
)

// StatusEnabledChecker is a function type that checks if status tracking is enabled for a given assistant type.
type StatusEnabledChecker func(assistantType string) bool

// StatusTracker incrementally infers ACP event states from stdout chunks.
type StatusTracker struct {
	mu                    sync.Mutex
	assistantType         AIAssistantType
	active                bool
	idleTimeout           time.Duration
	pending               string
	lastState             AIAssistantState
	lastChangedAt         time.Time
	lastHadEscToInterrupt bool      // tracks if last chunk had "esc to interrupt"
	lastEscToInterruptAt  time.Time // timestamp of last "esc to interrupt" occurrence
	escPresentCount       int       // counts consecutive chunks WITH "esc to interrupt"
	escAbsentCount        int       // counts consecutive chunks WITHOUT "esc to interrupt"
	confirmedWorking      bool      // true after seeing escPresentThreshold consecutive "esc to interrupt"
	statusEnabledChecker  StatusEnabledChecker // optional function to check if tracking is enabled
	lastWasInterrupted    bool      // tracks if the last state change was due to user interruption (ESC)

	// State duration tracking
	thinkingDuration        time.Duration
	executingDuration       time.Duration
	waitingApprovalDuration time.Duration
	waitingInputDuration    time.Duration
}

// NewStatusTracker constructs a tracker with default settings.
func NewStatusTracker() *StatusTracker {
	return &StatusTracker{
		idleTimeout: defaultIdleTimeout,
		lastState:   AIAssistantStateUnknown,
	}
}

// SetStatusEnabledChecker sets the function to check if status tracking is enabled.
func (t *StatusTracker) SetStatusEnabledChecker(checker StatusEnabledChecker) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.statusEnabledChecker = checker
}

// Activate enables the tracker for assistants that support ACP progress signals.
func (t *StatusTracker) Activate(assistantType AIAssistantType) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !assistantType.SupportsProgressTracking() {
		t.resetLocked()
		return
	}

	// Check if status tracking is enabled for this assistant type via configuration
	if t.statusEnabledChecker != nil && !t.statusEnabledChecker(assistantType.String()) {
		t.resetLocked()
		return
	}

	t.assistantType = assistantType
	t.active = true
	if t.lastState == AIAssistantStateUnknown {
		t.lastState = AIAssistantStateWaitingInput
		t.lastChangedAt = time.Now()
	}
}

// Deactivate clears the current tracking state.
func (t *StatusTracker) Deactivate() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resetLocked()
}

// getDebounceTime returns the appropriate debounce time based on assistant type
func (t *StatusTracker) getDebounceTime() time.Duration {
	if t.assistantType == AIAssistantCodex {
		return codexEscToInterruptDebounceTime
	}
	return defaultEscToInterruptDebounceTime
}

// getAbsentThreshold returns the appropriate absent threshold based on assistant type
func (t *StatusTracker) getAbsentThreshold() int {
	if t.assistantType == AIAssistantCodex {
		return codexEscAbsentThreshold // 3 for Codex
	}
	return escAbsentThreshold // 3 for others
}

// Process consumes a chunk of stdout/stderr.
// It returns the new state and timestamp if a change was detected.
func (t *StatusTracker) Process(chunk []byte) (AIAssistantState, time.Time, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.active || len(chunk) == 0 {
		return AIAssistantStateUnknown, time.Time{}, false
	}
	text := t.pending + string(chunk)
	lines := strings.Split(text, "\n")
	if len(lines) > 0 {
		t.pending = lines[len(lines)-1]
		if len(t.pending) > maxBufferedLineBytes {
			t.pending = t.pending[len(t.pending)-maxBufferedLineBytes:]
		}
		lines = lines[:len(lines)-1]
	}

	var changed bool
	var newState AIAssistantState
	now := time.Now()
	hasEscToInterrupt := false
	detectedInterrupted := false // tracks if "interrupted" keyword was detected in this chunk

	for _, raw := range lines {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" {
			continue
		}

		// Check if this line has "esc to interrupt" based on assistant type
		if t.detectEscToInterruptByType(line) {
			hasEscToInterrupt = true
		}

		// Detect state based on assistant type
		if state := t.detectStateByType(line); state != AIAssistantStateUnknown {
			if state != t.lastState {
				// Accumulate duration for the previous state
				if !t.lastChangedAt.IsZero() {
					t.accumulateDuration(t.lastState, now.Sub(t.lastChangedAt))
				}
				changed = true
				newState = state

				// Check if this is an interruption by detecting "interrupted" keyword
				// Only mark as interrupted if transitioning to WaitingInput
				if state == AIAssistantStateWaitingInput && t.detectInterruptedKeyword(line) {
					detectedInterrupted = true
				}
			}
			t.lastState = state
			t.lastChangedAt = now
		}
	}

	// Critical: Two-phase detection to handle flickering "esc to interrupt"
	// Phase 1: Confirm working state (consecutive presence)
	// Phase 2: Confirm completion (consecutive absence + time threshold)
	if hasEscToInterrupt {
		// "esc to interrupt" is present
		t.escPresentCount++
		t.escAbsentCount = 0
		t.lastHadEscToInterrupt = true
		t.lastEscToInterruptAt = now

		// Phase 1: Confirm we're really in working state
		if t.escPresentCount >= escPresentThreshold {
			t.confirmedWorking = true
		}

		// Clear interrupted flag when entering a working state
		if t.lastState == AIAssistantStateThinking || t.lastState == AIAssistantStateExecuting {
			t.lastWasInterrupted = false
		}
	} else if t.lastHadEscToInterrupt {
		// "esc to interrupt" is absent
		t.escAbsentCount++
		t.escPresentCount = 0

		// Phase 2: Only check completion if we've confirmed working state
		if !t.confirmedWorking {
			// Haven't confirmed working yet, don't trigger completion via debounce
			// But if we detected an explicit state change (e.g., "Interrupted" keyword), allow it through
			if changed {
				// If this was triggered by "interrupted" keyword, mark it
				if detectedInterrupted {
					t.lastWasInterrupted = true
				}
				return newState, now, true
			}
			return AIAssistantStateUnknown, time.Time{}, false
		}

		// Check if completion conditions are met:
		// 1. Confirmed working state (phase 1 passed)
		// 2. Threshold consecutive chunks without "esc to interrupt"
		//    (3 for Claude Code/Qwen, 10 for Codex to handle spotlight animation)
		// 3. At least debounce time elapsed since last "esc to interrupt"
		//    (500ms for Claude Code/Qwen, 3s for Codex)
		// 4. Transitioning from a working state (Thinking/Executing)
		chunkThresholdMet := t.escAbsentCount >= t.getAbsentThreshold()
		timeThresholdMet := !t.lastEscToInterruptAt.IsZero() && now.Sub(t.lastEscToInterruptAt) >= t.getDebounceTime()

		if chunkThresholdMet && timeThresholdMet {
			// Check if we're transitioning from a working state
			isWorkingState := t.lastState == AIAssistantStateThinking || t.lastState == AIAssistantStateExecuting

			if isWorkingState {
				// Accumulate duration for the previous working state
				if !t.lastChangedAt.IsZero() {
					t.accumulateDuration(t.lastState, now.Sub(t.lastChangedAt))
				}
				// Valid completion: working state → waiting input (NOT interrupted, this is normal completion)
				t.lastState = AIAssistantStateWaitingInput
				t.lastChangedAt = now
				t.lastHadEscToInterrupt = false
				t.escAbsentCount = 0
				t.escPresentCount = 0
				t.confirmedWorking = false // Reset for next cycle
				t.lastWasInterrupted = false // This is normal completion, not an interruption
				return AIAssistantStateWaitingInput, now, true
			} else {
				// Not a working state, just clear flags
				t.lastHadEscToInterrupt = false
				t.escAbsentCount = 0
				t.escPresentCount = 0
				t.confirmedWorking = false
			}
		}
	}

	if changed {
		// If this was triggered by "interrupted" keyword, mark it
		if detectedInterrupted {
			t.lastWasInterrupted = true
		}
		return newState, now, true
	}
	return AIAssistantStateUnknown, time.Time{}, false
}

// ProcessDisplay analyzes the current terminal display lines for AI assistant state.
// This is more accurate than Process() as it works with rendered display content
// rather than raw chunks, handling all terminal escape sequences correctly.
func (t *StatusTracker) ProcessDisplay(lines []string) (AIAssistantState, time.Time, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.active || len(lines) == 0 {
		return AIAssistantStateUnknown, time.Time{}, false
	}

	var changed bool
	var newState AIAssistantState
	now := time.Now()
	hasEscToInterrupt := false
	detectedInterrupted := false

	// Use cross-line detection for interrupted state (more accurate)
	// This checks the bottom portion of the display for the complete pattern
	displayInterrupted := t.detectInterruptedFromDisplay(lines)

	// Process all visible lines from terminal display
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if this line has "esc to interrupt" based on assistant type
		if t.detectEscToInterruptByType(line) {
			hasEscToInterrupt = true
		}

		// Detect state based on assistant type
		if state := t.detectStateByType(line); state != AIAssistantStateUnknown {
			if state != t.lastState {
				// Accumulate duration for the previous state
				if !t.lastChangedAt.IsZero() {
					t.accumulateDuration(t.lastState, now.Sub(t.lastChangedAt))
				}
				changed = true
				newState = state

				// Check if this is an interruption using cross-line detection
				if state == AIAssistantStateWaitingInput && displayInterrupted {
					detectedInterrupted = true
				}
			}
			t.lastState = state
			t.lastChangedAt = now
		}
	}

	// Two-phase detection for state transitions
	if hasEscToInterrupt {
		// "esc to interrupt" is present on screen
		t.escPresentCount++
		t.escAbsentCount = 0
		t.lastHadEscToInterrupt = true
		t.lastEscToInterruptAt = now

		// Phase 1: Confirm we're really in working state
		if t.escPresentCount >= escPresentThreshold {
			t.confirmedWorking = true
		}

		// Clear interrupted flag when entering a working state
		if t.lastState == AIAssistantStateThinking || t.lastState == AIAssistantStateExecuting {
			t.lastWasInterrupted = false
		}
	} else if t.lastHadEscToInterrupt {
		// "esc to interrupt" is no longer visible
		t.escAbsentCount++
		t.escPresentCount = 0

		// Phase 2: Only check completion if we've confirmed working state
		if !t.confirmedWorking {
			if changed {
				if detectedInterrupted {
					t.lastWasInterrupted = true
				}
				return newState, now, true
			}
			return AIAssistantStateUnknown, time.Time{}, false
		}

		// Check completion conditions
		chunkThresholdMet := t.escAbsentCount >= t.getAbsentThreshold()
		timeThresholdMet := !t.lastEscToInterruptAt.IsZero() && now.Sub(t.lastEscToInterruptAt) >= t.getDebounceTime()

		if chunkThresholdMet && timeThresholdMet {
			isWorkingState := t.lastState == AIAssistantStateThinking || t.lastState == AIAssistantStateExecuting

			if isWorkingState {
				if !t.lastChangedAt.IsZero() {
					t.accumulateDuration(t.lastState, now.Sub(t.lastChangedAt))
				}
				t.lastState = AIAssistantStateWaitingInput
				t.lastChangedAt = now
				t.lastHadEscToInterrupt = false
				t.escAbsentCount = 0
				t.escPresentCount = 0
				t.confirmedWorking = false
				t.lastWasInterrupted = false
				return AIAssistantStateWaitingInput, now, true
			} else {
				t.lastHadEscToInterrupt = false
				t.escAbsentCount = 0
				t.escPresentCount = 0
				t.confirmedWorking = false
			}
		}
	}

	if changed {
		if detectedInterrupted {
			t.lastWasInterrupted = true
		}
		return newState, now, true
	}
	return AIAssistantStateUnknown, time.Time{}, false
}

// EvaluateTimeout forces the tracker to fall back to waiting_input after inactivity.
func (t *StatusTracker) EvaluateTimeout(now time.Time) (AIAssistantState, time.Time, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.active || t.lastState == AIAssistantStateUnknown {
		return AIAssistantStateUnknown, time.Time{}, false
	}
	// Don't timeout these stable states - they should persist until explicit state change
	if t.lastState == AIAssistantStateWaitingInput || t.lastState == AIAssistantStateWaitingApproval {
		return t.lastState, t.lastChangedAt, false
	}
	if now.Sub(t.lastChangedAt) > t.idleTimeout {
		t.lastState = AIAssistantStateWaitingInput
		t.lastChangedAt = now
		return t.lastState, now, true
	}
	return t.lastState, t.lastChangedAt, false
}

// State returns the last known state snapshot.
func (t *StatusTracker) State() (AIAssistantState, time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastState, t.lastChangedAt
}

// AssistantType reports the currently tracked assistant type.
func (t *StatusTracker) AssistantType() AIAssistantType {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.assistantType
}

func (t *StatusTracker) resetLocked() {
	t.active = false
	t.pending = ""
	t.assistantType = AIAssistantUnknown
	t.lastState = AIAssistantStateUnknown
	t.lastChangedAt = time.Time{}
	t.lastHadEscToInterrupt = false
	t.lastEscToInterruptAt = time.Time{}
	t.escPresentCount = 0
	t.escAbsentCount = 0
	t.confirmedWorking = false
	t.lastWasInterrupted = false
	// Reset duration tracking
	t.thinkingDuration = 0
	t.executingDuration = 0
	t.waitingApprovalDuration = 0
	t.waitingInputDuration = 0
}

// accumulateDuration adds the duration of the previous state to the appropriate counter
func (t *StatusTracker) accumulateDuration(oldState AIAssistantState, duration time.Duration) {
	switch oldState {
	case AIAssistantStateThinking:
		t.thinkingDuration += duration
	case AIAssistantStateExecuting:
		t.executingDuration += duration
	case AIAssistantStateWaitingApproval:
		t.waitingApprovalDuration += duration
	case AIAssistantStateWaitingInput:
		t.waitingInputDuration += duration
	}
}

// Stats returns the current state statistics
func (t *StatusTracker) Stats() *StateStats {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.active {
		return nil
	}

	// Calculate current state duration
	var currentDuration time.Duration
	if !t.lastChangedAt.IsZero() {
		currentDuration = time.Since(t.lastChangedAt)
	}

	return &StateStats{
		ThinkingDuration:        t.thinkingDuration,
		ExecutingDuration:       t.executingDuration,
		WaitingApprovalDuration: t.waitingApprovalDuration,
		WaitingInputDuration:    t.waitingInputDuration,
		CurrentStateDuration:    currentDuration,
	}
}

// detectStateByType routes to the appropriate detection function based on assistant type
func (t *StatusTracker) detectStateByType(line string) AIAssistantState {
	switch t.assistantType {
	case AIAssistantClaudeCode:
		return DetectClaudeCodeState(line)
	case AIAssistantCodex:
		return DetectCodexState(line)
	case AIAssistantQwenCode:
		return DetectQwenState(line)
	default:
		// Fallback to generic detection
		return DetectStateFromLine(line)
	}
}

// detectEscToInterruptByType routes to the appropriate esc detection based on assistant type
func (t *StatusTracker) detectEscToInterruptByType(line string) bool {
	switch t.assistantType {
	case AIAssistantClaudeCode:
		return DetectClaudeCodeEscToInterrupt(line)
	case AIAssistantCodex:
		return DetectCodexEscToInterrupt(line)
	case AIAssistantQwenCode:
		return DetectQwenEscToCancel(line)
	default:
		// Fallback: check for any "esc to interrupt" or "esc to cancel" pattern
		cleaned := CleanLine(line)
		lower := strings.ToLower(cleaned)
		return strings.Contains(lower, "esc to interrupt") || strings.Contains(lower, "esc to cancel")
	}
}

// detectInterruptedKeyword checks if the line contains the specific interrupted pattern
// indicating user interruption (ESC key press).
// Pattern: ⎿  Interrupted · What should Claude do instead?
// Must have the special character ⎿ (or ⌙) followed by "Interrupted" to avoid false positives.
func (t *StatusTracker) detectInterruptedKeyword(line string) bool {
	cleaned := CleanLine(line)
	lower := strings.ToLower(cleaned)
	// More precise pattern: must have the special character followed by "interrupted"
	// This avoids false positives from random text containing "interrupted"
	return strings.Contains(lower, "⎿") && strings.Contains(lower, "interrupted") ||
		strings.Contains(lower, "⌙") && strings.Contains(lower, "interrupted")
}

// detectInterruptedFromDisplay checks if the display content shows an interrupted state.
// This uses cross-line detection to ensure we're looking at the current input area,
// not historical content in the scrollback.
func (t *StatusTracker) detectInterruptedFromDisplay(lines []string) bool {
	if len(lines) == 0 {
		return false
	}

	// Join lines for cross-line pattern matching
	// Only check the bottom portion of the display (last 20 lines) to avoid false positives from history
	startIdx := 0
	if len(lines) > 20 {
		startIdx = len(lines) - 20
	}
	bottomLines := lines[startIdx:]
	content := strings.Join(bottomLines, "\n")
	lower := strings.ToLower(content)

	switch t.assistantType {
	case AIAssistantClaudeCode:
		// Claude Code pattern:
		// ⎿  Interrupted · What should Claude do instead?
		// ────────────────────────
		// > _
		// Must have: interrupted keyword + input separator (─) or prompt (>)
		hasInterrupted := (strings.Contains(lower, "⎿") || strings.Contains(lower, "⌙")) &&
			strings.Contains(lower, "interrupted")
		if !hasInterrupted {
			return false
		}
		// Check for input area indicators after the interrupted message
		interruptedIdx := strings.Index(lower, "interrupted")
		afterInterrupted := lower[interruptedIdx:]
		hasInputArea := strings.Contains(afterInterrupted, "─") || // separator line
			strings.Contains(afterInterrupted, ">") // prompt
		return hasInputArea

	case AIAssistantCodex:
		// Codex pattern:
		// ■ Conversation interrupted - tell the model what to do differently...
		// ›
		//   11
		//   100% context left
		// Must have: ■ Conversation interrupted + empty › prompt + context left
		if !strings.Contains(lower, "■") || !strings.Contains(lower, "conversation interrupted") {
			return false
		}
		interruptedIdx := strings.Index(lower, "conversation interrupted")
		afterInterrupted := lower[interruptedIdx:]
		// Check for empty prompt (› followed by newline or spaces, not text input)
		// and "context left" indicator
		hasEmptyPrompt := false
		hasContextLeft := strings.Contains(afterInterrupted, "context left")

		// Look for › that's not followed by user input on the same line
		for i, line := range bottomLines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "›") {
				// Check if it's an empty prompt or just "›" followed by nothing significant
				rest := strings.TrimPrefix(trimmed, "›")
				rest = strings.TrimSpace(rest)
				// Empty or just contains cursor/status info
				if rest == "" || (i < len(bottomLines)-1 && !strings.Contains(rest, " ")) {
					hasEmptyPrompt = true
					break
				}
			}
		}
		return hasEmptyPrompt && hasContextLeft

	default:
		// Fallback: simple detection
		return strings.Contains(lower, "interrupted")
	}
}

// WasInterrupted returns whether the last state change was due to user interruption
func (t *StatusTracker) WasInterrupted() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastWasInterrupted
}

// ClearInterrupted clears the interrupted flag
func (t *StatusTracker) ClearInterrupted() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastWasInterrupted = false
}
