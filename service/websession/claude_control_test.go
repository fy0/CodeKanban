package websession

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"code-kanban/model"
	"code-kanban/model/tables"
	"go.uber.org/zap"
)

type claudeControlCapture struct {
	mu     sync.Mutex
	data   []byte
	closed bool
}

func (w *claudeControlCapture) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data = append(w.data, data...)
	return len(data), nil
}

func (w *claudeControlCapture) Close() error {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	return nil
}

func (w *claudeControlCapture) message(t *testing.T) map[string]any {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	var message map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(w.data))), &message); err != nil {
		t.Fatalf("decode Claude control response: %v; raw=%q", err, string(w.data))
	}
	return message
}

var _ io.WriteCloser = (*claudeControlCapture)(nil)

func TestDecodeClaudeControlRequestAllowsSDKShapeWithoutToolUseID(t *testing.T) {
	request, ok := decodeClaudeControlRequest(map[string]any{
		"type":       "control_request",
		"request_id": "control-sdk",
		"request": map[string]any{
			"subtype":   "can_use_tool",
			"tool_name": "Bash",
			"input":     map[string]any{"command": "echo hello"},
		},
	})
	if !ok {
		t.Fatal("expected SDK-shaped control request to decode")
	}
	if request.ToolUseID != "" {
		t.Fatalf("expected missing tool_use_id to remain optional, got %q", request.ToolUseID)
	}
	if stringValue(decodeRawObject(request.Input)["command"]) != "echo hello" {
		t.Fatalf("unexpected decoded input: %#v", request.Input)
	}
}

func TestClaudeCompactionStatusIgnoresOrdinarySystemStatuses(t *testing.T) {
	for _, raw := range []map[string]any{
		{"type": "system", "subtype": "status", "status": "completed"},
		{"type": "system", "subtype": "status", "status": "error", "message": "request failed"},
	} {
		started, completed, succeeded, _ := claudeCompactionStatus(raw)
		if started || completed || succeeded {
			t.Fatalf("ordinary system status was classified as compaction: %#v", raw)
		}
	}

	started, completed, succeeded, _ := claudeCompactionStatus(map[string]any{
		"type": "system", "subtype": "status", "status": "compacting",
	})
	if !started || completed || succeeded {
		t.Fatalf("unexpected compaction start state: started=%v completed=%v succeeded=%v", started, completed, succeeded)
	}
	started, completed, succeeded, _ = claudeCompactionStatus(map[string]any{
		"type": "system", "subtype": "status", "compact_result": "success",
	})
	if started || !completed || !succeeded {
		t.Fatalf("unexpected compaction completion state: started=%v completed=%v succeeded=%v", started, completed, succeeded)
	}
}

func TestClaudeResultUsageNormalizesCachedInputTokens(t *testing.T) {
	input, cached, output := claudeResultUsage(map[string]any{
		"usage": map[string]any{
			"input_tokens":                10,
			"cache_read_input_tokens":     90,
			"cache_creation_input_tokens": 5,
			"output_tokens":               7,
		},
	})
	if input != 105 || cached != 95 || output != 7 {
		t.Fatalf("expected full Claude input normalization, got input=%d cached=%d output=%d", input, cached, output)
	}
}

func TestClaudeResultContextWindowPrefersResolvedModelAndAvoidsAmbiguousFallback(t *testing.T) {
	session := tables.WebSessionTable{Model: "sonnet"}
	window := claudeResultContextWindow(map[string]any{
		"modelUsage": map[string]any{
			"claude-haiku-4":  map[string]any{"contextWindow": 200000},
			"claude-sonnet-5": map[string]any{"contextWindow": 500000},
		},
	}, session, "claude-haiku-4")
	if window != 200000 {
		t.Fatalf("expected resolved session model context window, got %d", window)
	}
	window = claudeResultContextWindow(map[string]any{
		"modelUsage": map[string]any{
			"claude-haiku-4":  map[string]any{"contextWindow": 200000},
			"claude-sonnet-5": map[string]any{"contextWindow": 500000},
		},
	}, session, "")
	if window != 500000 {
		t.Fatalf("expected configured session model context window, got %d", window)
	}
	session.Model = "unknown"
	window = claudeResultContextWindow(map[string]any{
		"modelUsage": map[string]any{
			"claude-haiku-4":  map[string]any{"contextWindow": 200000},
			"claude-sonnet-5": map[string]any{"contextWindow": 500000},
		},
	}, session, "")
	if window != 0 {
		t.Fatalf("expected ambiguous multi-model result to remain unknown, got %d", window)
	}
	window = claudeResultContextWindow(map[string]any{
		"modelUsage": map[string]any{
			"claude-sonnet-5": map[string]any{"contextWindow": 500000},
		},
	}, session, "")
	if window != 500000 {
		t.Fatalf("expected unambiguous single-model fallback, got %d", window)
	}
}

