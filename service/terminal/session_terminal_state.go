package terminal

import (
	"bytes"
	"fmt"
	"io"
	"runtime"
	"time"

	"github.com/tuzig/vt10x"
)

const (
	terminalAttrReverse   int16 = 1 << 0
	terminalAttrUnderline int16 = 1 << 1
	terminalAttrBold      int16 = 1 << 2
	terminalAttrItalic    int16 = 1 << 4
	terminalAttrBlink     int16 = 1 << 5
	terminalAttrFaint     int16 = 1 << 7
	terminalAttrWideDummy int16 = 1 << 9
	terminalAttrVisible   int16 = terminalAttrReverse |
		terminalAttrUnderline |
		terminalAttrBold |
		terminalAttrItalic |
		terminalAttrBlink |
		terminalAttrFaint
)

type TerminalMirrorSnapshot struct {
	Rows          int
	Cols          int
	Lines         [][]byte
	Cursor        []byte
	TerminalModes *TerminalModesSnapshot
	AltScreen     bool
	CursorVisible bool
	ModeFlags     uint32
	CapturedAt    time.Time
}

type terminalStateReplyWriter struct {
	session *Session
}

func (w terminalStateReplyWriter) Write(p []byte) (int, error) {
	if w.session == nil {
		return len(p), nil
	}
	return w.session.writeTerminalStateReply(p)
}

func (s *Session) writeTerminalStateReply(p []byte) (int, error) {
	if len(p) == 0 || s == nil {
		return len(p), nil
	}
	if s.terminalStateSuppressReplies.Load() {
		return len(p), nil
	}

	writer := s.Writer()
	if writer == nil {
		return 0, io.EOF
	}
	return writer.Write(p)
}

func (s *Session) initTerminalStateLocked(cols, rows int) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	s.terminalState = vt10x.New(
		vt10x.WithXtermStyle(),
		vt10x.WithSize(cols, rows),
		vt10x.WithWriter(terminalStateReplyWriter{session: s}),
	)
	s.terminalStateCapturedAt = time.Time{}
}

func (s *Session) terminalStateEnabledForPlatform() bool {
	return runtime.GOOS != "windows" && s.terminalStateEnabled.Load()
}

func (s *Session) appendTerminalState(chunk []byte) {
	if len(chunk) == 0 || !s.terminalStateEnabledForPlatform() {
		return
	}

	s.terminalStateMu.Lock()
	defer s.terminalStateMu.Unlock()

	if s.terminalState == nil {
		s.mu.RLock()
		cols := s.cols
		rows := s.rows
		s.mu.RUnlock()
		s.initTerminalStateLocked(cols, rows)
	}
	if s.terminalState == nil {
		return
	}

	_, _ = s.terminalState.Write(chunk)
	s.terminalStateCapturedAt = time.Now()
}

func (s *Session) resizeTerminalState(cols, rows int) {
	if !s.terminalStateEnabledForPlatform() {
		return
	}

	s.terminalStateMu.Lock()
	defer s.terminalStateMu.Unlock()

	if s.terminalState == nil {
		s.initTerminalStateLocked(cols, rows)
		return
	}
	s.terminalState.Resize(cols, rows)
}

func (s *Session) rebuildTerminalStateFromScrollbackLocked() {
	s.mu.RLock()
	cols := s.cols
	rows := s.rows
	s.mu.RUnlock()

	if s.terminalState == nil {
		s.initTerminalStateLocked(cols, rows)
	}
	if s.terminalState == nil {
		return
	}

	s.terminalState.Resize(cols, rows)
	s.terminalStateSuppressReplies.Store(true)
	defer s.terminalStateSuppressReplies.Store(false)
	_, _ = s.terminalState.Write([]byte("\x1bc\x1b[2J\x1b[H"))

	s.scrollMu.RLock()
	scrollback := make([][]byte, len(s.scrollback))
	copy(scrollback, s.scrollback)
	timestamps := make([]time.Time, len(s.scrollbackTimestamps))
	copy(timestamps, s.scrollbackTimestamps)
	s.scrollMu.RUnlock()

	for _, chunk := range scrollback {
		if len(chunk) == 0 {
			continue
		}
		_, _ = s.terminalState.Write(chunk)
	}
	if len(timestamps) > 0 {
		s.terminalStateCapturedAt = timestamps[len(timestamps)-1]
	} else {
		s.terminalStateCapturedAt = time.Time{}
	}
}

