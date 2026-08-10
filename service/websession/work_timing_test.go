package websession

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"gorm.io/gorm"
)

func TestScanSessionWorkTimingEventsKeepsSteerStartAndExcludesNestedWaits(t *testing.T) {
	manager, session := newWorkTimingScanManager(t)
	rootThreadID := "root-thread"
	session.NativeSessionID = &rootThreadID
	startedAt := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{Seq: 1, Type: "run_st", RunID: "run-1", Timestamp: startedAt},
		{Seq: 2, Type: "msg_u", RunID: "run-1", Timestamp: startedAt.Add(5 * time.Second)},
		{Seq: 3, Type: "approval_req", RunID: "run-1", Timestamp: startedAt.Add(10 * time.Second)},
		{Seq: 4, Type: "user_input_req", RunID: "run-1", Timestamp: startedAt.Add(12 * time.Second)},
		{Seq: 5, Type: "approval_res", RunID: "run-1", Timestamp: startedAt.Add(20 * time.Second)},
		{Seq: 6, Type: "user_input_res", RunID: "run-1", Timestamp: startedAt.Add(25 * time.Second)},
		{
			Seq:       7,
			Type:      "txt_end",
			RunID:     "run-1",
			ThreadID:  rootThreadID,
			Timestamp: startedAt.Add(30 * time.Second),
			Payload:   map[string]any{"mid": "assistant-1"},
		},
		{Seq: 8, Type: "run_done", RunID: "run-1", Timestamp: startedAt.Add(40 * time.Second)},
	}
	appendWorkTimingEvents(t, manager.store, session.ID, events)

	result, err := manager.scanSessionWorkTimingEvents(session)
	if err != nil {
		t.Fatalf("scanSessionWorkTimingEvents: %v", err)
	}
	if result.incomplete || result.missing {
		t.Fatalf("expected an exact scan, got %#v", result)
	}
	if len(result.runs) != 1 {
		t.Fatalf("expected one run, got %d", len(result.runs))
	}
	run := result.runs[0]
	if run.StartedAt != startedAt {
		t.Fatalf("steer changed run start: got %v want %v", run.StartedAt, startedAt)
	}
	if run.PausedDurationMs != 15_000 {
		t.Fatalf("expected 15s paused, got %dms", run.PausedDurationMs)
	}
	if run.DurationMs != 25_000 {
		t.Fatalf("expected 25s work duration, got %dms", run.DurationMs)
	}
	if run.Anchor.SourceItemID == nil || *run.Anchor.SourceItemID != "assistant:assistant-1" {
		t.Fatalf("unexpected assistant anchor: %#v", run.Anchor)
	}
}

func TestScanSessionWorkTimingEventsClosesPauseOnCancel(t *testing.T) {
	manager, session := newWorkTimingScanManager(t)
	startedAt := time.Date(2026, 8, 10, 11, 0, 0, 0, time.UTC)
	appendWorkTimingEvents(t, manager.store, session.ID, []Event{
		{Seq: 1, Type: "run_st", RunID: "run-1", Timestamp: startedAt},
		{Seq: 2, Type: "approval_req", RunID: "run-1", Timestamp: startedAt.Add(10 * time.Second)},
		{Seq: 3, Type: "run_abort", RunID: "run-1", Timestamp: startedAt.Add(30 * time.Second)},
	})

	result, err := manager.scanSessionWorkTimingEvents(session)
	if err != nil {
		t.Fatalf("scanSessionWorkTimingEvents: %v", err)
	}
	if len(result.runs) != 1 {
		t.Fatalf("expected one run, got %d", len(result.runs))
	}
	run := result.runs[0]
	if run.DurationMs != 10_000 || run.PausedDurationMs != 20_000 {
		t.Fatalf("unexpected canceled timing: %#v", run)
	}
	if run.Outcome != WorkTimingOutcomeCanceled || run.Anchor.TerminalType != "run_abort" {
		t.Fatalf("unexpected canceled outcome: %#v", run)
	}
}

