package websession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
)

func TestEditUserMessageForksBeforeTargetAndPreservesSource(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	codexPath, requestLog := writeMessageEditCodexAppServer(t)
	manager, err := NewManager(Config{DataDir: t.TempDir(), CodexPath: codexPath}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:       project.ID,
		Agent:           AgentCodex,
		Model:           "gpt-5.4",
		ReasoningEffort: ReasoningEffortHigh,
		WorkflowMode:    WorkflowModePlan,
		PermissionLevel: PermissionLevelYolo,
		Title:           "Original session",
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if err := manager.updateRuntimeState(context.Background(), created.ID, map[string]any{
		"native_session_id": "thread_source",
		"sync_state":        SyncStateFresh,
	}); err != nil {
		t.Fatalf("updateRuntimeState returned error: %v", err)
	}

	firstTurn := "turn_1"
	secondTurn := "turn_2"
	seedMessageEditTurns(t, created.ID, firstTurn, secondTurn)
	if _, err := manager.appendHistoryItem(context.Background(), created.ID, HistoryItem{
		SourceTurnID: &firstTurn,
		SourceItemID: ptr("msg_1"),
		Kind:         "user",
		ItemType:     "userMessage",
		Text:         "First request",
		OrderIndex:   1,
	}); err != nil {
		t.Fatalf("append first user item failed: %v", err)
	}
	if _, err := manager.appendHistoryItem(context.Background(), created.ID, HistoryItem{
		SourceTurnID: &firstTurn,
		SourceItemID: ptr("reply_1"),
		Kind:         "assistant",
		ItemType:     "agentMessage",
		Text:         "First reply",
		OrderIndex:   2,
	}); err != nil {
		t.Fatalf("append first reply failed: %v", err)
	}
	target, err := manager.appendHistoryItem(context.Background(), created.ID, HistoryItem{
		SourceTurnID: &secondTurn,
		SourceItemID: ptr("msg_2"),
		Kind:         "user",
		ItemType:     "user_message",
		Text:         "Original request",
		OrderIndex:   3,
	})
	if err != nil {
		t.Fatalf("append target item failed: %v", err)
	}
	if _, err := manager.appendHistoryItem(context.Background(), created.ID, HistoryItem{
		SourceTurnID: &secondTurn,
		SourceItemID: ptr("reply_2"),
		Kind:         "assistant",
		ItemType:     "agent_message",
		Text:         "Old reply",
		OrderIndex:   4,
	}); err != nil {
		t.Fatalf("append old reply failed: %v", err)
	}

	branchSnapshot, err := manager.EditUserMessage(
		context.Background(),
		created.ID,
		target.ID,
		"Revised request",
	)
	if err != nil {
		t.Fatalf("EditUserMessage returned error: %v", err)
	}
	if branchSnapshot.Session.ID == created.ID {
		t.Fatal("expected a new web session")
	}
	if branchSnapshot.Session.NativeSessionID == nil || *branchSnapshot.Session.NativeSessionID != "thread_fork" {
		t.Fatalf("expected forked native thread, got %#v", branchSnapshot.Session.NativeSessionID)
	}
	if branchSnapshot.Session.Title != "Revised request" {
		t.Fatalf("expected edited title, got %q", branchSnapshot.Session.Title)
	}
	if branchSnapshot.Session.WorkflowMode != WorkflowModePlan ||
		branchSnapshot.Session.PermissionLevel != PermissionLevelYolo ||
		branchSnapshot.Session.ReasoningEffort != ReasoningEffortHigh {
		t.Fatalf("expected branch settings to be inherited, got %#v", branchSnapshot.Session)
	}

	branchTexts := historyMessageTexts(branchSnapshot.History.Items)
	if strings.Join(branchTexts, "|") != "First request|First reply|Revised request" {
		t.Fatalf("unexpected branch history: %#v", branchTexts)
	}
	sourceSnapshot, err := manager.Snapshot(context.Background(), created.ID, DefaultHistoryWindow)
	if err != nil {
		t.Fatalf("source Snapshot returned error: %v", err)
	}
	if got := strings.Join(historyMessageTexts(sourceSnapshot.History.Items), "|"); got != "First request|First reply|Original request|Old reply" {
		t.Fatalf("source history was modified: %q", got)
	}

	requests := waitForMessageEditRequests(t, requestLog, "turn/start")
	forkRequest := findRequestByMethod(requests, "thread/fork")
	if stringValue(forkRequest["threadId"]) != "thread_source" || stringValue(forkRequest["lastTurnId"]) != "turn_1" {
		t.Fatalf("unexpected fork params: %#v", forkRequest)
	}
	if findRequestByMethod(requests, "thread/read") != nil {
		t.Fatalf("local turn mapping should avoid thread/read: %#v", requests)
	}
	turnRequest := findRequestByMethod(requests, "turn/start")
	inputs := decodeRawArray(turnRequest["input"])
	if len(inputs) == 0 || stringValue(inputs[0]["text"]) != "Revised request" {
		t.Fatalf("unexpected replacement turn input: %#v", turnRequest)
	}

	if err := manager.AbortSession(branchSnapshot.Session.ID); err != nil {
		t.Fatalf("AbortSession returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, branchSnapshot.Session.ID)
}

func TestResolveEditedMessageTurnFallsBackToValidatedUserOrdinal(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Fallback", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if _, err := manager.appendHistoryItem(context.Background(), session.ID, HistoryItem{
		Kind: "user", ItemType: "user_message", Text: "First", OrderIndex: 1,
	}); err != nil {
		t.Fatalf("append first item failed: %v", err)
	}
	target, err := manager.appendHistoryItem(context.Background(), session.ID, HistoryItem{
		Kind: "user", ItemType: "userMessage", Text: "Second", OrderIndex: 2,
	})
	if err != nil {
		t.Fatalf("append target item failed: %v", err)
	}
	turns := []map[string]any{
		codexTestTurn("turn_1", "msg_1", "First"),
		codexTestTurn("turn_2", "msg_2", "Second"),
	}

	index, err := manager.resolveEditedMessageTurn(context.Background(), session.ID, target, turns)
	if err != nil || index != 1 {
		t.Fatalf("expected second turn, got index=%d err=%v", index, err)
	}
	target.Text = "Changed underneath"
	if _, err := manager.resolveEditedMessageTurn(context.Background(), session.ID, target, turns); !errors.Is(err, ErrMessageEditHistoryConflict) {
		t.Fatalf("expected history conflict, got %v", err)
	}
}

func TestEnsureEditedMessageStartsTurnRejectsSteeredMessage(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Steered", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	turnID := "turn_1"
	first, err := manager.appendHistoryItem(context.Background(), session.ID, HistoryItem{
		SourceTurnID: &turnID,
		Kind:         "user",
		ItemType:     "userMessage",
		Text:         "Initial request",
		OrderIndex:   1,
	})
	if err != nil {
		t.Fatalf("append initial user message: %v", err)
	}
	steered, err := manager.appendHistoryItem(context.Background(), session.ID, HistoryItem{
		SourceTurnID: &turnID,
		Kind:         "user",
		ItemType:     "userMessage",
		Text:         "Steer request",
		OrderIndex:   2,
	})
	if err != nil {
		t.Fatalf("append steered user message: %v", err)
	}

	if err := manager.ensureEditedMessageStartsTurn(context.Background(), session.ID, first); err != nil {
		t.Fatalf("expected first message in turn to remain editable: %v", err)
	}
	if err := manager.ensureEditedMessageStartsTurn(context.Background(), session.ID, steered); !errors.Is(err, ErrMessageEditSteeredMessage) {
		t.Fatalf("expected steered message edit rejection, got %v", err)
	}
}

func TestResolveEditedMessageTurnRejectsRemoteSteeredMessage(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	turn := codexTestTurn("turn_1", "msg_1", "Initial request")
	turn["items"] = append(decodeRawArray(turn["items"]), map[string]any{
		"id":   "msg_steer",
		"type": "userMessage",
		"content": []any{map[string]any{
			"type": "text",
			"text": "Steer request",
		}},
	})
	targetItemID := "msg_steer"
	target := HistoryItem{
		SourceItemID: &targetItemID,
		Kind:         "user",
		ItemType:     "userMessage",
		Text:         "Steer request",
	}

	if _, err := manager.resolveEditedMessageTurn(context.Background(), "unused", target, []map[string]any{turn}); !errors.Is(err, ErrMessageEditSteeredMessage) {
		t.Fatalf("expected remote steered message edit rejection, got %v", err)
	}
}

func TestCreateEditedCodexBranchRemoteFallbackReusesQueryClient(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	codexPath, requestLog := writeMessageEditCodexAppServer(t)
	manager, err := NewManager(Config{DataDir: t.TempDir(), CodexPath: codexPath}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	nativeThreadID := "thread_source"
	summary, branchPoint, err := manager.createEditedCodexBranchFromRemote(
		context.Background(),
		tables.WebSessionTable{
			Agent:           string(AgentCodex),
			Backend:         string(SessionBackendCodexAppServer),
			Model:           "gpt-5.4",
			WorkflowMode:    string(WorkflowModeDefault),
			PermissionLevel: string(PermissionLevelElevated),
			Cwd:             t.TempDir(),
			NativeSessionID: &nativeThreadID,
		},
		HistoryItem{
			SourceItemID: ptr("msg_2"),
			Kind:         "user",
			ItemType:     "userMessage",
			Text:         "Original request",
		},
	)
	if err != nil {
		t.Fatalf("createEditedCodexBranchFromRemote returned error: %v", err)
	}
	if summary.ID != "thread_fork" || branchPoint.previousTurnID != "turn_1" {
		t.Fatalf("unexpected remote branch result: summary=%#v branchPoint=%#v", summary, branchPoint)
	}
	requests := waitForMessageEditRequests(t, requestLog, "thread/fork")
	if findRequestByMethod(requests, "thread/read") == nil {
		t.Fatalf("remote fallback should read the source thread: %#v", requests)
	}
	initializeCount := 0
	for _, request := range requests {
		if stringValue(request["method"]) == "initialize" {
			initializeCount++
		}
	}
	if initializeCount != 1 {
		t.Fatalf("remote read and fork should share one query client, got %d initialize requests", initializeCount)
	}
}

func TestCreateEditedCodexThreadStartsFreshBeforeFirstTurn(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	codexPath, requestLog := writeMessageEditCodexAppServer(t)
	manager, err := NewManager(Config{DataDir: t.TempDir(), CodexPath: codexPath}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	nativeThreadID := "thread_source"
	summary, err := manager.createEditedCodexThread(context.Background(), tables.WebSessionTable{
		Agent:           string(AgentCodex),
		Backend:         string(SessionBackendCodexAppServer),
		Model:           "gpt-5.4",
		WorkflowMode:    string(WorkflowModeDefault),
		PermissionLevel: string(PermissionLevelElevated),
		Cwd:             t.TempDir(),
		NativeSessionID: &nativeThreadID,
	}, "")
	if err != nil {
		t.Fatalf("createEditedCodexThread returned error: %v", err)
	}
	if summary.ID != "thread_fresh" {
		t.Fatalf("expected fresh thread id, got %q", summary.ID)
	}
	requests := waitForMessageEditRequests(t, requestLog, "thread/start")
	if findRequestByMethod(requests, "thread/fork") != nil {
		t.Fatalf("first turn edit should not fork an existing turn: %#v", requests)
	}
}

func TestEditUserMessageRejectsUnsupportedAndActiveSessions(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	claude, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentClaude,
	})
	if err != nil {
		t.Fatalf("CreateSession(Claude) returned error: %v", err)
	}
	if _, err := manager.EditUserMessage(context.Background(), claude.ID, "missing", "edited"); !errors.Is(err, ErrMessageEditUnsupported) {
		t.Fatalf("expected unsupported error, got %v", err)
	}

	codex, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession(Codex) returned error: %v", err)
	}
	if err := manager.updateRuntimeState(context.Background(), codex.ID, map[string]any{
		"native_session_id": "thread_active",
		"status":            string(StatusRunning),
	}); err != nil {
		t.Fatalf("updateRuntimeState returned error: %v", err)
	}
	if _, err := manager.EditUserMessage(context.Background(), codex.ID, "missing", "edited"); !errors.Is(err, ErrMessageEditSessionActive) {
		t.Fatalf("expected active-session error, got %v", err)
	}
}

