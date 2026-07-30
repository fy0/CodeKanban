package websession

import (
	"context"
	"testing"
	"time"
)

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

func TestClaimPendingInputRechecksModePauseAndDeadline(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Minute)
	manager := &Manager{
		pendingInputs: map[string][]PendingInput{
			"session-1": {{
				ID:      "next-1",
				Mode:    PendingInputModeQueue,
				Text:    "next",
				ReadyAt: &future,
			}},
		},
	}

	if _, ok := manager.claimPendingInput("session-1", "next-1", PendingInputModeRedirect, now); ok {
		t.Fatal("expected a mode change to invalidate a stale redirect claim")
	}

	manager.mu.Lock()
	manager.pendingInputs["session-1"][0].Mode = PendingInputModeRedirect
	manager.mu.Unlock()
	if _, ok := manager.claimPendingInput("session-1", "next-1", PendingInputModeRedirect, now); ok {
		t.Fatal("expected a future deadline to block the claim")
	}

	manager.mu.Lock()
	manager.pendingInputs["session-1"][0].ReadyAt = nil
	manager.pendingInputs["session-1"][0].Paused = true
	manager.mu.Unlock()
	if _, ok := manager.claimPendingInput("session-1", "next-1", PendingInputModeRedirect, now); ok {
		t.Fatal("expected a paused input to block the claim")
	}

	manager.mu.Lock()
	manager.pendingInputs["session-1"][0].Paused = false
	manager.mu.Unlock()
	claimed, ok := manager.claimPendingInput("session-1", "next-1", PendingInputModeRedirect, now)
	if !ok || claimed.ID != "next-1" {
		t.Fatalf("expected the current ready input to be claimed, got %#v, ok=%v", claimed, ok)
	}
}

func TestRedirectDoesNotInterruptAutoRetryRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := &Manager{
		runs: map[string]*activeRun{
			"session-1": {
				fromAutoRetry: true,
				cancel:        cancel,
			},
		},
		pendingInputs: map[string][]PendingInput{
			"session-1": {{ID: "next-1", Mode: PendingInputModeRedirect, Text: "next"}},
		},
	}

	manager.maybeInterruptForRedirect("session-1")
	select {
	case <-ctx.Done():
		t.Fatal("expected redirect to leave the automatic retry run active")
	case <-time.After(25 * time.Millisecond):
	}
}

func TestStaleRedirectInterruptDoesNotCancelReplacementAutoRetryRun(t *testing.T) {
	normalCtx, cancelNormal := context.WithCancel(context.Background())
	defer cancelNormal()
	retryCtx, cancelRetry := context.WithCancel(context.Background())
	defer cancelRetry()

	normalRun := &activeRun{cancel: cancelNormal}
	retryRun := &activeRun{fromAutoRetry: true, cancel: cancelRetry}
	manager := &Manager{
		runs: map[string]*activeRun{
			"session-1": retryRun,
		},
	}

	manager.abortRunForRedirect("session-1", "next-1", normalRun)
	select {
	case <-retryCtx.Done():
		t.Fatal("expected stale redirect interrupt to leave the replacement retry run active")
	case <-time.After(25 * time.Millisecond):
	}
	select {
	case <-normalCtx.Done():
		t.Fatal("expected stale redirect interrupt to ignore the replaced normal run")
	default:
	}
}
