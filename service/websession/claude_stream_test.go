package websession

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/utils/ai_assistant2/log_watcher"

	"go.uber.org/zap"
)

func envContainsValue(env []string, key, value string) bool {
	prefix := key + "="
	for _, item := range env {
		if item == prefix+value {
			return true
		}
	}
	return false
}

func TestBuildExecCommandClaudeUsesStreamJSONInput(t *testing.T) {
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatalf("newStore returned error: %v", err)
	}
	manager := &Manager{cfg: Config{DataDir: t.TempDir(), ClaudePath: "claude"}, store: store, logger: zap.NewNop(), runs: map[string]*activeRun{}, clients: map[*client]struct{}{}}
	session := tables.WebSessionTable{
		Agent:           string(AgentClaude),
		Model:           "opus",
		ReasoningEffort: string(ReasoningEffortHigh),
		WorkflowMode:    string(WorkflowModePlan),
		PermissionLevel: string(PermissionLevelElevated),
		Cwd:             "/tmp/project",
	}

	cmd, stdinBytes, closeStdinAfterWrite, err := manager.buildExecCommand(
		context.Background(),
		session,
		"inspect this repository",
		nil,
	)
	if err != nil {
		t.Fatalf("buildExecCommand returned error: %v", err)
	}
	if !closeStdinAfterWrite {
		t.Fatalf("expected Claude stdin to close after the turn payload is written")
	}
	joinedArgs := strings.Join(cmd.Args, " ")
	for _, expected := range []string{
		"--input-format stream-json",
		"--output-format stream-json",
		"--replay-user-messages",
		"--permission-mode plan",
		"--effort high",
	} {
		if !strings.Contains(joinedArgs, expected) {
			t.Fatalf("expected args to contain %q, got %v", expected, cmd.Args)
		}
	}
	if strings.Contains(joinedArgs, "inspect this repository") {
		t.Fatalf("expected Claude prompt to be provided via stdin, got args %v", cmd.Args)
	}
	stdinText := string(stdinBytes)
	if strings.Contains(stdinText, "You are operating in planning mode.") {
		t.Fatalf("expected Claude native plan mode without preamble injection, got %q", stdinText)
	}
	if !strings.Contains(stdinText, "inspect this repository") {
		t.Fatalf("expected prompt text in stdin payload, got %q", stdinText)
	}
}

func TestBuildExecCommandClaudeRouterUsesCCRCodeAndInjectsHookSettings(t *testing.T) {
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatalf("newStore returned error: %v", err)
	}
	manager := &Manager{
		cfg:     Config{DataDir: t.TempDir(), ClaudePath: "claude", CCRPath: "ccr"},
		store:   store,
		logger:  zap.NewNop(),
		runs:    map[string]*activeRun{},
		clients: map[*client]struct{}{},
	}
	session := tables.WebSessionTable{
		Agent:           string(AgentClaude),
		ClaudeRuntime:   string(ClaudeRuntimeCCR),
		Model:           "sonnet",
		WorkflowMode:    string(WorkflowModeDefault),
		PermissionLevel: string(PermissionLevelElevated),
		Cwd:             "/tmp/project",
	}

	cmd, _, _, err := manager.buildExecCommand(context.Background(), session, "hi", nil)
	if err != nil {
		t.Fatalf("buildExecCommand returned error: %v", err)
	}
	if len(cmd.Args) < 2 || cmd.Args[0] != "ccr" || cmd.Args[1] != "code" {
		t.Fatalf("expected CCR runtime to launch ccr code, got %v", cmd.Args)
	}
	joinedArgs := strings.Join(cmd.Args, " ")
	for _, expected := range []string{
		"--input-format stream-json",
		"--output-format stream-json",
		"--model sonnet",
	} {
		if !strings.Contains(joinedArgs, expected) {
			t.Fatalf("expected args to contain %q, got %v", expected, cmd.Args)
		}
	}
	if strings.Contains(joinedArgs, "--settings") {
		t.Fatalf("expected CCR runtime to let ccr code own --settings args, got %v", cmd.Args)
	}
	if manager.ccrHookClaudePath == "" {
		t.Fatalf("expected CCR runtime to configure a Claude settings shim")
	}
	if !envContainsValue(cmd.Env, "CLAUDE_PATH", manager.ccrHookClaudePath) {
		t.Fatalf("expected CCR runtime to pass shim via CLAUDE_PATH, got %v", cmd.Env)
	}
	shimData, err := os.ReadFile(strings.TrimSuffix(manager.ccrHookClaudePath, ".cmd") + ".js")
	if err != nil {
		t.Fatalf("failed to read CodeKanban CCR shim: %v", err)
	}
	if !strings.Contains(string(shimData), "settings.allowedHttpHookUrls = mergeUniqueStrings") ||
		!strings.Contains(string(shimData), "settings.hooks = mergeHooks") {
		t.Fatalf("expected CodeKanban CCR shim to merge hook settings, got %s", string(shimData))
	}
	settingsData, err := os.ReadFile(manager.claudeHookSettingsPath)
	if err != nil {
		t.Fatalf("failed to read Claude hook settings: %v", err)
	}
	if strings.Contains(string(settingsData), `"http://127.0.0.1:`) &&
		!strings.Contains(string(settingsData), "/claude-hooks/pre-tool-use") {
		t.Fatalf("expected allowedHttpHookUrls to allow the full hook URL, got %s", string(settingsData))
	}
	if !strings.Contains(string(settingsData), `"matcher":"AskUserQuestion"`) ||
		!strings.Contains(string(settingsData), `"matcher":"ExitPlanMode"`) {
		t.Fatalf("expected hook settings for AskUserQuestion and ExitPlanMode, got %s", string(settingsData))
	}
}