func TestEditUserMessageRejectsOldCodexBeforeThreadOperations(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	codexPath, requestLog := writeMessageEditCodexAppServerVersion(t, "0.145.9")
	manager, err := NewManager(Config{DataDir: t.TempDir(), CodexPath: codexPath}, zap.NewNop())
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
	if err := manager.updateRuntimeState(context.Background(), created.ID, map[string]any{
		"native_session_id": "thread_source",
	}); err != nil {
		t.Fatalf("updateRuntimeState returned error: %v", err)
	}

	_, err = manager.EditUserMessage(context.Background(), created.ID, "missing", "edited")
	expected := "Codex web sessions require Codex >= 0.146.0. Current version: 0.145.9."
	if err == nil || err.Error() != expected {
		t.Fatalf("expected error %q, got %v", expected, err)
	}
	if raw, readErr := os.ReadFile(requestLog); readErr == nil && strings.TrimSpace(string(raw)) != "" {
		t.Fatalf("expected no app-server thread operations, got %s", raw)
	} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		t.Fatalf("read request log: %v", readErr)
	}
}

func codexTestTurn(turnID string, itemID string, text string) map[string]any {
	return map[string]any{
		"id":     turnID,
		"status": "completed",
		"items": []any{map[string]any{
			"id":   itemID,
			"type": "userMessage",
			"content": []any{map[string]any{
				"type": "text",
				"text": text,
			}},
		}},
	}
}

