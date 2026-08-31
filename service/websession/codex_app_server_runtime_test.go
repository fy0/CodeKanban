package websession

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"go.uber.org/zap"
)

func TestCodexAppServerRuntimeStates(t *testing.T) {
	newRun := func() *activeRun {
		return &activeRun{
			sessionID: "session-1",
			projectID: "project-1",
			agent:     AgentCodex,
			backend:   SessionBackendCodexAppServer,
			runID:     "run-1",
			done:      make(chan struct{}),
		}
	}

	t.Run("inactive", func(t *testing.T) {
		manager := &Manager{
			runs:           map[string]*activeRun{},
			codexRunDrains: map[string]*activeRun{},
		}
		got := manager.codexAppServerRuntime("session-1")
		if got.State != CodexAppServerInactive || got.CanTerminate {
			t.Fatalf("runtime = %#v, want inactive", got)
		}
	})

	t.Run("starting", func(t *testing.T) {
		run := newRun()
		manager := &Manager{
			runs:           map[string]*activeRun{"session-1": run},
			codexRunDrains: map[string]*activeRun{},
		}
		got := manager.codexAppServerRuntime("session-1")
		if got.State != CodexAppServerStarting || got.RunID != "run-1" || got.CanTerminate {
			t.Fatalf("runtime = %#v, want starting", got)
		}
	})

	t.Run("active", func(t *testing.T) {
		run := newRun()
		run.app = &codexAppServerClient{}
		run.cmd = &exec.Cmd{Process: &os.Process{Pid: 4242}}
		manager := &Manager{
			runs:           map[string]*activeRun{"session-1": run},
			codexRunDrains: map[string]*activeRun{},
		}
		got := manager.codexAppServerRuntime("session-1")
		if got.State != CodexAppServerActive || !got.CanTerminate || got.ProcessRootPID != 4242 {
			t.Fatalf("runtime = %#v, want active PID 4242", got)
		}
	})

	t.Run("draining", func(t *testing.T) {
		run := newRun()
		run.app = &codexAppServerClient{}
		manager := &Manager{
			runs:           map[string]*activeRun{},
			codexRunDrains: map[string]*activeRun{"session-1": run},
		}
		got := manager.codexAppServerRuntime("session-1")
		if got.State != CodexAppServerDraining || !got.CanTerminate {
			t.Fatalf("runtime = %#v, want draining", got)
		}
	})

	t.Run("terminating", func(t *testing.T) {
		run := newRun()
		run.app = &codexAppServerClient{}
		run.forceTerminateRequested = true
		manager := &Manager{
			runs:           map[string]*activeRun{"session-1": run},
			codexRunDrains: map[string]*activeRun{},
		}
		got := manager.codexAppServerRuntime("session-1")
		if got.State != CodexAppServerTerminating || got.CanTerminate {
			t.Fatalf("runtime = %#v, want terminating", got)
		}
	})
}

func TestCodexAppServerWireFrameUsesDedicatedCompactPayload(t *testing.T) {
	encoded, err := json.Marshal(newCodexAppServerFrame("session-1", CodexAppServerRuntime{
		State:          CodexAppServerActive,
		RunID:          "run-1",
		ProcessRootPID: 4242,
		CanTerminate:   true,
	}))
	if err != nil {
		t.Fatalf("marshal app-server frame: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode app-server frame: %v", err)
	}
	if payload["k"] != "evt" || payload["op"] != "app_server" {
		t.Fatalf("unexpected frame envelope: %s", encoded)
	}
	runtimeState, _ := payload["cas"].(map[string]any)
	if runtimeState["st"] != "active" || runtimeState["rid"] != "run-1" ||
		runtimeState["pid"] != float64(4242) || runtimeState["ct"] != true {
		t.Fatalf("unexpected runtime payload: %#v", runtimeState)
	}
}

func TestUnchangedSnapshotIncludesCodexAppServerRuntime(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	created, err := manager.CreateSession(context.Background(), CreateParams{
		ProjectID: project.ID,
		Agent:     AgentCodex,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	run := &activeRun{
		sessionID: created.ID,
		projectID: project.ID,
		agent:     AgentCodex,
		backend:   SessionBackendCodexAppServer,
		runID:     "run-starting",
		done:      make(chan struct{}),
	}
	manager.mu.Lock()
	manager.runs[created.ID] = run
	manager.mu.Unlock()

	response, err := manager.SnapshotIfChanged(
		context.Background(),
		created.ID,
		DefaultHistoryWindow,
		created.Revision,
	)
	if err != nil {
		t.Fatalf("SnapshotIfChanged: %v", err)
	}
	if !response.Unchanged || response.Session != nil {
		t.Fatalf("response = %#v, want unchanged response without session", response)
	}
	if response.CodexAppServer.State != CodexAppServerStarting ||
		response.CodexAppServer.RunID != "run-starting" {
		t.Fatalf("runtime = %#v, want starting", response.CodexAppServer)
	}
}
