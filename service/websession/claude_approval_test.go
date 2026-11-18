package websession

import (
	"context"
	"testing"
	"time"

	"code-kanban/model"

	"go.uber.org/zap"
)

func TestClaudeDeferredPlanApprovalPermissionTransition(t *testing.T) {
	for _, action := range []string{"approve", "reject"} {
		t.Run(action, func(t *testing.T) {
			cleanup := initTestDB(t)
			defer cleanup()
			project := seedProject(t)
			session := seedWebSession(t, project.ID, "Claude restored plan", 1000)
			if err := model.GetDB().Model(session).Updates(map[string]any{
				"agent": string(AgentClaude), "source_kind": sourceKindClaudeStreamJSON,
				"permission_level": string(PermissionLevelElevated), "workflow_mode": string(WorkflowModePlan),
				"native_session_id": "claude-session-test", "cwd": t.TempDir(),
			}).Error; err != nil {
				t.Fatal(err)
			}
			manager, err := NewManager(Config{DataDir: t.TempDir(), ClaudePath: writeFakeClaudeDeferredCLI(t)}, zap.NewNop())
			if err != nil {
				t.Fatal(err)
			}
			record, err := manager.GetSession(context.Background(), session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := manager.appendAndBroadcast(context.Background(), session.ID, record, Event{
				ID: "restored-approval", Type: "approval_req", RunID: "old-run", Timestamp: time.Now(),
				Payload: map[string]any{"iid": "restored-plan", "kind": string(pendingServerRequestPlanApproval), "prompt": "Implement the plan"},
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := manager.UpdatePermissionLevel(context.Background(), session.ID, PermissionLevelYolo); err != nil {
				t.Fatal(err)
			}
			if err := manager.respondToApproval(session.ID, action); err != nil {
				t.Fatal(err)
			}
			waitForSessionToSettle(t, manager, session.ID)
			record, err = manager.GetSession(context.Background(), session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if record.Status != string(StatusDone) {
				t.Fatalf("expected Claude resume to complete, got status %q: %s", record.Status, pointerString(record.LastError))
			}
			wantWorkflow := WorkflowModePlan
			if action == "approve" {
				wantWorkflow = WorkflowModeDefault
			}
			if effectiveWorkflowMode(record) != wantWorkflow || effectivePermissionLevel(record) != PermissionLevelYolo {
				t.Fatalf("wrong restored mode: %s/%s", record.WorkflowMode, record.PermissionLevel)
			}
			events, err := manager.store.readEvents(session.ID)
			if err != nil {
				t.Fatal(err)
			}
			resumed := false
			for _, event := range events {
				if event.Type == "run_st" {
					resumed = true
					if stringValue(event.Payload["wm"]) != string(wantWorkflow) || stringValue(event.Payload["pl"]) != string(PermissionLevelYolo) {
						t.Fatalf("resume used a stale permission snapshot: %#v", event.Payload)
					}
				}
			}
			if !resumed {
				t.Fatal("expected deferred Claude resume")
			}
		})
	}
}

func TestClaudeApprovalPermissionTransition(t *testing.T) {
	for _, test := range []struct {
		name         string
		plan         bool
		action       string
		initialLevel PermissionLevel
		currentLevel PermissionLevel
		wantBypass   bool
		failWrite    bool
	}{
		{name: "tool becomes automatic", action: "approve", initialLevel: PermissionLevelElevated, currentLevel: PermissionLevelYolo, wantBypass: true},
		{name: "plan becomes automatic", plan: true, action: "approve", initialLevel: PermissionLevelElevated, currentLevel: PermissionLevelYolo, wantBypass: true},
		{name: "ordinary tool approval", action: "approve", initialLevel: PermissionLevelElevated, currentLevel: PermissionLevelElevated},
		{name: "ordinary plan approval", plan: true, action: "approve", initialLevel: PermissionLevelElevated, currentLevel: PermissionLevelElevated},
		{name: "tool rejection", action: "reject", initialLevel: PermissionLevelElevated, currentLevel: PermissionLevelYolo},
		{name: "plan rejection", plan: true, action: "reject", initialLevel: PermissionLevelElevated, currentLevel: PermissionLevelYolo},
		{name: "permission downgrade", action: "approve", initialLevel: PermissionLevelYolo, currentLevel: PermissionLevelElevated},
		{name: "failed plan response", plan: true, action: "approve", initialLevel: PermissionLevelElevated, currentLevel: PermissionLevelYolo, failWrite: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			cleanup := initTestDB(t)
			defer cleanup()
			project := seedProject(t)
			session := seedWebSession(t, project.ID, "Claude permission transition", 1000)
			workflow, toolName := WorkflowModeDefault, "Bash"
			if test.plan {
				workflow, toolName = WorkflowModePlan, "ExitPlanMode"
			}
			if err := model.GetDB().Model(session).Updates(map[string]any{
				"agent": string(AgentClaude), "source_kind": sourceKindClaudeStreamJSON,
				"permission_level": string(test.initialLevel), "workflow_mode": string(workflow),
			}).Error; err != nil {
				t.Fatal(err)
			}
			manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := manager.GetSession(context.Background(), session.ID)
			if err != nil {
				t.Fatal(err)
			}
			capture := &claudeControlCapture{}
			run := &activeRun{sessionID: session.ID, agent: AgentClaude, runID: "run-transition"}
			if !test.failWrite {
				run.setInput(capture)
			}
			manager.runs[session.ID] = run
			defer delete(manager.runs, session.ID)
			input := map[string]any{"command": "echo hello"}
			if test.plan {
				input = map[string]any{"plan": "Implement the change"}
			}
			manager.handleClaudeEvent(snapshot, run, map[string]any{
				"type": "control_request", "request_id": "control-transition",
				"request": map[string]any{
					"subtype": "can_use_tool", "tool_name": toolName,
					"tool_use_id": "tool-transition", "input": input,
				},
			})
			if _, ok := run.pendingApprovalRequest(); !ok {
				t.Fatal("expected a pending approval")
			}
			// The process snapshot predates the user's permission change.
			if _, err := manager.UpdatePermissionLevel(context.Background(), session.ID, test.currentLevel); err != nil {
				t.Fatal(err)
			}
			err = manager.respondToApproval(session.ID, test.action)
			if test.failWrite {
				if err == nil {
					t.Fatal("expected missing runtime input to fail the approval")
				}
				if _, ok := run.pendingApprovalRequest(); !ok || !run.completedPlanToolSeen() {
					t.Fatal("failed response must retain the pending plan approval")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				response := decodeRawObject(capture.message(t)["response"])
				if stringValue(response["request_id"]) != "control-transition" {
					t.Fatalf("wrong control response: %#v", response)
				}
				result := decodeRawObject(response["response"])
				wantBehavior := "allow"
				if test.action == "reject" {
					wantBehavior = "deny"
				}
				if stringValue(result["behavior"]) != wantBehavior {
					t.Fatalf("wrong decision: %#v", result)
				}
				if test.wantBypass {
					updates, ok := result["updatedPermissions"].([]any)
					if !ok || len(updates) != 1 {
						t.Fatalf("expected one permission update: %#v", result)
					}
					update := decodeRawObject(updates[0])
					if stringValue(update["type"]) != "setMode" || stringValue(update["mode"]) != "bypassPermissions" || stringValue(update["destination"]) != "session" {
						t.Fatalf("expected session-scoped native bypass update: %#v", update)
					}
				} else if _, ok := result["updatedPermissions"]; ok {
					t.Fatalf("approval must not escalate permissions: %#v", result)
				}
				if wantBehavior == "allow" {
					updatedInput := decodeRawObject(result["updatedInput"])
					for key, value := range input {
						if updatedInput[key] != value {
							t.Fatalf("approval changed the tool input: %#v", updatedInput)
						}
					}
				}
				if _, ok := run.pendingApprovalRequest(); ok || run.completedPlanToolSeen() {
					t.Fatal("successful response must clear the pending approval")
				}
			}
			record, err := manager.GetSession(context.Background(), session.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantWorkflow, wantState := workflow, AssistantStateWorking
			if test.plan && test.action == "approve" && !test.failWrite {
				wantWorkflow = WorkflowModeDefault
			}
			if test.failWrite {
				wantState = AssistantStateWaitingPlanApproval
			}
			if effectiveWorkflowMode(record) != wantWorkflow || effectivePermissionLevel(record) != test.currentLevel || record.AssistantState != string(wantState) {
				t.Fatalf("unexpected session mode/state: %s/%s/%s", record.WorkflowMode, record.PermissionLevel, record.AssistantState)
			}
		})
	}
}
