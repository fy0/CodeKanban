package websession

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
)

func TestCodexRawCollaborationFailureIsProjectedOnce(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Raw collaboration failure", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	run := &activeRun{runID: "run-1", agent: AgentCodex, assistantMessageID: "message-1"}
	scope := codexTurnScope{threadID: "thread-1", turnID: "turn-1"}

	handleCodexTestNotification(t, manager, *session, run, scope, "rawResponseItem/completed", map[string]any{
		"threadId": "thread-1",
		"turnId":   "turn-1",
		"item": map[string]any{
			"type":      "function_call",
			"namespace": "collaboration",
			"name":      "spawn_agent",
			"call_id":   "call-1",
			"arguments": `{"message":"encrypted-secret","task_name":"review"}`,
		},
	})
	handleCodexTestNotification(t, manager, *session, run, scope, "rawResponseItem/completed", map[string]any{
		"threadId": "thread-1",
		"turnId":   "turn-1",
		"item": map[string]any{
			"type":    "function_call_output",
			"call_id": "call-1",
			"output":  "failed to parse function arguments: missing field message",
		},
	})
	// Duplicate output notifications are possible across source reconciliation.
	handleCodexTestNotification(t, manager, *session, run, scope, "rawResponseItem/completed", map[string]any{
		"threadId": "thread-1",
		"turnId":   "turn-1",
		"item": map[string]any{
			"type":    "function_call_output",
			"call_id": "call-1",
			"output":  "failed to parse function arguments: missing field message",
		},
	})

	events, err := manager.store.readEvents(session.ID)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	starts := 0
	ends := 0
	for _, event := range events {
		if eventToolID(event) != "call-1" {
			continue
		}
		switch event.Type {
		case "tool_st":
			starts++
			if input := mustJSONText(event.Payload["in"]); strings.Contains(input, "encrypted-secret") || strings.Contains(input, "message") {
				t.Fatalf("encrypted message leaked into event: %s", input)
			}
		case "tool_end":
			ends++
			if eventToolSucceeded(event) || !strings.Contains(eventToolOutput(event), "missing field") {
				t.Fatalf("unexpected failure event: %#v", event)
			}
			if input := mustJSONText(event.Payload["in"]); strings.Contains(input, "encrypted-secret") || strings.Contains(input, "message") {
				t.Fatalf("encrypted message leaked into failure event: %s", input)
			}
		}
	}
	if starts != 0 || ends != 1 {
		t.Fatalf("expected one self-contained failure event, got starts=%d ends=%d events=%#v", starts, ends, events)
	}
}

func TestCodexRawCollaborationTypedActivitySuppressesFallback(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Raw collaboration success", 1000)
	if err := model.GetDB().Save(session).Error; err != nil {
		t.Fatalf("save session: %v", err)
	}
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	run := &activeRun{runID: "run-1", agent: AgentCodex}
	scope := codexTurnScope{threadID: "thread-1", turnID: "turn-1"}

	handleCodexTestNotification(t, manager, *session, run, scope, "rawResponseItem/completed", map[string]any{
		"threadId": "thread-1",
		"turnId":   "turn-1",
		"item": map[string]any{
			"type":      "function_call",
			"namespace": "collaboration",
			"name":      "spawn_agent",
			"call_id":   "call-1",
			"arguments": `{"message":"encrypted","task_name":"review"}`,
		},
	})
	for _, method := range []string{"item/started", "item/completed"} {
		handleCodexTestNotification(t, manager, *session, run, scope, method, map[string]any{
			"threadId": "thread-1",
			"turnId":   "turn-1",
			"item": map[string]any{
				"type":          "subAgentActivity",
				"id":            "call-1",
				"kind":          "started",
				"agentThreadId": "thread-child",
				"agentPath":     "/root/review",
			},
		})
	}
	handleCodexTestNotification(t, manager, *session, run, scope, "rawResponseItem/completed", map[string]any{
		"threadId": "thread-1",
		"turnId":   "turn-1",
		"item": map[string]any{
			"type":    "function_call_output",
			"call_id": "call-1",
			"output":  `{"task_name":"/root/review"}`,
		},
	})

	events, err := manager.store.readEvents(session.ID)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	activity := 0
	tools := 0
	for _, event := range events {
		if event.Type == "sub_agent_activity" && stringValue(event.Payload["itemId"]) == "call-1" {
			activity++
		}
		if eventToolID(event) == "call-1" ||
			(event.Type == "tool_st" && stringValue(event.Payload["kind"]) == "sub_agent_activity") {
			tools++
		}
	}
	if activity != 1 || tools != 0 {
		t.Fatalf("expected one typed activity and no raw fallback, got activity=%d tools=%d", activity, tools)
	}
}

