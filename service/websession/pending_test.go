package websession

import "testing"

func TestReorderPendingInputAcrossPartitions(t *testing.T) {
	queue := []PendingInput{
		{ID: "redirect-1", Mode: PendingInputModeRedirect, Text: "redirect-1"},
		{ID: "queue-1", Mode: PendingInputModeQueue, Text: "queue-1"},
		{ID: "queue-2", Mode: PendingInputModeQueue, Text: "queue-2"},
	}

	reordered, ok := reorderPendingInput(queue, "queue-2", PendingInputModeRedirect, 0)
	if !ok {
		t.Fatal("expected reorder to succeed")
	}
	if len(reordered) != 3 {
		t.Fatalf("expected 3 items, got %d", len(reordered))
	}
	if reordered[0].ID != "queue-2" || reordered[0].Mode != PendingInputModeRedirect {
		t.Fatalf("expected queue-2 to become the first redirect item, got %#v", reordered[0])
	}
	if reordered[1].ID != "redirect-1" || reordered[1].Mode != PendingInputModeRedirect {
		t.Fatalf("expected redirect-1 to remain second, got %#v", reordered[1])
	}
	if reordered[2].ID != "queue-1" || reordered[2].Mode != PendingInputModeQueue {
		t.Fatalf("expected queue-1 to remain in queue partition, got %#v", reordered[2])
	}
}

func TestReorderPendingInputWithinQueuePartition(t *testing.T) {
	queue := []PendingInput{
		{ID: "redirect-1", Mode: PendingInputModeRedirect, Text: "redirect-1"},
		{ID: "queue-1", Mode: PendingInputModeQueue, Text: "queue-1"},
		{ID: "queue-2", Mode: PendingInputModeQueue, Text: "queue-2"},
		{ID: "queue-3", Mode: PendingInputModeQueue, Text: "queue-3"},
	}

	reordered, ok := reorderPendingInput(queue, "queue-3", PendingInputModeQueue, 0)
	if !ok {
		t.Fatal("expected reorder to succeed")
	}
	if got := []string{reordered[0].ID, reordered[1].ID, reordered[2].ID, reordered[3].ID}; got[0] != "redirect-1" || got[1] != "queue-3" || got[2] != "queue-1" || got[3] != "queue-2" {
		t.Fatalf("unexpected queue partition order: %#v", got)
	}
}