func (s *Session) SetTerminalStateSnapshotEnabled(enabled bool) {
	enabled = enabled && runtime.GOOS != "windows"
	s.terminalStateEnabled.Store(enabled)

	s.terminalStateMu.Lock()
	defer s.terminalStateMu.Unlock()

	if !enabled {
		s.terminalState = nil
		s.terminalStateCapturedAt = time.Time{}
		return
	}

	s.rebuildTerminalStateFromScrollbackLocked()
}

func (s *Session) TerminalStateSnapshot() *TerminalStateSnapshot {
	if !s.terminalStateEnabledForPlatform() {
		return nil
	}

	s.terminalStateMu.Lock()
	defer s.terminalStateMu.Unlock()

	if s.terminalState == nil {
		s.rebuildTerminalStateFromScrollbackLocked()
	}
	if s.terminalState == nil {
		return nil
	}

	cols, rows := s.terminalState.Size()
	if cols <= 0 || rows <= 0 {
		return nil
	}

	cells := make([][]TerminalStateCell, rows)
	for row := 0; row < rows; row++ {
		rowCells := make([]TerminalStateCell, cols)
		for col := 0; col < cols; col++ {
			cell := s.terminalState.Cell(col, row)
			rowCells[col] = snapshotCellFromGlyph(cell)
		}
		cells[row] = rowCells
	}

	cursor := s.terminalState.Cursor()
	return &TerminalStateSnapshot{
		Rows:            rows,
		Cols:            cols,
		Cells:           cells,
		CursorX:         cursor.X,
		CursorY:         cursor.Y,
		CursorVisible:   s.terminalState.CursorVisible(),
		CursorMode:      cursor.Attr.Mode,
		CursorFG:        snapshotColorValue(cursor.Attr.FG),
		CursorBG:        snapshotColorValue(cursor.Attr.BG),
		CursorFGDefault: cursor.Attr.FG == vt10x.DefaultFG,
		CursorBGDefault: cursor.Attr.BG == vt10x.DefaultBG,
		CapturedAt:      s.terminalStateCapturedAt,
	}
}

func (s *Session) TerminalMirrorSnapshot() *TerminalMirrorSnapshot {
	if !s.terminalStateEnabledForPlatform() {
		return nil
	}

	s.terminalStateMu.Lock()
	defer s.terminalStateMu.Unlock()

	if s.terminalState == nil {
		s.rebuildTerminalStateFromScrollbackLocked()
	}
	if s.terminalState == nil {
		return nil
	}

	return s.terminalMirrorSnapshotLocked()
}

func (s *Session) terminalMirrorSnapshotLocked() *TerminalMirrorSnapshot {
	if s.terminalState == nil {
		return nil
	}

	cols, rows := s.terminalState.Size()
	if cols <= 0 || rows <= 0 {
		return nil
	}

	lines := make([][]byte, rows)
	for row := 0; row < rows; row++ {
		var line bytes.Buffer
		previousStyle := ""
		endCol := terminalMirrorLineEndColumn(s.terminalState, row, cols)
		for col := 0; col < endCol; col++ {
			glyph := s.terminalState.Cell(col, row)
			if terminalCellModeHas(glyph.Mode, terminalAttrWideDummy) {
				continue
			}
			nextStyle := buildTerminalSnapshotSGRFromGlyph(glyph)
			if nextStyle != previousStyle {
				line.WriteString(nextStyle)
				previousStyle = nextStyle
			}
			if glyph.Char != 0 {
				line.WriteRune(glyph.Char)
			} else {
				line.WriteByte(' ')
			}
		}
		lines[row] = line.Bytes()
	}

	cursor := s.terminalState.Cursor()
	altScreen := s.terminalState.Mode()&vt10x.ModeAltScreen != 0
	var cursorBuffer bytes.Buffer
	cursorBuffer.WriteString(buildTerminalSnapshotSGRFromColors(cursor.Attr.Mode, cursor.Attr.FG, cursor.Attr.BG))
	cursorBuffer.WriteString(fmt.Sprintf(
		"\x1b[%d;%dH",
		clampTerminalCoordinate(cursor.Y+1, 1, rows),
		clampTerminalCoordinate(cursor.X+1, 1, cols),
	))
	if s.terminalState.CursorVisible() {
		cursorBuffer.WriteString("\x1b[?25h")
	} else {
		cursorBuffer.WriteString("\x1b[?25l")
	}

	return &TerminalMirrorSnapshot{
		Rows:          rows,
		Cols:          cols,
		Lines:         lines,
		Cursor:        cursorBuffer.Bytes(),
		TerminalModes: s.TerminalModesSnapshot(),
		AltScreen:     altScreen,
		CursorVisible: s.terminalState.CursorVisible(),
		ModeFlags:     uint32(s.terminalState.Mode()),
		CapturedAt:    s.terminalStateCapturedAt,
	}
}

