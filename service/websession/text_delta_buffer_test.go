package websession

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"code-kanban/model/tables"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestTextDeltaBufferMergesConsecutiveDeltasWithoutChangingText(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)

	var expected strings.Builder
	for index := 0; index < 100; index++ {
		chunk := fmt.Sprintf("[%03d]", index)
		expected.WriteString(chunk)
		if err := manager.enqueueTextDelta(context.Background(), session.ID, *session, textDeltaEvent("run-1", "message-1", chunk)); err != nil {
			t.Fatalf("enqueueTextDelta returned error: %v", err)
		}
	}
	if _, err := manager.appendAndBroadcast(context.Background(), session.ID, *session, textEndEvent("run-1", "message-1")); err != nil {
		t.Fatalf("appendAndBroadcast returned error: %v", err)
	}

	events := readTextDeltaTestEvents(t, manager, session.ID)
	if count := countEventsByType(events, "txt_d"); count >= 100 {
		t.Fatalf("expected significantly fewer than 100 persisted txt_d events, got %d", count)
	}
	if count := countEventsByType(events, "txt_d"); count != 1 {
		t.Fatalf("expected 1 persisted txt_d event, got %d", count)
	}
	if got := joinedTextDeltaPayload(events); got != expected.String() {
		t.Fatalf("merged text mismatch: got %q, want %q", got, expected.String())
	}
	assertEventTypes(t, events, "txt_d", "txt_end")
}

func TestTextDeltaBufferFlushesAfterDefaultWindow(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	if manager.textDeltaFlushWindow != 40*time.Millisecond {
		t.Fatalf("default flush window = %s, want 40ms", manager.textDeltaFlushWindow)
	}

	started := time.Now()
	if err := manager.enqueueTextDelta(context.Background(), session.ID, *session, textDeltaEvent("run-1", "message-1", "timer text")); err != nil {
		t.Fatalf("enqueueTextDelta returned error: %v", err)
	}
	if events := readTextDeltaTestEvents(t, manager, session.ID); len(events) != 0 {
		t.Fatalf("expected delta to remain buffered before timer, got %#v", events)
	}

	events := waitForTextDeltaTestEvents(t, manager, session.ID, 1)
	if elapsed := time.Since(started); elapsed < defaultTextDeltaFlushWindow-5*time.Millisecond {
		t.Fatalf("timer flushed too early after %s", elapsed)
	}
	assertEventTypes(t, events, "txt_d")
	if got := stringValue(events[0].Payload["txt"]); got != "timer text" {
		t.Fatalf("timer-flushed text = %q, want %q", got, "timer text")
	}
}

func TestTextDeltaBufferFlushesAtSizeLimit(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	chunk := strings.Repeat("x", maxPendingTextDeltaBytes+1)

	if err := manager.enqueueTextDelta(context.Background(), session.ID, *session, textDeltaEvent("run-1", "message-1", chunk)); err != nil {
		t.Fatalf("enqueueTextDelta returned error: %v", err)
	}

	events := readTextDeltaTestEvents(t, manager, session.ID)
	assertEventTypes(t, events, "txt_d")
	if got := stringValue(events[0].Payload["txt"]); got != chunk {
		t.Fatalf("size-flushed text length = %d, want %d", len(got), len(chunk))
	}
	assertNoPendingTextDelta(t, manager, session.ID)
}

