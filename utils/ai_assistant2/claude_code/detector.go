package claude_code

import (
	"regexp"
	"strings"
	"time"

	"github.com/tuzig/vt10x"

	"code-kanban/utils/ai_assistant2/types"
)

// StatusDetector implements state detection for Claude Code
type StatusDetector struct {
	recentInput  string
	recentInput2 string
}

// NewStatusDetector creates a new Claude Code state detector
func NewStatusDetector() *StatusDetector {
	return &StatusDetector{}
}

// DetectStateFromLines implements structure-based state detection.
// The raw grid is currently unused but reserved for future improvements.
func (d *StatusDetector) DetectStateFromLines(lines []string, raw [][]vt10x.Glyph, cols int, timestamp time.Time, currentState types.State, lastDetectedAt time.Time, cursorX int, cursorY int) (types.State, bool) {
	// Claude Code doesn't need stability checking like Codex
	// Its UI is more stable and reliable
	s := d.detectStateWorkingAndWaiting(lines, cols)
	if s == types.StateUnknown {
		s = d.detectStateApproval(lines, cols)
	}

	// If state detected, it was actually detected from display
	if s != types.StateUnknown {
		return s, true
	}

	return types.StateUnknown, false
}

// containsTipLine checks if a line contains the Tip indicator
func (d *StatusDetector) containsTipLine(line string) bool {
	// Only match exact pattern: "  ⎿  Tip:"
	return strings.HasPrefix(line, "  ⎿  Tip: ")
}

// isWorkingTaskLine checks if a line represents a working task
func (d *StatusDetector) isWorkingTaskLine(line string) bool {
	// Pattern: symbol + text + … + (esc to interrupt
	pattern := regexp.MustCompile(`^[✻✽✶∴·○◆▪▫□■☐☑☒★☆✓✔✗✘⚬⚫⚪⬤◯▸▹►▻◂◃◄◅✢*]\s+.+…\s*\(esc\s+to\s+interrupt`)
	return pattern.MatchString(line)
}

func (d *StatusDetector) isSeparatorLine(line string, cols int) bool {
	separatorPattern := "─"
	chatBoxBorder := strings.Repeat(separatorPattern, cols)
	return line == chatBoxBorder
}

func (d *StatusDetector) GetRecentInput() string {
	if d.recentInput == "" {
		return d.recentInput2
	}
	return d.recentInput
}