func TestScanSessionWorkTimingEventsCountsExplicitAutoRetryWait(t *testing.T) {
	manager, session := newWorkTimingScanManager(t)
	startedAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	appendWorkTimingEvents(t, manager.store, session.ID, []Event{
		{Seq: 1, Type: "run_st", RunID: "run-1", Timestamp: startedAt},
		{Seq: 2, Type: "run_fail", RunID: "run-1", Timestamp: startedAt.Add(10 * time.Second)},
		{
			Seq:       3,
			Type:      "run_st",
			RunID:     "run-2",
			Timestamp: startedAt.Add(40 * time.Second),
			Payload:   map[string]any{"autoRetry": true},
		},
		{Seq: 4, Type: "run_done", RunID: "run-2", Timestamp: startedAt.Add(50 * time.Second)},
	})

	result, err := manager.scanSessionWorkTimingEvents(session)
	if err != nil {
		t.Fatalf("scanSessionWorkTimingEvents: %v", err)
	}
	if len(result.runs) != 2 {
		t.Fatalf("expected two runs, got %d", len(result.runs))
	}
	if result.runs[1].StartedAt != startedAt.Add(10*time.Second) {
		t.Fatalf("auto retry did not start at explicit retry wait boundary: %v", result.runs[1].StartedAt)
	}
	if result.runs[1].DurationMs != 40_000 {
		t.Fatalf("expected retry wait plus execution to be 40s, got %dms", result.runs[1].DurationMs)
	}
}

func TestScanSessionWorkTimingEventsStrictFailureStates(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		manager, session := newWorkTimingScanManager(t)
		result, err := manager.scanSessionWorkTimingEvents(session)
		if err != nil || !result.missing {
			t.Fatalf("expected missing history, result=%#v err=%v", result, err)
		}
	})

	t.Run("incomplete", func(t *testing.T) {
		manager, session := newWorkTimingScanManager(t)
		appendWorkTimingEvents(t, manager.store, session.ID, []Event{{
			Seq: 1, Type: "run_st", RunID: "run-1", Timestamp: time.Now(),
		}})
		result, err := manager.scanSessionWorkTimingEvents(session)
		if err != nil || !result.incomplete {
			t.Fatalf("expected incomplete history, result=%#v err=%v", result, err)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		manager, session := newWorkTimingScanManager(t)
		if err := manager.store.ensureSessionDir(session.ID); err != nil {
			t.Fatalf("ensureSessionDir: %v", err)
		}
		if err := os.WriteFile(manager.store.historyPath(session.ID), []byte("{bad}\n"), 0o644); err != nil {
			t.Fatalf("write malformed history: %v", err)
		}
		if _, err := manager.scanSessionWorkTimingEvents(session); err == nil {
			t.Fatal("expected malformed history to fail")
		}
	})
}

func TestWorkTimingOutcomeForTerminalEvents(t *testing.T) {
	tests := []struct {
		name    string
		event   Event
		outcome WorkTimingOutcome
	}{
		{name: "complete", event: Event{Type: "run_done"}, outcome: WorkTimingOutcomeCompleted},
		{name: "failed", event: Event{Type: "run_fail"}, outcome: WorkTimingOutcomeFailed},
		{
			name:    "failure timeout",
			event:   Event{Type: "run_fail", Payload: map[string]any{"code": "call_timeout"}},
			outcome: WorkTimingOutcomeTimeout,
		},
		{name: "canceled", event: Event{Type: "run_abort"}, outcome: WorkTimingOutcomeCanceled},
		{
			name:    "abort timeout",
			event:   Event{Type: "run_abort", Payload: map[string]any{"reason": activeCallTimeoutReason}},
			outcome: WorkTimingOutcomeTimeout,
		},
		{
			name:    "restart",
			event:   Event{Type: "run_abort", Payload: map[string]any{"reason": recoveryReasonProcessRestart}},
			outcome: WorkTimingOutcomeInterrupted,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := workTimingOutcomeForEvent(test.event); got != test.outcome {
				t.Fatalf("got %q want %q", got, test.outcome)
			}
		})
	}
}