func TestTextDeltaBufferBarriersPreserveJSONLAndCacheOrder(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	ctx := context.Background()

	if err := manager.enqueueTextDelta(ctx, session.ID, *session, textDeltaEvent("run-1", "message-1", "first")); err != nil {
		t.Fatalf("enqueue first delta: %v", err)
	}
	if _, err := manager.appendAndBroadcast(ctx, session.ID, *session, textEndEvent("run-1", "message-1")); err != nil {
		t.Fatalf("append txt_end: %v", err)
	}
	if err := manager.enqueueTextDelta(ctx, session.ID, *session, textDeltaEvent("run-1", "message-2", "second")); err != nil {
		t.Fatalf("enqueue second delta: %v", err)
	}
	if _, err := manager.appendAndBroadcast(ctx, session.ID, *session, Event{
		Type:      "tool_st",
		RunID:     "run-1",
		ParentID:  "message-2",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"tid":  "tool-1",
			"name": "shell",
			"kind": "command_execution",
		},
	}); err != nil {
		t.Fatalf("append tool_st: %v", err)
	}
	if err := manager.enqueueTextDelta(ctx, session.ID, *session, textDeltaEvent("run-1", "message-3", "third")); err != nil {
		t.Fatalf("enqueue third delta: %v", err)
	}
	if _, err := manager.appendAndBroadcast(ctx, session.ID, *session, Event{
		Type:      "run_fail",
		RunID:     "run-1",
		Timestamp: time.Now(),
		Payload:   map[string]any{"msg": "failed"},
	}); err != nil {
		t.Fatalf("append run_fail: %v", err)
	}

	events := readTextDeltaTestEvents(t, manager, session.ID)
	assertEventTypes(t, events, "txt_d", "txt_end", "txt_d", "tool_st", "txt_d", "run_fail")
	for index, event := range events {
		if event.Seq != int64(index+1) {
			t.Fatalf("event %d seq = %d, want %d", index, event.Seq, index+1)
		}
	}

	history, err := manager.loadHistoryWindow(ctx, session.ID, DefaultHistoryWindow, nil)
	if err != nil {
		t.Fatalf("loadHistoryWindow returned error: %v", err)
	}
	if len(history.Items) != 5 {
		t.Fatalf("history item count = %d, want 5: %#v", len(history.Items), history.Items)
	}
	if history.Items[0].Text != "first" || !history.Items[0].Done {
		t.Fatalf("first assistant item mismatch: %#v", history.Items[0])
	}
	if history.Items[1].Text != "second" || history.Items[1].Done {
		t.Fatalf("second assistant item mismatch: %#v", history.Items[1])
	}
	if history.Items[2].Tool == nil || history.Items[2].Tool.ID != "tool-1" {
		t.Fatalf("tool history item mismatch: %#v", history.Items[2])
	}
	if history.Items[3].Text != "third" || history.Items[3].Done {
		t.Fatalf("third assistant item mismatch: %#v", history.Items[3])
	}
	if history.Items[4].ItemType != "run_fail" {
		t.Fatalf("run_fail history item mismatch: %#v", history.Items[4])
	}
}

func TestTextDeltaBufferDoesNotMergeDifferentMessagesOrRuns(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	ctx := context.Background()

	inputs := []Event{
		textDeltaEvent("run-1", "message-1", "one"),
		textDeltaEvent("run-1", "message-2", "two"),
		textDeltaEvent("run-2", "message-2", "three"),
	}
	for _, event := range inputs {
		if err := manager.enqueueTextDelta(ctx, session.ID, *session, event); err != nil {
			t.Fatalf("enqueueTextDelta returned error: %v", err)
		}
	}
	if _, err := manager.appendAndBroadcast(ctx, session.ID, *session, textEndEvent("run-2", "message-2")); err != nil {
		t.Fatalf("append txt_end: %v", err)
	}

	events := readTextDeltaTestEvents(t, manager, session.ID)
	assertEventTypes(t, events, "txt_d", "txt_d", "txt_d", "txt_end")
	for index, want := range []struct {
		runID     string
		messageID string
		text      string
	}{
		{runID: "run-1", messageID: "message-1", text: "one"},
		{runID: "run-1", messageID: "message-2", text: "two"},
		{runID: "run-2", messageID: "message-2", text: "three"},
	} {
		got := events[index]
		if got.RunID != want.runID || got.ParentID != want.messageID || stringValue(got.Payload["txt"]) != want.text {
			t.Fatalf("event %d = %#v, want run=%q message=%q text=%q", index, got, want.runID, want.messageID, want.text)
		}
	}
}