func TestCodexRawListAgentsIsIgnored(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Raw list agents", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	run := &activeRun{runID: "run-1", agent: AgentCodex}
	scope := codexTurnScope{threadID: "thread-1", turnID: "turn-1"}

	handleCodexTestNotification(t, manager, *session, run, scope, "rawResponseItem/completed", map[string]any{
		"threadId": "thread-1",
		"turnId":   "turn-1",
		"item": map[string]any{
			"type":      "function_call",
			"namespace": "collaboration",
			"name":      "list_agents",
			"call_id":   "call-list",
			"arguments": `{}`,
		},
	})
	handleCodexTestNotification(t, manager, *session, run, scope, "rawResponseItem/completed", map[string]any{
		"threadId": "thread-1",
		"turnId":   "turn-1",
		"item": map[string]any{
			"type":    "function_call_output",
			"call_id": "call-list",
			"output":  `{"agents":[]}`,
		},
	})
	events, err := manager.store.readEvents(session.ID)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	for _, event := range events {
		if eventToolID(event) == "call-list" {
			t.Fatalf("list_agents should not be projected: %#v", event)
		}
	}
}

func TestCodexRawWaitUsesTypedLifecycleAndFallsBackOnValidationFailure(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Raw wait", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	run := &activeRun{runID: "run-1", agent: AgentCodex}
	scope := codexTurnScope{threadID: "thread-1", turnID: "turn-1"}

	handleCodexTestNotification(t, manager, *session, run, scope, "rawResponseItem/completed", map[string]any{
		"threadId": "thread-1", "turnId": "turn-1",
		"item": map[string]any{
			"type": "function_call", "namespace": "collaboration", "name": "wait_agent",
			"call_id": "call-wait-ok", "arguments": `{"timeout_ms":10000}`,
		},
	})
	for _, method := range []string{"item/started", "item/completed"} {
		handleCodexTestNotification(t, manager, *session, run, scope, method, map[string]any{
			"threadId": "thread-1", "turnId": "turn-1",
			"item": map[string]any{
				"type": "collabAgentToolCall", "id": "call-wait-ok", "tool": "wait",
				"status": "completed", "senderThreadId": "thread-1",
				"receiverThreadIds": []any{}, "agentsStates": map[string]any{},
			},
		})
	}
	handleCodexTestNotification(t, manager, *session, run, scope, "rawResponseItem/completed", map[string]any{
		"threadId": "thread-1", "turnId": "turn-1",
		"item": map[string]any{
			"type": "function_call_output", "call_id": "call-wait-ok",
			"output": `{"message":"Wait timed out.","timed_out":true}`,
		},
	})

	handleCodexTestNotification(t, manager, *session, run, scope, "rawResponseItem/completed", map[string]any{
		"threadId": "thread-1", "turnId": "turn-1",
		"item": map[string]any{
			"type": "function_call", "namespace": "collaboration", "name": "wait_agent",
			"call_id": "call-wait-error", "arguments": `{"timeout_ms":1000}`,
		},
	})
	handleCodexTestNotification(t, manager, *session, run, scope, "rawResponseItem/completed", map[string]any{
		"threadId": "thread-1", "turnId": "turn-1",
		"item": map[string]any{
			"type": "function_call_output", "call_id": "call-wait-error",
			"output": "timeout_ms must be at least 10000",
		},
	})

	events, err := manager.store.readEvents(session.ID)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	counts := map[string]int{}
	for _, event := range events {
		if id := eventToolID(event); id != "" {
			counts[id]++
		}
	}
	if counts["call-wait-ok"] != 2 || counts["call-wait-error"] != 1 {
		t.Fatalf("expected one lifecycle for each wait call, got %#v", counts)
	}
}

