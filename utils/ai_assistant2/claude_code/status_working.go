package claude_code

import (
	"code-kanban/utils/ai_assistant2/types"
	"fmt"
	"strings"
)

func (d *StatusDetector) detectStateWorkingAndWaiting(lines []string, cols int) types.State {
	if len(lines) == 0 || cols <= 0 {
		return types.StateUnknown
	}

	currentLine := len(lines) - 1

	// Step 1: Find the input text box by locating two separator lines
	// Search from bottom to top for lines filled with '─' characters
	firstSepIdx := -1
	secondSepIdx := -1

	for ; currentLine >= 0; currentLine-- {
		line := lines[currentLine]

		// Check if this line is a separator (filled with ─)
		if d.isSeparatorLine(line, cols) {
			if firstSepIdx == -1 {
				firstSepIdx = currentLine
			} else {
				secondSepIdx = currentLine
				break
			}
		}
	}

	if firstSepIdx == -1 || secondSepIdx == -1 {
		return types.StateUnknown
	}

	// 顺手取出两线之中的内容
	recentInputs := lines[secondSepIdx+1 : firstSepIdx]
	for i := range recentInputs {
		recentInputs[i] = strings.TrimSpace(recentInputs[i])
	}
	recentInput := strings.Join(recentInputs, "")
	recentInput, _ = strings.CutPrefix(recentInput, ">")
	recentInput = strings.TrimSpace(recentInput)
	if recentInput != d.recentInput {
		d.recentInput2 = d.recentInput
		d.recentInput = recentInput
	}

	// If we found the input text box (two separator lines)
	// The text box is located, which means the interface is active
	// Now search upward from the second separator to determine the state

	// Step 2: Look for "  ⎿  Tip: " above the text box
	currentLine = secondSepIdx - 1
	for ; currentLine >= 0; currentLine-- {
		line := lines[currentLine]
		if d.containsTipLine(line) {
			fmt.Println(firstSepIdx, secondSepIdx, d.isWorkingTaskLine(lines[currentLine-1]))

			if currentLine > 0 && d.isWorkingTaskLine(lines[currentLine-1]) {
				return types.StateWorking
			}
		}
		if d.isWorkingTaskLine(line) {
			return types.StateWorking
		}
	}

	// No Tip line found = waiting for input
	return types.StateWaitingInput
}
