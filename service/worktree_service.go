package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/utils"
	"code-kanban/utils/git"

	"go.uber.org/zap"
)

// WorktreeService coordinates CRUD operations between git worktrees and the database.
type WorktreeService struct {
	asyncStatusRefresh bool
}

// NewWorktreeService builds a WorktreeService with async status refresh enabled.
func NewWorktreeService() *WorktreeService {
	return &WorktreeService{
		asyncStatusRefresh: true,
	}
}

// AsyncRefresh toggles async status refresh behaviour (useful for tests).
func (s *WorktreeService) AsyncRefresh(enabled bool) {
	if s == nil {
		return
	}
	s.asyncStatusRefresh = enabled
}

// CreateWorktree provisions a new git worktree and persists its metadata.
func (s *WorktreeService) CreateWorktree(
	ctx context.Context,
	projectID string,
	branchName string,
	baseBranch string,
	createBranch bool,
) (*model.Worktree, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	q, err := model.ResolveQueries(nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("project id is required")
	}
	if strings.TrimSpace(branchName) == "" {
		return nil, fmt.Errorf("branch name is required")
	}

	project, err := q.ProjectGetByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrWorktreeNotFound
		}
		return nil, err
	}

	gitRepo, err := git.DetectRepository(project.Path)
	if err != nil {
		return nil, err
	}

	targetBranch := strings.TrimSpace(branchName)
	if createBranch {
		refBranch := strings.TrimSpace(baseBranch)
		if refBranch == "" {
			if project.DefaultBranch != nil && *project.DefaultBranch != "" {
				refBranch = *project.DefaultBranch
			} else {
				refBranch = "main"
			}
		}
		if err := gitRepo.CreateBranch(targetBranch, refBranch); err != nil {
			return nil, err
		}
	}

	worktreePath, err := s.resolveWorktreePath(project, targetBranch)
	if err != nil {
		return nil, err
	}

	if err := gitRepo.AddWorktree(worktreePath, targetBranch, false); err != nil {
		return nil, err
	}

	now := time.Now()
	idVal := utils.NewID()
	zeroVal := int64(0)
	worktree, err := q.WorktreeCreate(ctx, &model.WorktreeCreateParams{
		Id:              idVal,
		CreatedAt:       now,
		UpdatedAt:       now,
		ProjectId:       projectID,
		BranchName:      targetBranch,
		Path:            worktreePath,
		IsMain:          false,
		IsBare:          false,
		HeadCommit:      nil,
		StatusAhead:     &zeroVal,
		StatusBehind:    &zeroVal,
		StatusModified:  &zeroVal,
		StatusStaged:    &zeroVal,
		StatusUntracked: &zeroVal,
		StatusConflicts: &zeroVal,
		StatusUpdatedAt: nil,
	})
	if err != nil {
		_ = gitRepo.RemoveWorktree(worktreePath, true)
		return nil, err
	}

	// 同步刷新状态，确保返回的 worktree 包含最新的 git 状态信息
	refreshed, err := s.RefreshWorktreeStatus(ctx, worktree.Id)
	if err != nil {
		// 如果刷新状态失败，记录警告但不影响创建流程
		utils.Logger().Warn("failed to refresh worktree status after creation",
			zap.Error(err),
			zap.String("worktreeId", worktree.Id),
		)
		return worktree, nil
	}

	return refreshed, nil
}

// ListWorktrees returns worktrees for a project ordered by main flag then creation.
func (s *WorktreeService) ListWorktrees(ctx context.Context, projectID string) ([]*model.Worktree, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	q, err := model.ResolveQueries(nil)
	if err != nil {
		return nil, err
	}

	return q.WorktreeListByProject(ctx, projectID)
}

// GetWorktree fetches a worktree by identifier.
func (s *WorktreeService) GetWorktree(ctx context.Context, id string) (*model.Worktree, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	q, err := model.ResolveQueries(nil)
	if err != nil {
		return nil, err
	}

	wt, err := q.WorktreeGetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrWorktreeNotFound
		}
		return nil, err
	}
	return wt, nil
}

