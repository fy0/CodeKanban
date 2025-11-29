package ai_assistant2

import (
	"sync"

	"github.com/tuzig/vt10x"
)

var captureTerminalPool = sync.Pool{
	New: func() any {
		return vt10x.New()
	},
}

var captureClearSequence = []byte("\x1b[2J\x1b[H")

// getVisibleLinesLocked extracts visible lines from the emulator (must be called with lock held)
func getVisibleLinesLocked(t *StatusTracker) ([]string, [][]vt10x.Glyph) {
	if t.emulator == nil {
		return nil, t.raw
	}

	lines, raw := renderLinesFromTerminal(t.emulator, t.raw, t.rows, t.cols)
	t.raw = raw
	return lines, raw
}

// renderLinesFromTerminal captures terminal contents and optionally copies glyphs into the provided raw grid.
func renderLinesFromTerminal(term vt10x.Terminal, raw [][]vt10x.Glyph, rows, cols int) ([]string, [][]vt10x.Glyph) {
	if term == nil || rows <= 0 || cols <= 0 {
		return nil, raw
	}

	termCols, termRows := term.Size()
	if termCols <= 0 || termRows <= 0 {
		return nil, raw
	}

	if rows > termRows {
		rows = termRows
	}
	if cols > termCols {
		cols = termCols
	}

	if raw != nil {
		if len(raw) != rows || (rows > 0 && (len(raw) == 0 || len(raw[0]) != cols)) {
			raw = ensureGlyphGrid(raw, rows, cols)
		}
	}

	lines := make([]string, 0, rows)
	runes := make([]rune, 0, cols)

	for row := 0; row < rows; row++ {
		runes = runes[:0]
		var rowRaw []vt10x.Glyph
		if raw != nil {
			rowRaw = raw[row]
		}
		for col := 0; col < cols; col++ {
			cell := term.Cell(col, row)
			if raw != nil {
				rowRaw[col] = cell
			}
			if cell.Char != 0 {
				runes = append(runes, cell.Char)
			}
		}
		lines = append(lines, string(runes))
	}

	return lines, raw
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

	lines, _ := renderLinesFromTerminal(term, nil, rows, cols)
	return lines
}

// RenderGlyphGridFromBuffer feeds data into a pooled terminal and returns the raw glyph grid.
func RenderGlyphGridFromBuffer(data []byte, rows, cols int) [][]vt10x.Glyph {
	if len(data) == 0 || rows <= 0 || cols <= 0 {
		return nil
	}

	term := captureTerminalPool.Get().(vt10x.Terminal)
	defer captureTerminalPool.Put(term)

	term.Resize(cols, rows)
	_, _ = term.Write(captureClearSequence)
	_, _ = term.Write(data)

	return renderRawFromTerminal(term, rows, cols)
}

func renderRawFromTerminal(term vt10x.Terminal, rows, cols int) [][]vt10x.Glyph {
	if term == nil || rows <= 0 || cols <= 0 {
		return nil
	}

	termCols, termRows := term.Size()
	if termCols <= 0 || termRows <= 0 {
		return nil
	}

	if rows > termRows {
		rows = termRows
	}
	if cols > termCols {
		cols = termCols
	}

	lines := make([][]vt10x.Glyph, 0, rows)
	runes := make([]vt10x.Glyph, 0, cols)

	for row := 0; row < rows; row++ {
		runes = runes[:0]
		for col := 0; col < cols; col++ {
			cell := term.Cell(col, row)
			runes = append(runes, cell)
		}
		rowCopy := make([]vt10x.Glyph, len(runes))
		copy(rowCopy, runes)
		lines = append(lines, rowCopy)
	}

	return lines
}

func ensureGlyphGrid(raw [][]vt10x.Glyph, rows, cols int) [][]vt10x.Glyph {
	if rows <= 0 || cols <= 0 {
		return nil
	}

	if len(raw) != rows {
		newRaw := make([][]vt10x.Glyph, rows)
		copy(newRaw, raw)
		raw = newRaw
	}

	for i := 0; i < rows; i++ {
		row := raw[i]
		if cap(row) < cols {
			row = make([]vt10x.Glyph, cols)
		} else {
			row = row[:cols]
		}
		raw[i] = row
	}

	return raw
}
