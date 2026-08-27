package websession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/utils"

	goGit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"

	"go.uber.org/zap"
)

func TestScheduleInputIncludesScheduledInputsInSnapshot(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexVersionCLI(t, "0.146.0"),
	}, zap.NewNop())
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

	scheduledFor := time.Now().Add(30 * time.Minute).Round(time.Millisecond)
	item, err := manager.ScheduleInput(
		context.Background(),
		created.ID,
		"Follow up later",
		nil,
		ScheduledInputModeSend,
		scheduledFor,
	)
	if err != nil {
		t.Fatalf("ScheduleInput returned error: %v", err)
	}
	t.Cleanup(func() {
		manager.cancelScheduledInputTimersForSession(created.ID)
	})

	snapshot, err := manager.Snapshot(context.Background(), created.ID, DefaultHistoryWindow)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if len(snapshot.ScheduledInputs) != 1 {
		t.Fatalf("expected 1 scheduled input, got %#v", snapshot.ScheduledInputs)
	}
	got := snapshot.ScheduledInputs[0]
	if got.ID != item.ID {
		t.Fatalf("expected scheduled input id %q, got %#v", item.ID, got)
	}
	if got.Mode != ScheduledInputModeSend {
		t.Fatalf("expected scheduled input mode %q, got %#v", ScheduledInputModeSend, got.Mode)
	}
	if got.Status != ScheduledInputStatusScheduled {
		t.Fatalf("expected scheduled input status %q, got %#v", ScheduledInputStatusScheduled, got.Status)
	}
	if got.Text != "Follow up later" {
		t.Fatalf("expected scheduled input text %q, got %#v", "Follow up later", got.Text)
	}
	if !got.ScheduledFor.Equal(scheduledFor) {
		t.Fatalf("expected scheduled time %v, got %v", scheduledFor, got.ScheduledFor)
	}
	if snapshot.Session.HasScheduledPlanExecution {
		t.Fatalf("expected a delayed message not to mark the session as a scheduled plan, got %#v", snapshot.Session)
	}
}

func TestScheduledInputDispatchesAtDueTime(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
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

	if _, err := manager.ScheduleInput(
		context.Background(),
		created.ID,
		"Run this later",
		nil,
		ScheduledInputModeSend,
		time.Now().Add(500*time.Millisecond),
	); err != nil {
		t.Fatalf("ScheduleInput returned error: %v", err)
	}

	waitForUserMessageCount(t, manager, created.ID, 1)
	waitForSessionToSettle(t, manager, created.ID)
	waitForScheduledInputCount(t, manager, created.ID, 0)

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := strings.Join(userMessageTexts(rawEvents), "|"); got != "Run this later" {
		t.Fatalf("expected scheduled message to dispatch once, got %#v", got)
	}
}

func TestScheduledInputWhenIdleDispatchesAfterStableCleanPeriod(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	initScheduledTestGitRepository(t, project.Path)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	previousPollInterval := scheduledIdlePollInterval
	previousStablePeriod := scheduledIdleStablePeriod
	scheduledIdlePollInterval = 10 * time.Millisecond
	scheduledIdleStablePeriod = 50 * time.Millisecond
	defer func() {
		manager.cancelScheduledIdleMonitor()
		scheduledIdlePollInterval = previousPollInterval
		scheduledIdleStablePeriod = previousStablePeriod
	}()

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	item, err := manager.ScheduleInputWhenIdle(
		context.Background(),
		created.ID,
		"Run this when idle",
		nil,
		ScheduledInputModeSend,
	)
	if err != nil {
		t.Fatalf("ScheduleInputWhenIdle returned error: %v", err)
	}
	if item.ScheduleKind != ScheduledInputScheduleWhenIdle || item.ScheduledFor != nil {
		t.Fatalf("expected when-idle schedule without timestamp, got %#v", item)
	}

	waitForUserMessageCount(t, manager, created.ID, 1)
	waitForSessionToSettle(t, manager, created.ID)
	waitForScheduledInputCount(t, manager, created.ID, 0)

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := strings.Join(userMessageTexts(rawEvents), "|"); got != "Run this when idle" {
		t.Fatalf("expected idle message to dispatch once, got %#v", got)
	}
}

