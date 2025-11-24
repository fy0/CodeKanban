package ai_assistant2

import (
	"context"
	"sync"
	"time"

	"github.com/hinshun/vt10x"

	"code-kanban/utils/ai_assistant2/claude_code"
	"code-kanban/utils/ai_assistant2/codex"
	"code-kanban/utils/ai_assistant2/types"
)

const (
	// periodicCheckInterval is how often we check state when no new chunks arrive
	periodicCheckInterval = 500 * time.Millisecond
)

// StateChangeCallback is called when state changes are detected
type StateChangeCallback func(state types.State, ts time.Time)

// StatusTracker tracks AI assistant state from terminal display
type StatusTracker struct {
	mu              sync.Mutex
	assistantType   types.AssistantType
	active          bool
	lastState       types.State
	lastChangedAt   time.Time // Time when state changed to a different state
	recentUpdatedAt time.Time // Time when the same state was last detected (updated every chunk)
	lastProcessTime time.Time // Time when ProcessChunk was last called

	// Virtual terminal emulator for display simulation
	emulator vt10x.Terminal
	rows     int
	cols     int

	// Status detector for the current assistant
	detector types.StatusDetector

	// Periodic state checking
	checkCtx    context.Context
	checkCancel context.CancelFunc
	callback    StateChangeCallback
}

// NewStatusTracker creates a new status tracker
func NewStatusTracker() *StatusTracker {
	return &StatusTracker{
		lastState: types.StateUnknown,
	}
}

// SetStateChangeCallback sets the callback for state changes detected by periodic checking
func (t *StatusTracker) SetStateChangeCallback(callback StateChangeCallback) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.callback = callback
}

// Activate enables tracking for a specific AI assistant
func (t *StatusTracker) Activate(assistantType types.AssistantType, rows, cols int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !assistantType.SupportsProgressTracking() {
		t.resetLocked()
		return
	}

	// If already active with same type, just update size if changed
	if t.active && t.assistantType == assistantType {
		if t.rows != rows || t.cols != cols {
			t.rows = rows
			t.cols = cols
			if t.emulator != nil {
				t.emulator.Resize(cols, rows)
			}
		}
		return
	}

	// Create new emulator and detector for this assistant
	t.assistantType = assistantType
	t.active = true
	t.rows = rows
	t.cols = cols
	t.emulator = vt10x.New(vt10x.WithSize(cols, rows))
	t.detector = createDetector(assistantType)

	// Initialize state and timestamps
	now := time.Now()
	if t.lastState == types.StateUnknown {
		t.lastState = types.StateWaitingInput
		t.lastChangedAt = now
		t.recentUpdatedAt = now
	} else {
		// If we're reactivating with a previous state, ensure recentUpdatedAt is valid
		// This prevents issues when switching between assistants
		if t.recentUpdatedAt.IsZero() {
			t.recentUpdatedAt = now
		}
	}
	t.lastProcessTime = now

	// Start periodic state checking goroutine
	t.startPeriodicCheckLocked()
}

// createDetector creates a status detector for the given assistant type
func createDetector(assistantType types.AssistantType) types.StatusDetector {
	switch assistantType {
	case types.AssistantTypeClaudeCode:
		return claude_code.NewStatusDetector()
	case types.AssistantTypeCodex:
		return codex.NewStatusDetector()
	case types.AssistantTypeQwenCode:
		// TODO: implement qwen_code status detector
		return nil
	case types.AssistantTypeGemini:
		// TODO: implement gemini status detector
		return nil
	default:
		return nil
	}
}

// Deactivate stops tracking
func (t *StatusTracker) Deactivate() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resetLocked()
}

