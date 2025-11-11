package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectRepository(t *testing.T) {
	repoDir := initTestRepo(t)

	repo, err := DetectRepository(repoDir)
	if err != nil {
		t.Fatalf("DetectRepository returned error: %v", err)
	}

	if !equalPath(repo.Path, repoDir) {
		t.Fatalf("expected repository path %q got %q", repoDir, repo.Path)
	}

	if branch, err := repo.GetCurrentBranch(); err != nil || branch != "main" {
		t.Fatalf("unexpected current branch: %q (%v)", branch, err)
	}

	if value, ok := repo.ConfigValue("core.autocrlf"); !ok || strings.ToLower(value) != "true" {
		t.Fatalf("expected core.autocrlf=true, got %q (present=%v)", value, ok)
	}

	remotes, err := repo.GetRemotes()
	if err != nil {
		t.Fatalf("GetRemotes error: %v", err)
	}
	if len(remotes) != 1 || remotes[0].Name != "origin" {
		t.Fatalf("unexpected remotes: %#v", remotes)
	}
}

func TestBranchAndWorktreeOperations(t *testing.T) {
	repoDir := initTestRepo(t)
	repo, err := DetectRepository(repoDir)
	if err != nil {
		t.Fatalf("DetectRepository returned error: %v", err)
	}

	if err := repo.CreateBranch("feature/test", "HEAD"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	local, _, err := repo.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}

	found := false
	for _, branch := range local {
		if branch.Name == "feature/test" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("feature/test branch not present in local branches: %#v", local)
	}

	worktreeParent := t.TempDir()
	worktreePath := filepath.Join(worktreeParent, "feature-test")

	if err := repo.AddWorktree(worktreePath, "feature/test", false); err != nil {
		t.Fatalf("AddWorktree failed: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.RemoveWorktree(worktreePath, true)
	})

	worktrees, err := repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	if len(worktrees) < 2 {
		t.Fatalf("expected at least 2 worktrees got %d", len(worktrees))
	}

	status, err := GetWorktreeStatus(worktreePath)
	if err != nil {
		t.Fatalf("GetWorktreeStatus failed: %v", err)
	}
	if status.Branch == "" {
		t.Fatalf("expected branch name in worktree status")
	}
}

func TestGetWorktreeStatusUntracked(t *testing.T) {
	repoDir := initTestRepo(t)
	file := filepath.Join(repoDir, "new-file.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	status, err := GetWorktreeStatus(repoDir)
	if err != nil {
		t.Fatalf("GetWorktreeStatus failed: %v", err)
	}

	if status.Untracked == 0 {
		t.Fatalf("expected untracked file count > 0, got %#v", status)
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "core.autocrlf", "true")

	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test Repo\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial commit")
	runGit(t, dir, "remote", "add", "origin", "git@example.com:repo.git")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}
