package websession

import (
	"bufio"
	"bytes"
	"io"
)

// nativeHistoryJSONLScanner preserves bufio.Scanner's line semantics without
// imposing its fixed maximum token size on locally generated history records.
type nativeHistoryJSONLScanner struct {
	reader *bufio.Reader
	line   []byte
	err    error
	done   bool
}

func newNativeHistoryJSONLScanner(reader io.Reader) *nativeHistoryJSONLScanner {
	return &nativeHistoryJSONLScanner{
		reader: bufio.NewReaderSize(reader, 64*1024),
	}
}

func (s *nativeHistoryJSONLScanner) Scan() bool {
	if s == nil || s.done {
		return false
	}

	line, err := s.reader.ReadBytes('\n')
	switch err {
	case nil:
	case io.EOF:
		s.done = true
		if len(line) == 0 {
			return false
		}
	default:
		s.done = true
		s.err = err
		return false
	}

	line = bytes.TrimSuffix(line, []byte{'\n'})
	line = bytes.TrimSuffix(line, []byte{'\r'})
	s.line = line
	return true
}

func (s *nativeHistoryJSONLScanner) Text() string {
	if s == nil {
		return ""
	}
	return string(s.line)
}

func (s *nativeHistoryJSONLScanner) Err() error {
	if s == nil {
		return nil
	}
	return s.err
}
