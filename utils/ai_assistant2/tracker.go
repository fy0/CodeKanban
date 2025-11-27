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

type TrackingMode string

const (
	// periodicCheckInterval is how often we check state when no new chunks arrive
	periodicCheckInterval = 500 * time.Millisecond
	minProcessInterval    = 100 * time.Millisecond

	// NOTE: TrackingModeCapture 失败了，往往连续1s从系统终端中拿到的行都不变，无法应对codex这种不总是显示工作状态的cli
	TrackingModeCapture         TrackingMode = "capture"
	TrackingModeVirtualTerminal TrackingMode = "virtual-terminal"
)

// ParseTrackingMode normalizes incoming config to a supported mode.
func ParseTrackingMode(mode string) TrackingMode {
	switch TrackingMode(mode) {
	case TrackingModeVirtualTerminal:
		return TrackingModeVirtualTerminal
	default:
		return TrackingModeCapture
	}
}

// CaptureLinesFunc retrieves the latest terminal display as a list of visible lines.
type CaptureLinesFunc func(rows, cols int) ([]string, error)

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
	emulator     vt10x.Terminal
	rows         int
	cols         int
	trackingMode TrackingMode
	captureFunc  CaptureLinesFunc
	captureBusy  bool
	totalChunks  int64

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
		lastState:    types.StateUnknown,
		trackingMode: TrackingModeCapture,
	}
}

// SetCaptureFunc configures how capture mode retrieves the latest terminal lines.
func (t *StatusTracker) SetCaptureFunc(fn CaptureLinesFunc) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.captureFunc = fn
}

// SetTrackingMode updates how the tracker collects lines.
func (t *StatusTracker) SetTrackingMode(mode TrackingMode) {
	t.mu.Lock()
	defer t.mu.Unlock()
	mode = ParseTrackingMode(string(mode))
	if t.trackingMode == mode {
		return
	}
	t.trackingMode = mode
	if !t.active {
		return
	}

	if mode == TrackingModeVirtualTerminal {
		t.emulator = vt10x.New(vt10x.WithSize(t.cols, t.rows))
	} else {
		t.emulator = nil
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
			t.ensureEmulatorSizeLocked(cols, rows)
		}
		return
	}

	// Create new emulator and detector for this assistant
	t.assistantType = assistantType
	t.active = true
	t.rows = rows
	t.cols = cols
	if t.trackingMode == TrackingModeVirtualTerminal {
		t.emulator = vt10x.New(vt10x.WithSize(cols, rows))
	} else {
		t.emulator = nil
	}
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

func (t *StatusTracker) ProcessChunkInvoke(chunk []byte) {
	t.ProcessChunk(chunk)
}

// ProcessChunk feeds a terminal output chunk to the emulator and detects state changes
func (t *StatusTracker) ProcessChunk(chunk []byte) (types.State, time.Time, bool) {
	if len(chunk) == 0 {
		return types.StateUnknown, time.Time{}, false
	}

	now := time.Now()
	t.totalChunks++

	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.active || t.detector == nil {
		return types.StateUnknown, time.Time{}, false
	}

	var lines []string
	var err error

	// Virtual terminal mode processes chunks sequentially while holding the lock.
	t.ensureEmulatorSizeLocked(t.cols, t.rows)
	if t.emulator == nil {
		return types.StateUnknown, time.Time{}, false
	}
	t.emulator.Write(chunk)

	// 节流，但必须确保写入chunk
	if !t.lastProcessTime.IsZero() && now.Sub(t.lastProcessTime) < minProcessInterval {
		return types.StateUnknown, time.Time{}, false
	}

	t.lastProcessTime = now
	lines = getVisibleLinesLocked(t)

	if err != nil || len(lines) == 0 || !t.active || t.detector == nil {
		return types.StateUnknown, time.Time{}, false
	}

	state, ts, changed := t.detectStateFromLinesLocked(lines, now)
	if changed {
		t.emitStateChangeLocked(state, ts)
	}
	return state, ts, changed
}

func (t *StatusTracker) detectStateFromLinesLocked(lines []string, now time.Time) (types.State, time.Time, bool) {
	if t.detector == nil || len(lines) == 0 {
		return types.StateUnknown, time.Time{}, false
	}

	detectedState, changeRecentUpdate := t.detector.DetectStateFromLines(lines, t.cols, now, t.lastState, t.recentUpdatedAt)

	if changeRecentUpdate {
		t.recentUpdatedAt = now
	}

	if detectedState == types.StateUnknown {
		return types.StateUnknown, time.Time{}, false
	}

	if detectedState != t.lastState {
		t.lastState = detectedState
		t.lastChangedAt = now
		return detectedState, now, true
	}

	return types.StateUnknown, time.Time{}, false
}

// State returns the current state and timestamp
func (t *StatusTracker) State() (types.State, time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastState, t.lastChangedAt
}

// ChunkCount returns how many chunks have been processed.
func (t *StatusTracker) ChunkCount() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.totalChunks
}

// TrackingMode returns the current tracking mode.
func (t *StatusTracker) TrackingMode() TrackingMode {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.trackingMode
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

	if !t.active || t.detector == nil {
		return
	}

	now := time.Now()

	// Only check if ProcessChunk hasn't been called for periodicCheckInterval
	if now.Sub(t.lastProcessTime) < periodicCheckInterval {
		return
	}

	if t.emulator == nil {
		return
	}
	lines := getVisibleLinesLocked(t)

	if len(lines) == 0 {
		return
	}

	state, ts, changed := t.detectStateFromLinesLocked(lines, now)
	if changed {
		t.emitStateChangeLocked(state, ts)
	}
}

// ensureEmulatorSizeLocked lazily creates or resizes the vt10x emulator to match the current terminal size.
func (t *StatusTracker) ensureEmulatorSizeLocked(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}

	if t.emulator == nil {
		t.emulator = vt10x.New(vt10x.WithSize(cols, rows))
		return
	}

	curCols, curRows := t.emulator.Size()
	if curCols == cols && curRows == rows {
		return
	}
	t.emulator.Resize(cols, rows)
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
	t.captureBusy = false
	t.totalChunks = 0
}

func (t *StatusTracker) emitStateChangeLocked(state types.State, ts time.Time) {
	callback := t.callback
	if callback == nil {
		return
	}
	t.mu.Unlock()
	callback(state, ts)
	t.mu.Lock()
}