func TestTextDeltaBufferTimerAndBarrierRaceDoesNotDuplicateOrReorder(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	manager.textDeltaFlushWindow = time.Minute
	ctx := context.Background()

	for index := 0; index < 20; index++ {
		messageID := fmt.Sprintf("message-%02d", index)
		text := fmt.Sprintf("text-%02d", index)
		if err := manager.enqueueTextDelta(ctx, session.ID, *session, textDeltaEvent("run-1", messageID, text)); err != nil {
			t.Fatalf("enqueueTextDelta returned error: %v", err)
		}

		state := manager.sessionEventState(session.ID)
		state.mu.Lock()
		generation := state.timerGeneration
		state.mu.Unlock()

		start := make(chan struct{})
		barrierErr := make(chan error, 1)
		var wait sync.WaitGroup
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			manager.flushPendingTextDeltaTimer(session.ID, state, generation)
		}()
		go func() {
			defer wait.Done()
			<-start
			_, err := manager.appendAndBroadcast(ctx, session.ID, *session, textEndEvent("run-1", messageID))
			barrierErr <- err
		}()
		close(start)
		wait.Wait()
		if err := <-barrierErr; err != nil {
			t.Fatalf("barrier returned error: %v", err)
		}
	}

	events := readTextDeltaTestEvents(t, manager, session.ID)
	if len(events) != 40 {
		t.Fatalf("event count = %d, want 40", len(events))
	}
	for index := 0; index < 20; index++ {
		delta := events[index*2]
		end := events[index*2+1]
		messageID := fmt.Sprintf("message-%02d", index)
		text := fmt.Sprintf("text-%02d", index)
		if delta.Type != "txt_d" || delta.ParentID != messageID || stringValue(delta.Payload["txt"]) != text {
			t.Fatalf("delta %d mismatch: %#v", index, delta)
		}
		if end.Type != "txt_end" || end.ParentID != messageID {
			t.Fatalf("txt_end %d mismatch: %#v", index, end)
		}
	}
	assertNoPendingTextDelta(t, manager, session.ID)
}

func TestTextDeltaBufferFinalSnapshotHasExactDoneAssistantText(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	ctx := context.Background()

	for _, chunk := range []string{"hello", " ", "world"} {
		if err := manager.enqueueTextDelta(ctx, session.ID, *session, textDeltaEvent("run-1", "message-1", chunk)); err != nil {
			t.Fatalf("enqueueTextDelta returned error: %v", err)
		}
	}
	if _, err := manager.appendAndBroadcast(ctx, session.ID, *session, textEndEvent("run-1", "message-1")); err != nil {
		t.Fatalf("append txt_end: %v", err)
	}
	if _, err := manager.appendAndBroadcast(ctx, session.ID, *session, Event{
		Type:      "run_done",
		RunID:     "run-1",
		Timestamp: time.Now(),
		Payload:   map[string]any{"ok": true, "st": string(StatusDone)},
	}); err != nil {
		t.Fatalf("append run_done: %v", err)
	}
	if err := manager.updateRuntimeState(ctx, session.ID, map[string]any{"status": string(StatusDone)}); err != nil {
		t.Fatalf("updateRuntimeState returned error: %v", err)
	}

	snapshot, err := manager.Snapshot(ctx, session.ID, DefaultHistoryWindow)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if snapshot.Session.Status != StatusDone {
		t.Fatalf("snapshot status = %q, want %q", snapshot.Session.Status, StatusDone)
	}
	if len(snapshot.History.Items) != 1 {
		t.Fatalf("snapshot history item count = %d, want 1", len(snapshot.History.Items))
	}
	item := snapshot.History.Items[0]
	if item.Text != "hello world" || !item.Done {
		t.Fatalf("snapshot assistant item mismatch: %#v", item)
	}
}

func TestTextDeltaBufferClearsPendingStateOnRunEndAndDelete(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	ctx := context.Background()

	if err := manager.enqueueTextDelta(ctx, session.ID, *session, textDeltaEvent("run-1", "message-1", "ending")); err != nil {
		t.Fatalf("enqueueTextDelta returned error: %v", err)
	}
	if _, err := manager.appendAndBroadcast(ctx, session.ID, *session, Event{Type: "run_done", RunID: "run-1", Timestamp: time.Now()}); err != nil {
		t.Fatalf("append run_done: %v", err)
	}
	assertNoPendingTextDelta(t, manager, session.ID)

	project := seedProject(t)
	deleting := seedWebSession(t, project.ID, "Delete buffered session", 2000)
	if err := manager.enqueueTextDelta(ctx, deleting.ID, *deleting, textDeltaEvent("run-2", "message-2", "delete me")); err != nil {
		t.Fatalf("enqueue deleting delta: %v", err)
	}
	if err := manager.DeleteSession(ctx, deleting.ID); err != nil {
		t.Fatalf("DeleteSession returned error: %v", err)
	}

	manager.eventStatesMu.Lock()
	_, exists := manager.eventStates[deleting.ID]
	manager.eventStatesMu.Unlock()
	if exists {
		t.Fatal("expected deleted session event state to be removed")
	}
	time.Sleep(defaultTextDeltaFlushWindow + 20*time.Millisecond)
	manager.eventStatesMu.Lock()
	_, exists = manager.eventStates[deleting.ID]
	manager.eventStatesMu.Unlock()
	if exists {
		t.Fatal("stale timer recreated deleted session event state")
	}
	if _, err := os.Stat(manager.store.sessionDir(deleting.ID)); !os.IsNotExist(err) {
		t.Fatalf("deleted session directory still exists or stat failed: %v", err)
	}
	if _, err := manager.GetSession(ctx, deleting.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetSession after delete error = %v, want record not found", err)
	}
}

