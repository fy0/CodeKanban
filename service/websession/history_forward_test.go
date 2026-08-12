package websession

import (
	"context"
	"fmt"
	"testing"

	"go.uber.org/zap"
)

func TestHistoryAfterPagesForwardFromStart(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Forward history", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	for index := 1; index <= 5; index++ {
		if _, err := manager.appendHistoryItem(context.Background(), session.ID, HistoryItem{
			ID:       fmt.Sprintf("history-%d", index),
			Kind:     "user",
			ItemType: "message",
			Text:     fmt.Sprintf("message %d", index),
		}); err != nil {
			t.Fatalf("appendHistoryItem(%d) returned error: %v", index, err)
		}
	}

	first, err := manager.HistoryAfter(context.Background(), session.ID, 2, 0)
	if err != nil {
		t.Fatalf("HistoryAfter(first page) returned error: %v", err)
	}
	assertForwardHistoryPage(t, first, []int64{1, 2}, true, "2", 5)
	if first.HasMore || first.BeforeCursor != "" {
		t.Fatalf("forward page exposed an earlier-history cursor: %#v", first)
	}

	second, err := manager.HistoryAfter(context.Background(), session.ID, 2, 2)
	if err != nil {
		t.Fatalf("HistoryAfter(second page) returned error: %v", err)
	}
	assertForwardHistoryPage(t, second, []int64{3, 4}, true, "4", 5)

	last, err := manager.HistoryAfter(context.Background(), session.ID, 2, 4)
	if err != nil {
		t.Fatalf("HistoryAfter(last page) returned error: %v", err)
	}
	assertForwardHistoryPage(t, last, []int64{5}, false, "", 5)
}

func assertForwardHistoryPage(
	t *testing.T,
	window HistoryWindow,
	wantOrder []int64,
	wantHasLater bool,
	wantCursor string,
	wantTotal int,
) {
	t.Helper()
	if len(window.Items) != len(wantOrder) {
		t.Fatalf("history item count = %d, want %d: %#v", len(window.Items), len(wantOrder), window)
	}
	for index, want := range wantOrder {
		if window.Items[index].OrderIndex != want {
			t.Fatalf("history item %d order = %d, want %d", index, window.Items[index].OrderIndex, want)
		}
	}
	if window.HasLater != wantHasLater || window.AfterCursor != wantCursor {
		t.Fatalf(
			"forward metadata = hasLater %v, cursor %q; want %v, %q",
			window.HasLater,
			window.AfterCursor,
			wantHasLater,
			wantCursor,
		)
	}
	if window.Total != wantTotal {
		t.Fatalf("history total = %d, want %d", window.Total, wantTotal)
	}
}
