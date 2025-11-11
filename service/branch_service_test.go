package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go-template/model"
	"go-template/utils/git"
)

func TestBranchServiceListMarksWorktrees(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	repoPath := createProjectTestRepo(t)
	projectService := &model.ProjectService{}
	project, err := projectService.CreateProject(context.Background(), model.CreateProjectParams{
		Name: "Branch Project",
		Path: repoPath,
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	branchSvc := NewBranchService()
	result, err := branchSvc.ListBranches(context.Background(), project.Id)
	if err != nil {
		t.Fatalf("ListBranches returned error: %v", err)
	}
	if len(result.Local) == 0 {
		t.Fatalf("expected at least one local branch")
	}

	found := false
	for _, branch := range result.Local {
		if branch.Name == project.DefaultBranch {
			found = true
			if !branch.HasWorktree {
				t.Fatalf("expected default branch %s to be marked with worktree", branch.Name)
			}
			break
		}
	}
	if !found {
		t.Fatalf("default branch %s not found in local list", project.DefaultBranch)
	}
}

func TestBranchServiceDeleteProtected(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	repoPath := createProjectTestRepo(t)
	projectService := &model.ProjectService{}
	project, err := projectService.CreateProject(context.Background(), model.CreateProjectParams{
		Name: "Protected Project",
		Path: repoPath,
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	branchSvc := NewBranchService()
	ctx := context.Background()

	if err := branchSvc.DeleteBranch(ctx, project.Id, project.DefaultBranch, false); !errors.Is(err, model.ErrProtectedBranch) {
		t.Fatalf("expected ErrProtectedBranch deleting default branch, got %v", err)
	}

	if err := branchSvc.CreateBranch(ctx, project.Id, "feature/protected", "", false); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	repo, err := git.DetectRepository(repoPath)
	if err != nil {
		t.Fatalf("DetectRepository failed: %v", err)
	}
	if err := repo.CheckoutBranch("feature/protected"); err != nil {
		t.Fatalf("checkout to feature/protected failed: %v", err)
	}
	defer repo.CheckoutBranch(project.DefaultBranch)

	if err := branchSvc.DeleteBranch(ctx, project.Id, "feature/protected", false); !errors.Is(err, model.ErrProtectedBranch) {
		t.Fatalf("expected ErrProtectedBranch deleting current branch, got %v", err)
	}

	if err := repo.CheckoutBranch(project.DefaultBranch); err != nil {
		t.Fatalf("checkout back to default failed: %v", err)
	}
}

func TestBranchServiceForceDeleteWithWorktree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("force deleting worktrees is flaky on Windows")
	}

	cleanup := initTestDB(t)
	defer cleanup()

	repoPath := createProjectTestRepo(t)
	projectService := &model.ProjectService{}
	project, err := projectService.CreateProject(context.Background(), model.CreateProjectParams{
		Name: "Force Delete Project",
		Path: repoPath,
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	branchSvc := NewBranchService()
	ctx := context.Background()

	if err := branchSvc.CreateBranch(ctx, project.Id, "feature/force", "", true); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	worktreeService := NewWorktreeService()
	worktrees, err := worktreeService.ListWorktrees(ctx, project.Id)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	var target *model.Worktree
	for _, wt := range worktrees {
		if wt.BranchName == "feature/force" {
			target = wt
			break
		}
	}
	if target == nil {
		t.Fatalf("expected worktree for feature/force branch")
	}

	err = branchSvc.DeleteBranch(ctx, project.Id, "feature/force", false)
	if !errors.Is(err, model.ErrBranchHasWorktree) {
		t.Fatalf("expected ErrBranchHasWorktree, got %v", err)
	}

	if err := branchSvc.DeleteBranch(ctx, project.Id, "feature/force", true); err != nil {
		t.Fatalf("force DeleteBranch failed: %v", err)
	}

	if _, err := worktreeService.GetWorktree(ctx, target.Id); !errors.Is(err, model.ErrWorktreeNotFound) {
		t.Fatalf("expected worktree removed, got %v", err)
	}

	repo, err := git.DetectRepository(repoPath)
	if err != nil {
		t.Fatalf("DetectRepository failed: %v", err)
	}
	local, _, err := repo.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}
	for _, branch := range local {
		if branch.Name == "feature/force" {
			t.Fatalf("feature/force branch still exists after deletion")
		}
	}
}

func TestBranchServiceMergeSuccess(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	repoPath := createProjectTestRepo(t)
	projectService := &model.ProjectService{}
	project, err := projectService.CreateProject(context.Background(), model.CreateProjectParams{
		Name: "Merge Project",
		Path: repoPath,
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	branchSvc := NewBranchService()
	ctx := context.Background()

	const sourceBranch = "feature/merge"
	if err := branchSvc.CreateBranch(ctx, project.Id, sourceBranch, "", false); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	runGitCommand(t, repoPath, "checkout", sourceBranch)
	featureFile := filepath.Join(repoPath, "merge.txt")
	if err := os.WriteFile(featureFile, []byte("merge content"), 0o644); err != nil {
		t.Fatalf("write merge file failed: %v", err)
	}
	runGitCommand(t, repoPath, "add", "merge.txt")
	runGitCommand(t, repoPath, "commit", "-m", "add merge file")
	runGitCommand(t, repoPath, "checkout", project.DefaultBranch)

	worktreeService := NewWorktreeService()
	worktrees, err := worktreeService.ListWorktrees(ctx, project.Id)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	var mainWT *model.Worktree
	for _, wt := range worktrees {
		if wt.BranchName == project.DefaultBranch {
			mainWT = wt
			break
		}
	}
	if mainWT == nil {
		t.Fatalf("failed to locate default branch worktree")
	}

	result, err := branchSvc.MergeBranch(ctx, mainWT.Id, sourceBranch, model.MergeBranchOptions{
		TargetBranch: mainWT.BranchName,
		Strategy:     "merge",
	})
	if err != nil {
		t.Fatalf("MergeBranch returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected merge success, got result: %+v", result)
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", result.Conflicts)
	}
}

func TestBranchServiceSquashMergeCommit(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	repoPath := createProjectTestRepo(t)
	projectService := &model.ProjectService{}
	project, err := projectService.CreateProject(context.Background(), model.CreateProjectParams{
		Name: "Squash Merge Project",
		Path: repoPath,
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	branchSvc := NewBranchService()
	ctx := context.Background()

	const sourceBranch = "feature/squash"
	if err := branchSvc.CreateBranch(ctx, project.Id, sourceBranch, "", false); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	runGitCommand(t, repoPath, "checkout", sourceBranch)
	featureFile := filepath.Join(repoPath, "squash.txt")
	if err := os.WriteFile(featureFile, []byte("squash content"), 0o644); err != nil {
		t.Fatalf("write squash file failed: %v", err)
	}
	runGitCommand(t, repoPath, "add", "squash.txt")
	runGitCommand(t, repoPath, "commit", "-m", "add squash file")
	runGitCommand(t, repoPath, "checkout", project.DefaultBranch)

	worktreeService := NewWorktreeService()
	worktrees, err := worktreeService.ListWorktrees(ctx, project.Id)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	var targetWT *model.Worktree
	for _, wt := range worktrees {
		if wt.BranchName == project.DefaultBranch {
			targetWT = wt
			break
		}
	}
	if targetWT == nil {
		t.Fatalf("failed to locate default branch worktree")
	}

	const commitMessage = "feat: squash merge commit"
	result, err := branchSvc.MergeBranch(ctx, targetWT.Id, sourceBranch, model.MergeBranchOptions{
		TargetBranch:  targetWT.BranchName,
		Strategy:      "squash",
		Commit:        true,
		CommitMessage: commitMessage,
	})
	if err != nil {
		t.Fatalf("MergeBranch returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected merge success, got result: %+v", result)
	}

	repo, err := git.DetectRepository(repoPath)
	if err != nil {
		t.Fatalf("DetectRepository failed: %v", err)
	}
	status, err := repo.GetWorktreeStatus(targetWT.Path)
	if err != nil {
		t.Fatalf("GetWorktreeStatus failed: %v", err)
	}
	if status.LastCommit == nil || status.LastCommit.Message != commitMessage {
		t.Fatalf("expected last commit message %q, got %+v", commitMessage, status.LastCommit)
	}
	if status.Staged != 0 || status.Modified != 0 {
		t.Fatalf("expected clean worktree after commit, staged=%d modified=%d", status.Staged, status.Modified)
	}
}
