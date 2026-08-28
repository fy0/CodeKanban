package git

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWithSystemGitTimeoutAddsHardDeadline(t *testing.T) {
	const timeout = 200 * time.Millisecond
	started := time.Now()
	ctx, cancel := withSystemGitTimeout(nil, timeout)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("system Git context has no deadline")
	}
	if elapsed := deadline.Sub(started); elapsed <= 0 || elapsed > timeout+50*time.Millisecond {
		t.Fatalf("system Git deadline = %s, want at most %s", elapsed, timeout)
	}
}

func TestWithSystemGitTimeoutKeepsShorterCallerDeadline(t *testing.T) {
	parent, parentCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer parentCancel()
	parentDeadline, _ := parent.Deadline()
	ctx, cancel := withSystemGitTimeout(parent, time.Second)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("system Git context has no deadline")
	}
	if deadline.After(parentDeadline) {
		t.Fatalf("system Git deadline %s exceeds caller deadline %s", deadline, parentDeadline)
	}
}

func TestSystemGitCommandStopsAfterCallerDeadline(t *testing.T) {
	info := ProbeSystemGit(t.Context(), true)
	if !info.Available {
		t.Skip("system Git is unavailable")
	}
	previous := CurrentEngineSettings()
	ConfigureEngines(EngineSettings{Read: EnginePreferenceSystem, Write: EnginePreferenceSystem, Executable: info.Executable})
	t.Cleanup(func() { ConfigureEngines(previous) })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := runSystemGitOutput(
		ctx,
		"",
		OperationStatus,
		"-c",
		"alias.codekanban-wait=!sleep 5",
		"codekanban-wait",
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("slow system Git error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("slow system Git returned after %s, want at most 2s", elapsed)
	}
}