func TestScheduledIdleCheckDoesNotHoldManagementLock(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	initScheduledTestGitRepository(t, project.Path)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	previousPollInterval := scheduledIdlePollInterval
	previousStablePeriod := scheduledIdleStablePeriod
	defer func() {
		manager.cancelScheduledIdleMonitor()
		scheduledIdlePollInterval = previousPollInterval
		scheduledIdleStablePeriod = previousStablePeriod
	}()

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	// Keep the automatic monitor out of the test and make the condition check
	// eligible immediately. The evaluator below deliberately blocks.
	scheduledIdlePollInterval = time.Hour
	scheduledIdleStablePeriod = time.Hour
	plan := insertScheduledPlanHistoryItem(t, manager, created.ID, "Management lock test")

	runBlockedCheck := func(inputID string, operation func() error) {
		t.Helper()
		manager.cancelScheduledIdleMonitor()
		scheduledIdlePollInterval = 0
		started := make(chan struct{})
		release := make(chan struct{})
		checkDone := make(chan error, 1)
		go func() {
			checkDone <- manager.processScheduledIdleInputWithEvaluator(
				context.Background(),
				inputID,
				newScheduledIdleSweepCache(),
				func(
					context.Context,
					tables.WebSessionTable,
					*scheduledIdleSweepCache,
				) ([]ScheduledInputBlockingReason, string, error) {
					close(started)
					<-release
					return nil, "", nil
				},
			)
		}()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("scheduled idle evaluator did not start")
		}

		operationDone := make(chan error, 1)
		go func() { operationDone <- operation() }()
		select {
		case operationErr := <-operationDone:
			if operationErr != nil {
				t.Fatalf("scheduled management operation failed while check was blocked: %v", operationErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("scheduled management operation waited for the idle evaluator")
		}
		close(release)
		select {
		case checkErr := <-checkDone:
			if checkErr != nil {
				t.Fatalf("blocked scheduled idle check returned error: %v", checkErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("scheduled idle check did not finish after release")
		}
	}

	item, err := manager.SchedulePlanExecutionWhenIdle(
		context.Background(), created.ID, plan.ID, scheduledPlanExecutionPayload{},
	)
	if err != nil {
		t.Fatalf("SchedulePlanExecutionWhenIdle returned error: %v", err)
	}
	atTime := ScheduledInputScheduleAtTime
	runBlockedCheck(item.ID, func() error {
		_, updateErr := manager.UpdateScheduledInput(context.Background(), created.ID, item.ID, scheduledInputUpdate{
			ScheduleKind: &atTime,
			ScheduledFor: time.Now().Add(time.Hour),
		})
		return updateErr
	})
	var updatedRecord tables.WebSessionScheduledInputTable
	if err := model.GetDB().First(&updatedRecord, "id = ?", item.ID).Error; err != nil {
		t.Fatalf("load updated schedule: %v", err)
	}
	if updatedRecord.ScheduleKind != string(ScheduledInputScheduleAtTime) || updatedRecord.IdleSince != nil {
		t.Fatalf("blocked idle check overwrote the updated schedule: %#v", updatedRecord)
	}
	manager.cancelScheduledInputTimer(item.ID)
	if err := manager.RemoveScheduledInput(context.Background(), created.ID, item.ID); err != nil {
		t.Fatalf("remove updated schedule: %v", err)
	}

	scheduledIdlePollInterval = time.Hour
	item, err = manager.SchedulePlanExecutionWhenIdle(
		context.Background(), created.ID, plan.ID, scheduledPlanExecutionPayload{},
	)
	if err != nil {
		t.Fatalf("SchedulePlanExecutionWhenIdle (remove case) returned error: %v", err)
	}
	runBlockedCheck(item.ID, func() error {
		return manager.RemoveScheduledInput(context.Background(), created.ID, item.ID)
	})
	var removedRecord tables.WebSessionScheduledInputTable
	if err := model.GetDB().First(&removedRecord, "id = ?", item.ID).Error; err != nil {
		t.Fatalf("load canceled schedule: %v", err)
	}
	if removedRecord.Status != string(ScheduledInputStatusCanceled) {
		t.Fatalf("blocked idle check revived the canceled schedule: %#v", removedRecord)
	}

	scheduledIdlePollInterval = time.Hour
	messageItem, err := manager.ScheduleInputWhenIdle(
		context.Background(), created.ID, "Dispatch while checking", nil, ScheduledInputModeSend,
	)
	if err != nil {
		t.Fatalf("ScheduleInputWhenIdle returned error: %v", err)
	}
	runBlockedCheck(messageItem.ID, func() error {
		if updateErr := model.GetDB().Model(&tables.WebSessionScheduledInputTable{}).
			Where("id = ?", messageItem.ID).
			Update("mode", "invalid").Error; updateErr != nil {
			return updateErr
		}
		dispatchErr := manager.DispatchScheduledInputNow(context.Background(), created.ID, messageItem.ID)
		if !errors.Is(dispatchErr, errInvalidScheduledInputMode) {
			return fmt.Errorf("unexpected immediate dispatch result: %w", dispatchErr)
		}
		return nil
	})
	var dispatchedRecord tables.WebSessionScheduledInputTable
	if err := model.GetDB().First(&dispatchedRecord, "id = ?", messageItem.ID).Error; err != nil {
		t.Fatalf("load immediately dispatched schedule: %v", err)
	}
	if dispatchedRecord.Status != string(ScheduledInputStatusFailed) {
		t.Fatalf("blocked idle check overwrote the immediate dispatch result: %#v", dispatchedRecord)
	}
}

func TestScheduledInputDispatchRechecksCodexAvailability(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
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
	item, err := manager.ScheduleInput(
		context.Background(),
		created.ID,
		"Do not dispatch without a runtime",
		nil,
		ScheduledInputModeInterrupt,
		time.Now().Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("ScheduleInput returned error: %v", err)
	}
	manager.cancelScheduledInputTimer(item.ID)

	manager.cfg.CodexPath = filepath.Join(t.TempDir(), "missing-codex")
	manager.codexContextWindow.mu.Lock()
	manager.codexContextWindow.bins = codexBinaryCapabilityCache{}
	manager.codexContextWindow.mu.Unlock()

	err = manager.DispatchScheduledInputNow(context.Background(), created.ID, item.ID)
	expected := errCodexNotInstalled
	if err == nil || err.Error() != expected {
		t.Fatalf("expected error %q, got %v", expected, err)
	}
	rawEvents, readErr := manager.store.readEvents(created.ID)
	if readErr != nil {
		t.Fatalf("readEvents returned error: %v", readErr)
	}
	if messages := userMessageTexts(rawEvents); len(messages) != 0 {
		t.Fatalf("expected dispatch gate before message side effects, got %#v", messages)
	}
	snapshot, snapshotErr := manager.Snapshot(context.Background(), created.ID, DefaultHistoryWindow)
	if snapshotErr != nil {
		t.Fatalf("Snapshot returned error: %v", snapshotErr)
	}
	if len(snapshot.ScheduledInputs) != 1 || snapshot.ScheduledInputs[0].Status != ScheduledInputStatusFailed {
		t.Fatalf("expected failed scheduled input, got %#v", snapshot.ScheduledInputs)
	}
	if snapshot.ScheduledInputs[0].LastError != expected {
		t.Fatalf("expected persisted runtime error, got %#v", snapshot.ScheduledInputs[0])
	}
}

func TestUpdateScheduledInputInvalidatesOldTimerAndDispatchesUpdatedMessage(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
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
	originalTime := time.Now().Add(time.Hour).Round(time.Millisecond)
	item, err := manager.ScheduleInput(
		context.Background(),
		created.ID,
		"Original message",
		nil,
		ScheduledInputModeSend,
		originalTime,
	)
	if err != nil {
		t.Fatalf("ScheduleInput returned error: %v", err)
	}
	t.Cleanup(func() {
		manager.cancelScheduledInputTimersForSession(created.ID)
	})

	updatedText := "Updated message"
	updatedMode := ScheduledInputModeSend
	updatedTime := originalTime.Add(time.Hour)
	updated, err := manager.UpdateScheduledInput(context.Background(), created.ID, item.ID, scheduledInputUpdate{
		Text:         &updatedText,
		Mode:         &updatedMode,
		ScheduledFor: updatedTime,
	})
	if err != nil {
		t.Fatalf("UpdateScheduledInput returned error: %v", err)
	}
	if updated.Text != updatedText || updated.Status != ScheduledInputStatusScheduled || !updated.ScheduledFor.Equal(updatedTime) {
		t.Fatalf("unexpected updated scheduled input: %#v", updated)
	}

	manager.executeScheduledInput(item.ID, originalTime)
	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := userMessageTexts(rawEvents); len(got) != 0 {
		t.Fatalf("expected stale timer callback not to dispatch, got %#v", got)
	}

	if err := manager.DispatchScheduledInputNow(context.Background(), created.ID, item.ID); err != nil {
		t.Fatalf("DispatchScheduledInputNow returned error: %v", err)
	}
	waitForUserMessageCount(t, manager, created.ID, 1)
	waitForSessionToSettle(t, manager, created.ID)
	rawEvents, err = manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error after immediate dispatch: %v", err)
	}
	if got := strings.Join(userMessageTexts(rawEvents), "|"); got != updatedText {
		t.Fatalf("expected updated message to dispatch once, got %q", got)
	}
}

func TestUpdateScheduledMessageSwitchesBetweenAtTimeAndWhenIdle(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexVersionCLI(t, "0.146.0"),
	}, zap.NewNop())
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
	t.Cleanup(func() {
		manager.cancelScheduledInputTimersForSession(created.ID)
		manager.cancelScheduledIdleMonitor()
	})
	item, err := manager.ScheduleInput(
		context.Background(),
		created.ID,
		"Switch this message",
		nil,
		ScheduledInputModeQueue,
		time.Now().Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("ScheduleInput returned error: %v", err)
	}

	whenIdle := ScheduledInputScheduleWhenIdle
	text := item.Text
	mode := item.Mode
	updated, err := manager.UpdateScheduledInput(context.Background(), created.ID, item.ID, scheduledInputUpdate{
		Text:         &text,
		Mode:         &mode,
		ScheduleKind: &whenIdle,
	})
	if err != nil {
		t.Fatalf("switch message to when-idle returned error: %v", err)
	}
	if updated.ScheduleKind != ScheduledInputScheduleWhenIdle || updated.ScheduledFor != nil ||
		updated.Text != text || updated.Mode != mode {
		t.Fatalf("expected when-idle message schedule, got %#v", updated)
	}

	atTime := ScheduledInputScheduleAtTime
	nextTime := time.Now().Add(2 * time.Hour)
	updated, err = manager.UpdateScheduledInput(context.Background(), created.ID, item.ID, scheduledInputUpdate{
		Text:         &text,
		Mode:         &mode,
		ScheduleKind: &atTime,
		ScheduledFor: nextTime,
	})
	if err != nil {
		t.Fatalf("switch message to at-time returned error: %v", err)
	}
	if updated.ScheduleKind != ScheduledInputScheduleAtTime || updated.ScheduledFor == nil ||
		updated.ScheduledFor.UnixMilli() != nextTime.UnixMilli() {
		t.Fatalf("expected at-time message schedule, got %#v", updated)
	}
}