func historyMessageTexts(items []HistoryItem) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item.Kind == "user" || item.Kind == "assistant" {
			result = append(result, item.Text)
		}
	}
	return result
}

func seedMessageEditTurns(t *testing.T, sessionID string, turnIDs ...string) {
	t.Helper()
	for index, turnID := range turnIDs {
		row := tables.WebSessionTurnTable{
			WebSessionID:  sessionID,
			SourceTurnID:  ptr(turnID),
			OrderIndex:    int64(index + 1),
			Status:        "completed",
			SourceCreated: true,
		}
		row.Init()
		if err := model.GetDB().Create(&row).Error; err != nil {
			t.Fatalf("seed message edit turn %q: %v", turnID, err)
		}
	}
}

func findRequestByMethod(requests []map[string]any, method string) map[string]any {
	for _, request := range requests {
		if stringValue(request["method"]) == method {
			return decodeRawObject(request["params"])
		}
	}
	return nil
}

func waitForMessageEditRequests(t *testing.T, path string, requiredMethod string) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(path)
		requests := make([]map[string]any, 0)
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var request map[string]any
			if json.Unmarshal([]byte(line), &request) == nil {
				requests = append(requests, request)
			}
		}
		if findRequestByMethod(requests, requiredMethod) != nil {
			return requests
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s in %s", requiredMethod, path)
	return nil
}

