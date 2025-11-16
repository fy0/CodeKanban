package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"go-template/model"
	"go-template/model/tables"
	"go-template/utils"
	"go-template/utils/cache"
	"go-template/utils/git"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BranchService coordinates git branch operations with persistence.
type BranchService struct {
	cache *cache.Cache
}

// NewBranchService constructs a BranchService with a default ttl cache.
func NewBranchService() *BranchService {
	return &BranchService{
		cache: cache.NewCache(1 * time.Minute),
	}
}

// ListBranches enumerates local/remote branches for a project, marking worktree associations.
func (s *BranchService) ListBranches(ctx context.Context, projectID string, forceRefresh bool) (_ *model.BranchListResult, err error) {
	ctx = ensureContext(ctx)
	logger := s.logger(ctx)
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("project id is required")
	}

	if forceRefresh {
		s.invalidateCache(projectID)
	} else if result := s.getCached(projectID); result != nil {
		logger.Debug("branch list cache hit", zap.String("projectId", projectID))
		return result, nil
	}

	project, err := s.getProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	repo, err := git.DetectRepository(project.Path)
	if err != nil {
		return nil, err
	}

	local, remote, err := repo.ListBranches()
	if err != nil {
		logger.Error("list branches failed", zap.Error(err), zap.String("projectId", projectID))
		return nil, err
	}

	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return nil, err
	}

	var worktrees []tables.WorktreeTable
	if err := dbCtx.Where("project_id = ?", projectID).Find(&worktrees).Error; err != nil {
		return nil, err
	}

	worktreeMap := make(map[string]struct{}, len(worktrees))
	for _, wt := range worktrees {
		worktreeMap[wt.BranchName] = struct{}{}
	}

	for idx := range local {
		if _, ok := worktreeMap[local[idx].Name]; ok {
			local[idx].HasWorktree = true
		}
	}

	result := &model.BranchListResult{
		Local:  local,
		Remote: remote,
	}
	s.setCache(projectID, result)
	return result, nil
}

// CreateBranch provisions a git branch and optionally its worktree.
func (s *BranchService) CreateBranch(ctx context.Context, projectID, name, base string, createWorktree bool) (err error) {
	ctx = ensureContext(ctx)
	logger := s.logger(ctx)

	project, repo, err := s.getProjectAndRepo(ctx, projectID)
	if err != nil {
		return err
	}

	branchName := strings.TrimSpace(name)
	if branchName == "" {
		return fmt.Errorf("branch name is required")
	}
	if err := repo.ValidateBranchName(branchName); err != nil {
		return fmt.Errorf("%w: %v", model.ErrInvalidBranchName, err)
	}

	baseBranch := strings.TrimSpace(base)
	if baseBranch == "" {
		if project.DefaultBranch != nil && *project.DefaultBranch != "" {
			baseBranch = *project.DefaultBranch
		} else {
			baseBranch = "main"
		}
	}

	if err := repo.CreateBranch(branchName, baseBranch); err != nil {
		logger.Error("create branch failed",
			zap.Error(err),
			zap.String("projectId", projectID),
			zap.String("branch", branchName),
			zap.String("base", baseBranch),
		)
		return err
	}

	if createWorktree {
		worktreeService := NewWorktreeService()
		if _, err := worktreeService.CreateWorktree(ctx, projectID, branchName, baseBranch, false); err != nil {
			logger.Error("create worktree for branch failed",
				zap.Error(err),
				zap.String("projectId", projectID),
				zap.String("branch", branchName),
			)
			return err
		}
	}

	s.invalidateCache(projectID)
	logger.Info("branch created",
		zap.String("projectId", projectID),
		zap.String("branch", branchName),
		zap.String("base", baseBranch),
		zap.Bool("worktreeProvisioned", createWorktree),
	)
	return nil
}

