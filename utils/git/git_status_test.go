package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHasTrackedWorktreeChanges(t *testing.T) {
	repoDir := initTestRepo(t)

	assertChanged := func(want bool) {
		t.Helper()
		changed, err := HasTrackedWorktreeChanges(repoDir)
		if err != nil {
			t.Fatalf("HasTrackedWorktreeChanges returned error: %v", err)
		}
		if changed != want {
			t.Fatalf("HasTrackedWorktreeChanges = %v, want %v", changed, want)
		}
	}

	assertChanged(false)

	untrackedPath := filepath.Join(repoDir, "untracked.txt")
	if err := os.WriteFile(untrackedPath, []byte("untracked\n"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	assertChanged(false)

	trackedPath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(trackedPath, []byte("modified\n"), 0o644); err != nil {
		t.Fatalf("modify tracked file: %v", err)
	}
	assertChanged(true)

	if err := os.WriteFile(trackedPath, []byte("# Test Repo\n"), 0o644); err != nil {
		t.Fatalf("restore tracked file: %v", err)
	}
	if err := os.WriteFile(trackedPath, []byte("staged\n"), 0o644); err != nil {
		t.Fatalf("write staged file: %v", err)
	}
	stageAllTestFiles(t, repoDir)
	assertChanged(true)
}

func TestHasTrackedWorktreeChangesContextHonorsCancellation(t *testing.T) {
	repoDir := initTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := HasTrackedWorktreeChangesContext(ctx, repoDir); err == nil {
		t.Fatal("HasTrackedWorktreeChangesContext succeeded with a canceled context")
	}
}

func TestGetWorktreeStatusIncludesBranchAndCommit(t *testing.T) {
	repoDir := initTestRepo(t)
	status, err := GetWorktreeStatus(repoDir)
	if err != nil {
		t.Fatalf("GetWorktreeStatus returned error: %v", err)
	}
	if status.Branch != "main" {
		t.Fatalf("expected branch main, got %q", status.Branch)
	}
	if status.LastCommit == nil || status.LastCommit.Message != "initial commit" {
		t.Fatalf("unexpected last commit: %#v", status.LastCommit)
	}
}
