package websession

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreDeleteSessionFilesRejectsUnsafeIDs(t *testing.T) {
	dataDir := t.TempDir()
	store, err := newStore(dataDir)
	if err != nil {
		t.Fatalf("newStore returned error: %v", err)
	}
	attachmentMarker := filepath.Join(store.attachmentsDir, "keep.txt")
	if err := os.WriteFile(attachmentMarker, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write attachment marker: %v", err)
	}
	outsideMarker := filepath.Join(dataDir, "keep.txt")
	if err := os.WriteFile(outsideMarker, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write outside marker: %v", err)
	}

	for _, sessionID := range []string{"", ".", "..", "_attachments", "_attachments.", " session", "session ", "session:stream", "../keep.txt", `..\keep.txt`, filepath.Join(dataDir, "keep.txt")} {
		if err := store.deleteSessionFiles(sessionID); err == nil {
			t.Errorf("deleteSessionFiles(%q) returned nil", sessionID)
		}
	}
	for _, marker := range []string{attachmentMarker, outsideMarker} {
		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("unsafe deletion removed %s: %v", marker, err)
		}
	}
}

func TestStoreReadEventsAfterIsStrictAndSupportsLargeEvents(t *testing.T) {
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatalf("newStore returned error: %v", err)
	}
	sessionID := "strict-reader"
	largeText := strings.Repeat("x", 8*1024*1024+1)
	for _, event := range []Event{
		{ID: "evt-2", Seq: 2, Type: "usage", Timestamp: time.Now()},
		{ID: "evt-5", Seq: 5, Type: "usage", Timestamp: time.Now(), Payload: map[string]any{"raw": largeText}},
	} {
		if err := store.appendEvent(sessionID, event); err != nil {
			t.Fatalf("append event %d: %v", event.Seq, err)
		}
	}
	events, err := store.readEventsAfter(sessionID, 2)
	if err != nil || len(events) != 1 || events[0].Seq != 5 {
		t.Fatalf("readEventsAfter = %#v, %v", events, err)
	}

	file, err := os.OpenFile(store.historyPath(sessionID), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open history tail: %v", err)
	}
	if _, err := file.WriteString(`{"id":"evt-5-duplicate","seq":5,"type":"usage"}` + "\n"); err != nil {
		_ = file.Close()
		t.Fatalf("append duplicate sequence: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close history tail: %v", err)
	}
	if _, err := store.readEventsAfter(sessionID, 0); err == nil {
		t.Fatal("readEventsAfter accepted a duplicate event sequence")
	}
}