func TestCodexThreadParamsUseUpstreamRawHistoryContract(t *testing.T) {
	session := tables.WebSessionTable{
		Cwd:                "C:/project",
		Model:              "gpt-test",
		PermissionLevel:    string(PermissionLevelElevated),
		SessionStartSource: string(SessionStartSourceClear),
	}
	start := codexThreadStartParams(session, true)
	if start["experimentalRawEvents"] != true || start["historyMode"] != "paginated" {
		t.Fatalf("unexpected thread/start params: %#v", start)
	}
	if _, ok := start["persistExtendedHistory"]; ok {
		t.Fatalf("thread/start must not send removed persistExtendedHistory: %#v", start)
	}
	if start["sessionStartSource"] != string(SessionStartSourceClear) {
		t.Fatalf("fresh thread/start must preserve the clear source: %#v", start)
	}
	if config := decodeRawObject(start["config"]); config["features.multi_agent_v2.enabled"] != true ||
		config["features.multi_agent_v2.tool_namespace"] != "collaboration" ||
		config["shell_environment_policy.inherit"] != "all" ||
		config["shell_environment_policy.ignore_default_excludes"] != true {
		t.Fatalf("thread/start must force the V2 collaboration protocol: %#v", start)
	}

	resume := codexThreadResumeParams(session, "thread-1", true)
	if _, ok := resume["experimentalRawEvents"]; ok {
		t.Fatalf("thread/resume does not support experimentalRawEvents: %#v", resume)
	}
	if _, ok := resume["historyMode"]; ok {
		t.Fatalf("thread/resume does not support historyMode: %#v", resume)
	}
	if _, ok := resume["persistExtendedHistory"]; ok {
		t.Fatalf("thread/resume must not send removed persistExtendedHistory: %#v", resume)
	}
	if _, ok := resume["sessionStartSource"]; ok {
		t.Fatalf("thread/resume must not send sessionStartSource: %#v", resume)
	}
	if config := decodeRawObject(resume["config"]); config["features.multi_agent_v2.enabled"] != true ||
		config["features.multi_agent_v2.tool_namespace"] != "collaboration" ||
		config["shell_environment_policy.inherit"] != "all" ||
		config["shell_environment_policy.ignore_default_excludes"] != true {
		t.Fatalf("thread/resume must force the V2 collaboration protocol: %#v", resume)
	}
}

func TestCodexThreadParamsUseCompatibilityContractWithoutMultiAgentV2(t *testing.T) {
	session := tables.WebSessionTable{
		Cwd:             "C:/project",
		Model:           "gpt-test",
		PermissionLevel: string(PermissionLevelElevated),
	}
	start := codexThreadStartParams(session, false)
	if start["persistExtendedHistory"] != true {
		t.Fatalf("compatibility thread/start must preserve extended history: %#v", start)
	}
	for _, key := range []string{"historyMode", "experimentalRawEvents", "sessionStartSource"} {
		if _, ok := start[key]; ok {
			t.Fatalf("compatibility thread/start must omit %s: %#v", key, start)
		}
	}
	if config := decodeRawObject(start["config"]); config["shell_environment_policy.inherit"] != "all" ||
		config["shell_environment_policy.ignore_default_excludes"] != true ||
		config["features.multi_agent_v2.enabled"] != nil {
		t.Fatalf("compatibility thread/start must only force full environment inheritance: %#v", start)
	}

	resume := codexThreadResumeParams(session, "thread-1", false)
	if resume["persistExtendedHistory"] != true {
		t.Fatalf("compatibility thread/resume must preserve extended history: %#v", resume)
	}
	if config := decodeRawObject(resume["config"]); config["shell_environment_policy.inherit"] != "all" ||
		config["shell_environment_policy.ignore_default_excludes"] != true ||
		config["features.multi_agent_v2.enabled"] != nil {
		t.Fatalf("compatibility thread/resume must only force full environment inheritance: %#v", resume)
	}
}