func TestEnsureCCRClaudeHookSettingsDoesNotRequireCCRConfig(t *testing.T) {
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatalf("newStore returned error: %v", err)
	}
	manager := &Manager{
		cfg:     Config{DataDir: t.TempDir(), ClaudePath: "claude", CCRPath: "ccr", CCRConfigPath: filepath.Join(t.TempDir(), "missing", "config.json")},
		store:   store,
		logger:  zap.NewNop(),
		runs:    map[string]*activeRun{},
		clients: map[*client]struct{}{},
	}

	if err := manager.ensureCCRClaudeHookSettings(); err != nil {
		t.Fatalf("expected CCR hook setup to use the shim without reading CCR config, got %v", err)
	}
	if manager.ccrHookClaudePath == "" {
		t.Fatal("expected CCR hook setup to configure the Claude shim path")
	}
}

func TestBuildExecCommandClaudeRejectsDefaultPermissionLevel(t *testing.T) {
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatalf("newStore returned error: %v", err)
	}
	manager := &Manager{cfg: Config{DataDir: t.TempDir(), ClaudePath: "claude"}, store: store, logger: zap.NewNop(), runs: map[string]*activeRun{}, clients: map[*client]struct{}{}}
	session := tables.WebSessionTable{
		Agent:           string(AgentClaude),
		Model:           "opus",
		WorkflowMode:    string(WorkflowModeDefault),
		PermissionLevel: string(PermissionLevelDefault),
		Cwd:             "/tmp/project",
	}

	if _, _, _, err := manager.buildExecCommand(context.Background(), session, "hi", nil); err == nil {
		t.Fatal("expected Claude default permission level to be rejected")
	}
}