func TestClaudeControlResponseInputAcceptsQuestionIndexes(t *testing.T) {
	manager := &Manager{}
	pending := &pendingServerRequest{
		Input: map[string]any{
			"questions": []any{map[string]any{"question": "Which direction?"}},
		},
		Questions: []toolRequestQuestion{{ID: "Which direction?", Question: "Which direction?"}},
	}
	updated := manager.claudeControlResponseInput(pending, map[string][]string{
		"0": {"Implement", "Add tests"},
	})
	answers := decodeRawObject(updated["answers"])
	if stringValue(answers["Which direction?"]) != "Implement,Add tests" {
		t.Fatalf("expected index answer to be converted to Claude's question-text shape, got %#v", updated)
	}
}

func TestClaudeHookBypassesDecisionForActiveStdioRun(t *testing.T) {
	manager := &Manager{
		claudeHookToken: "hook-token",
		runs:            map[string]*activeRun{},
	}
	run := &activeRun{sessionID: "web-session", agent: AgentClaude}
	run.setClaudeIdentity("native-session", t.TempDir())
	run.setClaudeStdioControl(true)
	manager.runs[run.sessionID] = run
	requestBody, err := json.Marshal(map[string]any{
		"session_id":  "native-session",
		"tool_use_id": "tool-1",
		"tool_name":   "AskUserQuestion",
		"tool_input":  map[string]any{"questions": []any{}},
	})
	if err != nil {
		t.Fatalf("marshal hook request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/claude-hooks/pre-tool-use", strings.NewReader(string(requestBody)))
	request.Header.Set("Authorization", "Bearer hook-token")
	recorder := httptest.NewRecorder()
	manager.handleClaudePreToolUseHook(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected active stdio hook bypass to return 204, got %d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestShouldBypassClaudeHookDoesNotCrossDeferredRuns(t *testing.T) {
	sharedCwd := t.TempDir()
	live := &activeRun{sessionID: "web-live", agent: AgentClaude}
	live.setClaudeIdentity("native-live", sharedCwd)
	live.setClaudeStdioControl(true)
	deferred := &activeRun{
		sessionID:        "web-deferred",
		agent:            AgentClaude,
		claudeResumeOnly: true,
	}
	deferred.setClaudeIdentity("native-deferred", sharedCwd)
	manager := &Manager{runs: map[string]*activeRun{
		live.sessionID:     live,
		deferred.sessionID: deferred,
	}}

	if !manager.shouldBypassClaudeHook("native-live", "") {
		t.Fatal("expected native stdio session ID to bypass the legacy hook")
	}
	if manager.shouldBypassClaudeHook("native-deferred", sharedCwd) {
		t.Fatal("deferred resume must retain the legacy hook decision path")
	}
	if manager.shouldBypassClaudeHook("unseen-session", sharedCwd) {
		t.Fatal("ambiguous cwd must not route a deferred request to stdio")
	}

	delete(manager.runs, deferred.sessionID)
	if !manager.shouldBypassClaudeHook("unseen-session", sharedCwd) {
		t.Fatal("expected cwd fallback while the stdio run awaits its native session ID")
	}
}

func TestClaudeControlRequestAnswersInPlace(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Claude control", 1000)
	if err := model.GetDB().Model(session).Updates(map[string]any{
		"agent":            string(AgentClaude),
		"source_kind":      sourceKindClaudeStreamJSON,
		"permission_level": string(PermissionLevelElevated),
	}).Error; err != nil {
		t.Fatalf("update session: %v", err)
	}
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	capture := &claudeControlCapture{}
	run := &activeRun{sessionID: session.ID, agent: AgentClaude, runID: "run-control"}
	run.setInput(capture)
	manager.mu.Lock()
	manager.runs[session.ID] = run
	manager.mu.Unlock()
	defer func() {
		manager.mu.Lock()
		delete(manager.runs, session.ID)
		manager.mu.Unlock()
	}()

	manager.handleClaudeEvent(*session, run, map[string]any{
		"type":       "control_request",
		"request_id": "control-1",
		"request": map[string]any{
			"subtype":                   "can_use_tool",
			"tool_name":                 "AskUserQuestion",
			"display_name":              "AskUserQuestion",
			"tool_use_id":               "tool-1",
			"requires_user_interaction": true,
			"input": map[string]any{
				"questions": []any{map[string]any{
					"header":      "Color",
					"question":    "Which color?",
					"multiSelect": false,
					"options": []any{
						map[string]any{"label": "Red", "description": "Warm"},
						map[string]any{"label": "Blue", "description": "Cool"},
					},
				}},
			},
		},
	})

	pending, ok := run.pendingUserInputRequest()
	if !ok || pending.ControlRequestID != "control-1" {
		t.Fatalf("expected live Claude control request, got %#v", pending)
	}
	if err := manager.respondToUserInput(session.ID, pending.ItemID, map[string][]string{
		pending.Questions[0].ID: {"Red"},
	}); err != nil {
		t.Fatalf("respondToUserInput: %v", err)
	}

	message := capture.message(t)
	response := decodeRawObject(message["response"])
	if stringValue(response["request_id"]) != "control-1" {
		t.Fatalf("unexpected request id: %#v", message)
	}
	result := decodeRawObject(response["response"])
	if stringValue(result["behavior"]) != "allow" {
		t.Fatalf("expected allow response: %#v", message)
	}
	updatedInput := decodeRawObject(result["updatedInput"])
	answers := decodeRawObject(updatedInput["answers"])
	if stringValue(answers["Which color?"]) != "Red" {
		t.Fatalf("expected Claude-shaped answer, got %#v", message)
	}
	if _, ok := run.pendingUserInputRequest(); ok {
		t.Fatal("expected pending control request to clear after response")
	}
	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if record.AssistantState != string(AssistantStateWorking) {
		t.Fatalf("expected working state after in-place response, got %q", record.AssistantState)
	}
}

func TestClaudeControlRequestApprovalUsesToolResponse(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Claude approval", 1000)
	if err := model.GetDB().Model(session).Updates(map[string]any{
		"agent":            string(AgentClaude),
		"source_kind":      sourceKindClaudeStreamJSON,
		"permission_level": string(PermissionLevelElevated),
	}).Error; err != nil {
		t.Fatalf("update session: %v", err)
	}
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	capture := &claudeControlCapture{}
	run := &activeRun{sessionID: session.ID, agent: AgentClaude, runID: "run-approval"}
	run.setInput(capture)
	manager.mu.Lock()
	manager.runs[session.ID] = run
	manager.mu.Unlock()
	defer func() {
		manager.mu.Lock()
		delete(manager.runs, session.ID)
		manager.mu.Unlock()
	}()

	manager.handleClaudeEvent(*session, run, map[string]any{
		"type":       "control_request",
		"request_id": "control-2",
		"request": map[string]any{
			"subtype":      "can_use_tool",
			"tool_name":    "Bash",
			"display_name": "Bash",
			"tool_use_id":  "tool-2",
			"input":        map[string]any{"command": "echo hello"},
		},
	})
	pending, ok := run.pendingApprovalRequest()
	if !ok || pending.Kind != pendingServerRequestCommandApproval || pending.ItemID != "tool-2" {
		t.Fatalf("expected structured Claude command approval, got %#v", pending)
	}
	if err := manager.respondToApproval(session.ID, "approve"); err != nil {
		t.Fatalf("respondToApproval: %v", err)
	}
	message := capture.message(t)
	result := decodeRawObject(decodeRawObject(message["response"])["response"])
	if stringValue(result["behavior"]) != "allow" || stringValue(decodeRawObject(result["updatedInput"])["command"]) != "echo hello" {
		t.Fatalf("unexpected Claude approval response: %#v", message)
	}
}

func TestClaudePlanControlClearsWaitingStateOnRejectAndCancel(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	for _, action := range []string{"reject", "cancel"} {
		t.Run(action, func(t *testing.T) {
			session := seedWebSession(t, project.ID, "Claude plan "+action, 1000)
			if err := model.GetDB().Model(session).Updates(map[string]any{
				"agent":            string(AgentClaude),
				"source_kind":      sourceKindClaudeStreamJSON,
				"permission_level": string(PermissionLevelElevated),
			}).Error; err != nil {
				t.Fatalf("update session: %v", err)
			}

			capture := &claudeControlCapture{}
			run := &activeRun{sessionID: session.ID, agent: AgentClaude, runID: "run-plan-" + action}
			run.setInput(capture)
			manager.mu.Lock()
			manager.runs[session.ID] = run
			manager.mu.Unlock()
			defer func() {
				manager.mu.Lock()
				delete(manager.runs, session.ID)
				manager.mu.Unlock()
			}()

			manager.handleClaudeEvent(*session, run, map[string]any{
				"type":       "control_request",
				"request_id": "control-plan-" + action,
				"request": map[string]any{
					"subtype":      "can_use_tool",
					"tool_name":    "ExitPlanMode",
					"display_name": "ExitPlanMode",
					"tool_use_id":  "tool-plan-" + action,
					"input":        map[string]any{"plan": "Implement the change"},
				},
			})
			pending, ok := run.pendingApprovalRequest()
			if !ok || pending.Kind != pendingServerRequestPlanApproval || !run.completedPlanToolSeen() {
				t.Fatalf("expected pending Claude plan approval, got %#v", pending)
			}
			run.mu.Lock()
			paused := run.activeCallPausedAt != nil
			run.mu.Unlock()
			if !paused {
				t.Fatal("expected active-call timeout to pause while plan approval is pending")
			}

			if action == "reject" {
				if err := manager.respondToApproval(session.ID, "reject"); err != nil {
					t.Fatalf("respondToApproval: %v", err)
				}
				result := decodeRawObject(decodeRawObject(capture.message(t)["response"])["response"])
				if stringValue(result["behavior"]) != "deny" {
					t.Fatalf("expected denied plan response, got %#v", result)
				}
			} else {
				manager.handleClaudeEvent(*session, run, map[string]any{
					"type":       "control_cancel_request",
					"request_id": "control-plan-" + action,
				})
				events, err := manager.store.readEvents(session.ID)
				if err != nil {
					t.Fatalf("readEvents: %v", err)
				}
				foundCancel := false
				for _, event := range events {
					if event.Type == "approval_res" && stringValue(event.Payload["act"]) == "cancel" {
						foundCancel = true
						break
					}
				}
				if !foundCancel {
					t.Fatalf("expected persisted cancellation response, got %#v", events)
				}
				history, err := manager.History(context.Background(), session.ID, 50, nil)
				if err != nil {
					t.Fatalf("History: %v", err)
				}
				foundProjectedCancel := false
				for _, item := range history.Items {
					if item.Detail != nil && item.Detail.Type == "approval_response" && item.Detail.Action == "cancel" {
						foundProjectedCancel = true
						break
					}
				}
				if !foundProjectedCancel {
					t.Fatalf("expected projected cancellation response, got %#v", history.Items)
				}
			}

			if _, ok := run.pendingApprovalRequest(); ok {
				t.Fatal("expected pending plan approval to clear")
			}
			if run.completedPlanToolSeen() {
				t.Fatal("expected completed plan marker to clear")
			}
			run.mu.Lock()
			paused = run.activeCallPausedAt != nil
			run.mu.Unlock()
			if paused {
				t.Fatal("expected active-call timeout to resume")
			}
			record, err := manager.GetSession(context.Background(), session.ID)
			if err != nil {
				t.Fatalf("GetSession: %v", err)
			}
			if record.AssistantState != string(AssistantStateWorking) {
				t.Fatalf("expected working state after %s, got %q", action, record.AssistantState)
			}
		})
	}
}