func writeMessageEditCodexAppServer(t *testing.T) (string, string) {
	return writeMessageEditCodexAppServerVersion(t, "0.146.0")
}

func writeMessageEditCodexAppServerVersion(t *testing.T, version string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	requestLog := filepath.Join(dir, "requests.jsonl")
	rolloutPath := filepath.Join(dir, "message-edit-rollout.jsonl")
	scriptPath := filepath.Join(dir, "message-edit-codex.js")
	script := fmt.Sprintf(`#!/usr/bin/env node
if (process.argv.includes('--version')) {
  process.stdout.write('codex %s\n');
  process.exit(0);
}
const fs = require('fs');
const readline = require('readline');
const logPath = %q;
const rolloutPath = %q;
const input = readline.createInterface({ input: process.stdin });
const send = message => process.stdout.write(JSON.stringify(message) + '\n');
const writeRollout = threadId => fs.writeFileSync(
  rolloutPath,
  JSON.stringify({ timestamp: new Date().toISOString(), type: 'session_meta', payload: { id: threadId } }) + '\n',
);
const turns = [
  { id: 'turn_1', status: 'completed', error: null, items: [
    { id: 'msg_1', type: 'userMessage', content: [{ type: 'text', text: 'First request' }] },
    { id: 'reply_1', type: 'agentMessage', text: 'First reply' },
  ] },
  { id: 'turn_2', status: 'completed', error: null, items: [
    { id: 'msg_2', type: 'userMessage', content: [{ type: 'text', text: 'Original request' }] },
    { id: 'reply_2', type: 'agentMessage', text: 'Old reply' },
  ] },
];
input.on('line', line => {
  const message = JSON.parse(line);
  fs.appendFileSync(logPath, JSON.stringify({ method: message.method, params: message.params || {} }) + '\n');
  if (message.method === 'initialize') {
    send({ id: message.id, result: { userAgent: 'message-edit-test' } });
    return;
  }
  if (message.method === 'thread/read') {
    send({ id: message.id, result: { thread: {
      id: message.params.threadId,
      cwd: message.params.cwd || '',
      status: 'idle',
      createdAt: 1712793600,
      updatedAt: 1712797200,
      turns,
    } } });
    return;
  }
  if (message.method === 'thread/goal/get') {
    send({ id: message.id, result: { goal: null } });
    return;
  }
  if (message.method === 'thread/list') {
    send({ id: message.id, result: { data: [], nextCursor: '' } });
    return;
  }
  if (message.method === 'thread/fork') {
    send({ id: message.id, result: { thread: {
      id: 'thread_fork', preview: 'Fork', path: '/tmp/thread-fork.jsonl',
      cwd: message.params.cwd || '', status: 'idle', createdAt: 1712793600,
      updatedAt: 1712797200, turns: turns.slice(0, 1),
    } } });
    return;
  }
  if (message.method === 'thread/start') {
    writeRollout('thread_fresh');
    send({ id: message.id, result: { thread: { id: 'thread_fresh', path: rolloutPath }, modelProvider: 'TestProvider' } });
    return;
  }
  if (message.method === 'thread/resume') {
    writeRollout(message.params.threadId);
    send({ id: message.id, result: { thread: { id: message.params.threadId, path: rolloutPath }, modelProvider: 'TestProvider' } });
    return;
  }
  if (message.method === 'config/read') {
    send({ id: message.id, result: { config: {}, origins: {} } });
    return;
  }
  if (message.method === 'turn/start') {
    send({ id: message.id, result: { turn: { id: 'turn_edit', status: 'inProgress', items: [] } } });
    return;
  }
  if (message.method === 'thread/delete') {
    send({ id: message.id, result: {} });
  }
});
`, version, requestLog, filepath.ToSlash(rolloutPath))
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake Codex app server failed: %v", err)
	}
	if runtime.GOOS != "windows" {
		return scriptPath, requestLog
	}
	wrapper := filepath.Join(dir, "message-edit-codex.cmd")
	content := "@echo off\r\nnode \"%~dp0message-edit-codex.js\" %*\r\nexit /b %ERRORLEVEL%\r\n"
	if err := os.WriteFile(wrapper, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake Codex wrapper failed: %v", err)
	}
	return wrapper, requestLog
}