func terminalMirrorLineEndColumn(view vt10x.View, row, cols int) int {
	for col := cols - 1; col >= 0; col-- {
		glyph := view.Cell(col, row)
		if terminalMirrorGlyphKeepsLineOpen(glyph) {
			return col + 1
		}
	}
	return 0
}

// vt10x stores both untouched padding and default-styled trailing spaces as blank-space glyphs,
// so snapshot trimming can only preserve suffix cells that still carry visible content or style.
func terminalMirrorGlyphKeepsLineOpen(glyph vt10x.Glyph) bool {
	if terminalCellModeHas(glyph.Mode, terminalAttrWideDummy) {
		return false
	}

	if glyph.Char != 0 && glyph.Char != ' ' {
		return true
	}

	if glyph.FG != vt10x.DefaultFG || glyph.BG != vt10x.DefaultBG {
		return true
	}

	return glyph.Mode&terminalAttrVisible != 0
}

func snapshotCellFromGlyph(cell vt10x.Glyph) TerminalStateCell {
	char := ""
	if cell.Char != 0 {
		char = string(cell.Char)
	}
	return TerminalStateCell{
		Char:      char,
		Mode:      cell.Mode,
		FG:        snapshotColorValue(cell.FG),
		BG:        snapshotColorValue(cell.BG),
		FGDefault: cell.FG == vt10x.DefaultFG,
		BGDefault: cell.BG == vt10x.DefaultBG,
	}
}

func snapshotColorValue(color vt10x.Color) uint32 {
	if color == vt10x.DefaultFG || color == vt10x.DefaultBG || color == vt10x.DefaultCursor {
		return 0
	}
	return uint32(color)
}

func terminalCellModeHas(mode, flag int16) bool {
	return mode&flag == flag
}

func buildTerminalSnapshotSGRFromGlyph(cell vt10x.Glyph) string {
	return buildTerminalSnapshotSGRFromColors(cell.Mode, cell.FG, cell.BG)
}

func buildTerminalSnapshotSGRFromColors(mode int16, fg, bg vt10x.Color) string {
	codes := make([]any, 0, 10)

	if terminalCellModeHas(mode, terminalAttrReverse) {
		codes = append(codes, 7)
	} else {
		codes = append(codes, 27)
	}

	if terminalCellModeHas(mode, terminalAttrUnderline) {
		codes = append(codes, 4)
	} else {
		codes = append(codes, 24)
	}

	if terminalCellModeHas(mode, terminalAttrBold) {
		codes = append(codes, 1)
	} else {
		codes = append(codes, 22)
	}

	if terminalCellModeHas(mode, terminalAttrItalic) {
		codes = append(codes, 3)
	} else {
		codes = append(codes, 23)
	}

	if terminalCellModeHas(mode, terminalAttrBlink) {
		codes = append(codes, 5)
	} else {
		codes = append(codes, 25)
	}

	if terminalCellModeHas(mode, terminalAttrFaint) {
		codes = append(codes, 2)
	}

	if fg == vt10x.DefaultFG || fg == vt10x.DefaultBG || fg == vt10x.DefaultCursor {
		codes = append(codes, 39)
	} else {
		r := (fg >> 16) & 0xff
		g := (fg >> 8) & 0xff
		b := fg & 0xff
		codes = append(codes, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
	}

	if bg == vt10x.DefaultFG || bg == vt10x.DefaultBG || bg == vt10x.DefaultCursor {
		codes = append(codes, 49)
	} else {
		r := (bg >> 16) & 0xff
		g := (bg >> 8) & 0xff
		b := bg & 0xff
		codes = append(codes, fmt.Sprintf("48;2;%d;%d;%d", r, g, b))
	}

	return fmt.Sprintf("\x1b[%sm", joinTerminalSGRCodes(codes))
}

func joinTerminalSGRCodes(codes []any) string {
	if len(codes) == 0 {
		return "0"
	}

	var buffer bytes.Buffer
	for index, code := range codes {
		if index > 0 {
			buffer.WriteByte(';')
		}
		buffer.WriteString(fmt.Sprint(code))
	}
	return buffer.String()
}

func clampTerminalCoordinate(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
