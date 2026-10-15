package websession

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"code-kanban/model/tables"
	"go.uber.org/zap"
)

func TestCodexFastModePersistsAndResetsOnAgentChange(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, initial := range []bool{false, true} {
		created, err := manager.CreateSession(ctx, CreateParams{
			ProjectID: project.ID, Agent: AgentCodex, CodexFastMode: initial,
		})
		if err != nil {
			t.Fatal(err)
		}
		if created.CodexFastMode != initial {
			t.Fatalf("created fast mode = %v, want %v", created.CodexFastMode, initial)
		}
		for _, enabled := range []bool{true, false, true} {
			updated, err := manager.UpdateCodexFastMode(ctx, created.ID, enabled)
			if err != nil {
				t.Fatal(err)
			}
			record, err := manager.GetSession(ctx, created.ID)
			if err != nil {
				t.Fatal(err)
			}
			if updated.CodexFastMode != enabled || record.CodexFastMode != enabled {
				t.Fatalf("fast mode did not persist %v", enabled)
			}
			raw, err := json.Marshal(mapWireSession(updated))
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if fields["cfm"] != enabled {
				t.Fatalf("wire fast mode = %#v, want %v", fields["cfm"], enabled)
			}
		}
		updated, err := manager.UpdateAgent(ctx, created.ID, AgentClaude)
		if err != nil {
			t.Fatal(err)
		}
		if updated.CodexFastMode {
			t.Fatal("switching agents must reset Codex fast mode")
		}
		if _, err := manager.UpdateCodexFastMode(ctx, created.ID, true); err == nil {
			t.Fatal("must reject fast mode for other agents")
		}
	}
	if _, err := manager.CreateSession(ctx, CreateParams{
		ProjectID: project.ID, Agent: AgentClaude, CodexFastMode: true,
	}); err == nil {
		t.Fatal("must reject creating other agents with Codex fast mode")
	}
}

func TestCodexFastModeOverridesEveryRuntimeEntryPoint(t *testing.T) {
	manager := &Manager{cfg: Config{CodexPath: "codex"}}
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			session := tables.WebSessionTable{
				Agent: string(AgentCodex), Model: "gpt-6-astra", CodexFastMode: enabled,
			}
			want := "default"
			if enabled {
				want = "priority"
			}
			for _, modern := range []bool{false, true} {
				for _, params := range []map[string]any{
					codexThreadStartParams(session, modern),
					codexThreadResumeParams(session, "thread", modern),
					manager.codexTurnStartParams(session, "thread", "hello", nil),
				} {
					if params["serviceTier"] != want {
						t.Fatalf("serviceTier = %#v, want %q", params["serviceTier"], want)
					}
				}
			}
			for _, nativeID := range []*string{nil, ptr("thread")} {
				session.NativeSessionID = nativeID
				cmd, _, _, err := manager.buildExecCommand(context.Background(), session, "hello", nil)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for i := 0; i+1 < len(cmd.Args); i++ {
					if cmd.Args[i] == "-c" && cmd.Args[i+1] == fmt.Sprintf("service_tier=%q", want) {
						found = true
					}
				}
				if !found {
					t.Fatalf("missing service tier override in %v", cmd.Args)
				}
			}
		})
	}
}

func TestCodexFastModeCommandRequiresExplicitBoolean(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Fast mode", 1000)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, tc := range []struct {
		payload string
		want    bool
		valid   bool
	}{
		{`{"cfm":true}`, true, true},
		{`{}`, true, false},
		{`{"cfm":null}`, true, false},
		{`{"cfm":"false"}`, true, false},
		{`{"cfm":false}`, false, true},
	} {
		conn := &captureWSConn{}
		client := &client{conn: conn}
		frame := fmt.Sprintf(`{"v":1,"k":"cmd","rid":"fast-mode","sid":%q,"op":"set_cfm","p":%s}`, session.ID, tc.payload)
		if err := manager.HandleCommand(ctx, client, []byte(frame)); err != nil {
			t.Fatal(err)
		}
		if len(conn.frames) != 1 {
			t.Fatalf("unexpected response for %s: %#v", tc.payload, conn.frames)
		}
		response := conn.frames[0]
		if tc.valid {
			if response.Kind != wireKindAck || response.Revision == "" {
				t.Fatalf("missing revision in mutation ack: %#v", response)
			}
		} else if response.Kind != wireKindError || response.Code != "bad_req" {
			t.Fatalf("invalid payload must be rejected: %#v", response)
		}
		record, err := manager.GetSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if record.CodexFastMode != tc.want {
			t.Fatalf("unexpected persisted fast mode after %s", tc.payload)
		}
	}
}
