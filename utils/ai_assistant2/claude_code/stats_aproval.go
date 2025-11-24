package claude_code

import (
	"code-kanban/utils/ai_assistant2/types"
	"regexp"
	"strings"
)

func (d *StatusDetector) detectStateApproval(lines []string, cols int) types.State {
	if len(lines) == 0 || cols <= 0 {
		return types.StateUnknown
	}

	// Search from bottom to top
	totalLines := len(lines)

	// Navigation prompt pattern for selection menu
	// Must start with "Enter to select"
	navPrompt := "Enter to select"

	// Selection arrow pattern: ❯ followed by digit and dot
	arrowPattern := regexp.MustCompile(`^❯\s+\d+\.`)

	// approval flag pattern: ← at start, → at end
	appovalPattern := regexp.MustCompile(`^←.*→\s*$`)

	// Case 1: Look for navigation prompt line
	for i := totalLines - 1; i >= 0; i-- {
		line := lines[i]

		// Check for navigation prompt that starts with "Enter to select"
		if strings.HasPrefix(line, navPrompt) && strings.Contains(line, "Tab/Arrow keys to navigate") {
			// Found navigation prompt, now search upward for arrow selection line
			for j := i - 1; j >= 0; j-- {
				if arrowPattern.MatchString(lines[j]) {
					// Found arrow line, now search upward for box border line
					for k := j - 1; k >= 0; k-- {
						if appovalPattern.MatchString(lines[k]) {
							// Found approval flag, check if previous line is separator
							if k > 0 && d.isSeparatorLine(lines[k-1], cols) {
								return types.StateWaitingApproval
							}
						}
					}
					break
				}
			}
		}

		// Case 2: Look for "Ready to submit your answers?" line (not at the very bottom)
		if strings.HasPrefix(line, "Ready to submit your answers?") && i < totalLines-1 {
			// Found the prompt, search upward for box border line
			for k := i - 1; k >= 0; k-- {
				if appovalPattern.MatchString(lines[k]) {
					// Found approval flag, check if previous line is separator
					if k > 0 && d.isSeparatorLine(lines[k-1], cols) {
						return types.StateWaitingApproval
					}
				}
			}
		}

		// Case 3: Do you want to .... (permission required, Do you want to create xxxx[Create file] ....)
		if strings.HasPrefix(line, " Do you want to ") && i < totalLines-1 {
			for k := i - 1; k >= 0; k-- {
				if arrowPattern.MatchString(lines[k]) {
					return types.StateWaitingApproval
				}
			}
		}

		// Case 4: Do you want to proceed? (permission required)
		if strings.HasPrefix(line, " Esc to exit") && i < totalLines-1 {
			for k := i - 1; k >= 0; k-- {
				if strings.HasPrefix(lines[k], " Do you want to proceed?") {
					return types.StateWaitingApproval
				}
			}
		}
	}

	return types.StateUnknown
}