// ProcessChunk feeds a terminal output chunk to the emulator and detects state changes
func (t *StatusTracker) ProcessChunk(chunk []byte) (types.State, time.Time, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.active || len(chunk) == 0 || t.emulator == nil {
		return types.StateUnknown, time.Time{}, false
	}

	// Feed chunk to virtual terminal
	t.emulator.Write(chunk)

	now := time.Now()

	// Skip if called too frequently (throttle to avoid excessive processing)
	if !t.lastProcessTime.IsZero() {
		if now.Sub(t.lastProcessTime) < 100*time.Millisecond {
			return types.StateUnknown, time.Time{}, false
		}
	}

	t.lastProcessTime = now // Track when ProcessChunk was last called

	// Get visible lines from emulator
	lines := t.getVisibleLinesLocked()
	if len(lines) == 0 {
		return types.StateUnknown, time.Time{}, false
	}

	// Use detector to analyze display
	var detectedState types.State = types.StateUnknown
	var changeRecentUpdate bool = false

	if t.detector != nil {
		// Pass current state and recentUpdatedAt (time when same state was last detected)
		detectedState, changeRecentUpdate = t.detector.DetectStateFromLines(lines, t.cols, now, t.lastState, t.recentUpdatedAt)
	}

	if detectedState != types.StateUnknown {
		if changeRecentUpdate {
			t.recentUpdatedAt = now
		}

		if detectedState != t.lastState {
			// State changed to a different state
			t.lastState = detectedState
			t.lastChangedAt = now
			return detectedState, now, true
		}
	}

	return types.StateUnknown, time.Time{}, false
}

// getVisibleLinesLocked extracts visible lines from the emulator (must be called with lock held)
func (t *StatusTracker) getVisibleLinesLocked() []string {
	if t.emulator == nil {
		return nil
	}

	lines := make([]string, 0, t.rows)
	runes := make([]rune, 0, t.cols) // Reusable rune buffer for building each line

	for row := 0; row < t.rows; row++ {
		runes = runes[:0] // Reset buffer, reusing capacity

		for col := 0; col < t.cols; col++ {
			cell := t.emulator.Cell(col, row)
			if cell.Char != 0 {
				runes = append(runes, cell.Char)
			}
		}
		lines = append(lines, string(runes))
	}

	return lines
}

// State returns the current state and timestamp
func (t *StatusTracker) State() (types.State, time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastState, t.lastChangedAt
}

// AssistantType returns the current assistant type
func (t *StatusTracker) AssistantType() types.AssistantType {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.assistantType
}

// startPeriodicCheckLocked starts a goroutine that periodically checks state
// Must be called with lock held
func (t *StatusTracker) startPeriodicCheckLocked() {
	// Stop previous checker if exists
	t.stopPeriodicCheckLocked()

	// Create new context
	ctx, cancel := context.WithCancel(context.Background())
	t.checkCtx = ctx
	t.checkCancel = cancel

	go t.periodicCheckLoop(ctx)
}

// stopPeriodicCheckLocked stops the periodic check goroutine
// Must be called with lock held
func (t *StatusTracker) stopPeriodicCheckLocked() {
	if t.checkCancel != nil {
		t.checkCancel()
		t.checkCancel = nil
		t.checkCtx = nil
	}
}

// periodicCheckLoop runs in a goroutine and checks state periodically
func (t *StatusTracker) periodicCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(periodicCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.checkStateIfIdle()
		}
	}
}

// checkStateIfIdle checks state if ProcessChunk hasn't been called recently
func (t *StatusTracker) checkStateIfIdle() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.active || t.emulator == nil || t.detector == nil {
		return
	}

	now := time.Now()

	// Only check if ProcessChunk hasn't been called for periodicCheckInterval
	if now.Sub(t.lastProcessTime) < periodicCheckInterval {
		return
	}

	// Get current terminal display (no new chunk, just re-check)
	lines := t.getVisibleLinesLocked()
	if len(lines) == 0 {
		return
	}

	// Use detector to analyze display
	detectedState, changeRecentUpdate := t.detector.DetectStateFromLines(lines, t.cols, now, t.lastState, t.recentUpdatedAt)

	if detectedState != types.StateUnknown {
		if changeRecentUpdate {
			t.recentUpdatedAt = now
		}

		if detectedState != t.lastState {
			// State changed to a different state
			t.lastState = detectedState
			t.lastChangedAt = now

			// Call callback if set (must call without holding lock to avoid deadlock)
			callback := t.callback
			if callback != nil {
				// Release lock before calling callback
				t.mu.Unlock()
				callback(detectedState, now)
				t.mu.Lock()
			}
		}
	}
}

func (t *StatusTracker) resetLocked() {
	t.stopPeriodicCheckLocked()
	t.active = false
	t.assistantType = types.AssistantTypeUnknown
	t.lastState = types.StateUnknown
	t.lastChangedAt = time.Time{}
	t.recentUpdatedAt = time.Time{}
	t.lastProcessTime = time.Time{}
	t.emulator = nil
	t.detector = nil
	t.rows = 0
	t.cols = 0
}