func TestCodexRolloutTailerProjectsResumedCollaborationFailure(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Resumed rollout failure", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	run := &activeRun{runID: "run-tail", agent: AgentCodex, assistantMessageID: "message-1"}
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(`{"timestamp":"2026-07-30T01:00:00Z","type":"session_meta","payload":{"id":"thread-1"}}`+"\n"), 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	tailer, err := newCodexRolloutTailer(path, "thread-1")
	if err != nil {
		t.Fatalf("newCodexRolloutTailer: %v", err)
	}
	appendRolloutTestData(t, path, strings.Join([]string{
		`{"timestamp":"2026-07-30T01:00:01Z","type":"response_item","payload":{"type":"function_call","namespace":"collaboration","name":"spawn_agent","arguments":"{}","call_id":"call-tail","internal_chat_message_metadata_passthrough":{"turn_id":"turn-1"}}}`,
		`{"timestamp":"2026-07-30T01:00:02Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-tail","output":"failed to parse function arguments: missing field message","internal_chat_message_metadata_passthrough":{"turn_id":"turn-1"}}}`,
		"",
	}, "\n"))
	if err := tailer.drain(func(entry codexRolloutEntry) error {
		return manager.handleCodexRolloutEntry(*session, run, "thread-1", entry)
	}); err != nil {
		t.Fatalf("drain rollout: %v", err)
	}

	events, err := manager.store.readEvents(session.ID)
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	starts, ends := 0, 0
	for _, event := range events {
		if eventToolID(event) != "call-tail" {
			continue
		}
		if event.Type == "tool_st" {
			starts++
		}
		if event.Type == "tool_end" {
			ends++
			if event.Timestamp.Format(time.RFC3339) != "2026-07-30T01:00:02Z" {
				t.Fatalf("unexpected output timestamp: %s", event.Timestamp)
			}
		}
	}
	if starts != 0 || ends != 1 {
		t.Fatalf("expected one tailed failure event, got starts=%d ends=%d", starts, ends)
	}
}

func TestCodexRolloutSubAgentActivitySuppressesRawFallback(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Resumed rollout activity", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	run := &activeRun{runID: "run-tail", agent: AgentCodex}

	entries := []codexRolloutEntry{
		{
			Timestamp: "2026-07-30T01:00:01Z",
			Type:      "response_item",
			Payload: map[string]any{
				"type": "function_call", "namespace": "collaboration", "name": "spawn_agent",
				"arguments": `{}`, "call_id": "call-covered",
				"internal_chat_message_metadata_passthrough": map[string]any{"turn_id": "turn-1"},
			},
		},
		{
			Timestamp: "2026-07-30T01:00:02Z",
			Type:      "event_msg",
			Payload: map[string]any{
				"type": "sub_agent_activity", "event_id": "call-covered",
				"agent_thread_id": "thread-child", "agent_path": "/root/review", "kind": "started",
			},
		},
		{
			Timestamp: "2026-07-30T01:00:03Z",
			Type:      "response_item",
			Payload: map[string]any{
				"type": "function_call_output", "call_id": "call-covered", "output": "unrecognized success payload",
				"internal_chat_message_metadata_passthrough": map[string]any{"turn_id": "turn-1"},
			},
		},
	}
	for _, entry := range entries {
		if err := manager.handleCodexRolloutEntry(*session, run, "thread-1", entry); err != nil {
			t.Fatalf("handle rollout entry: %v", err)
		}
	}
	events, err := manager.store.readEvents(session.ID)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	for _, event := range events {
		if eventToolID(event) == "call-covered" {
			t.Fatalf("covered collaboration call must not emit a raw fallback: %#v", event)
		}
	}
}

