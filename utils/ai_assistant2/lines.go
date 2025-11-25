package ai_assistant2

import (
	"sync"

	"github.com/hinshun/vt10x"
)

var captureTerminalPool = sync.Pool{
	New: func() any {
		return vt10x.New()
	},
}

var captureClearSequence = []byte("\x1b[2J\x1b[H")

// getVisibleLinesLocked extracts visible lines from the emulator (must be called with lock held)
func getVisibleLinesLocked(t *StatusTracker) []string {
	if t.emulator == nil {
		return nil
	}

	return renderLinesFromTerminal(t.emulator, t.rows, t.cols)
}

func renderLinesFromTerminal(term vt10x.Terminal, rows, cols int) []string {
	if term == nil || rows <= 0 || cols <= 0 {
		return nil
	}

	lines := make([]string, 0, rows)
	runes := make([]rune, 0, cols)

	for row := 0; row < rows; row++ {
		runes = runes[:0]
		for col := 0; col < cols; col++ {
			cell := term.Cell(col, row)
			if cell.Char != 0 {
				runes = append(runes, cell.Char)
			}
		}
		lines = append(lines, string(runes))
	}

	return lines
}

// RenderLinesFromBuffer feeds data into a pooled terminal and returns visible rows.
func RenderLinesFromBuffer(data []byte, rows, cols int) []string {
	if len(data) == 0 || rows <= 0 || cols <= 0 {
		return nil
	}

	term := captureTerminalPool.Get().(vt10x.Terminal)
	defer captureTerminalPool.Put(term)

	term.Resize(cols, rows)
	_, _ = term.Write(captureClearSequence)
	_, _ = term.Write(data)

	return renderLinesFromTerminal(term, rows, cols)
}