func TestLiveWorkTimingPersistsAnchorAndUsesCurrentRunForRestartAbort(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	session := seedWebSession(t, project.ID, "timed", 1000)
	manager := &Manager{}
	startedAt := time.Date(2026, 8, 10, 13, 0, 0, 0, time.UTC)

	applyWorkTimingEventForTest(t, manager, session.ID, Event{
		Type: "run_st", RunID: "run-1", Timestamp: startedAt,
	})
	applyWorkTimingEventForTest(t, manager, session.ID, Event{
		Type: "approval_req", RunID: "run-1", Timestamp: startedAt.Add(10 * time.Second),
	})
	applyWorkTimingEventForTest(t, manager, session.ID, Event{
		Type: "approval_res", RunID: "run-1", Timestamp: startedAt.Add(20 * time.Second),
	})

	runID := "run-1"
	anchor := tables.WebSessionItemTable{
		WebSessionID: session.ID,
		OrderIndex:   10,
		RunID:        &runID,
		ItemKind:     "assistant",
		ItemType:     "message",
		Done:         true,
	}
	anchor.Init()
	if err := model.GetDB().Create(&anchor).Error; err != nil {
		t.Fatalf("create assistant anchor: %v", err)
	}

	item := applyWorkTimingEventForTest(t, manager, session.ID, Event{
		Seq: 4, Type: "run_abort", Timestamp: startedAt.Add(30 * time.Second),
		Payload: map[string]any{"reason": recoveryReasonProcessRestart},
	})
	if item == nil || item.RunDurationMs == nil || *item.RunDurationMs != 20_000 {
		t.Fatalf("unexpected timing item: %#v", item)
	}
	if item.RunOutcome != WorkTimingOutcomeInterrupted {
		t.Fatalf("unexpected outcome: %q", item.RunOutcome)
	}

	var refreshed tables.WebSessionTable
	if err := model.GetDB().First(&refreshed, "id = ?", session.ID).Error; err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if refreshed.WorkDurationMs != 20_000 || refreshed.WorkCurrentRunID != nil {
		t.Fatalf("unexpected persisted session timing: %#v", refreshed)
	}
	var refreshedAnchor tables.WebSessionItemTable
	if err := model.GetDB().First(&refreshedAnchor, "id = ?", anchor.ID).Error; err != nil {
		t.Fatalf("reload anchor: %v", err)
	}
	if refreshedAnchor.RunDurationMs == nil || *refreshedAnchor.RunDurationMs != 20_000 {
		t.Fatalf("anchor was not annotated: %#v", refreshedAnchor)
	}
}

