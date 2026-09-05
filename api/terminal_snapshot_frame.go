package api

import (
	"bytes"
	"encoding/binary"
	"sync"

	"code-kanban/service/terminal"
)

const terminalSnapshotFrameVersion = 6

type terminalSnapshotFrameKind uint8

const (
	terminalSnapshotFrameKindFull terminalSnapshotFrameKind = iota
	terminalSnapshotFrameKindDelta
)

type terminalMirrorChangedRow struct {
	Index   int
	Content []byte
}

type terminalMirrorBaseline struct {
	Rows          int
	Cols          int
	Lines         [][]byte
	Cursor        []byte
	AltScreen     bool
	CursorVisible bool
	ModeFlags     uint32
	CapturedAt    int64
	Sequence      uint32
}

type terminalMirrorSenderState struct {
	mu                 sync.Mutex
	last               *terminalMirrorBaseline
	incrementalEnabled bool
}

func newTerminalMirrorSenderState() *terminalMirrorSenderState {
	return &terminalMirrorSenderState{
		incrementalEnabled: true,
	}
}

func (s *terminalMirrorSenderState) IncrementalEnabled() bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.incrementalEnabled
}

func (s *terminalMirrorSenderState) SetIncrementalEnabled(enabled bool) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := s.incrementalEnabled != enabled
	s.incrementalEnabled = enabled
	return changed
}