// DeleteBranch removes a local branch, optionally cascading to worktrees.
func (s *BranchService) DeleteBranch(ctx context.Context, projectID, name string, force bool) (err error) {
	ctx = ensureContext(ctx)
	logger := s.logger(ctx)

	project, repo, err := s.getProjectAndRepo(ctx, projectID)
	if err != nil {
		return err
	}

	branchName := strings.TrimSpace(name)
	if branchName == "" {
		return fmt.Errorf("branch name is required")
	}
	if project.DefaultBranch != nil {
		defaultBranch := strings.TrimSpace(*project.DefaultBranch)
		if defaultBranch != "" && branchName == defaultBranch {
			logger.Warn("attempted to delete default branch",
				zap.String("projectId", projectID),
				zap.String("branch", branchName),
			)
			return model.ErrProtectedBranch
		}
	}
	if current, currErr := repo.GetCurrentBranch(); currErr == nil && branchName == current {
		logger.Warn("attempted to delete current branch",
			zap.String("projectId", projectID),
			zap.String("branch", branchName),
		)
		return model.ErrProtectedBranch
	}

	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return err
	}

	var worktrees []tables.WorktreeTable
	if err := dbCtx.Where("project_id = ? AND branch_name = ?", projectID, branchName).Find(&worktrees).Error; err != nil {
		return err
	}
	if len(worktrees) > 0 {
		if !force {
			logger.Warn("branch has associated worktrees",
				zap.String("projectId", projectID),
				zap.String("branch", branchName),
				zap.Int("worktreeCount", len(worktrees)),
			)
			return model.ErrBranchHasWorktree
		}
		worktreeService := NewWorktreeService()
		for _, wt := range worktrees {
			if err := worktreeService.DeleteWorktree(ctx, wt.ID, true, false); err != nil && !errors.Is(err, model.ErrWorktreeNotFound) {
				logger.Error("failed to delete worktree before branch removal",
					zap.Error(err),
					zap.String("projectId", projectID),
					zap.String("branch", branchName),
					zap.String("worktreeId", wt.ID),
				)
				return err
			}
			logger.Info("worktree removed before branch deletion",
				zap.String("projectId", projectID),
				zap.String("branch", branchName),
				zap.String("worktreeId", wt.ID),
			)
		}
	}

	if err := repo.DeleteBranch(branchName, force); err != nil {
		logger.Error("delete branch failed",
			zap.Error(err),
			zap.String("projectId", projectID),
			zap.String("branch", branchName),
			zap.Bool("force", force),
		)
		return err
	}

	s.invalidateCache(projectID)
	logger.Info("branch deleted",
		zap.String("projectId", projectID),
		zap.String("branch", branchName),
		zap.Bool("force", force),
	)
	return nil
}

// MergeBranch merges source branch into the selected worktree using the requested strategy.
func (s *BranchService) MergeBranch(ctx context.Context, worktreeID, sourceBranch string, opts model.MergeBranchOptions) (_ *model.MergeResult, err error) {
	ctx = ensureContext(ctx)
	logger := s.logger(ctx)

	source := strings.TrimSpace(sourceBranch)
	if source == "" {
		return nil, fmt.Errorf("source branch is required")
	}

	worktreeService := NewWorktreeService()
	worktree, err := worktreeService.GetWorktree(ctx, worktreeID)
	if err != nil {
		return nil, err
	}

	project, repo, err := s.getProjectAndRepo(ctx, worktree.ProjectId)
	if err != nil {
		return nil, err
	}

	targetBranch := strings.TrimSpace(opts.TargetBranch)
	if targetBranch == "" {
		targetBranch = worktree.BranchName
	}
	if targetBranch == "" {
		return nil, fmt.Errorf("target branch is required")
	}

	strategy := parseMergeStrategy(opts.Strategy)
	if strategy == "" {
		return nil, fmt.Errorf("unsupported merge strategy: %s", opts.Strategy)
	}

	if opts.Commit && strategy != git.MergeStrategySquash {
		return nil, errors.New("commit option is only available for squash merges")
	}

	status, err := git.GetWorktreeStatus(worktree.Path)
	if err != nil {
		return nil, err
	}
	if status.Modified > 0 || status.Staged > 0 || status.Conflicted > 0 {
		logger.Warn("worktree dirty before merge",
			zap.String("projectId", project.Id),
			zap.String("worktreeId", worktree.Id),
			zap.String("path", worktree.Path),
		)
		return nil, model.ErrWorktreeDirty
	}

	if err := repo.MergeBranch(worktree.Path, source, strategy); err != nil {
		if git.IsConflictError(err) {
			conflicts := repo.GetConflictFiles(worktree.Path)
			logger.Warn("merge encountered conflicts",
				zap.String("projectId", project.Id),
				zap.String("worktreeId", worktree.Id),
				zap.String("source", source),
				zap.String("target", targetBranch),
				zap.Strings("conflicts", conflicts),
			)
			return &model.MergeResult{
				Success:   false,
				Conflicts: conflicts,
				Message:   "merge has conflicts",
			}, nil
		}
		logger.Error("merge failed",
			zap.Error(err),
			zap.String("projectId", project.Id),
			zap.String("worktreeId", worktree.Id),
			zap.String("source", source),
			zap.String("strategy", string(strategy)),
		)
		return nil, err
	}

	if opts.Commit {
		commitMessage := strings.TrimSpace(opts.CommitMessage)
		if commitMessage == "" {
			commitMessage = fmt.Sprintf("merge %s into %s", source, targetBranch)
		}
		if err := repo.Commit(worktree.Path, commitMessage); err != nil {
			logger.Error("commit after squash failed",
				zap.Error(err),
				zap.String("projectId", project.Id),
				zap.String("worktreeId", worktree.Id),
				zap.String("source", source),
			)
			return nil, err
		}
		logger.Info("squash merge committed",
			zap.String("projectId", project.Id),
			zap.String("worktreeId", worktree.Id),
			zap.String("source", source),
			zap.String("message", commitMessage),
		)
	}

	s.refreshBranches(ctx, worktreeService, project.Id, targetBranch, source)
	s.invalidateCache(project.Id)
	logger.Info("merge completed",
		zap.String("projectId", project.Id),
		zap.String("worktreeId", worktree.Id),
		zap.String("source", source),
		zap.String("target", targetBranch),
		zap.String("strategy", string(strategy)),
	)

	resultMsg := "merged successfully"
	if opts.Commit {
		resultMsg = "merged and committed successfully"
	}
	return &model.MergeResult{
		Success: true,
		Message: resultMsg,
	}, nil
}

