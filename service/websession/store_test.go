package websession

import (
	"os"
	"path/filepath"
	"testing"
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