func (s *terminalMirrorSenderState) EncodeFrame(
	snapshot *terminal.TerminalMirrorSnapshot,
	forceFull bool,
	compressionEnabled bool,
) ([]byte, bool, error) {
	if s == nil || snapshot == nil {
		return nil, false, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !forceFull && terminalMirrorBaselineEqualsSnapshot(s.last, snapshot) {
		return nil, false, nil
	}

	nextSequence := uint32(1)
	if s.last != nil {
		nextSequence = s.last.Sequence + 1
	}

	fullFrame := encodeTerminalMirrorFrame(
		snapshot,
		nil,
		terminalSnapshotFrameKindFull,
		nextSequence,
		0,
		compressionEnabled,
	)
	selectedFrame := fullFrame

	if !forceFull && s.incrementalEnabled && terminalMirrorCanDelta(s.last, snapshot) {
		changedRows := diffTerminalMirrorRows(s.last, snapshot)
		deltaFrame := encodeTerminalMirrorFrame(
			snapshot,
			changedRows,
			terminalSnapshotFrameKindDelta,
			nextSequence,
			s.last.Sequence,
			compressionEnabled,
		)
		if len(deltaFrame) < len(fullFrame) {
			selectedFrame = deltaFrame
		}
	}

	s.last = cloneTerminalMirrorBaseline(snapshot, nextSequence)
	return selectedFrame, true, nil
}

func terminalMirrorCanDelta(
	last *terminalMirrorBaseline,
	snapshot *terminal.TerminalMirrorSnapshot,
) bool {
	if last == nil || snapshot == nil {
		return false
	}
	if last.Rows != snapshot.Rows || last.Cols != snapshot.Cols {
		return false
	}
	if last.AltScreen != snapshot.AltScreen {
		return false
	}
	if last.ModeFlags != snapshot.ModeFlags {
		return false
	}
	return true
}

func terminalMirrorBaselineEqualsSnapshot(
	last *terminalMirrorBaseline,
	snapshot *terminal.TerminalMirrorSnapshot,
) bool {
	if !terminalMirrorCanDelta(last, snapshot) {
		return false
	}
	if last.CursorVisible != snapshot.CursorVisible {
		return false
	}
	if !bytes.Equal(last.Cursor, snapshot.Cursor) {
		return false
	}
	if len(last.Lines) != len(snapshot.Lines) {
		return false
	}
	for row := range last.Lines {
		if !bytes.Equal(last.Lines[row], snapshot.Lines[row]) {
			return false
		}
	}
	return true
}

func diffTerminalMirrorRows(
	last *terminalMirrorBaseline,
	snapshot *terminal.TerminalMirrorSnapshot,
) []terminalMirrorChangedRow {
	if snapshot == nil {
		return nil
	}
	if last == nil || len(last.Lines) != len(snapshot.Lines) {
		changed := make([]terminalMirrorChangedRow, 0, len(snapshot.Lines))
		for index, line := range snapshot.Lines {
			changed = append(changed, terminalMirrorChangedRow{
				Index:   index,
				Content: cloneSnapshotBytes(line),
			})
		}
		return changed
	}

	changed := make([]terminalMirrorChangedRow, 0, len(snapshot.Lines))
	for index, line := range snapshot.Lines {
		if bytes.Equal(last.Lines[index], line) {
			continue
		}
		changed = append(changed, terminalMirrorChangedRow{
			Index:   index,
			Content: cloneSnapshotBytes(line),
		})
	}
	return changed
}

func cloneTerminalMirrorBaseline(
	snapshot *terminal.TerminalMirrorSnapshot,
	sequence uint32,
) *terminalMirrorBaseline {
	if snapshot == nil {
		return nil
	}

	lines := make([][]byte, len(snapshot.Lines))
	for index, line := range snapshot.Lines {
		lines[index] = cloneSnapshotBytes(line)
	}

	capturedAt := int64(0)
	if !snapshot.CapturedAt.IsZero() {
		capturedAt = snapshot.CapturedAt.UnixMilli()
	}

	return &terminalMirrorBaseline{
		Rows:          snapshot.Rows,
		Cols:          snapshot.Cols,
		Lines:         lines,
		Cursor:        cloneSnapshotBytes(snapshot.Cursor),
		AltScreen:     snapshot.AltScreen,
		CursorVisible: snapshot.CursorVisible,
		ModeFlags:     snapshot.ModeFlags,
		CapturedAt:    capturedAt,
		Sequence:      sequence,
	}
}

func encodeTerminalMirrorFrame(
	snapshot *terminal.TerminalMirrorSnapshot,
	changedRows []terminalMirrorChangedRow,
	kind terminalSnapshotFrameKind,
	sequence uint32,
	baseSequence uint32,
	compressionEnabled bool,
) []byte {
	if snapshot == nil {
		return nil
	}

	payload := encodeTerminalMirrorPayload(snapshot, changedRows, kind)
	compressed := false
	if compressionEnabled {
		if compressedPayload, ok := compressSnapshotPayload(payload); ok {
			payload = compressedPayload
			compressed = true
		}
	}

	header := make([]byte, 27)
	header[0] = terminalSnapshotFrameVersion
	binary.BigEndian.PutUint16(header[1:3], uint16(snapshot.Rows))
	binary.BigEndian.PutUint16(header[3:5], uint16(snapshot.Cols))
	if !snapshot.CapturedAt.IsZero() {
		binary.BigEndian.PutUint64(header[5:13], uint64(snapshot.CapturedAt.UnixMilli()))
	}
	flags := uint8(0)
	if snapshot.AltScreen {
		flags |= 1 << 0
	}
	if snapshot.CursorVisible {
		flags |= 1 << 1
	}
	if compressed {
		flags |= 1 << 2
	}
	header[13] = flags
	binary.BigEndian.PutUint32(header[14:18], snapshot.ModeFlags)
	binary.BigEndian.PutUint32(header[18:22], sequence)
	binary.BigEndian.PutUint32(header[22:26], baseSequence)
	header[26] = uint8(kind)

	return append(header, payload...)
}

func encodeTerminalMirrorPayload(
	snapshot *terminal.TerminalMirrorSnapshot,
	changedRows []terminalMirrorChangedRow,
	kind terminalSnapshotFrameKind,
) []byte {
	var buffer bytes.Buffer

	switch kind {
	case terminalSnapshotFrameKindDelta:
		_ = binary.Write(&buffer, binary.BigEndian, uint16(len(changedRows)))
		for _, row := range changedRows {
			_ = binary.Write(&buffer, binary.BigEndian, uint16(row.Index))
			_ = binary.Write(&buffer, binary.BigEndian, uint32(len(row.Content)))
			buffer.Write(row.Content)
		}
	default:
		for _, line := range snapshot.Lines {
			_ = binary.Write(&buffer, binary.BigEndian, uint32(len(line)))
			buffer.Write(line)
		}
	}

	_ = binary.Write(&buffer, binary.BigEndian, uint32(len(snapshot.Cursor)))
	buffer.Write(snapshot.Cursor)
	return buffer.Bytes()
}

func cloneSnapshotBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}
