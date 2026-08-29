package websession

import (
	"errors"
	"strings"
	"testing"
	"testing/iotest"
)

func TestNativeHistoryJSONLScannerReadsLargeLinesAndTrailingRecord(t *testing.T) {
	largeLine := strings.Repeat("x", 2*1024*1024+1024)
	scanner := newNativeHistoryJSONLScanner(strings.NewReader("first\r\n\n" + largeLine))

	var lines []string
	for scanner.Scan() {
		if line := scanner.Text(); strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan native history: %v", err)
	}
	if len(lines) != 2 || lines[0] != "first" || lines[1] != largeLine {
		t.Fatalf("unexpected native history lines: count=%d", len(lines))
	}
}

func TestNativeHistoryJSONLScannerPropagatesReadErrors(t *testing.T) {
	expectedErr := errors.New("read failed")
	scanner := newNativeHistoryJSONLScanner(iotest.ErrReader(expectedErr))

	if scanner.Scan() {
		t.Fatal("expected scan to stop on a read error")
	}
	if !errors.Is(scanner.Err(), expectedErr) {
		t.Fatalf("expected read error, got %v", scanner.Err())
	}
}