func TestCalculateSessionWorkTimingUsesLazyBackfillStates(t *testing.T) {
	t.Run("exact", func(t *testing.T) {
		cleanup := initTestDB(t)
		defer cleanup()
		manager, session := seedBackfillSession(t)
		startedAt := time.Date(2026, 8, 10, 14, 0, 0, 0, time.UTC)
		appendWorkTimingEvents(t, manager.store, session.ID, []Event{
			{Seq: 1, Type: "run_st", RunID: "run-1", Timestamp: startedAt},
			{
				Seq: 2, Type: "txt_end", RunID: "run-1", Timestamp: startedAt.Add(8 * time.Second),
				Payload: map[string]any{"mid": "answer-1"},
			},
			{Seq: 3, Type: "run_done", RunID: "run-1", Timestamp: startedAt.Add(10 * time.Second)},
		})
		seedWorkTimingAnchor(t, session.ID, "assistant:answer-1", 10)

		result, err := manager.CalculateSessionWorkTiming(context.Background(), session.ID)
		if err != nil {
			t.Fatalf("CalculateSessionWorkTiming: %v", err)
		}
		if result.Status != WorkTimingCalculationCalculated ||
			result.Session.WorkTiming.BackfillState != WorkTimingBackfillComplete ||
			result.Session.WorkTiming.CompletedDurationMs != 10_000 ||
			len(result.Items) != 1 {
			t.Fatalf("unexpected exact result: %#v", result)
		}
	})

	t.Run("partial", func(t *testing.T) {
		cleanup := initTestDB(t)
		defer cleanup()
		manager, session := seedBackfillSession(t)
		startedAt := time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC)
		appendWorkTimingEvents(t, manager.store, session.ID, []Event{
			{Seq: 1, Type: "run_st", RunID: "run-1", Timestamp: startedAt},
			{Seq: 2, Type: "run_done", RunID: "run-1", Timestamp: startedAt.Add(10 * time.Second)},
			{Seq: 3, Type: "run_st", RunID: "run-2", Timestamp: startedAt.Add(20 * time.Second)},
		})
		seedTerminalWorkTimingAnchor(t, session.ID, "run_done", startedAt.Add(10*time.Second), 10)

		result, err := manager.CalculateSessionWorkTiming(context.Background(), session.ID)
		if err != nil {
			t.Fatalf("CalculateSessionWorkTiming: %v", err)
		}
		if result.Status != WorkTimingCalculationPartial ||
			result.Session.WorkTiming.BackfillState != WorkTimingBackfillPartial {
			t.Fatalf("unexpected partial result: %#v", result)
		}
	})

	t.Run("missing", func(t *testing.T) {
		cleanup := initTestDB(t)
		defer cleanup()
		manager, session := seedBackfillSession(t)
		result, err := manager.CalculateSessionWorkTiming(context.Background(), session.ID)
		if err != nil {
			t.Fatalf("CalculateSessionWorkTiming: %v", err)
		}
		if result.Status != WorkTimingCalculationUnavailable ||
			result.Session.WorkTiming.BackfillState != WorkTimingBackfillUnavailable {
			t.Fatalf("unexpected missing result: %#v", result)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		cleanup := initTestDB(t)
		defer cleanup()
		manager, session := seedBackfillSession(t)
		if err := manager.store.ensureSessionDir(session.ID); err != nil {
			t.Fatalf("ensureSessionDir: %v", err)
		}
		if err := os.WriteFile(manager.store.historyPath(session.ID), []byte("{bad}\n"), 0o644); err != nil {
			t.Fatalf("write malformed history: %v", err)
		}
		result, err := manager.CalculateSessionWorkTiming(context.Background(), session.ID)
		if err != nil {
			t.Fatalf("CalculateSessionWorkTiming: %v", err)
		}
		if result.Status != WorkTimingCalculationFailed ||
			result.Session.WorkTiming.BackfillState != WorkTimingBackfillFailed {
			t.Fatalf("unexpected malformed result: %#v", result)
		}
	})
}

func TestWorkTimingBackfillStatusAndLimits(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	idle := seedWebSession(t, project.ID, "idle", 1000)
	waiting := seedWebSession(t, project.ID, "waiting", 2000)
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", waiting.ID).
		Update("status", string(StatusWaitingApproval)).Error; err != nil {
		t.Fatalf("mark waiting session: %v", err)
	}
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	manager := &Manager{store: store}

	status, err := manager.WorkTimingBackfillStatus(context.Background())
	if err != nil {
		t.Fatalf("WorkTimingBackfillStatus: %v", err)
	}
	if status.RemainingSessionCount != 2 || status.BusySessionCount != 1 {
		t.Fatalf("unexpected status: %#v", status)
	}
	if _, err := manager.RunWorkTimingBackfill(context.Background(), WorkTimingBackfillParams{Limit: 501}); !errors.Is(err, ErrInvalidWorkTimingBackfill) {
		t.Fatalf("expected invalid limit, got %v", err)
	}
	manager.workTimingBackfillMu.Lock()
	_, err = manager.RunWorkTimingBackfill(context.Background(), WorkTimingBackfillParams{Limit: 1})
	manager.workTimingBackfillMu.Unlock()
	if !errors.Is(err, ErrWorkTimingBackfillBusy) {
		t.Fatalf("expected busy error, got %v", err)
	}

	sessionLock := &manager.workTimingLocks[sessionRevisionLockIndex(idle.ID)]
	sessionLock.Lock()
	skipped, err := manager.RunWorkTimingBackfill(
		context.Background(),
		WorkTimingBackfillParams{Limit: 1},
	)
	sessionLock.Unlock()
	if err != nil {
		t.Fatalf("RunWorkTimingBackfill with locked session: %v", err)
	}
	if skipped.AttemptedSessionCount != 0 || skipped.RemainingSessionCount != 2 {
		t.Fatalf("locked session should be skipped without blocking: %#v", skipped)
	}

	result, err := manager.RunWorkTimingBackfill(context.Background(), WorkTimingBackfillParams{Limit: 1})
	if err != nil {
		t.Fatalf("RunWorkTimingBackfill: %v", err)
	}
	if result.AttemptedSessionCount != 1 || result.UnavailableResultCount != 1 ||
		result.RemainingSessionCount != 1 {
		t.Fatalf("unexpected batch result for %s: %#v", idle.ID, result)
	}
}