func TestCodexResumedRunAttachesNewDescendantFromSubAgentActivity(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Resumed descendant rollout", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	run := &activeRun{runID: "run-descendant", agent: AgentCodex, assistantMessageID: "message-1"}
	rootPath := filepath.Join(t.TempDir(), "root-rollout.jsonl")
	childPath := filepath.Join(t.TempDir(), "child-rollout.jsonl")
	if err := os.WriteFile(rootPath, []byte(
		`{"timestamp":"2026-07-30T01:00:00Z","type":"session_meta","payload":{"id":"thread-root"}}`+"\n",
	), 0o600); err != nil {
		t.Fatalf("write root rollout: %v", err)
	}
	childLines := strings.Join([]string{
		`{"timestamp":"2026-07-30T01:00:01Z","type":"session_meta","payload":{"id":"thread-child"}}`,
		`{"timestamp":"2026-07-30T01:00:02Z","type":"response_item","payload":{"type":"function_call","namespace":"collaboration","name":"spawn_agent","arguments":"{\"message\":\"encrypted\",\"task_name\":\"nested\"}","call_id":"call-child-fail","internal_chat_message_metadata_passthrough":{"turn_id":"turn-child"}}}`,
		`{"timestamp":"2026-07-30T01:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-child-fail","output":"failed to spawn nested agent"}}`,
		"",
	}, "\n")
	if err := os.WriteFile(childPath, []byte(childLines), 0o600); err != nil {
		t.Fatalf("write child rollout: %v", err)
	}

	rootTailer, err := newCodexRolloutTailer(rootPath, "thread-root")
	if err != nil {
		t.Fatalf("root tailer: %v", err)
	}
	monitor, err := newCodexRolloutMonitor(
		context.Background(),
		time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC),
		map[string]*codexRolloutTailer{"thread-root": rootTailer},
		func(threadID string, entry codexRolloutEntry) error {
			return manager.handleCodexRolloutEntry(*session, run, threadID, entry)
		},
		nil,
	)
	if err != nil {
		t.Fatalf("new rollout monitor: %v", err)
	}
	run.setCodexRolloutMonitor(monitor)
	defer run.stopCodexRolloutMonitor()

	client := newCodexThreadTestClient(t, func(request codexThreadTestRequest) map[string]any {
		if request.Method != "thread/read" || request.Params["threadId"] != "thread-child" ||
			request.Params["includeTurns"] != false {
			return map[string]any{"error": map[string]any{"code": -32602, "message": "unexpected request"}}
		}
		return map[string]any{"result": map[string]any{"thread": map[string]any{
			"id": "thread-child", "path": childPath,
		}}}
	})
	params, err := json.Marshal(map[string]any{
		"threadId": "thread-root",
		"turnId":   "turn-root",
		"item": map[string]any{
			"type":          "subAgentActivity",
			"id":            "call-parent-spawn",
			"kind":          "started",
			"agentThreadId": "thread-child",
			"agentPath":     "/root/child",
		},
	})
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	if _, err := manager.handleCodexAppServerMessage(
		*session,
		run,
		client,
		codexTurnScope{threadID: "thread-root", turnID: "turn-root"},
		codexAppServerIncoming{Method: "item/completed", Params: params},
	); err != nil {
		t.Fatalf("handle sub-agent activity: %v", err)
	}
	attachDeadline := time.Now().Add(time.Second)
	for !monitor.hasThread("thread-child") && time.Now().Before(attachDeadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !monitor.hasThread("thread-child") {
		t.Fatal("child rollout was not attached")
	}
	run.stopCodexRolloutMonitor()

	events, err := manager.store.readEvents(session.ID)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	starts, ends, activities := 0, 0, 0
	for _, event := range events {
		if event.Type == "sub_agent_activity" && stringValue(event.Payload["agentThreadId"]) == "thread-child" {
			activities++
		}
		if eventToolID(event) != "call-child-fail" {
			continue
		}
		if event.Type == "tool_st" {
			starts++
		}
		if event.Type == "tool_end" {
			ends++
		}
	}
	if starts != 0 || ends != 1 || activities != 1 {
		t.Fatalf("expected descendant failure and activity once, got starts=%d ends=%d activities=%d", starts, ends, activities)
	}
}