func TestHandleClaudeDeferredResultCreatesPendingInput(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Claude Ask", 1000)
	if err := model.GetDB().Model(session).Updates(map[string]any{
		"agent":            string(AgentClaude),
		"source_kind":      sourceKindClaudeStreamJSON,
		"permission_level": string(PermissionLevelElevated),
	}).Error; err != nil {
		t.Fatalf("failed to update session agent: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	run := &activeRun{
		sessionID: session.ID,
		agent:     AgentClaude,
		runID:     "run_test",
	}

	manager.handleClaudeEvent(*session, run, map[string]any{
		"type":        "result",
		"session_id":  "claude-session-1",
		"stop_reason": "tool_deferred",
		"deferred_tool_use": map[string]any{
			"id":   "tool_ask_1",
			"name": "AskUserQuestion",
			"input": map[string]any{
				"questions": []any{
					map[string]any{
						"header":      "Direction",
						"question":    "What should happen next?",
						"multiSelect": false,
						"options": []any{
							map[string]any{
								"label":       "Implement",
								"description": "Start implementing now.",
							},
							map[string]any{
								"label":       "Plan",
								"description": "Stay in planning mode.",
							},
						},
					},
				},
			},
		},
	})

	request, ok := run.pendingUserInputRequest()
	if !ok || request == nil {
		t.Fatal("expected pending AskUserQuestion request")
	}
	if request.ItemID != "tool_ask_1" {
		t.Fatalf("expected pending item id tool_ask_1, got %q", request.ItemID)
	}
	if len(request.Questions) != 1 {
		t.Fatalf("expected one question, got %d", len(request.Questions))
	}
	if request.Questions[0].ID != "What should happen next?" {
		t.Fatalf("expected question id fallback to question text, got %q", request.Questions[0].ID)
	}
	if request.Questions[0].MultiSelect {
		t.Fatalf("expected single-select question, got %#v", request.Questions[0])
	}

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.AssistantState != string(AssistantStateWaitingInput) {
		t.Fatalf("expected assistant state %q, got %q", AssistantStateWaitingInput, record.AssistantState)
	}

	rawEvents, err := manager.store.readEvents(session.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if !historyHasEvent(rawEvents, "user_input_req") {
		t.Fatalf("expected user_input_req event, got %#v", rawEvents)
	}
}

func TestSyncClaudeSessionFromSourceRebuildsAskUserQuestionHistory(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Claude Sync", 1000)
	cwd := filepath.Join(homeDir, "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("failed to create cwd: %v", err)
	}
	if err := model.GetDB().Model(session).Updates(map[string]any{
		"agent":             string(AgentClaude),
		"cwd":               cwd,
		"native_session_id": "claude-session-1",
		"source_kind":       sourceKindClaudeStreamJSON,
		"permission_level":  string(PermissionLevelElevated),
	}).Error; err != nil {
		t.Fatalf("failed to update Claude session fields: %v", err)
	}

	projectDir := filepath.Join(
		homeDir,
		log_watcher.ClaudeCodeDirName,
		log_watcher.ClaudeCodeProjectsSubDir,
		log_watcher.EncodePathForClaude(cwd),
	)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("failed to create Claude project dir: %v", err)
	}
	sessionFile := filepath.Join(projectDir, "claude-session-1.jsonl")
	content := strings.Join([]string{
		`{"type":"queue-operation","operation":"enqueue","timestamp":"2026-04-11T10:00:00.000Z","sessionId":"claude-session-1","content":"Start implementing the feature"}`,
		`{"parentUuid":null,"isSidechain":false,"promptId":"prompt_1","type":"user","message":{"role":"user","content":"Start implementing the feature"},"uuid":"user_1","timestamp":"2026-04-11T10:00:01.000Z","permissionMode":"acceptEdits","userType":"external","entrypoint":"sdk-cli","cwd":"` + cwd + `","sessionId":"claude-session-1","version":"2.1.97"}`,
		`{"parentUuid":"user_1","isSidechain":false,"type":"assistant","uuid":"assistant_tool","timestamp":"2026-04-11T10:00:02.000Z","message":{"id":"assistant_tool_msg","type":"message","role":"assistant","content":[{"type":"tool_use","id":"tool_bash_1","name":"Bash","input":{"command":"pwd","description":"Confirm working directory"}}],"stop_reason":"tool_use"}}`,
		`{"parentUuid":"assistant_tool","isSidechain":false,"promptId":"prompt_1","type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool_bash_1","content":"/tmp/repo","is_error":false}]},"uuid":"tool_result_1","timestamp":"2026-04-11T10:00:03.000Z","toolUseResult":{"stdout":"/tmp/repo","stderr":"","interrupted":false,"isImage":false,"noOutputExpected":false},"sourceToolAssistantUUID":"assistant_tool"}`,
		`{"parentUuid":"tool_result_1","isSidechain":false,"type":"assistant","uuid":"assistant_text","timestamp":"2026-04-11T10:00:04.000Z","message":{"id":"assistant_text_msg","type":"message","role":"assistant","content":[{"type":"text","text":"Before I continue, choose a direction."}],"stop_reason":"end_turn"}}`,
		`{"parentUuid":"assistant_text","isSidechain":false,"type":"assistant","uuid":"assistant_ask","timestamp":"2026-04-11T10:00:05.000Z","message":{"id":"assistant_ask_msg","type":"message","role":"assistant","content":[{"type":"tool_use","id":"tool_ask_1","name":"AskUserQuestion","input":{"questions":[{"header":"Direction","question":"What should happen next?","multiSelect":false,"options":[{"label":"Implement feature","description":"Start coding immediately."},{"label":"Stay in plan mode","description":"Hold off on changes."}]}]}}],"stop_reason":"tool_use"}}`,
		`{"parentUuid":"assistant_ask","isSidechain":false,"promptId":"prompt_1","type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool_ask_1","content":"{\"answers\":{\"What should happen next?\":[\"Implement feature\"]}}","is_error":false}]},"uuid":"tool_result_ask","timestamp":"2026-04-11T10:00:06.000Z","toolUseResult":"{\"answers\":{\"What should happen next?\":[\"Implement feature\"]}}","sourceToolAssistantUUID":"assistant_ask"}`,
		`{"parentUuid":"tool_result_ask","isSidechain":false,"type":"assistant","uuid":"assistant_done","timestamp":"2026-04-11T10:00:07.000Z","message":{"id":"assistant_done_msg","type":"message","role":"assistant","content":[{"type":"text","text":"Understood. I will implement the feature next."}],"stop_reason":"end_turn"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(sessionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write Claude session file: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	snapshot, err := manager.SyncSessionWithMode(context.Background(), session.ID, SyncModeFast, false)
	if err != nil {
		t.Fatalf("SyncSessionWithMode returned error: %v", err)
	}

	if snapshot.Session.SourceKind != sourceKindClaudeStreamJSON {
		t.Fatalf("expected source kind %q, got %q", sourceKindClaudeStreamJSON, snapshot.Session.SourceKind)
	}
	if snapshot.Session.ThreadPath == nil || !strings.HasSuffix(*snapshot.Session.ThreadPath, "claude-session-1.jsonl") {
		t.Fatalf("expected synced thread path, got %#v", snapshot.Session.ThreadPath)
	}
	if snapshot.History.Total != 6 {
		t.Fatalf("expected 6 history items, got %d", snapshot.History.Total)
	}

	var sawCommand, sawAsk, sawAskResponse bool
	for _, item := range snapshot.History.Items {
		switch item.ItemType {
		case "command_execution":
			sawCommand = item.Tool != nil && item.Tool.Status == "done" && strings.Contains(item.Tool.Output, "/tmp/repo")
		case "user_input_request":
			sawAsk = item.Detail != nil && len(item.Detail.Questions) == 1 && item.Detail.Questions[0].ID == "What should happen next?"
		case "user_input_response":
			sawAskResponse = item.Detail != nil &&
				len(item.Detail.Answers) == 1 &&
				len(item.Detail.Answers[0].Values) == 1 &&
				item.Detail.Answers[0].Values[0] == "Implement feature"
		}
	}
	if !sawCommand {
		t.Fatalf("expected synced Bash command execution item, got %#v", snapshot.History.Items)
	}
	if !sawAsk {
		t.Fatalf("expected synced AskUserQuestion request, got %#v", snapshot.History.Items)
	}
	if !sawAskResponse {
		t.Fatalf("expected synced AskUserQuestion response, got %#v", snapshot.History.Items)
	}
}

func TestParseClaudeStreamHistoryPlanModeStripsPreambleAndBuildsPlanTool(t *testing.T) {
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatalf("newStore returned error: %v", err)
	}
	manager := &Manager{cfg: Config{DataDir: t.TempDir()}, store: store, logger: zap.NewNop(), runs: map[string]*activeRun{}, clients: map[*client]struct{}{}}

	filePath := filepath.Join(t.TempDir(), "claude-plan.jsonl")
	content := strings.Join([]string{
		`{"type":"queue-operation","operation":"enqueue","timestamp":"2026-04-12T01:00:00.000Z","sessionId":"plan-session","content":"You are operating in planning mode. Inspect the project first, summarize the goal, and propose a concrete plan before making changes. Do not mutate files until the user confirms execution or explicitly asks you to proceed immediately. If additional permissions are needed, call them out explicitly.\n\nUser request:\n你在计划模式吗"}`,
		`{"parentUuid":null,"isSidechain":false,"promptId":"prompt_1","type":"user","message":{"role":"user","content":"You are operating in planning mode. Inspect the project first, summarize the goal, and propose a concrete plan before making changes. Do not mutate files until the user confirms execution or explicitly asks you to proceed immediately. If additional permissions are needed, call them out explicitly.\n\nUser request:\n你在计划模式吗"},"uuid":"user_1","timestamp":"2026-04-12T01:00:01.000Z","permissionMode":"acceptEdits","sessionId":"plan-session"}`,
		`{"parentUuid":"user_1","isSidechain":false,"type":"assistant","uuid":"assistant_plan","timestamp":"2026-04-12T01:00:02.000Z","message":{"id":"assistant_plan_msg","type":"message","role":"assistant","content":[{"type":"tool_use","id":"exit_plan_1","name":"ExitPlanMode","input":{"plan":"# Plan: Create 123.md\n\n## Context\nCreate a harmless file.\n\n## Implementation Steps\n1. Create the file\n2. Verify it exists","planFilePath":"/home/dev/.claude/plans/test.md"}}],"stop_reason":"tool_use"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	parsed, err := manager.parseClaudeStreamHistory(filePath, WorkflowModePlan)
	if err != nil {
		t.Fatalf("parseClaudeStreamHistory returned error: %v", err)
	}
	if len(parsed.Items) != 2 {
		t.Fatalf("expected 2 parsed items, got %d: %#v", len(parsed.Items), parsed.Items)
	}
	if parsed.Items[0].Kind != "user" || parsed.Items[0].Text != "你在计划模式吗" {
		t.Fatalf("expected stripped user text, got %#v", parsed.Items[0])
	}
	if parsed.Items[1].Kind != "tool" || parsed.Items[1].ItemType != "plan" || parsed.Items[1].Tool == nil {
		t.Fatalf("expected plan tool item, got %#v", parsed.Items[1])
	}
	if !strings.Contains(parsed.Items[1].Tool.Output, "## Implementation Steps") {
		t.Fatalf("expected plan output to preserve approval text, got %#v", parsed.Items[1].Tool)
	}
	if got := strings.TrimSpace(parsed.Items[1].Tool.Meta["path"].(string)); got != "/home/dev/.claude/plans/test.md" {
		t.Fatalf("expected plan file path in tool meta, got %#v", parsed.Items[1].Tool.Meta)
	}
}

func TestHandleClaudeEventExitPlanModeCreatesPlanToolAndWaitingApproval(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Claude Plan", 1000)
	if err := model.GetDB().Model(session).Updates(map[string]any{
		"agent":            string(AgentClaude),
		"workflow_mode":    string(WorkflowModePlan),
		"permission_level": string(PermissionLevelElevated),
		"source_kind":      sourceKindClaudeStreamJSON,
	}).Error; err != nil {
		t.Fatalf("failed to update session agent: %v", err)
	}

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	run := &activeRun{
		sessionID: session.ID,
		agent:     AgentClaude,
		runID:     "run_plan",
	}

	manager.handleClaudeEvent(*session, run, map[string]any{
		"type": "assistant",
		"uuid": "assistant_exit_plan",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{
					"type": "tool_use",
					"id":   "exit_plan_tool_1",
					"name": "ExitPlanMode",
					"input": map[string]any{
						"plan":         "# Plan\n\n## Context\nCreate file\n\n## Implementation Steps\n1. Write file",
						"planFilePath": "/home/dev/.claude/plans/exit-plan.md",
					},
				},
			},
			"stop_reason": "tool_use",
		},
	})
	manager.handleClaudeEvent(*session, run, map[string]any{
		"type":        "result",
		"session_id":  "claude-plan-session",
		"stop_reason": "tool_deferred",
		"deferred_tool_use": map[string]any{
			"id":   "exit_plan_tool_1",
			"name": "ExitPlanMode",
			"input": map[string]any{
				"plan":         "# Plan\n\n## Context\nCreate file\n\n## Implementation Steps\n1. Write file",
				"planFilePath": "/home/dev/.claude/plans/exit-plan.md",
			},
		},
	})

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.AssistantState != string(AssistantStateWaitingPlanApproval) {
		t.Fatalf("expected waiting plan approval state, got %q", record.AssistantState)
	}

	history, err := manager.History(context.Background(), session.ID, 50, nil)
	if err != nil {
		t.Fatalf("History returned error: %v", err)
	}
	if !historyHasToolKind(history.Events, "plan") {
		t.Fatalf("expected plan tool history, got %#v", history.Events)
	}
	rawEvents, err := manager.store.readEvents(session.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if !historyHasEvent(rawEvents, "approval_req") {
		t.Fatalf("expected approval_req event, got %#v", rawEvents)
	}
}

func TestClaudeRunClosesInputAfterEndTurn(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:    t.TempDir(),
		ClaudePath: writeFakeClaudeStreamCLI(t),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:       project.ID,
		Agent:           AgentClaude,
		PermissionLevel: PermissionLevelElevated,
		Model:           "haiku",
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	if err := manager.SendMessage(context.Background(), created.ID, "say hi", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	waitForSessionToSettle(t, manager, created.ID)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.Status != string(StatusDone) {
		t.Fatalf("expected status %q, got %q", StatusDone, record.Status)
	}
	if record.AssistantState != "" {
		t.Fatalf("expected cleared assistant state, got %q", record.AssistantState)
	}

	rawEvents, err := manager.store.readEvents(created.ID)
	if err != nil {
		t.Fatalf("readEvents returned error: %v", err)
	}
	if !historyHasEvent(rawEvents, "run_done") {
		t.Fatalf("expected run_done event, got %#v", rawEvents)
	}
	if historyHasEvent(rawEvents, "run_fail") {
		t.Fatalf("did not expect run_fail event, got %#v", rawEvents)
	}
}

func TestClaudeHookServerDefersAndAllowsAskUserQuestion(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	settingsPath, err := manager.ensureClaudeHookServer()
	if err != nil {
		t.Fatalf("ensureClaudeHookServer returned error: %v", err)
	}
	if !strings.HasSuffix(settingsPath, "claude-hook-settings.json") {
		t.Fatalf("unexpected settings path %q", settingsPath)
	}
	if !filepath.IsAbs(settingsPath) {
		t.Fatalf("expected absolute settings path, got %q", settingsPath)
	}

	requestBody := map[string]any{
		"session_id":  "session-1",
		"tool_use_id": "tool-1",
		"tool_name":   "AskUserQuestion",
		"tool_input": map[string]any{
			"questions": []any{
				map[string]any{
					"header":      "Direction",
					"question":    "What should happen next?",
					"multiSelect": false,
					"options": []any{
						map[string]any{"label": "Implement", "description": "Start coding"},
						map[string]any{"label": "Plan", "description": "Stay in planning"},
					},
				},
			},
		},
	}

	response, err := httpPostJSON(
		manager.claudeHookBaseURL+"/claude-hooks/pre-tool-use",
		"Bearer "+manager.claudeHookToken,
		requestBody,
	)
	if err != nil {
		t.Fatalf("httpPostJSON returned error: %v", err)
	}
	hookSpecificOutput := decodeRawObject(response["hookSpecificOutput"])
	if strings.TrimSpace(stringValue(hookSpecificOutput["permissionDecision"])) != "defer" {
		t.Fatalf("expected defer response, got %#v", response)
	}

	if err := manager.writeClaudeHookAnswer("session-1", "tool-1", claudeHookAnswerFile{
		Answers: map[string]string{
			"What should happen next?": "Implement",
		},
	}); err != nil {
		t.Fatalf("writeClaudeHookAnswer returned error: %v", err)
	}
	response, err = httpPostJSON(
		manager.claudeHookBaseURL+"/claude-hooks/pre-tool-use",
		"Bearer "+manager.claudeHookToken,
		requestBody,
	)
	if err != nil {
		t.Fatalf("httpPostJSON returned error: %v", err)
	}
	hookSpecificOutput = decodeRawObject(response["hookSpecificOutput"])
	if strings.TrimSpace(stringValue(hookSpecificOutput["permissionDecision"])) != "allow" {
		t.Fatalf("expected allow response, got %#v", response)
	}
	updatedInput := decodeRawObject(hookSpecificOutput["updatedInput"])
	answers := decodeRawObject(updatedInput["answers"])
	if got := strings.TrimSpace(stringValue(answers["What should happen next?"])); got != "Implement" {
		t.Fatalf("expected updatedInput answers to contain Implement, got %#v", response)
	}
}

func TestClaudeHookAnswerIsMirroredToNativeSessionID(t *testing.T) {
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatalf("newStore returned error: %v", err)
	}
	manager := &Manager{store: store}
	nativeSessionID := "claude-native-session"
	session := tables.WebSessionTable{
		NativeSessionID: &nativeSessionID,
	}
	session.ID = "web-session-1"
	payload := claudeHookAnswerFile{
		Answers: map[string]string{
			"What should happen next?": "Implement",
		},
	}

	if err := manager.writeClaudeHookAnswerForSession(session, "tool-1", payload); err != nil {
		t.Fatalf("writeClaudeHookAnswerForSession returned error: %v", err)
	}
	for _, sessionID := range []string{session.ID, nativeSessionID} {
		answer, err := manager.readClaudeHookAnswer(sessionID, "tool-1")
		if err != nil {
			t.Fatalf("readClaudeHookAnswer(%q) returned error: %v", sessionID, err)
		}
		if got := answer.Answers["What should happen next?"]; got != "Implement" {
			t.Fatalf("expected mirrored answer for %q, got %#v", sessionID, answer.Answers)
		}
	}

	manager.deleteClaudeHookAnswerForSession(session, "tool-1")
	for _, sessionID := range []string{session.ID, nativeSessionID} {
		if _, err := os.Stat(manager.store.claudeHookAnswerPath(sessionID, "tool-1")); !os.IsNotExist(err) {
			t.Fatalf("expected answer for %q to be cleaned up, got err=%v", sessionID, err)
		}
	}
}

func TestRespondToUserInputClaudeResumesAfterDeferredTool(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir:    t.TempDir(),
		ClaudePath: writeFakeClaudeDeferredCLI(t),
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID:       project.ID,
		Agent:           AgentClaude,
		PermissionLevel: PermissionLevelElevated,
		Model:           "haiku",
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	if err := manager.SendMessage(context.Background(), created.ID, "ask me a question", nil); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)

	record, err := manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.AssistantState != string(AssistantStateWaitingInput) {
		t.Fatalf("expected waiting input after deferred tool, got %q", record.AssistantState)
	}
	if manager.hasActiveRun(created.ID) {
		t.Fatalf("expected no active process after deferred tool exit")
	}

	if err := manager.respondToUserInput(created.ID, "tool_ask_resume", map[string][]string{
		"What should happen next?": {"Implement"},
	}); err != nil {
		t.Fatalf("respondToUserInput returned error: %v", err)
	}
	waitForSessionToSettle(t, manager, created.ID)

	record, err = manager.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if record.AssistantState != "" {
		t.Fatalf("expected assistant state cleared after resumed answer, got %q", record.AssistantState)
	}

	snapshot, err := manager.Snapshot(context.Background(), created.ID, 80)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	foundFinal := false
	for _, item := range snapshot.History.Items {
		if item.Kind == "assistant" && strings.Contains(item.Text, "continuing after the answer") {
			foundFinal = true
		}
	}
	if !foundFinal {
		t.Fatalf("expected resumed assistant reply in history, got %#v", snapshot.History.Items)
	}
	if _, err := os.Stat(manager.store.claudeHookAnswerPath(created.ID, "tool_ask_resume")); !os.IsNotExist(err) {
		t.Fatalf("expected deferred answer file to be cleaned up, got err=%v", err)
	}
	if record.NativeSessionID == nil || strings.TrimSpace(*record.NativeSessionID) == "" {
		t.Fatalf("expected native Claude session id to be stored")
	}
	if _, err := os.Stat(manager.store.claudeHookAnswerPath(*record.NativeSessionID, "tool_ask_resume")); !os.IsNotExist(err) {
		t.Fatalf("expected native deferred answer file to be cleaned up, got err=%v", err)
	}
}

func httpPostJSON(url string, auth string, body map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(auth) != "" {
		request.Header.Set("Authorization", auth)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}
