package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"go-template/model"
)

func TestWorktreeServiceCreateAndRefresh(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	repoPath := createProjectTestRepo(t)
	projectService := &model.ProjectService{}
	project, err := projectService.CreateProject(context.Background(), model.CreateProjectParams{
		Name: "WT Project",
		Path: repoPath,
	})
	if err != nil {
		t.Fatalf("create project failed: %v", err)
	}

	svc := NewWorktreeService()
	svc.AsyncRefresh(false)
	ctx := context.Background()

	worktree, err := svc.CreateWorktree(ctx, project.Id, "feature/testing", "main", true)
	if err != nil {
		t.Fatalf("CreateWorktree returned error: %v", err)
	}
	if worktree.Id == "" {
		t.Fatalf("expected worktree to have ID")
	}
	if worktree.Path == "" {
		t.Fatalf("expected worktree path to be set")
	}

	if _, err := os.Stat(worktree.Path); err != nil {
		t.Fatalf("git did not create worktree path: %v", err)
	}

	got, err := svc.GetWorktree(ctx, worktree.Id)
	if err != nil {
		t.Fatalf("GetWorktree failed: %v", err)
	}
	if got.BranchName != "feature/testing" {
		t.Fatalf("expected branch name feature/testing, got %s", got.BranchName)
	}

	refreshed, err := svc.RefreshWorktreeStatus(ctx, worktree.Id)
	if err != nil {
		t.Fatalf("RefreshWorktreeStatus failed: %v", err)
	}
	if refreshed.StatusUpdatedAt == nil {
		t.Fatalf("expected status updated timestamp to be set")
	}
}

func TestWorktreeServiceDeleteAndSync(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	repoPath := createProjectTestRepo(t)
	projectService := &model.ProjectService{}
	project, err := projectService.CreateProject(context.Background(), model.CreateProjectParams{
		Name: "Delete Project",
		Path: repoPath,
	})
	if err != nil {
		t.Fatalf("create project failed: %v", err)
	}

	if runtime.GOOS == "windows" {
		t.Skip("DeleteWorktree integration test is flaky on Windows due to git worktree remove permissions")
	}

	svc := NewWorktreeService()
	svc.AsyncRefresh(false)
	ctx := context.Background()

	worktree, err := svc.CreateWorktree(ctx, project.Id, "feature/delete", "main", true)
	if err != nil {
		t.Fatalf("CreateWorktree returned error: %v", err)
	}

	if err := svc.DeleteWorktree(ctx, worktree.Id, true, true); err != nil {
		t.Fatalf("DeleteWorktree returned error: %v", err)
	}

	if _, err := svc.GetWorktree(ctx, worktree.Id); err == nil {
		t.Fatalf("expected worktree to be deleted")
	}

	runGitCommand(t, repoPath, "worktree", "add", filepath.Join(repoPath, "manual"), "main")
	defer runGitCommand(t, repoPath, "worktree", "remove", filepath.Join(repoPath, "manual"))

	if err := svc.SyncWorktrees(ctx, project.Id); err != nil {
		t.Fatalf("SyncWorktrees returned error: %v", err)
	}

	worktrees, err := svc.ListWorktrees(ctx, project.Id)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	found := false
	for _, wt := range worktrees {
		if wt.Path == filepath.Join(repoPath, "manual") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected manual worktree to be synced into database")
	}
}

func TestWorktreeServiceRefreshAll(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	repoPath := createProjectTestRepo(t)
	projectService := &model.ProjectService{}
	project, err := projectService.CreateProject(context.Background(), model.CreateProjectParams{
		Name: "Refresh Project",
		Path: repoPath,
	})
	if err != nil {
		t.Fatalf("create project failed: %v", err)
	}

	svc := NewWorktreeService()
	svc.AsyncRefresh(false)
	ctx := context.Background()

	if _, err := svc.CreateWorktree(ctx, project.Id, "feature/all", "main", true); err != nil {
		t.Fatalf("CreateWorktree returned error: %v", err)
	}

	updated, failed, err := svc.RefreshAllWorktrees(ctx, project.Id)
	if err != nil {
		t.Fatalf("RefreshAllWorktrees returned error: %v", err)
	}
	if updated == 0 || failed != 0 {
		t.Fatalf("unexpected refresh counts updated=%d failed=%d", updated, failed)
	}

	time.Sleep(10 * time.Millisecond)
	updatedWTs, err := svc.ListWorktrees(ctx, project.Id)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	if len(updatedWTs) == 0 || updatedWTs[0].StatusUpdatedAt == nil {
		t.Fatalf("expected worktree status to be refreshed")
	}
}

func TestWorktreeServiceCommit(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	repoPath := createProjectTestRepo(t)
	projectService := &model.ProjectService{}
	project, err := projectService.CreateProject(context.Background(), model.CreateProjectParams{
		Name: "Commit Project",
		Path: repoPath,
	})
	if err != nil {
		t.Fatalf("create project failed: %v", err)
	}

	svc := NewWorktreeService()
	svc.AsyncRefresh(false)
	ctx := context.Background()

	worktree, err := svc.CreateWorktree(ctx, project.Id, "feature/commit", "main", true)
	if err != nil {
		t.Fatalf("CreateWorktree returned error: %v", err)
	}

	targetFile := filepath.Join(worktree.Path, "commit.txt")
	if err := os.WriteFile(targetFile, []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to write file in worktree: %v", err)
	}

	updated, err := svc.CommitWorktree(ctx, worktree.Id, "feat: add commit file")
	if err != nil {
		t.Fatalf("CommitWorktree returned error: %v", err)
	}
	if updated == nil {
		t.Fatalf("expected updated worktree after commit")
	}

	if _, err := svc.CommitWorktree(ctx, worktree.Id, "noop"); !errors.Is(err, model.ErrWorktreeClean) {
		t.Fatalf("expected ErrWorktreeClean, got %v", err)
	}
}

func initTestDB(t *testing.T) func() {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	if err := model.InitWithDSN(dsn, 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}
	return func() {
		model.DBClose()
	}
}

func createProjectTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	runGitCommand(t, dir, "init", "-b", "main")
	runGitCommand(t, dir, "config", "user.email", "test@example.com")
	runGitCommand(t, dir, "config", "user.name", "Test User")

	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("demo"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	runGitCommand(t, dir, "add", "README.md")
	runGitCommand(t, dir, "commit", "-m", "init")
	return dir
}

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}