func newWorkTimingScanManager(t *testing.T) (*Manager, tables.WebSessionTable) {
	t.Helper()
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	session := tables.WebSessionTable{}
	session.Init()
	return &Manager{store: store}, session
}

func appendWorkTimingEvents(t *testing.T, store *store, sessionID string, events []Event) {
	t.Helper()
	for _, event := range events {
		if event.ID == "" {
			event.ID = "event-" + time.UnixMilli(event.Timestamp.UnixMilli()).Format("150405.000")
		}
		if event.Payload == nil {
			event.Payload = map[string]any{}
		}
		if err := store.appendEvent(sessionID, event); err != nil {
			t.Fatalf("appendEvent: %v", err)
		}
	}
}

func applyWorkTimingEventForTest(
	t *testing.T,
	manager *Manager,
	sessionID string,
	event Event,
) *HistoryItem {
	t.Helper()
	var item *HistoryItem
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		updates, updatedItem, err := manager.applyWorkTimingEventDB(context.Background(), tx, sessionID, event)
		if err != nil {
			return err
		}
		item = updatedItem
		return manager.updateRuntimeStateDB(context.Background(), tx, sessionID, updates)
	})
	if err != nil {
		t.Fatalf("applyWorkTimingEventDB(%s): %v", event.Type, err)
	}
	return item
}

func seedBackfillSession(t *testing.T) (*Manager, *tables.WebSessionTable) {
	t.Helper()
	project := seedProject(t)
	session := seedWebSession(t, project.ID, "backfill", 1000)
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", session.ID).
		Updates(map[string]any{
			"work_timing_backfill_state":   string(WorkTimingBackfillPending),
			"work_timing_backfill_version": 0,
		}).Error; err != nil {
		t.Fatalf("mark session pending: %v", err)
	}
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	return &Manager{store: store}, session
}

func seedWorkTimingAnchor(t *testing.T, sessionID, sourceItemID string, orderIndex int64) {
	t.Helper()
	row := tables.WebSessionItemTable{
		WebSessionID: sessionID,
		SourceItemID: &sourceItemID,
		OrderIndex:   orderIndex,
		ItemKind:     "assistant",
		ItemType:     "message",
		Done:         true,
	}
	row.Init()
	if err := model.GetDB().Create(&row).Error; err != nil {
		t.Fatalf("seed work timing anchor: %v", err)
	}
}

func seedTerminalWorkTimingAnchor(
	t *testing.T,
	sessionID string,
	itemType string,
	timestamp time.Time,
	orderIndex int64,
) {
	t.Helper()
	row := tables.WebSessionItemTable{
		WebSessionID: sessionID,
		OrderIndex:   orderIndex,
		ItemKind:     "system",
		ItemType:     itemType,
		Timestamp:    &timestamp,
	}
	row.Init()
	if err := model.GetDB().Create(&row).Error; err != nil {
		t.Fatalf("seed terminal work timing anchor: %v", err)
	}
}
