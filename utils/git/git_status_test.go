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

	runGit(t, repoDir, "restore", "README.md")
	if err := os.WriteFile(trackedPath, []byte("staged\n"), 0o644); err != nil {
		t.Fatalf("write staged file: %v", err)
	}
	runGit(t, repoDir, "add", "README.md")
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

func TestParseGitStatusOutput(t *testing.T) {
	output := `
# branch.oid 3d2b07e3ce0b
# branch.head main
# branch.upstream origin/main
# branch.ab +2 -1
1 M. N... 0000000 0000000 0000000 README.md
1 .M N... 0000000 0000000 0000000 file.go
1 MM N... 0000000 0000000 0000000 both.txt
? new.txt
u UU N... 0000000 0000000 0000000 conflict.txt
`

	status := parseGitStatusOutput(output)
	if status == nil {
		t.Fatalf("parseGitStatusOutput returned nil")
	}

	if status.Branch != "main" {
		t.Fatalf("expected branch main got %q", status.Branch)
	}
	if status.Ahead != 2 || status.Behind != 1 {
		t.Fatalf("unexpected ahead/behind counts: %+v", status)
	}
	if status.Staged != 2 {
		t.Fatalf("expected staged=2 got %d", status.Staged)
	}
	if status.Modified != 2 {
		t.Fatalf("expected modified=2 got %d", status.Modified)
	}
	if status.Untracked != 1 {
		t.Fatalf("expected untracked=1 got %d", status.Untracked)
	}
	if status.Conflicted != 1 {
		t.Fatalf("expected conflicted=1 got %d", status.Conflicted)
	}
}
