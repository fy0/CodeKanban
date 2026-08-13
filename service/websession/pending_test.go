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

func TestExpireHeadPendingRedirectLockedOnlyExpiresFirstReadyRedirect(t *testing.T) {
	now := time.Now()
	firstReadyAt := now.Add(time.Minute)
	secondReadyAt := now.Add(2 * time.Minute)
	manager := &Manager{
		pendingInputs: map[string][]PendingInput{
			"session-1": {
				{ID: "redirect-1", Mode: PendingInputModeRedirect, Text: "first", ReadyAt: &firstReadyAt},
				{ID: "redirect-2", Mode: PendingInputModeRedirect, Text: "second", ReadyAt: &secondReadyAt},
			},
		},
		pendingInputTimers: map[string]*time.Timer{
			"session-1": time.NewTimer(time.Minute),
		},
		pendingInputTimerDeadlines: map[string]time.Time{
			"session-1": firstReadyAt,
		},
	}
	t.Cleanup(func() {
		if timer := manager.pendingInputTimers["session-1"]; timer != nil {
			timer.Stop()
		}
	})

	hasRedirect, changed := manager.expireHeadPendingRedirectLocked("session-1", now)
	if !hasRedirect || !changed {
		t.Fatalf("expected the head redirect to expire, hasRedirect=%v changed=%v", hasRedirect, changed)
	}
	queue := manager.pendingInputs["session-1"]
	if queue[0].ReadyAt == nil || !queue[0].ReadyAt.Equal(now) {
		t.Fatalf("expected first redirect deadline to become now, got %#v", queue[0].ReadyAt)
	}
	if queue[1].ReadyAt == nil || !queue[1].ReadyAt.Equal(secondReadyAt) {
		t.Fatalf("expected second redirect deadline to remain unchanged, got %#v", queue[1].ReadyAt)
	}
	if _, ok := manager.pendingInputTimers["session-1"]; ok {
		t.Fatal("expected the old pending timer to be removed")
	}
	if _, ok := manager.pendingInputTimerDeadlines["session-1"]; ok {
		t.Fatal("expected the old pending timer deadline to be removed")
	}
}

func TestExpireHeadPendingRedirectLockedLeavesPausedRedirectUnchanged(t *testing.T) {
	now := time.Now()
	readyAt := now.Add(time.Minute)
	manager := &Manager{
		pendingInputs: map[string][]PendingInput{
			"session-1": {{
				ID: "redirect-1", Mode: PendingInputModeRedirect, Text: "paused", ReadyAt: &readyAt, Paused: true,
			}},
		},
		pendingInputTimers:         make(map[string]*time.Timer),
		pendingInputTimerDeadlines: make(map[string]time.Time),
	}

	hasRedirect, changed := manager.expireHeadPendingRedirectLocked("session-1", now)
	if hasRedirect || changed {
		t.Fatalf("expected paused redirect to remain untouched, hasRedirect=%v changed=%v", hasRedirect, changed)
	}
	got := manager.pendingInputs["session-1"][0].ReadyAt
	if got == nil || !got.Equal(readyAt) {
		t.Fatalf("expected paused redirect deadline to remain unchanged, got %#v", got)
	}
}

func TestAbortSessionDoesNotExpirePendingRedirect(t *testing.T) {
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readyAt := time.Now().Add(time.Minute)
	manager := &Manager{
		runs: map[string]*activeRun{
			"session-1": {cancel: cancel},
		},
		pendingInputs: map[string][]PendingInput{
			"session-1": {{ID: "redirect-1", Mode: PendingInputModeRedirect, Text: "next", ReadyAt: &readyAt}},
		},
	}

	if err := manager.AbortSession("session-1"); err != nil {
		t.Fatalf("AbortSession returned error: %v", err)
	}
	select {
	case <-runCtx.Done():
	default:
		t.Fatal("expected internal abort to cancel the active run")
	}
	got := manager.pendingInputs["session-1"][0].ReadyAt
	if got == nil || !got.Equal(readyAt) {
		t.Fatalf("expected internal abort to preserve redirect deadline, got %#v", got)
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