func TestTextDeltaBufferThousandFastDeltasUseOnePersistence(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	manager.textDeltaFlushWindow = time.Minute
	ctx := context.Background()

	for index := 0; index < 1000; index++ {
		if err := manager.enqueueTextDelta(ctx, session.ID, *session, textDeltaEvent("run-1", "message-1", "x")); err != nil {
			t.Fatalf("enqueueTextDelta returned error at %d: %v", index, err)
		}
	}
	if _, err := manager.appendAndBroadcast(ctx, session.ID, *session, textEndEvent("run-1", "message-1")); err != nil {
		t.Fatalf("append txt_end: %v", err)
	}

	events := readTextDeltaTestEvents(t, manager, session.ID)
	if count := countEventsByType(events, "txt_d"); count != 1 {
		t.Fatalf("1000 fast deltas produced %d txt_d persistence operations, want 1", count)
	}
	if got := len(joinedTextDeltaPayload(events)); got != 1000 {
		t.Fatalf("persisted text length = %d, want 1000", got)
	}
}

func newTextDeltaTestManager(t *testing.T) (*Manager, *tables.WebSessionTable) {
	t.Helper()
	cleanup := initTestDB(t)
	t.Cleanup(cleanup)
	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Text delta buffering", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	t.Cleanup(func() {
		manager.eventStatesMu.Lock()
		states := make([]*sessionEventState, 0, len(manager.eventStates))
		for _, state := range manager.eventStates {
			states = append(states, state)
		}
		manager.eventStatesMu.Unlock()
		for _, state := range states {
			state.mu.Lock()
			if state.timer != nil {
				state.timer.Stop()
				state.timer = nil
			}
			state.pending = nil
			state.mu.Unlock()
		}
	})
	return manager, session
}

func textDeltaEvent(runID, messageID, text string) Event {
	return Event{
		Type:      "txt_d",
		RunID:     runID,
		ParentID:  messageID,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"mid": messageID,
			"txt": text,
		},
	}
}

func textEndEvent(runID, messageID string) Event {
	return Event{
		Type:      "txt_end",
		RunID:     runID,
		ParentID:  messageID,
		Timestamp: time.Now(),
		Payload:   map[string]any{"mid": messageID},
	}
}

func readTextDeltaTestEvents(t *testing.T, manager *Manager, sessionID string) []Event {
	t.Helper()
	events, err := manager.store.readEvents(sessionID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	return events
}

func waitForTextDeltaTestEvents(t *testing.T, manager *Manager, sessionID string, count int) []Event {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		events := readTextDeltaTestEvents(t, manager, sessionID)
		if len(events) >= count {
			return events
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d persisted events", count)
	return nil
}

func joinedTextDeltaPayload(events []Event) string {
	var joined strings.Builder
	for _, event := range events {
		if event.Type == "txt_d" {
			joined.WriteString(stringValue(event.Payload["txt"]))
		}
	}
	return joined.String()
}

func assertEventTypes(t *testing.T, events []Event, want ...string) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("event count = %d, want %d: %#v", len(events), len(want), events)
	}
	for index, event := range events {
		if event.Type != want[index] {
			t.Fatalf("event %d type = %q, want %q", index, event.Type, want[index])
		}
	}
}

func assertNoPendingTextDelta(t *testing.T, manager *Manager, sessionID string) {
	t.Helper()
	state := manager.sessionEventState(sessionID)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.pending != nil {
		t.Fatalf("pending text delta remains: %#v", state.pending)
	}
	if state.timer != nil {
		t.Fatal("text delta timer remains")
	}
}