func TestCodexRolloutActivityAttachesDeeperDescendant(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Nested descendant rollout", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	run := &activeRun{runID: "run-nested", agent: AgentCodex, assistantMessageID: "message-1"}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	sessionDir := filepath.Join(home, ".codex", "sessions", "2026", "07", "30")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("create Codex session directory: %v", err)
	}
	childPath := filepath.Join(t.TempDir(), "child-rollout.jsonl")
	grandchildPath := filepath.Join(sessionDir, "rollout-test-thread-grandchild.jsonl")
	if err := os.WriteFile(childPath, []byte(
		`{"timestamp":"2026-07-30T01:00:00Z","type":"session_meta","payload":{"id":"thread-child"}}`+"\n",
	), 0o600); err != nil {
		t.Fatalf("write child rollout: %v", err)
	}
	grandchildLines := strings.Join([]string{
		`{"timestamp":"2026-07-30T01:00:01Z","type":"session_meta","payload":{"id":"thread-grandchild"}}`,
		`{"timestamp":"2026-07-30T01:00:02Z","type":"response_item","payload":{"type":"function_call","namespace":"collaboration","name":"spawn_agent","arguments":"{}","call_id":"call-grandchild-fail","internal_chat_message_metadata_passthrough":{"turn_id":"turn-grandchild"}}}`,
		`{"timestamp":"2026-07-30T01:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-grandchild-fail","output":"failed to parse function arguments: missing field message"}}`,
		"",
	}, "\n")
	if err := os.WriteFile(grandchildPath, []byte(grandchildLines), 0o600); err != nil {
		t.Fatalf("write grandchild rollout: %v", err)
	}
	childTailer, err := newCodexRolloutTailer(childPath, "thread-child")
	if err != nil {
		t.Fatalf("new child tailer: %v", err)
	}
	monitor, err := newCodexRolloutMonitor(
		context.Background(),
		time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC),
		map[string]*codexRolloutTailer{"thread-child": childTailer},
		func(threadID string, entry codexRolloutEntry) error {
			return manager.handleCodexRolloutEntry(*session, run, threadID, entry)
		},
		nil,
	)
	if err != nil {
		t.Fatalf("new rollout monitor: %v", err)
	}
	run.setCodexRolloutMonitor(monitor)
	defer run.stopCodexRolloutMonitor()

	appendRolloutTestData(t, childPath,
		`{"timestamp":"2026-07-30T01:00:04Z","type":"event_msg","payload":{"type":"sub_agent_activity","event_id":"call-child-spawn","agent_thread_id":"thread-grandchild","agent_path":"/root/child/grandchild","kind":"started"}}`+"\n",
	)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		events, readErr := manager.store.readEvents(session.ID)
		if readErr != nil {
			t.Fatalf("read events: %v", readErr)
		}
		for _, event := range events {
			if event.Type == "tool_end" && eventToolID(event) == "call-grandchild-fail" {
				if !monitor.hasThread("thread-grandchild") {
					t.Fatal("grandchild failure was projected without retaining its rollout tailer")
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("grandchild collaboration failure was not projected")
}

func handleCodexTestNotification(
	t *testing.T,
	manager *Manager,
	session tables.WebSessionTable,
	run *activeRun,
	scope codexTurnScope,
	method string,
	params map[string]any,
) {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	if _, err := manager.handleCodexAppServerMessage(
		session,
		run,
		nil,
		scope,
		codexAppServerIncoming{Method: method, Params: raw},
	); err != nil {
		t.Fatalf("handle %s: %v", method, err)
	}
}
