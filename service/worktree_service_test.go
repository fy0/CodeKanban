package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/utils/git"

	goGit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
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

	worktree, err := svc.CreateWorktree(ctx, project.Id, "feature/testing", CreateWorktreeOptions{
		BaseBranch:   "main",
		CreateBranch: true,
	})
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

	svc := NewWorktreeService()
	svc.AsyncRefresh(false)
	ctx := context.Background()

	worktree, err := svc.CreateWorktree(ctx, project.Id, "feature/delete", CreateWorktreeOptions{
		BaseBranch:   "main",
		CreateBranch: true,
	})
	if err != nil {
		t.Fatalf("CreateWorktree returned error: %v", err)
	}

	if err := svc.DeleteWorktree(ctx, worktree.Id, true, true); err != nil {
		t.Fatalf("DeleteWorktree returned error: %v", err)
	}

	if _, err := svc.GetWorktree(ctx, worktree.Id); err == nil {
		t.Fatalf("expected worktree to be deleted")
	}

	manualPath := filepath.Join(repoPath, "manual")
	repository, err := git.DetectRepository(repoPath)
	if err != nil {
		t.Fatalf("detect repository: %v", err)
	}
	if err := repository.CreateBranch("feature/manual-sync", "main"); err != nil {
		t.Fatalf("create manual branch: %v", err)
	}
	if err := repository.AddWorktree(manualPath, "feature/manual-sync", false); err != nil {
		t.Fatalf("create manual worktree: %v", err)
	}
	_ = repository.Close()
	defer func() {
		cleanupRepo, openErr := git.DetectRepository(repoPath)
		if openErr == nil {
			_ = cleanupRepo.RemoveWorktree(manualPath, true)
			_ = cleanupRepo.Close()
		}
	}()

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

	if _, err := svc.CreateWorktree(ctx, project.Id, "feature/all", CreateWorktreeOptions{
		BaseBranch:   "main",
		CreateBranch: true,
	}); err != nil {
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

func TestWorktreeServiceRefreshAllNonGitProject(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	projectService := &model.ProjectService{}
	projectPath := t.TempDir()
	project, err := projectService.CreateProject(context.Background(), model.CreateProjectParams{
		Name: "Plain Folder",
		Path: projectPath,
	})
	if err != nil {
		t.Fatalf("create project failed: %v", err)
	}

	svc := NewWorktreeService()
	worktrees, err := svc.ListWorktrees(context.Background(), project.Id)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	if len(worktrees) != 1 {
		t.Fatalf("expected virtual main worktree, got %d", len(worktrees))
	}

	updated, failed, err := svc.RefreshAllWorktrees(context.Background(), project.Id)
	if err != nil {
		t.Fatalf("RefreshAllWorktrees returned error: %v", err)
	}
	if updated != 0 || failed != 0 {
		t.Fatalf("expected no refresh work for non-git project, got updated=%d failed=%d", updated, failed)
	}

	refreshed, err := svc.RefreshWorktreeStatus(context.Background(), worktrees[0].Id)
	if err != nil {
		t.Fatalf("RefreshWorktreeStatus returned error: %v", err)
	}
	if refreshed.Id != worktrees[0].Id {
		t.Fatalf("expected same worktree to be returned")
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

	worktree, err := svc.CreateWorktree(ctx, project.Id, "feature/commit", CreateWorktreeOptions{
		BaseBranch:   "main",
		CreateBranch: true,
	})
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

func TestWorktreeServiceCreateWorktree_PersistWorktreeBasePath(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	repoPath := createProjectTestRepo(t)
	projectService := &model.ProjectService{}
	project, err := projectService.CreateProject(context.Background(), model.CreateProjectParams{
		Name: "Persist Project",
		Path: repoPath,
	})
	if err != nil {
		t.Fatalf("create project failed: %v", err)
	}

	q, err := model.ResolveQueries(nil)
	if err != nil {
		t.Fatalf("resolve queries failed: %v", err)
	}

	svc := NewWorktreeService()
	svc.AsyncRefresh(false)
	ctx := context.Background()

	globalBaseDir := t.TempDir()
	if _, err := svc.CreateWorktree(ctx, project.Id, "feature/global", CreateWorktreeOptions{
		BaseBranch:           "main",
		CreateBranch:         true,
		Location:             "global",
		GlobalBaseDir:        globalBaseDir,
		GlobalDirNamePattern: "{projectName}-{branch}",
	}); err != nil {
		t.Fatalf("CreateWorktree(global) returned error: %v", err)
	}

	updated, err := q.ProjectGetByID(ctx, project.Id)
	if err != nil {
		t.Fatalf("reload project failed: %v", err)
	}
	if updated.WorktreeBasePath == nil || filepath.Clean(*updated.WorktreeBasePath) != filepath.Clean(globalBaseDir) {
		t.Fatalf("expected worktreeBasePath to be persisted to %s, got %v", filepath.Clean(globalBaseDir), updated.WorktreeBasePath)
	}

	if _, err := svc.CreateWorktree(ctx, project.Id, "feature/project", CreateWorktreeOptions{
		BaseBranch:   "main",
		CreateBranch: true,
		Location:     "project",
	}); err != nil {
		t.Fatalf("CreateWorktree(project) returned error: %v", err)
	}

	cleared, err := q.ProjectGetByID(ctx, project.Id)
	if err != nil {
		t.Fatalf("reload project failed: %v", err)
	}
	if cleared.WorktreeBasePath != nil && strings.TrimSpace(*cleared.WorktreeBasePath) != "" {
		t.Fatalf("expected worktreeBasePath to be cleared, got %v", cleared.WorktreeBasePath)
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
	repository, err := goGit.PlainInit(dir, false, goGit.WithDefaultBranch(plumbing.NewBranchReferenceName("main")))
	if err != nil {
		t.Fatalf("init repository: %v", err)
	}
	cfg, err := repository.Config()
	if err != nil {
		t.Fatalf("read repository config: %v", err)
	}
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@example.com"
	cfg.Commit.GpgSign = config.OptBoolFalse
	if err := repository.SetConfig(cfg); err != nil {
		t.Fatalf("write repository config: %v", err)
	}

	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("demo"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatalf("open worktree: %v", err)
	}
	if err := worktree.AddWithOptions(&goGit.AddOptions{All: true}); err != nil {
		t.Fatalf("stage readme: %v", err)
	}
	if _, err := worktree.Commit("init", &goGit.CommitOptions{Author: &object.Signature{
		Name: "Test User", Email: "test@example.com", When: time.Now(),
	}}); err != nil {
		t.Fatalf("commit readme: %v", err)
	}
	_ = repository.Close()
	return dir
}

// TestSanitizeBranchName_Security tests that sanitizeBranchName correctly handles
// potentially dangerous branch names that could lead to path traversal.
func TestSanitizeBranchName_Security(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", "_invalid_branch_"},
		{"dot", ".", "_invalid_branch_"},
		{"dotdot", "..", "_invalid_branch_"},
		{"normal branch", "feature/test", "feature__test"},
		{"with backslash", "feature\\test", "feature__test"},
		{"dotdot in name", "feature..test", "_invalid_branch_"},
		{"trailing dots", "feature..", "_invalid_branch_"},
		{"leading dots", "..feature", "_invalid_branch_"},
		{"triple dot", "...", "_invalid_branch_"},
		{"valid dotfile", ".gitignore", ".gitignore"},
		{"spaces only", "   ", "_invalid_branch_"},
		{"mixed special chars", "feat:test*?<>|", "feat_test_____"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeBranchName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeBranchName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestExpandWorktreeDirNamePattern_Security tests that pattern expansion
// correctly rejects potentially dangerous patterns.
func TestExpandWorktreeDirNamePattern_Security(t *testing.T) {
	project := &model.Project{
		Id:   "test-id",
		Name: "Test Project",
	}

	tests := []struct {
		name        string
		pattern     string
		branchName  string
		shouldError bool
	}{
		{"normal pattern", "{projectName}-{branch}", "feature/test", false},
		{"dotdot branch", "{projectName}-{branch}", "..", false}, // sanitizeBranchName will handle this
		{"path separator in result", "{projectName}/{branch}", "test", true},
		{"backslash in result", "{projectName}\\{branch}", "test", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := expandWorktreeDirNamePattern(tt.pattern, project, tt.branchName)
			if tt.shouldError && err == nil {
				t.Errorf("expected error for pattern %q with branch %q, got nil", tt.pattern, tt.branchName)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("unexpected error for pattern %q with branch %q: %v", tt.pattern, tt.branchName, err)
			}
		})
	}
}
