package codex

import (
	"regexp"
	"strings"
	"time"

	"github.com/tuzig/vt10x"

	"code-kanban/utils/ai_assistant2/types"
)

const (
	// minWorkingExitInterval is the minimum time required to exit from working state
	// This prevents false negatives when working indicator temporarily disappears between chunks
	minWorkingExitInterval = 1000 * time.Millisecond
)

const (
	codexInputPrompt  = "› "
	codexIndentPrefix = "  "
)

// StatusDetector implements state detection for Codex
type StatusDetector struct {
	// Codex fixed format: "[symbol] [action] ([time] • esc to interrupt)"
	// Examples: "◦ Working (5s • esc to interrupt)"
	//           "• Confirming content (15s • esc to interrupt)"
	workingPattern *regexp.Regexp

	// Selection arrow pattern for approval
	selectionPattern *regexp.Regexp

	recentInput  string
	recentInput2 string
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

// DetectStateFromLines analyzes multiple lines and returns the detected state.
// The raw glyph grid is currently unused but provided for future heuristics.
func (d *StatusDetector) DetectStateFromLines(lines []string, raw [][]vt10x.Glyph, cols int, timestamp time.Time, currentState types.State, lastDetectedAt time.Time, cursorX int, cursorY int) (types.State, bool) {
	if len(lines) == 0 {
		return types.StateUnknown, true
	}

	// Detect new state from current display
	newState := d.detectFromDisplay(lines, raw)

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
func (d *StatusDetector) detectFromDisplay(lines []string, raw [][]vt10x.Glyph) types.State {
	if state := d.detectStateWorkingAndWaiting(lines, raw); state != types.StateUnknown {
		return state
	}

	// Search from bottom to top
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]

		if d.isWorkedLine(line) {
			return types.StateWaitingInput
		}

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

func (d *StatusDetector) isWorkedLine(line string) bool {
	if strings.HasPrefix(line, "─ Worked for ") && strings.HasSuffix(line, "─────────") {
		return true
	}
	return false
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

	ret := d.workingPattern.MatchString(line)

	if ret {
		// 启动时的mcp加载
		if strings.Contains(line, "Starting MCP servers") {
			return false
		}
	}

	return ret
}

// Default detector instance
var defaultDetector = NewStatusDetector()

// DetectStateFromLines uses the default detector to analyze multiple lines
func DetectStateFromLines(lines []string, raw [][]vt10x.Glyph, cols int, timestamp time.Time, currentState types.State, lastDetectedAt time.Time, cursorX int, cursorY int) (types.State, bool) {
	return defaultDetector.DetectStateFromLines(lines, raw, cols, timestamp, currentState, lastDetectedAt, cursorX, cursorY)
}

func (d *StatusDetector) GetRecentInput() string {
	if d.recentInput == "" {
		return d.recentInput2
	}
	return d.recentInput
}

func (d *StatusDetector) detectStateWorkingAndWaiting(lines []string, raw [][]vt10x.Glyph) types.State {
	if len(lines) == 0 {
		return types.StateUnknown
	}

	startIdx, endIdx, isEmpty := detectInputWindow(lines, raw)
	if !(startIdx == -1 || endIdx == -1) {
		d.captureRecentInput(lines[startIdx:endIdx], isEmpty)
	}

	currentLine := startIdx - 1
	for ; currentLine >= 0; currentLine-- {
		line := lines[currentLine]
		if d.containsTipLine(line) {
			if currentLine > 0 && d.isWorkingLine(lines[currentLine-1]) {
				return types.StateWorking
			}
		}
		if d.isWorkingLine(line) {
			return types.StateWorking
		}
	}

	return types.StateWaitingInput
}

func (d *StatusDetector) containsTipLine(line string) bool {
	return strings.HasPrefix(line, "  ?  Tip: ")
}

func detectInputWindow(lines []string, raw [][]vt10x.Glyph) (start, end int, isEmpty bool) {
	// 最后一行一般总是空行 所以跳过。接下来是 xx% context left
	for end = len(lines) - 2; end >= 0; end-- {
		if !isBlankLine(lines[end]) {
			continue
		}

		for start = end - 1; start >= 0; start-- {
			line := lines[start]
			switch {
			case strings.HasPrefix(line, codexInputPrompt):
				// check raw mode
				if raw[start][2].Mode&int16(vt10x.AttrFaint) != 0 {
					// faint text, empty line
					return start, end, true
				}
				return start, end, false
			case strings.HasPrefix(line, codexIndentPrefix):
				continue
			case isBlankLine(line):
				start = -1
			}
			break
		}
	}
	return -1, -1, false
}

func isBlankLine(line string) bool {
	return strings.TrimSpace(line) == ""
}

func (d *StatusDetector) captureRecentInput(inputLines []string, isEmpty bool) {
	if len(inputLines) == 0 {
		return
	}

	var recentInput string

	if !isEmpty {
		var builder strings.Builder
		for idx, rawLine := range inputLines {
			line := strings.TrimRight(rawLine, "\r")
			switch {
			case idx == 0 && strings.HasPrefix(line, codexInputPrompt):
				line = strings.TrimSpace(strings.TrimPrefix(line, codexInputPrompt))
			case strings.HasPrefix(line, codexIndentPrefix):
				line = strings.TrimSpace(strings.TrimPrefix(line, codexIndentPrefix))
			default:
				line = strings.TrimSpace(line)
			}

			if line == "" {
				continue
			}

			if builder.Len() > 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString(line)
		}

		recentInput = builder.String()
	}

	if recentInput == "" || recentInput == d.recentInput {
		return
	}

	d.recentInput2 = d.recentInput
	d.recentInput = recentInput
}
