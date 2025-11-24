package codex

import (
	"regexp"
	"strings"
	"time"

	"code-kanban/utils/ai_assistant2/types"
)

const (
	// minWorkingExitInterval is the minimum time required to exit from working state
	// This prevents false negatives when working indicator temporarily disappears between chunks
	minWorkingExitInterval = 1000 * time.Millisecond
)

// StatusDetector implements state detection for Codex
type StatusDetector struct {
	// Codex fixed format: "[symbol] [action] ([time] • esc to interrupt)"
	// Examples: "◦ Working (5s • esc to interrupt)"
	//           "• Confirming content (15s • esc to interrupt)"
	workingPattern *regexp.Regexp

	// Selection arrow pattern for approval
	selectionPattern *regexp.Regexp
}

// NewStatusDetector creates a new Codex state detector
func NewStatusDetector() *StatusDetector {
	return &StatusDetector{
		// Match: symbol (◦ or •) + text + (time • esc to interrupt)
		// (10h 19m 44s • esc to interrupt) codex#6729
		workingPattern: regexp.MustCompile(`^[◦•] .+\((\d+h )?(\d+m )?\d+s • esc to interrupt\)`),

		// Selection arrow: › followed by digit and dot
		selectionPattern: regexp.MustCompile(`^› \d+\. `),
	}
}

// DetectStateFromLines analyzes multiple lines and returns the detected state
func (d *StatusDetector) DetectStateFromLines(lines []string, cols int, timestamp time.Time, currentState types.State, lastDetectedAt time.Time) (types.State, bool) {
	if len(lines) == 0 {
		return types.StateUnknown, true
	}

	// Detect new state from current display
	newState := d.detectFromDisplay(lines)

	// If nothing detected from display
	if newState == types.StateUnknown {
		return types.StateUnknown, true
	}

	// Apply stability check: prevent premature exit from working state
	// Codex sometimes has chunks without the working indicator, but is still working
	if currentState == types.StateWorking && newState != types.StateWorking {
		// Calculate time since last detecting working state
		timeSinceLastDetection := timestamp.Sub(lastDetectedAt)

		// If less than minimum interval, ignore this detection
		// Return StateUnknown to indicate we should keep the current state without updating recentUpdatedAt
		if timeSinceLastDetection < minWorkingExitInterval {
			return currentState, false
		}
	}

	// State was actually detected from display and passed stability check
	return newState, true
}

// detectFromDisplay analyzes display lines and returns the detected state (without stability checks)
func (d *StatusDetector) detectFromDisplay(lines []string) types.State {
	// Search from bottom to top
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]

		// Check for working state (fast path first)
		if d.isWorkingLine(line) {
			return types.StateWorking
		}

		// Check for approval state: "  Press enter to confirm..."
		if strings.HasPrefix(line, "  Press enter to confirm or esc to cancel") {
			// Search upward for selection arrow
			for j := i - 1; j >= 0; j-- {
				if d.selectionPattern.MatchString(lines[j]) {
					return types.StateWaitingApproval
				}
			}
		}
	}

	return types.StateWaitingInput
}

// isWorkingLine checks if a line indicates Codex is working
func (d *StatusDetector) isWorkingLine(line string) bool {
	if line == "" {
		return false
	}

	// Fast path: check for "esc to interrupt)" substring first
	if !strings.Contains(line, "esc to interrupt)") {
		return false
	}

	return d.workingPattern.MatchString(line)
}

// Default detector instance
var defaultDetector = NewStatusDetector()

// DetectStateFromLines uses the default detector to analyze multiple lines
func DetectStateFromLines(lines []string, cols int, timestamp time.Time, currentState types.State, lastDetectedAt time.Time) (types.State, bool) {
	return defaultDetector.DetectStateFromLines(lines, cols, timestamp, currentState, lastDetectedAt)
}