func TestHandleScheduleSendCommandSupportsLegacyAtTimeAndWhenIdle(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexVersionCLI(t, "0.146.0"),
	}, zap.NewNop())
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
	t.Cleanup(func() {
		manager.cancelScheduledInputTimersForSession(created.ID)
		manager.cancelScheduledIdleMonitor()
	})

	conn := &captureWSConn{}
	commandClient := &client{conn: conn, logger: zap.NewNop()}
	handle := func(requestID string, payload map[string]any) {
		t.Helper()
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal schedule payload: %v", err)
		}
		if err := manager.handleScheduleSendCommand(context.Background(), commandClient, wireCommandFrame{
			Version:   protocolVersion,
			Kind:      "cmd",
			RequestID: requestID,
			SessionID: created.ID,
			Operation: "schedule_send",
			Payload:   encoded,
		}); err != nil {
			t.Fatalf("handleScheduleSendCommand returned error: %v", err)
		}
	}

	legacyTime := time.Now().Add(time.Hour).UnixMilli()
	handle("legacy-at-time", map[string]any{
		"txt":  "Legacy timed message",
		"atts": []string{},
		"mode": string(ScheduledInputModeSend),
		"at":   legacyTime,
	})
	handle("when-idle", map[string]any{
		"txt":  "Idle message",
		"atts": []string{},
		"mode": string(ScheduledInputModeQueue),
		"sk":   string(ScheduledInputScheduleWhenIdle),
	})

	items, err := manager.scheduledInputsSnapshot(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("scheduledInputsSnapshot returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two scheduled messages, got %#v", items)
	}
	byText := make(map[string]ScheduledInput, len(items))
	for _, item := range items {
		byText[item.Text] = item
	}
	legacy := byText["Legacy timed message"]
	if legacy.ScheduleKind != ScheduledInputScheduleAtTime || legacy.ScheduledFor == nil ||
		legacy.ScheduledFor.UnixMilli() != legacyTime {
		t.Fatalf("expected legacy payload to remain at-time, got %#v", legacy)
	}
	idle := byText["Idle message"]
	if idle.ScheduleKind != ScheduledInputScheduleWhenIdle || idle.ScheduledFor != nil {
		t.Fatalf("expected idle payload without timestamp, got %#v", idle)
	}
	if len(conn.frames) != 2 || conn.frames[0].Kind != "ack" || conn.frames[1].Kind != "ack" {
		t.Fatalf("expected schedule acknowledgements, got %#v", conn.frames)
	}
}

