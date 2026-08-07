package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"code-kanban/model"
	"code-kanban/utils/git"
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
	result, err := branchSvc.ListBranches(context.Background(), project.Id, true)
	if err != nil {
		t.Fatalf("ListBranches returned error: %v", err)
	}
	if len(result.Local) == 0 {
		t.Fatalf("expected at least one local branch")
	}

	found := false
	for _, branch := range result.Local {
		if branch.Name == defaultBranch(project) {
			found = true
			if !branch.HasWorktree {
				t.Fatalf("expected default branch %s to be marked with worktree", branch.Name)
			}
			break
		}
	}
	if !found {
		t.Fatalf("default branch %s not found in local list", defaultBranch(project))
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

	if err := branchSvc.DeleteBranch(ctx, project.Id, defaultBranch(project), false); !errors.Is(err, model.ErrProtectedBranch) {
		t.Fatalf("expected ErrProtectedBranch deleting default branch, got %v", err)
	}

	if err := branchSvc.CreateBranch(ctx, project.Id, "feature/protected", "", false); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	repo, err := git.DetectRepository(repoPath)
	if err != nil {
		t.Fatalf("DetectRepository failed: %v", err)
	}
	defer repo.Close()
	if err := repo.CheckoutBranch("feature/protected"); err != nil {
		t.Fatalf("checkout to feature/protected failed: %v", err)
	}
	defer repo.CheckoutBranch(defaultBranch(project))

	if err := branchSvc.DeleteBranch(ctx, project.Id, "feature/protected", false); !errors.Is(err, model.ErrProtectedBranch) {
		t.Fatalf("expected ErrProtectedBranch deleting current branch, got %v", err)
	}

	if err := repo.CheckoutBranch(defaultBranch(project)); err != nil {
		t.Fatalf("checkout back to default failed: %v", err)
	}
}

func TestBranchServiceForceDeleteWithWorktree(t *testing.T) {
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
	defer repo.Close()
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

	repo, err := git.DetectRepository(repoPath)
	if err != nil {
		t.Fatalf("DetectRepository failed: %v", err)
	}
	if err := repo.CheckoutBranch(sourceBranch); err != nil {
		t.Fatalf("checkout source branch failed: %v", err)
	}
	featureFile := filepath.Join(repoPath, "merge.txt")
	if err := os.WriteFile(featureFile, []byte("merge content"), 0o644); err != nil {
		t.Fatalf("write merge file failed: %v", err)
	}
	if err := repo.CommitAll(repoPath, "add merge file"); err != nil {
		t.Fatalf("commit source branch failed: %v", err)
	}
	if err := repo.CheckoutBranch(defaultBranch(project)); err != nil {
		t.Fatalf("checkout target branch failed: %v", err)
	}
	_ = repo.Close()

	worktreeService := NewWorktreeService()
	worktrees, err := worktreeService.ListWorktrees(ctx, project.Id)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	var mainWT *model.Worktree
	for _, wt := range worktrees {
		if wt.BranchName == defaultBranch(project) {
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

func TestBranchServiceSquashMergeUsesSystemGit(t *testing.T) {
	info := git.ProbeSystemGit(t.Context(), true)
	if !info.Available {
		t.Skip("system Git is unavailable")
	}
	previousEngines := git.CurrentEngineSettings()
	git.ConfigureEngines(git.EngineSettings{Read: git.EnginePreferenceBuiltin, Write: git.EnginePreferenceSystem, Executable: info.Executable})
	t.Cleanup(func() { git.ConfigureEngines(previousEngines) })
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

	repo, err := git.DetectRepository(repoPath)
	if err != nil {
		t.Fatalf("DetectRepository failed: %v", err)
	}
	if err := repo.CheckoutBranch(sourceBranch); err != nil {
		t.Fatalf("checkout source branch failed: %v", err)
	}
	featureFile := filepath.Join(repoPath, "squash.txt")
	if err := os.WriteFile(featureFile, []byte("squash content"), 0o644); err != nil {
		t.Fatalf("write squash file failed: %v", err)
	}
	if err := repo.CommitAll(repoPath, "add squash file"); err != nil {
		t.Fatalf("commit source branch failed: %v", err)
	}
	if err := repo.CheckoutBranch(defaultBranch(project)); err != nil {
		t.Fatalf("checkout target branch failed: %v", err)
	}
	_ = repo.Close()

	worktreeService := NewWorktreeService()
	worktrees, err := worktreeService.ListWorktrees(ctx, project.Id)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	var targetWT *model.Worktree
	for _, wt := range worktrees {
		if wt.BranchName == defaultBranch(project) {
			targetWT = wt
			break
		}
	}
	if targetWT == nil {
		t.Fatalf("failed to locate default branch worktree")
	}

	result, err := branchSvc.MergeBranch(ctx, targetWT.Id, sourceBranch, model.MergeBranchOptions{
		TargetBranch:  targetWT.BranchName,
		Strategy:      "squash",
		Commit:        true,
		CommitMessage: "squash feature",
	})
	if err != nil || result == nil || !result.Success {
		t.Fatalf("expected squash merge to succeed, result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(featureFile); err != nil {
		t.Fatalf("squashed file is missing: %v", err)
	}
}

func defaultBranch(project *model.Project) string {
	if project.DefaultBranch == nil {
		return ""
	}
	return *project.DefaultBranch
}