// DeleteWorktree removes a worktree from git and the database.
func (s *WorktreeService) DeleteWorktree(ctx context.Context, id string, force, deleteBranch bool) error {
	if ctx == nil {
		ctx = context.Background()
	}

	q, err := model.ResolveQueries(nil)
	if err != nil {
		return err
	}

	worktree, err := s.GetWorktree(ctx, id)
	if err != nil {
		return err
	}
	if worktree.IsMain {
		return model.ErrWorktreeIsMain
	}

	worktreeID := worktree.Id
	taskCount, err := q.TaskCountByWorktree(ctx, &worktreeID)
	if err != nil {
		return err
	}
	if taskCount > 0 && !force {
		return model.ErrWorktreeHasTasks
	}

	project, err := q.ProjectGetByID(ctx, worktree.ProjectId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ErrWorktreeNotFound
		}
		return err
	}

	// Check if project path exists
	var gitRepo *git.GitRepo
	if _, err := os.Stat(project.Path); os.IsNotExist(err) {
		utils.Logger().Warn("project path does not exist, skipping git operations",
			zap.String("projectPath", project.Path),
			zap.String("worktreeId", id),
		)
		// Skip git operations and proceed to database cleanup
	} else {
		// Project exists, try git operations
		gitRepo, err = git.DetectRepository(project.Path)
		if err != nil {
			utils.Logger().Warn("failed to detect git repository, skipping git removal",
				zap.Error(err),
				zap.String("projectPath", project.Path),
				zap.String("worktreeId", id),
			)
		} else {
			// Try to remove the worktree from git
			if err := gitRepo.RemoveWorktree(worktree.Path, force); err != nil {
				// If the worktree path doesn't exist anymore, we can still proceed
				// Check if the error is because the worktree doesn't exist
				if _, statErr := os.Stat(worktree.Path); os.IsNotExist(statErr) {
					utils.Logger().Warn("worktree path does not exist, skipping git removal",
						zap.String("path", worktree.Path),
						zap.String("worktreeId", id),
					)
				} else {
					// For other errors, return them
					return err
				}
			}
		}
	}

	if deleteBranch && gitRepo != nil {
		if err := gitRepo.DeleteBranch(worktree.BranchName, force); err != nil {
			utils.Logger().Warn("failed to delete branch",
				zap.Error(err),
				zap.String("branch", worktree.BranchName),
				zap.String("projectId", project.Id),
			)
		}
	}

	now := time.Now()
	_, err = q.WorktreeSoftDelete(ctx, &model.WorktreeSoftDeleteParams{
		DeletedAt: &now,
		UpdatedAt: now,
		Id:        id,
	})
	return err
}

// RefreshWorktreeStatus updates cached status fields for a worktree and returns the refreshed record.
func (s *WorktreeService) RefreshWorktreeStatus(ctx context.Context, id string) (*model.Worktree, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	q, err := model.ResolveQueries(nil)
	if err != nil {
		return nil, err
	}

	worktree, err := s.GetWorktree(ctx, id)
	if err != nil {
		return nil, err
	}

	status, err := git.GetWorktreeStatus(worktree.Path)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var headPtr *string
	var headDatePtr *time.Time
	if status.LastCommit != nil {
		head := status.LastCommit.SHA
		headPtr = &head
		if !status.LastCommit.Date.IsZero() {
			headDate := status.LastCommit.Date.UTC()
			headDatePtr = &headDate
		}
	}

	aheadVal := int64(status.Ahead)
	behindVal := int64(status.Behind)
	modifiedVal := int64(status.Modified)
	stagedVal := int64(status.Staged)
	untrackedVal := int64(status.Untracked)
	conflictsVal := int64(status.Conflicted)

	updated, err := q.WorktreeUpdateStatus(ctx, &model.WorktreeUpdateStatusParams{
		UpdatedAt:       now,
		StatusAhead:     &aheadVal,
		StatusBehind:    &behindVal,
		StatusModified:  &modifiedVal,
		StatusStaged:    &stagedVal,
		StatusUntracked: &untrackedVal,
		StatusConflicts: &conflictsVal,
		StatusUpdatedAt: &now,
		HeadCommit:      headPtr,
		HeadCommitDate:  headDatePtr,
		Id:              worktree.Id,
	})
	if err != nil {
		return nil, err
	}

	return updated, nil
}

// RefreshAllWorktrees refreshes status for every worktree belonging to a project.
func (s *WorktreeService) RefreshAllWorktrees(ctx context.Context, projectID string) (updated, failed int, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	worktrees, err := s.ListWorktrees(ctx, projectID)
	if err != nil {
		return 0, 0, err
	}

	for _, wt := range worktrees {
		if _, err := s.RefreshWorktreeStatus(ctx, wt.Id); err != nil {
			failed++
			utils.Logger().Warn("failed to refresh worktree status",
				zap.Error(err),
				zap.String("worktreeId", wt.Id),
				zap.String("projectId", projectID),
			)
		} else {
			updated++
		}
	}
	return updated, failed, nil
}

