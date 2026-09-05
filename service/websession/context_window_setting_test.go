package websession

import (
	"context"
	"encoding/json"
	"testing"

	"code-kanban/model/tables"
	"go.uber.org/zap"
)

func TestContextWindowWireFieldsRemainDistinct(t *testing.T) {
	raw, err := json.Marshal(mapWireSession(SessionSummary{ContextWindowSetting: 512000, AppliedContextWindowSetting: ptr(int64(768000)), ContextWindowSource: ContextWindowSourceSessionUsage}))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["cwset"] != float64(512000) || fields["acwset"] != float64(768000) || fields["cws"] != "session_usage" {
		t.Fatalf("wire field collision: %s", raw)
	}
}

func TestCodexModelMetadataFallbackWarningIsScopedAndPersisted(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Context window warning", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	run := &activeRun{runID: "run-1", agent: AgentCodex, assistantMessageID: "message-1"}
	scope := codexTurnScope{threadID: "thread-1", turnID: "turn-1"}
	warning := "Model metadata for `gpt-6-astra` not found. Defaulting to fallback metadata; this can degrade performance and cause issues."
	for _, child := range []bool{true, false} {
		threadID := "thread-1"
		if child {
			threadID = "child-1"
		}
		handleCodexTestNotification(t, manager, *session, run, scope, "warning", map[string]any{"threadId": threadID, "message": warning})
		record, err := manager.GetSession(context.Background(), session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if record.CodexModelMetadataFallback == child {
			t.Fatal("warning was not scoped to the root thread")
		}
		if !child {
			wire := mapWireSession(mapSessionRecord(record))
			if !wire.CodexModelMetadataFallback {
				t.Fatal("warning missing from session response")
			}
		}
	}
	if _, err := manager.UpdateModel(context.Background(), session.ID, "other-model"); err != nil {
		t.Fatal(err)
	}
	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil || record.CodexModelMetadataFallback {
		t.Fatal("model change must clear stale warning")
	}
}

func TestContextWindowRunSnapshotChangesOnNextRun(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir(), CodexPath: writeFakeCodexAppServerCLI(t, "basic")}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	created, err := manager.CreateSession(ctx, CreateParams{ProjectID: project.ID, Agent: AgentCodex, ContextWindowSetting: ptr(int64(512000))})
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range []int64{512000, 768000, 0} {
		if _, err := manager.UpdateContextWindowSetting(ctx, created.ID, setting); err != nil {
			t.Fatal(err)
		}
		if err := manager.SendMessage(ctx, created.ID, "inspect", nil); err != nil {
			t.Fatal(err)
		}
		waitForSessionToSettle(t, manager, created.ID)
		record, err := manager.GetSession(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if record.AppliedContextWindowSetting == nil || *record.AppliedContextWindowSetting != setting {
			t.Fatalf("run did not apply setting %d: %#v", setting, record.AppliedContextWindowSetting)
		}
	}
}

func TestContextWindowThreadOverrides(t *testing.T) {
	for _, setting := range []int64{0, 512000, 768000, 1000000} {
		for _, modern := range []bool{false, true} {
			session := tables.WebSessionTable{ContextWindowSetting: setting}
			for _, params := range []map[string]any{codexThreadStartParams(session, modern), codexThreadResumeParams(session, "thread", modern)} {
				config := params["config"].(map[string]any)
				value, exists := config["model_context_window"]
				if setting == 0 && exists || setting > 0 && value != setting {
					t.Fatalf("unexpected override: %#v", config)
				}
				if _, exists := config["model_auto_compact_token_limit"]; exists {
					t.Fatal("must preserve Codex compaction policy")
				}
			}
		}
	}
}

func TestContextWindowSettingsPersistIndependently(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	global := int64(512000)
	manager, err := NewManager(Config{DataDir: t.TempDir(), DefaultCodexContextWindow: func() int64 { return global }}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := manager.CreateSession(ctx, CreateParams{ProjectID: project.ID, Agent: AgentCodex})
	if err != nil {
		t.Fatal(err)
	}
	global = 768000
	second, err := manager.CreateSession(ctx, CreateParams{ProjectID: project.ID, Agent: AgentCodex})
	if err != nil {
		t.Fatal(err)
	}
	explicitDefault, err := manager.CreateSession(ctx, CreateParams{ProjectID: project.ID, Agent: AgentCodex, ContextWindowSetting: ptr(int64(0))})
	if err != nil {
		t.Fatal(err)
	}
	if first.ContextWindowSetting != 512000 || second.ContextWindowSetting != 768000 || explicitDefault.ContextWindowSetting != 0 {
		t.Fatal("creation defaults not snapshotted")
	}
	if err := manager.updateRuntimeState(ctx, first.ID, map[string]any{"applied_context_window_setting": int64(512000), "session_context_window_tokens": int64(486400)}); err != nil {
		t.Fatal(err)
	}
	updated, err := manager.UpdateContextWindowSetting(ctx, first.ID, 1000000)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContextWindowSetting != 1000000 || updated.AppliedContextWindowSetting == nil || *updated.AppliedContextWindowSetting != 512000 || updated.ContextWindowTokens == nil || *updated.ContextWindowTokens != 486400 {
		t.Fatal("updating setting must preserve actual running window")
	}
	unchanged, err := manager.GetSession(ctx, second.ID)
	if err != nil || unchanged.ContextWindowSetting != 768000 {
		t.Fatal("other session changed")
	}
	if _, err := manager.UpdateContextWindowSetting(ctx, first.ID, 42); err == nil {
		t.Fatal("invalid preset accepted")
	}
	if _, err := manager.CreateSession(ctx, CreateParams{ProjectID: project.ID, Agent: AgentClaude, ContextWindowSetting: ptr(int64(512000))}); err == nil {
		t.Fatal("non-Codex override accepted")
	}
	if _, err := manager.UpdateContextWindowSetting(ctx, first.ID, 0); err != nil {
		t.Fatal(err)
	}
	restored, err := manager.GetSession(ctx, first.ID)
	if err != nil || restored.ContextWindowSetting != 0 {
		t.Fatal("default reset was not persisted")
	}
}