func TestScheduledInputFailureReasonClearsWhenRescheduled(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
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
	item, err := manager.ScheduleInput(
		context.Background(),
		created.ID,
		"Retry later",
		nil,
		ScheduledInputModeSend,
		time.Now().Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("ScheduleInput returned error: %v", err)
	}
	t.Cleanup(func() {
		manager.cancelScheduledInputTimersForSession(created.ID)
	})
	if err := model.GetDB().Model(&tables.WebSessionScheduledInputTable{}).
		Where("id = ?", item.ID).
		Update("action", "unsupported").Error; err != nil {
		t.Fatalf("failed to corrupt scheduled action: %v", err)
	}
	if err := manager.DispatchScheduledInputNow(context.Background(), created.ID, item.ID); !errors.Is(err, errInvalidScheduledInputAction) {
		t.Fatalf("expected invalid action error, got %v", err)
	}

	snapshot, err := manager.Snapshot(context.Background(), created.ID, DefaultHistoryWindow)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if len(snapshot.ScheduledInputs) != 1 || snapshot.ScheduledInputs[0].Status != ScheduledInputStatusFailed {
		t.Fatalf("expected failed scheduled input, got %#v", snapshot.ScheduledInputs)
	}
	if snapshot.ScheduledInputs[0].LastError != errInvalidScheduledInputAction.Error() {
		t.Fatalf("expected persisted failure reason, got %#v", snapshot.ScheduledInputs[0])
	}

	if err := model.GetDB().Model(&tables.WebSessionScheduledInputTable{}).
		Where("id = ?", item.ID).
		Update("action", string(ScheduledInputActionMessage)).Error; err != nil {
		t.Fatalf("failed to restore scheduled action: %v", err)
	}
	updatedText := "Retry with changes"
	updatedMode := ScheduledInputModeQueue
	updated, err := manager.UpdateScheduledInput(context.Background(), created.ID, item.ID, scheduledInputUpdate{
		Text:         &updatedText,
		Mode:         &updatedMode,
		ScheduledFor: time.Now().Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("UpdateScheduledInput returned error: %v", err)
	}
	if updated.Status != ScheduledInputStatusScheduled || updated.LastError != "" {
		t.Fatalf("expected rescheduled input with cleared error, got %#v", updated)
	}
}

func TestSchedulePlanExecutionIncludesTargetAndRejectsDuplicate(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexVersionCLI(t, "0.146.0"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModePlan,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	plan := insertScheduledPlanHistoryItem(t, manager, created.ID, "Implement delayed plan")
	scheduledFor := time.Now().Add(30 * time.Minute)
	item, err := manager.SchedulePlanExecution(
		context.Background(),
		created.ID,
		plan.ID,
		scheduledPlanExecutionPayload{},
		scheduledFor,
	)
	if err != nil {
		t.Fatalf("SchedulePlanExecution returned error: %v", err)
	}
	t.Cleanup(func() {
		manager.cancelScheduledInputTimersForSession(created.ID)
	})

	if item.Action != ScheduledInputActionExecutePlan || item.TargetID != plan.ID {
		t.Fatalf("expected scheduled plan target %q, got %#v", plan.ID, item)
	}
	if _, err := manager.SchedulePlanExecution(
		context.Background(),
		created.ID,
		plan.ID,
		scheduledPlanExecutionPayload{},
		scheduledFor.Add(time.Minute),
	); !errors.Is(err, errScheduledPlanDuplicate) {
		t.Fatalf("expected duplicate error, got %v", err)
	}

	snapshot, err := manager.Snapshot(context.Background(), created.ID, DefaultHistoryWindow)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if len(snapshot.ScheduledInputs) != 1 || snapshot.ScheduledInputs[0].Action != ScheduledInputActionExecutePlan {
		t.Fatalf("expected scheduled plan in snapshot, got %#v", snapshot.ScheduledInputs)
	}
	if !snapshot.Session.HasScheduledPlanExecution {
		t.Fatalf("expected snapshot summary to mark the scheduled plan, got %#v", snapshot.Session)
	}
	sessions, err := manager.ListSessions(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(sessions) != 1 || !sessions[0].HasScheduledPlanExecution {
		t.Fatalf("expected list summary to mark the scheduled plan, got %#v", sessions)
	}
	if wire := mapWireSession(sessions[0]); !wire.HasScheduledPlanExecution {
		t.Fatalf("expected compact session payload to mark the scheduled plan, got %#v", wire)
	}
	if err := manager.expireScheduledInputByID(context.Background(), item.ID, "test expiration"); err != nil {
		t.Fatalf("expireScheduledInputByID returned error: %v", err)
	}
	sessions, err = manager.ListSessions(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListSessions after expiration returned error: %v", err)
	}
	if len(sessions) != 1 || sessions[0].HasScheduledPlanExecution {
		t.Fatalf("expected expired plan to restore the ordinary summary state, got %#v", sessions)
	}
}

func TestScheduledPlanWhenIdleDispatchesAfterStableCleanPeriod(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	initScheduledTestGitRepository(t, project.Path)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	previousPollInterval := scheduledIdlePollInterval
	previousStablePeriod := scheduledIdleStablePeriod
	scheduledIdlePollInterval = 10 * time.Millisecond
	scheduledIdleStablePeriod = 50 * time.Millisecond
	defer func() {
		manager.cancelScheduledIdleMonitor()
		scheduledIdlePollInterval = previousPollInterval
		scheduledIdleStablePeriod = previousStablePeriod
	}()

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModePlan,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	plan := insertScheduledPlanHistoryItem(t, manager, created.ID, "Implement when idle")
	item, err := manager.SchedulePlanExecutionWhenIdle(
		context.Background(),
		created.ID,
		plan.ID,
		scheduledPlanExecutionPayload{},
	)
	if err != nil {
		t.Fatalf("SchedulePlanExecutionWhenIdle returned error: %v", err)
	}
	if item.ScheduleKind != ScheduledInputScheduleWhenIdle || item.ScheduledFor != nil {
		t.Fatalf("expected when-idle schedule without timestamp, got %#v", item)
	}
	sessions, err := manager.ListSessions(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(sessions) != 1 || !sessions[0].HasScheduledPlanExecution {
		t.Fatalf("expected when-idle plan to mark the session summary, got %#v", sessions)
	}

	waitForUserMessageCount(t, manager, created.ID, 1)
	waitForSessionToSettle(t, manager, created.ID)
	waitForScheduledInputCount(t, manager, created.ID, 0)

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := strings.Join(userMessageTexts(rawEvents), "|"); got != "Implement the plan." {
		t.Fatalf("expected idle plan implementation prompt, got %q", got)
	}
}

func TestScheduledPlanWhenIdleResetsAfterGitChange(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	initScheduledTestGitRepository(t, project.Path)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexVersionCLI(t, "0.146.0"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	previousPollInterval := scheduledIdlePollInterval
	previousStablePeriod := scheduledIdleStablePeriod
	scheduledIdlePollInterval = 10 * time.Millisecond
	scheduledIdleStablePeriod = 5 * time.Second
	defer func() {
		manager.cancelScheduledIdleMonitor()
		scheduledIdlePollInterval = previousPollInterval
		scheduledIdleStablePeriod = previousStablePeriod
	}()

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModePlan,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	plan := insertScheduledPlanHistoryItem(t, manager, created.ID, "Wait for clean worktree")
	item, err := manager.SchedulePlanExecutionWhenIdle(
		context.Background(),
		created.ID,
		plan.ID,
		scheduledPlanExecutionPayload{},
	)
	if err != nil {
		t.Fatalf("SchedulePlanExecutionWhenIdle returned error: %v", err)
	}
	firstReady := waitForScheduledIdleRecord(t, item.ID, func(record tables.WebSessionScheduledInputTable) bool {
		return record.IdleSince != nil
	})

	dirtyPath := filepath.Join(project.Path, "tracked.txt")
	if err := os.WriteFile(dirtyPath, []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
	waitForScheduledIdleRecord(t, item.ID, func(record tables.WebSessionScheduledInputTable) bool {
		return record.IdleSince == nil && strings.Contains(record.BlockingReasons, string(ScheduledInputBlockedGitDirty))
	})
	if err := os.WriteFile(dirtyPath, []byte("clean\n"), 0o644); err != nil {
		t.Fatalf("restore tracked file: %v", err)
	}
	secondReady := waitForScheduledIdleRecord(t, item.ID, func(record tables.WebSessionScheduledInputTable) bool {
		return record.IdleSince != nil && record.IdleSince.After(*firstReady.IdleSince)
	})
	if secondReady.BlockingReasons != "[]" {
		t.Fatalf("expected cleared blocking reasons, got %#v", secondReady)
	}
	if err := manager.RemoveScheduledInput(context.Background(), created.ID, item.ID); err != nil {
		t.Fatalf("RemoveScheduledInput returned error: %v", err)
	}
}

func TestScheduledPlanWhenIdleIgnoresUntrackedFiles(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	initScheduledTestGitRepository(t, project.Path)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexVersionCLI(t, "0.146.0"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	previousPollInterval := scheduledIdlePollInterval
	previousStablePeriod := scheduledIdleStablePeriod
	scheduledIdlePollInterval = 10 * time.Millisecond
	scheduledIdleStablePeriod = 5 * time.Second
	defer func() {
		manager.cancelScheduledIdleMonitor()
		scheduledIdlePollInterval = previousPollInterval
		scheduledIdleStablePeriod = previousStablePeriod
	}()

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModePlan,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	plan := insertScheduledPlanHistoryItem(t, manager, created.ID, "Ignore untracked files")
	item, err := manager.SchedulePlanExecutionWhenIdle(
		context.Background(),
		created.ID,
		plan.ID,
		scheduledPlanExecutionPayload{},
	)
	if err != nil {
		t.Fatalf("SchedulePlanExecutionWhenIdle returned error: %v", err)
	}
	firstReady := waitForScheduledIdleRecord(t, item.ID, func(record tables.WebSessionScheduledInputTable) bool {
		return record.IdleSince != nil
	})

	untrackedPath := filepath.Join(project.Path, "untracked.txt")
	if err := os.WriteFile(untrackedPath, []byte("untracked\n"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	if err := manager.processScheduledIdleInput(
		context.Background(),
		item.ID,
		newScheduledIdleSweepCache(),
	); err != nil {
		t.Fatalf("processScheduledIdleInput returned error: %v", err)
	}

	var record tables.WebSessionScheduledInputTable
	if err := model.GetDB().First(&record, "id = ?", item.ID).Error; err != nil {
		t.Fatalf("load scheduled idle input: %v", err)
	}
	if record.IdleSince == nil || !record.IdleSince.Equal(*firstReady.IdleSince) {
		t.Fatalf("expected untracked file to preserve idle start, got %#v", record)
	}
	if strings.Contains(record.BlockingReasons, string(ScheduledInputBlockedGitDirty)) {
		t.Fatalf("expected no Git dirty blocker for untracked file, got %#v", record)
	}
	if err := manager.RemoveScheduledInput(context.Background(), created.ID, item.ID); err != nil {
		t.Fatalf("RemoveScheduledInput returned error: %v", err)
	}
}

func TestScheduledIdleConditionsBlockWaitingNonPlanSessions(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	initScheduledTestGitRepository(t, project.Path)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	target, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModePlan,
	})
	if err != nil {
		t.Fatalf("CreateSession target returned error: %v", err)
	}
	other, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModeDefault,
	})
	if err != nil {
		t.Fatalf("CreateSession other returned error: %v", err)
	}
	if err := model.GetDB().Model(&tables.WebSessionTable{}).
		Where("id = ?", other.ID).
		Updates(map[string]any{
			"status":          string(StatusDone),
			"assistant_state": string(AssistantStateWaitingInput),
		}).Error; err != nil {
		t.Fatalf("mark other session waiting: %v", err)
	}

	targetRecord, err := manager.GetSession(context.Background(), target.ID)
	if err != nil {
		t.Fatalf("GetSession target returned error: %v", err)
	}
	reasons, conditionError, err := manager.evaluateScheduledIdleConditions(context.Background(), targetRecord)
	if err != nil || conditionError != "" {
		t.Fatalf("evaluateScheduledIdleConditions returned error: %v, %q", err, conditionError)
	}
	if !scheduledBlockingReasonsContain(reasons, ScheduledInputBlockedNonPlanSessionActive) {
		t.Fatalf("expected waiting non-plan session blocker, got %#v", reasons)
	}
	if _, err := manager.UpdateWorkflowMode(context.Background(), other.ID, WorkflowModePlan); err != nil {
		t.Fatalf("UpdateWorkflowMode returned error: %v", err)
	}
	reasons, conditionError, err = manager.evaluateScheduledIdleConditions(context.Background(), targetRecord)
	if err != nil || conditionError != "" || len(reasons) != 0 {
		t.Fatalf("expected plan-mode session to be ignored, got reasons=%#v error=%v detail=%q", reasons, err, conditionError)
	}
}

func TestScheduledIdleConditionsWaitWhenGitIsUnavailable(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	target, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModePlan,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	targetRecord, err := manager.GetSession(context.Background(), target.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	reasons, conditionError, err := manager.evaluateScheduledIdleConditions(context.Background(), targetRecord)
	if err != nil {
		t.Fatalf("evaluateScheduledIdleConditions returned error: %v", err)
	}
	if !scheduledBlockingReasonsContain(reasons, ScheduledInputBlockedGitUnavailable) || conditionError == "" {
		t.Fatalf("expected Git-unavailable wait state, got reasons=%#v detail=%q", reasons, conditionError)
	}
}

func TestRecoverPendingIdleSchedulesResetsStablePeriod(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexVersionCLI(t, "0.146.0"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	previousPollInterval := scheduledIdlePollInterval
	scheduledIdlePollInterval = time.Hour
	defer func() {
		manager.cancelScheduledIdleMonitor()
		scheduledIdlePollInterval = previousPollInterval
	}()

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModePlan,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	plan := insertScheduledPlanHistoryItem(t, manager, created.ID, "Recover idle plan")
	item, err := manager.SchedulePlanExecutionWhenIdle(
		context.Background(),
		created.ID,
		plan.ID,
		scheduledPlanExecutionPayload{},
	)
	if err != nil {
		t.Fatalf("SchedulePlanExecutionWhenIdle returned error: %v", err)
	}
	manager.cancelScheduledIdleMonitor()
	idleSince := time.Now().Add(-time.Minute)
	if err := model.GetDB().Model(&tables.WebSessionScheduledInputTable{}).
		Where("id = ?", item.ID).
		Updates(map[string]any{
			"idle_since":            idleSince,
			"blocking_reasons_json": marshalScheduledInputBlockingReasons([]ScheduledInputBlockingReason{ScheduledInputBlockedGitDirty}),
			"condition_error":       "stale condition error",
		}).Error; err != nil {
		t.Fatalf("seed idle condition state: %v", err)
	}
	if err := manager.recoverPendingScheduledInputs(context.Background()); err != nil {
		t.Fatalf("recoverPendingScheduledInputs returned error: %v", err)
	}
	manager.cancelScheduledIdleMonitor()

	var recovered tables.WebSessionScheduledInputTable
	if err := model.GetDB().First(&recovered, "id = ?", item.ID).Error; err != nil {
		t.Fatalf("load recovered idle schedule: %v", err)
	}
	if recovered.IdleSince != nil || recovered.BlockingReasons != "[]" || recovered.ConditionError != "" {
		t.Fatalf("expected recovery to reset idle state, got %#v", recovered)
	}
}

func TestUpdateScheduledPlanSwitchesBetweenAtTimeAndWhenIdle(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexVersionCLI(t, "0.146.0"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModePlan,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	plan := insertScheduledPlanHistoryItem(t, manager, created.ID, "Switch schedule kind")
	item, err := manager.SchedulePlanExecution(
		context.Background(),
		created.ID,
		plan.ID,
		scheduledPlanExecutionPayload{},
		time.Now().Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("SchedulePlanExecution returned error: %v", err)
	}
	whenIdle := ScheduledInputScheduleWhenIdle
	updated, err := manager.UpdateScheduledInput(context.Background(), created.ID, item.ID, scheduledInputUpdate{
		ScheduleKind: &whenIdle,
	})
	if err != nil {
		t.Fatalf("switch to when-idle returned error: %v", err)
	}
	if updated.ScheduleKind != ScheduledInputScheduleWhenIdle || updated.ScheduledFor != nil {
		t.Fatalf("expected when-idle schedule, got %#v", updated)
	}
	atTime := ScheduledInputScheduleAtTime
	nextTime := time.Now().Add(2 * time.Hour)
	updated, err = manager.UpdateScheduledInput(context.Background(), created.ID, item.ID, scheduledInputUpdate{
		ScheduleKind: &atTime,
		ScheduledFor: nextTime,
	})
	if err != nil {
		t.Fatalf("switch to at-time returned error: %v", err)
	}
	if updated.ScheduleKind != ScheduledInputScheduleAtTime || updated.ScheduledFor == nil ||
		updated.ScheduledFor.UnixMilli() != nextTime.UnixMilli() {
		t.Fatalf("expected at-time schedule, got %#v", updated)
	}
	manager.cancelScheduledInputTimer(item.ID)
	manager.cancelScheduledIdleMonitor()
}

func TestScheduledPlanExecutionDispatchesOriginalPlan(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModePlan,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	plan := insertScheduledPlanHistoryItem(t, manager, created.ID, "Implement later")
	if _, err := manager.SchedulePlanExecution(
		context.Background(),
		created.ID,
		plan.ID,
		scheduledPlanExecutionPayload{},
		time.Now().Add(500*time.Millisecond),
	); err != nil {
		t.Fatalf("SchedulePlanExecution returned error: %v", err)
	}

	waitForUserMessageCount(t, manager, created.ID, 1)
	waitForSessionToSettle(t, manager, created.ID)
	waitForScheduledInputCount(t, manager, created.ID, 0)

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := strings.Join(userMessageTexts(rawEvents), "|"); got != "Implement the plan." {
		t.Fatalf("expected plan implementation prompt, got %q", got)
	}
	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if effectiveWorkflowMode(record) != WorkflowModeDefault {
		t.Fatalf("expected workflow mode %q, got %q", WorkflowModeDefault, record.WorkflowMode)
	}
}

func TestScheduledPlanExecutionAnswersStructuredPlanChoice(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "user_input"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModePlan,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := manager.SendMessage(context.Background(), created.ID, "prepare a plan", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	request := waitForPendingServerRequest(t, manager, created.ID, pendingServerRequestUserInput)
	if request == nil {
		t.Fatal("expected pending user input")
	}
	plan := insertScheduledPlanHistoryItem(t, manager, created.ID, "Structured plan")
	if _, err := manager.SchedulePlanExecution(
		context.Background(),
		created.ID,
		plan.ID,
		scheduledPlanExecutionPayload{
			PendingItemID:      request.ItemID,
			QuestionID:         "scope",
			ExecuteOptionLabel: "full migration",
		},
		time.Now().Add(500*time.Millisecond),
	); err != nil {
		t.Fatalf("SchedulePlanExecution returned error: %v", err)
	}

	waitForScheduledInputCount(t, manager, created.ID, 0)
	waitForSessionToSettle(t, manager, created.ID)
	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if !historyHasEvent(rawEvents, "user_input_res") {
		t.Fatalf("expected structured plan response, got %#v", rawEvents)
	}
	if got := strings.Join(userMessageTexts(rawEvents), "|"); got != "prepare a plan" {
		t.Fatalf("expected no follow-up implementation message, got %q", got)
	}
}

func TestScheduledPlanExecutionExpiresWhenPlanIsNoLongerCurrent(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexVersionCLI(t, "0.146.0"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:    project.ID,
		Agent:        AgentCodex,
		WorkflowMode: WorkflowModePlan,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	plan := insertScheduledPlanHistoryItem(t, manager, created.ID, "Original plan")
	item, err := manager.SchedulePlanExecution(
		context.Background(),
		created.ID,
		plan.ID,
		scheduledPlanExecutionPayload{},
		time.Now().Add(80*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("SchedulePlanExecution returned error: %v", err)
	}
	if _, err := manager.appendHistoryItem(context.Background(), created.ID, HistoryItem{
		Kind:     "user",
		ItemType: "message",
		Text:     "Change the plan first",
	}); err != nil {
		t.Fatalf("insertHistoryItem returned error: %v", err)
	}

	waitForScheduledInputStatus(t, item.ID, ScheduledInputStatusExpired)
	snapshot, err := manager.Snapshot(context.Background(), created.ID, DefaultHistoryWindow)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if len(snapshot.ScheduledInputs) != 1 || snapshot.ScheduledInputs[0].Status != ScheduledInputStatusExpired {
		t.Fatalf("expected expired scheduled plan, got %#v", snapshot.ScheduledInputs)
	}
	if snapshot.ScheduledInputs[0].LastError == "" {
		t.Fatalf("expected expired plan reason, got %#v", snapshot.ScheduledInputs[0])
	}
	if err := manager.RemoveScheduledInput(context.Background(), created.ID, item.ID); err != nil {
		t.Fatalf("RemoveScheduledInput returned error for expired plan: %v", err)
	}
}

func insertScheduledPlanHistoryItem(
	t *testing.T,
	manager *Manager,
	sessionID string,
	text string,
) HistoryItem {
	t.Helper()
	item, err := manager.appendHistoryItem(context.Background(), sessionID, HistoryItem{
		Kind:     "tool",
		ItemType: "plan",
		Text:     text,
		Tool: &HistoryTool{
			ID:     "plan-" + utils.NewID(),
			Name:   "Plan",
			Kind:   "plan",
			Output: text,
			Status: "done",
		},
	})
	if err != nil {
		t.Fatalf("insertHistoryItem returned error: %v", err)
	}
	return item
}

func initScheduledTestGitRepository(t *testing.T, path string) {
	t.Helper()
	repository, err := goGit.PlainInit(path, false, goGit.WithDefaultBranch(plumbing.NewBranchReferenceName("master")))
	if err != nil {
		t.Fatalf("init repository: %v", err)
	}
	cfg, err := repository.Config()
	if err != nil {
		t.Fatalf("read repository config: %v", err)
	}
	cfg.User.Email = "scheduled-test@example.com"
	cfg.User.Name = "Scheduled Test"
	cfg.Commit.GpgSign = config.OptBoolFalse
	if err := repository.SetConfig(cfg); err != nil {
		t.Fatalf("write repository config: %v", err)
	}
	trackedPath := filepath.Join(path, "tracked.txt")
	if err := os.WriteFile(trackedPath, []byte("clean\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatalf("open worktree: %v", err)
	}
	if err := worktree.AddWithOptions(&goGit.AddOptions{All: true}); err != nil {
		t.Fatalf("stage tracked file: %v", err)
	}
	if _, err := worktree.Commit("Initial commit", &goGit.CommitOptions{Author: &object.Signature{
		Name: "Scheduled Test", Email: "scheduled-test@example.com", When: time.Now(),
	}}); err != nil {
		t.Fatalf("commit tracked file: %v", err)
	}
	_ = repository.Close()
}

func waitForScheduledIdleRecord(
	t *testing.T,
	inputID string,
	predicate func(tables.WebSessionScheduledInputTable) bool,
) tables.WebSessionScheduledInputTable {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var record tables.WebSessionScheduledInputTable
		if err := model.GetDB().First(&record, "id = ?", inputID).Error; err == nil && predicate(record) {
			return record
		}
		time.Sleep(10 * time.Millisecond)
	}
	var record tables.WebSessionScheduledInputTable
	_ = model.GetDB().First(&record, "id = ?", inputID).Error
	t.Fatalf("scheduled idle input %q did not reach expected state: %#v", inputID, record)
	return tables.WebSessionScheduledInputTable{}
}

func scheduledBlockingReasonsContain(
	reasons []ScheduledInputBlockingReason,
	want ScheduledInputBlockingReason,
) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func waitForScheduledInputStatus(t *testing.T, inputID string, status ScheduledInputStatus) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var record tables.WebSessionScheduledInputTable
		if err := model.GetDB().First(&record, "id = ?", inputID).Error; err == nil &&
			normalizeScheduledInputStatus(ScheduledInputStatus(record.Status)) == status {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("scheduled input %q did not reach status %q", inputID, status)
}

func TestScheduledSendFollowsNormalSendBehaviorWhenRunActive(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "approval"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	manager.pendingSteerDelay = 500 * time.Millisecond

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	if err := manager.SendMessage(context.Background(), created.ID, "first", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	request := waitForPendingServerRequest(t, manager, created.ID, pendingServerRequestFileChangeApproval)
	if request == nil {
		t.Fatal("expected pending approval request for the first run")
	}

	if _, err := manager.ScheduleInput(
		context.Background(),
		created.ID,
		"Next after timer",
		nil,
		ScheduledInputModeSend,
		time.Now().Add(60*time.Millisecond),
	); err != nil {
		t.Fatalf("ScheduleInput returned error: %v", err)
	}

	waitForScheduledInputCount(t, manager, created.ID, 0)
	pending := manager.pendingInputsSnapshot(created.ID)
	if len(pending) != 1 || pending[0].Text != "Next after timer" || pending[0].ReadyAt == nil {
		t.Fatalf("expected scheduled send to enter the steer undo window, got %#v", pending)
	}
	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error before steer: %v", err)
	}
	if got := strings.Join(userMessageTexts(rawEvents), "|"); got != "first" {
		t.Fatalf("expected scheduled send to wait before steering, got %#v", got)
	}

	waitForUserMessageCount(t, manager, created.ID, 2)
	if pending := manager.pendingInputsSnapshot(created.ID); len(pending) != 0 {
		t.Fatalf("expected scheduled pending input to clear after steer, got %#v", pending)
	}

	rawEvents, err = manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := strings.Join(userMessageTexts(rawEvents), "|"); got != "first|Next after timer" {
		t.Fatalf("expected scheduled send to steer the active turn, got %#v", got)
	}

	if err := manager.respondToApproval(created.ID, "approve"); err != nil {
		t.Fatalf("respondToApproval returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	rawEvents, err = manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error after flush: %v", err)
	}
	if got := strings.Join(userMessageTexts(rawEvents), "|"); got != "first|Next after timer" {
		t.Fatalf("expected steer message to remain after approval, got %#v", got)
	}
}

func TestScheduledInterruptAbortsActiveRunBeforeSending(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:   t.TempDir(),
		CodexPath: writeFakeCodexAppServerCLI(t, "approval"),
	}, zap.NewNop())
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

	if err := manager.SendMessage(context.Background(), created.ID, "first", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	if request := waitForPendingServerRequest(t, manager, created.ID, pendingServerRequestFileChangeApproval); request == nil {
		t.Fatal("expected pending approval request for the first run")
	}

	if _, err := manager.ScheduleInput(
		context.Background(),
		created.ID,
		"Interrupt now",
		nil,
		ScheduledInputModeInterrupt,
		time.Now().Add(60*time.Millisecond),
	); err != nil {
		t.Fatalf("ScheduleInput returned error: %v", err)
	}

	waitForUserMessageCount(t, manager, created.ID, 2)
	waitForScheduledInputCount(t, manager, created.ID, 0)
	if pending := manager.pendingInputsSnapshot(created.ID); len(pending) != 0 {
		t.Fatalf("expected interrupt scheduled input not to create pending input, got %#v", pending)
	}

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if got := strings.Join(userMessageTexts(rawEvents), "|"); got != "first|Interrupt now" {
		t.Fatalf("expected interrupt scheduled message to send after abort, got %#v", got)
	}

	if request := waitForPendingServerRequest(t, manager, created.ID, pendingServerRequestFileChangeApproval); request == nil {
		t.Fatal("expected pending approval request for the interrupted follow-up run")
	}
	if err := manager.respondToApproval(created.ID, "approve"); err != nil {
		t.Fatalf("respondToApproval returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)
}

func TestNewManagerRecoversPendingScheduledInputs(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "scheduled-recovery.db")
	if err := model.InitWithDSN(dbPath, 0, true); err != nil {
		t.Fatalf("InitWithDSN returned error: %v", err)
	}

	project := seedProject(t)
	dataDir := t.TempDir()
	firstManager, err := NewManager(Config{
		DataDir:   dataDir,
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("first NewManager returned error: %v", err)
	}

	created, err := firstManager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	item, err := firstManager.ScheduleInput(
		context.Background(),
		created.ID,
		"Recovered after restart",
		nil,
		ScheduledInputModeSend,
		time.Now().Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("ScheduleInput returned error: %v", err)
	}

	firstManager.cancelScheduledInputTimersForSession(created.ID)
	dispatchAt := time.Now().Add(80 * time.Millisecond)
	if err := model.GetDB().
		Model(&tables.WebSessionScheduledInputTable{}).
		Where("id = ?", item.ID).
		Update("scheduled_for", dispatchAt).Error; err != nil {
		t.Fatalf("failed to update scheduled_for: %v", err)
	}
	model.DBClose()

	if err := model.InitWithDSN(dbPath, 0, true); err != nil {
		t.Fatalf("reopen InitWithDSN returned error: %v", err)
	}
	defer model.DBClose()

	secondManager, err := NewManager(Config{
		DataDir:   dataDir,
		CodexPath: writeFakeCodexAppServerCLI(t, "basic"),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("second NewManager returned error: %v", err)
	}

	waitForUserMessageCount(t, secondManager, created.ID, 1)
	waitForSessionToSettle(t, secondManager, created.ID)
	waitForScheduledInputCount(t, secondManager, created.ID, 0)

	rawEvents, err := secondManager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error after recovery: %v", err)
	}
	if got := strings.Join(userMessageTexts(rawEvents), "|"); got != "Recovered after restart" {
		t.Fatalf("expected recovered scheduled message to dispatch after restart, got %#v", got)
	}
}

func waitForScheduledInputCount(t *testing.T, manager *Manager, sessionID string, count int) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		items, err := manager.scheduledInputsSnapshot(context.Background(), sessionID)
		if err == nil && len(items) == count {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	items, err := manager.scheduledInputsSnapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("scheduledInputsSnapshot returned error: %v", err)
	}
	t.Fatalf("expected %d scheduled inputs, got %#v", count, items)
}