// SyncWorktrees ensures git worktrees and the database remain aligned.
func (s *WorktreeService) SyncWorktrees(ctx context.Context, projectID string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	q, err := model.ResolveQueries(nil)
	if err != nil {
		return err
	}

	project, err := q.ProjectGetByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ErrWorktreeNotFound
		}
		return err
	}

	// 如果不是 git 仓库，直接返回成功（非 git 目录没有 worktrees 需要同步）
	gitRepo, err := git.DetectRepository(project.Path)
	if err != nil {
		utils.Logger().Debug("project is not a git repository, skip worktree sync",
			zap.String("projectId", projectID),
			zap.String("path", project.Path),
			zap.Error(err),
		)
		return nil
	}

	gitWorktrees, err := gitRepo.ListWorktrees()
	if err != nil {
		return err
	}

	dbWorktrees, err := s.ListWorktrees(ctx, projectID)
	if err != nil {
		return err
	}

	gitByPath := make(map[string]git.WorktreeInfo, len(gitWorktrees))
	for _, wt := range gitWorktrees {
		gitByPath[model.NormalizePathCase(wt.Path)] = wt
	}

	dbByPath := make(map[string]*model.Worktree, len(dbWorktrees))
	for _, wt := range dbWorktrees {
		dbByPath[model.NormalizePathCase(wt.Path)] = wt
	}

	now := time.Now()
	for normPath, gitWT := range gitByPath {
		if existing, ok := dbByPath[normPath]; ok {
			var headPtr *string
			if gitWT.HeadCommit != "" {
				commit := gitWT.HeadCommit
				headPtr = &commit
			}
			if err := q.WorktreeUpdateMetadata(ctx, &model.WorktreeUpdateMetadataParams{
				UpdatedAt:  now,
				BranchName: gitWT.Branch,
				HeadCommit: headPtr,
				IsMain:     gitWT.IsMain,
				IsBare:     gitWT.IsBare,
				Id:         existing.Id,
			}); err != nil {
				return err
			}
			continue
		}

		var headPtr *string
		if gitWT.HeadCommit != "" {
			commit := gitWT.HeadCommit
			headPtr = &commit
		}
		zeroVal := int64(0)
		if _, err := q.WorktreeCreate(ctx, &model.WorktreeCreateParams{
			Id:              utils.NewID(),
			CreatedAt:       now,
			UpdatedAt:       now,
			ProjectId:       projectID,
			BranchName:      gitWT.Branch,
			Path:            filepath.Clean(gitWT.Path),
			IsMain:          gitWT.IsMain,
			IsBare:          gitWT.IsBare,
			HeadCommit:      headPtr,
			StatusAhead:     &zeroVal,
			StatusBehind:    &zeroVal,
			StatusModified:  &zeroVal,
			StatusStaged:    &zeroVal,
			StatusUntracked: &zeroVal,
			StatusConflicts: &zeroVal,
			StatusUpdatedAt: nil,
		}); err != nil {
			return err
		}
	}

	for normPath, dbWT := range dbByPath {
		if _, ok := gitByPath[normPath]; ok {
			continue
		}
		if _, err := q.WorktreeSoftDelete(ctx, &model.WorktreeSoftDeleteParams{
			DeletedAt: &now,
			UpdatedAt: now,
			Id:        dbWT.Id,
		}); err != nil {
			return err
		}
	}

	return nil
}

// CommitWorktree stages all changes within the worktree and creates a commit with the provided message.
func (s *WorktreeService) CommitWorktree(ctx context.Context, id, message string) (*model.Worktree, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	trimmedMessage := strings.TrimSpace(message)
	if trimmedMessage == "" {
		return nil, fmt.Errorf("commit message is required")
	}

	q, err := model.ResolveQueries(nil)
	if err != nil {
		return nil, err
	}

	worktree, err := s.GetWorktree(ctx, id)
	if err != nil {
		return nil, err
	}

	project, err := q.ProjectGetByID(ctx, worktree.ProjectId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrProjectNotFound
		}
		return nil, err
	}

	repo, err := git.DetectRepository(project.Path)
	if err != nil {
		return nil, err
	}

	status, err := repo.GetWorktreeStatus(worktree.Path)
	if err != nil {
		return nil, err
	}
	if status.Modified == 0 && status.Staged == 0 && status.Untracked == 0 {
		return nil, model.ErrWorktreeClean
	}

	if err := repo.AddAll(worktree.Path); err != nil {
		return nil, err
	}
	if err := repo.Commit(worktree.Path, trimmedMessage); err != nil {
		if strings.Contains(err.Error(), "nothing to commit") {
			return nil, model.ErrWorktreeClean
		}
		return nil, err
	}

	updated, err := s.RefreshWorktreeStatus(ctx, id)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *WorktreeService) resolveWorktreePath(project *model.Project, branchName string) (string, error) {
	basePath := ""
	if project.WorktreeBasePath != nil && strings.TrimSpace(*project.WorktreeBasePath) != "" {
		basePath = *project.WorktreeBasePath
	} else {
		basePath = filepath.Join(project.Path, ".worktrees")
	}
	if !filepath.IsAbs(basePath) {
		basePath = filepath.Join(project.Path, basePath)
	}
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return "", err
	}

	dirName := sanitizeBranchName(branchName)
	return filepath.Join(basePath, dirName), nil
}

func sanitizeBranchName(branch string) string {
	replacer := strings.NewReplacer(
		"/", "__",
		"\\", "__",
		":", "_",
		"*", "_",
		"?", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(strings.TrimSpace(branch))
}
