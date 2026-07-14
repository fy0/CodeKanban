package websession

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"go.uber.org/zap"
)

func TestSessionRevisionAdvancesAtomicallyAndControlsConditionalSnapshots(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	initial, err := manager.SnapshotWithAutoSyncIfChanged(
		context.Background(),
		created.ID,
		DefaultHistoryWindow,
		"",
	)
	if err != nil {
		t.Fatalf("initial snapshot returned error: %v", err)
	}
	if initial.Unchanged || initial.Session == nil || initial.Revision == "" {
		t.Fatalf("expected a full revisioned snapshot, got %#v", initial)
	}

	unchanged, err := manager.SnapshotWithAutoSyncIfChanged(
		context.Background(),
		created.ID,
		DefaultHistoryWindow,
		initial.Revision,
	)
	if err != nil {
		t.Fatalf("conditional snapshot returned error: %v", err)
	}
	if !unchanged.Unchanged || unchanged.Revision != initial.Revision || unchanged.Session != nil {
		t.Fatalf("expected compact unchanged response, got %#v", unchanged)
	}

	const advances = 8
	revisions := make(chan int64, advances)
	errors := make(chan error, advances)
	var waitGroup sync.WaitGroup
	for index := 0; index < advances; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			revision, advanceErr := manager.advanceSessionRevision(context.Background(), created.ID)
			if advanceErr != nil {
				errors <- advanceErr
				return
			}
			revisions <- revision
		}()
	}
	waitGroup.Wait()
	close(revisions)
	close(errors)
	for advanceErr := range errors {
		t.Fatalf("advanceSessionRevision returned error: %v", advanceErr)
	}
	seen := make(map[int64]struct{}, advances)
	for revision := range revisions {
		seen[revision] = struct{}{}
	}
	if len(seen) != advances {
		t.Fatalf("expected %d unique revisions, got %#v", advances, seen)
	}

	changed, err := manager.SnapshotWithAutoSyncIfChanged(
		context.Background(),
		created.ID,
		DefaultHistoryWindow,
		initial.Revision,
	)
	if err != nil {
		t.Fatalf("changed snapshot returned error: %v", err)
	}
	if changed.Unchanged || changed.Session == nil {
		t.Fatalf("expected a full snapshot after revision advancement, got %#v", changed)
	}
	initialRevision, _ := strconv.ParseInt(initial.Revision, 10, 64)
	changedRevision, _ := strconv.ParseInt(changed.Revision, 10, 64)
	if changedRevision < initialRevision+advances {
		t.Fatalf("revision = %d, want at least %d", changedRevision, initialRevision+advances)
	}
}
