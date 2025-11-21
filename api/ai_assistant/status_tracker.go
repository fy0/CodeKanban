package ai_assistant

import (
	"strings"
	"sync"
	"time"
)

const (
	defaultIdleTimeout         = 5 * time.Second
	maxBufferedLineBytes       = 4096
	escToInterruptDebounceTime = 500 * time.Millisecond // At least 500ms since last "esc to interrupt"

	// Unified logic for both Claude Code and Codex to handle flickering output
	escPresentThreshold = 3 // Need 3 consecutive chunks WITH "esc to interrupt" to confirm working state
	escAbsentThreshold  = 3 // Then need 3 consecutive chunks WITHOUT to confirm completion
)

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

// Activate enables the tracker for assistants that support ACP progress signals.
func (t *StatusTracker) Activate(assistantType AIAssistantType) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !assistantType.SupportsProgressTracking() {
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
	} else if t.lastHadEscToInterrupt {
		// "esc to interrupt" is absent
		t.escAbsentCount++
		t.escPresentCount = 0

		// Phase 2: Only check completion if we've confirmed working state
		if !t.confirmedWorking {
			// Haven't confirmed working yet, don't trigger completion via debounce
			// But if we detected an explicit state change (e.g., "Interrupted" keyword), allow it through
			if changed {
				return newState, now, true
			}
			return AIAssistantStateUnknown, time.Time{}, false
		}

		// Check if completion conditions are met:
		// 1. Confirmed working state (phase 1 passed)
		// 2. Threshold consecutive chunks without "esc to interrupt"
		// 3. At least 500ms elapsed since last "esc to interrupt"
		// 4. Transitioning from a working state (Thinking/Executing)
		chunkThresholdMet := t.escAbsentCount >= escAbsentThreshold
		timeThresholdMet := !t.lastEscToInterruptAt.IsZero() && now.Sub(t.lastEscToInterruptAt) >= escToInterruptDebounceTime

		if chunkThresholdMet && timeThresholdMet {
			// Check if we're transitioning from a working state
			isWorkingState := t.lastState == AIAssistantStateThinking || t.lastState == AIAssistantStateExecuting

			if isWorkingState {
				// Accumulate duration for the previous working state
				if !t.lastChangedAt.IsZero() {
					t.accumulateDuration(t.lastState, now.Sub(t.lastChangedAt))
				}
				// Valid completion: working state → waiting input
				t.lastState = AIAssistantStateWaitingInput
				t.lastChangedAt = now
				t.lastHadEscToInterrupt = false
				t.escAbsentCount = 0
				t.escPresentCount = 0
				t.confirmedWorking = false // Reset for next cycle
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
