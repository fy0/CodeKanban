package websession

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/service"

	"go.uber.org/zap"
)

func TestBuildTrustedPiRPCCommandRequiresTrustAndAddsApprove(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	manager, err := NewManager(Config{
		DataDir: t.TempDir(),
		PiPath:  `"` + os.Args[0] + `"`,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	_, err = manager.buildTrustedPiRPCCommand(
		context.Background(),
		project.ID,
		project.Path,
		"--name",
		"test",
	)
	if !errors.Is(err, service.ErrProjectAgentTrustRequired) {
		t.Fatalf("untrusted launch error = %v, want trust required", err)
	}
	if _, err := manager.TrustProjectForPi(context.Background(), project.ID); err != nil {
		t.Fatalf("TrustProjectForPi returned error: %v", err)
	}
	cmd, err := manager.buildTrustedPiRPCCommand(
		context.Background(),
		project.ID,
		project.Path,
		"--name",
		"test",
	)
	if err != nil {
		t.Fatalf("trusted launch returned error: %v", err)
	}
	if cmd.Dir != project.Path {
		t.Fatalf("command dir = %q, want %q", cmd.Dir, project.Path)
	}
	args := strings.Join(cmd.Args[1:], " ")
	if !strings.Contains(args, "--mode rpc") || !strings.Contains(args, "--approve") || !strings.Contains(args, "--extension") {
		t.Fatalf("trusted command args = %#v", cmd.Args)
	}
	if strings.Contains(args, "--no-approve") {
		t.Fatalf("trusted command contains --no-approve: %#v", cmd.Args)
	}
	for _, managedFlag := range []string{"--no-approve", "--approve=false", "--mode=interactive", "--extension=untrusted.ts", "-e"} {
		if _, err := manager.buildTrustedPiRPCCommand(
			context.Background(),
			project.ID,
			project.Path,
			managedFlag,
		); err == nil {
			t.Fatalf("expected caller-supplied managed flag %q to be rejected", managedFlag)
		}
	}
}

func TestRevokeProjectPiTrustStopsOnlyProjectRuntimes(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	otherProject := seedProject(t)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if _, err := manager.TrustProjectForPi(context.Background(), project.ID); err != nil {
		t.Fatalf("TrustProjectForPi returned error: %v", err)
	}

	var stopped atomic.Int32
	var otherStopped atomic.Int32
	manager.registerPiRuntimeTerminator("session-1", project.ID, func() { stopped.Add(1) })
	manager.registerPiRuntimeTerminator("session-2", project.ID, func() { stopped.Add(1) })
	manager.registerPiRuntimeTerminator("session-other", otherProject.ID, func() { otherStopped.Add(1) })
	status, err := manager.RevokeProjectPiTrust(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("RevokeProjectPiTrust returned error: %v", err)
	}
	if status.Trusted || status.RevokedAt == nil {
		t.Fatalf("unexpected revoked status: %#v", status)
	}
	if stopped.Load() != 2 || otherStopped.Load() != 0 {
		t.Fatalf("stopped=%d otherStopped=%d", stopped.Load(), otherStopped.Load())
	}
	manager.piRuntimeMu.Lock()
	_, otherExists := manager.piRuntimeTerminators["session-other"]
	remaining := len(manager.piRuntimeTerminators)
	manager.piRuntimeMu.Unlock()
	if !otherExists || remaining != 1 {
		t.Fatalf("remaining terminators = %d, other exists=%v", remaining, otherExists)
	}
}

func TestEnsureProjectPiTrustAcceptsManagedWorktree(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	worktree := &tables.WorktreeTable{
		ProjectID:  project.ID,
		BranchName: "feature/pi-trust",
		Path:       t.TempDir(),
	}
	worktree.Init()
	if err := model.GetDB().Create(worktree).Error; err != nil {
		t.Fatalf("seed worktree: %v", err)
	}
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if _, err := manager.TrustProjectForPi(context.Background(), project.ID); err != nil {
		t.Fatalf("TrustProjectForPi returned error: %v", err)
	}
	if err := manager.EnsureProjectPiTrust(context.Background(), project.ID, worktree.Path); err != nil {
		t.Fatalf("managed worktree rejected: %v", err)
	}
	if err := manager.EnsureProjectPiTrust(context.Background(), project.ID, t.TempDir()); !errors.Is(err, service.ErrProjectAgentPathNotAllowed) {
		t.Fatalf("unmanaged cwd result = %v, want path not allowed", err)
	}

}