func (s *BranchService) refreshBranches(ctx context.Context, worktreeService *WorktreeService, projectID string, branches ...string) {
	if worktreeService == nil {
		return
	}
	set := make(map[string]struct{}, len(branches))
	for _, branch := range branches {
		if trimmed := strings.TrimSpace(branch); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	if len(set) == 0 {
		return
	}

	values := make([]string, 0, len(set))
	for key := range set {
		values = append(values, key)
	}

	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		s.logger(ctx).Warn("refresh branch worktrees failed to open db", zap.Error(err))
		return
	}

	var worktrees []tables.WorktreeTable
	if err := dbCtx.Where("project_id = ? AND branch_name IN ?", projectID, values).Find(&worktrees).Error; err != nil {
		s.logger(ctx).Warn("refresh branch worktrees query failed", zap.Error(err))
		return
	}
	for _, wt := range worktrees {
		go worktreeService.RefreshWorktreeStatus(context.Background(), wt.ID)
	}
}

func (s *BranchService) getProject(ctx context.Context, projectID string) (*model.Project, error) {
	q, err := model.ResolveQueries(nil)
	if err != nil {
		return nil, err
	}
	project, err := q.ProjectGetByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrProjectNotFound
		}
		return nil, err
	}
	return project, nil
}

func (s *BranchService) getProjectAndRepo(ctx context.Context, projectID string) (*model.Project, *git.GitRepo, error) {
	project, err := s.getProject(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}

	repo, err := git.DetectRepository(project.Path)
	if err != nil {
		return nil, nil, err
	}

	return project, repo, nil
}

func (s *BranchService) dbWithContext(ctx context.Context) (*gorm.DB, error) {
	db := model.GetDB()
	if db == nil {
		return nil, model.ErrDBNotInitialized
	}
	return db.WithContext(ensureContext(ctx)), nil
}

func (s *BranchService) cacheKey(projectID string) string {
	return fmt.Sprintf("branch:%s", strings.TrimSpace(projectID))
}

func (s *BranchService) getCached(projectID string) *model.BranchListResult {
	if projectID == "" || s.cache == nil {
		return nil
	}
	if value, ok := s.cache.Get(s.cacheKey(projectID)); ok {
		if result, ok := value.(*model.BranchListResult); ok {
			return result
		}
	}
	return nil
}

func (s *BranchService) setCache(projectID string, result *model.BranchListResult) {
	if projectID == "" || result == nil || s.cache == nil {
		return
	}
	s.cache.Set(s.cacheKey(projectID), result)
}

func (s *BranchService) invalidateCache(projectID string) {
	if projectID == "" || s.cache == nil {
		return
	}
	s.cache.Delete(s.cacheKey(projectID))
}

func (s *BranchService) logger(ctx context.Context) *zap.Logger {
	return utils.LoggerFromContext(ctx).Named("branch-service")
}

func parseMergeStrategy(strategy string) git.MergeStrategy {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "", "merge":
		return git.MergeStrategyMerge
	case "rebase":
		return git.MergeStrategyRebase
	case "squash":
		return git.MergeStrategySquash
	default:
		return ""
	}
}
