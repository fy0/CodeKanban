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

// WorktreeService 协调 git worktree 与数据库之间的 CRUD 操作。
type WorktreeService struct {
	asyncStatusRefresh bool
}

// CreateWorktreeOptions 创建 Worktree 时的选项参数。
type CreateWorktreeOptions struct {
	BaseBranch            string // 基础分支（新建分支时的起始点）
	CreateBranch          bool   // 是否创建新分支
	Location              string // 创建位置："project"（项目目录）或 "global"（全局目录）
	GlobalBaseDirOverride string // 全局目录覆盖（仅本次生效，不持久化）
	GlobalBaseDir         string // 全局 Worktree 基础目录（来自配置）
	GlobalDirNamePattern  string // 全局目录命名模式（如 {projectName}-{branch}）
}

// NewWorktreeService 创建一个启用异步状态刷新的 WorktreeService 实例。
func NewWorktreeService() *WorktreeService {
	return &WorktreeService{
		asyncStatusRefresh: true,
	}
}

// AsyncRefresh 切换异步状态刷新行为（用于测试）。
func (s *WorktreeService) AsyncRefresh(enabled bool) {
	if s == nil {
		return
	}
	s.asyncStatusRefresh = enabled
}

// CreateWorktree 创建一个新的 git worktree 并持久化其元数据。
func (s *WorktreeService) CreateWorktree(
	ctx context.Context,
	projectID string,
	branchName string,
	opts CreateWorktreeOptions,
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
	if opts.CreateBranch {
		refBranch := strings.TrimSpace(opts.BaseBranch)
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

	worktreePath, baseDirToPersist, persistRequested, err := s.resolveWorktreePath(project, targetBranch, opts)
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

	if persistRequested {
		updatedAt := time.Now()
		var basePathParam *string
		if strings.TrimSpace(baseDirToPersist) != "" {
			cleaned := filepath.Clean(baseDirToPersist)
			basePathParam = &cleaned
		}

		if _, err := q.ProjectUpdateWorktreeBasePath(ctx, &model.ProjectUpdateWorktreeBasePathParams{
			UpdatedAt:        updatedAt,
			WorktreeBasePath: basePathParam,
			Id:               projectID,
		}); err != nil {
			_ = gitRepo.RemoveWorktree(worktreePath, true)
			_, _ = q.WorktreeSoftDelete(ctx, &model.WorktreeSoftDeleteParams{
				DeletedAt: &updatedAt,
				UpdatedAt: updatedAt,
				Id:        worktree.Id,
			})
			return nil, err
		}
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

// ListWorktrees 返回项目的所有 worktree，按主 worktree 标志和创建时间排序。
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

// GetWorktree 根据 ID 获取 worktree 记录。
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

// DeleteWorktree 从 git 和数据库中删除 worktree。
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

	// 检查项目路径是否存在
	var gitRepo *git.GitRepo
	if _, err := os.Stat(project.Path); os.IsNotExist(err) {
		utils.Logger().Warn("project path does not exist, skipping git operations",
			zap.String("projectPath", project.Path),
			zap.String("worktreeId", id),
		)
		// 跳过 git 操作，继续进行数据库清理
	} else {
		// 项目存在，尝试 git 操作
		gitRepo, err = git.DetectRepository(project.Path)
		if err != nil {
			utils.Logger().Warn("failed to detect git repository, skipping git removal",
				zap.Error(err),
				zap.String("projectPath", project.Path),
				zap.String("worktreeId", id),
			)
		} else {
			// 尝试从 git 中移除 worktree
			if err := gitRepo.RemoveWorktree(worktree.Path, force); err != nil {
				// 如果 worktree 路径已不存在，可以继续处理
				// 检查错误是否因为 worktree 不存在
				if _, statErr := os.Stat(worktree.Path); os.IsNotExist(statErr) {
					utils.Logger().Warn("worktree path does not exist, skipping git removal",
						zap.String("path", worktree.Path),
						zap.String("worktreeId", id),
					)
				} else {
					// 其他错误则返回
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

// RefreshWorktreeStatus 更新 worktree 的缓存状态字段并返回刷新后的记录。
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
	var headMessagePtr *string
	var headDatePtr *time.Time
	if status.LastCommit != nil {
		head := status.LastCommit.SHA
		headPtr = &head
		message := strings.TrimSpace(status.LastCommit.Message)
		if message != "" {
			headMessagePtr = &message
		}
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
		UpdatedAt:         now,
		StatusAhead:       &aheadVal,
		StatusBehind:      &behindVal,
		StatusModified:    &modifiedVal,
		StatusStaged:      &stagedVal,
		StatusUntracked:   &untrackedVal,
		StatusConflicts:   &conflictsVal,
		StatusUpdatedAt:   &now,
		HeadCommit:        headPtr,
		HeadCommitMessage: headMessagePtr,
		HeadCommitDate:    headDatePtr,
		Id:                worktree.Id,
	})
	if err != nil {
		return nil, err
	}

	return updated, nil
}

// RefreshAllWorktrees 刷新项目下所有 worktree 的状态。
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

// RefreshWorktreeCommitInfo 刷新所有 worktree 的提交/状态元数据并返回更新后的列表。
func (s *WorktreeService) RefreshWorktreeCommitInfo(ctx context.Context, projectID string) ([]*model.Worktree, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, _, err := s.RefreshAllWorktrees(ctx, projectID); err != nil {
		return nil, err
	}
	return s.ListWorktrees(ctx, projectID)
}

// SyncWorktrees 确保 git worktree 与数据库保持同步。
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

			branchName := strings.TrimSpace(gitWT.Branch)
			if branchName == "" {
				branchName = existing.BranchName
			}

			if err := q.WorktreeUpdateMetadata(ctx, &model.WorktreeUpdateMetadataParams{
				UpdatedAt:  now,
				BranchName: branchName,
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

// CommitWorktree 暂存 worktree 中的所有更改并使用指定消息创建提交。
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

// resolveWorktreePath 根据选项解析 worktree 的完整路径。
// 返回值：
//   - worktreePath: 最终的 worktree 目录路径
//   - baseDirToPersist: 需要持久化到项目的基础目录（仅当使用全局配置时）
//   - persistRequested: 是否需要持久化基础目录到项目
//   - err: 错误信息
func (s *WorktreeService) resolveWorktreePath(project *model.Project, branchName string, opts CreateWorktreeOptions) (worktreePath string, baseDirToPersist string, persistRequested bool, err error) {
	if project == nil {
		return "", "", false, fmt.Errorf("project is required")
	}

	location := strings.TrimSpace(opts.Location)
	if location != "" && location != "project" && location != "global" {
		return "", "", false, fmt.Errorf("invalid location: %s", location)
	}

	pattern := strings.TrimSpace(opts.GlobalDirNamePattern)
	if pattern == "" {
		pattern = "{projectName}-{branch}"
	}

	baseDir := ""
	globalMode := false
	persistRequested = location != ""

	switch location {
	case "project":
		baseDir = filepath.Join(project.Path, ".worktrees")
		globalMode = false
		baseDirToPersist = ""
	case "global":
		// 优先检查覆盖参数（仅本次生效，不持久化）
		overrideDir := strings.TrimSpace(opts.GlobalBaseDirOverride)
		configDir := strings.TrimSpace(opts.GlobalBaseDir)

		if overrideDir != "" {
			baseDir = overrideDir
			// 覆盖参数仅本次生效，不持久化到项目
			baseDirToPersist = ""
			persistRequested = false
		} else if configDir != "" {
			baseDir = configDir
			// 使用全局配置，持久化到项目以便后续使用
			baseDirToPersist = filepath.Clean(configDir)
		} else {
			return "", "", false, fmt.Errorf("global base dir is not configured")
		}

		if !filepath.IsAbs(baseDir) {
			return "", "", false, fmt.Errorf("global base dir must be an absolute path")
		}
		// 安全检查：全局基础目录不能是敏感系统目录
		if utils.IsSensitiveSystemDir(baseDir) {
			return "", "", false, fmt.Errorf("global base dir cannot be a system directory")
		}
		globalMode = true
	default:
		if project.WorktreeBasePath != nil && strings.TrimSpace(*project.WorktreeBasePath) != "" {
			baseDir = strings.TrimSpace(*project.WorktreeBasePath)
		} else {
			baseDir = filepath.Join(project.Path, ".worktrees")
		}

		if !filepath.IsAbs(baseDir) {
			baseDir = filepath.Join(project.Path, baseDir)
		}

		// 安全检查：确保 baseDir 不会通过 ".." 逃逸出项目目录
		absBase := filepath.Clean(baseDir)
		absProject := filepath.Clean(project.Path)
		rel, relErr := filepath.Rel(absProject, absBase)
		if relErr == nil && strings.HasPrefix(rel, "..") {
			// baseDir 逃逸出项目目录 - 仅当是绝对路径时允许
			// 对于包含 ".." 的相对路径，拒绝作为安全风险
			if project.WorktreeBasePath != nil && !filepath.IsAbs(*project.WorktreeBasePath) {
				return "", "", false, fmt.Errorf("worktree base path escapes project directory")
			}
		}

		globalMode = isGlobalWorktreeBaseDir(project.Path, baseDir)
		baseDirToPersist = ""
	}

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", "", false, err
	}

	dirName := ""
	if globalMode {
		dirName, err = expandWorktreeDirNamePattern(pattern, project, branchName)
		if err != nil {
			return "", "", false, err
		}
	} else {
		dirName = sanitizeBranchName(branchName)
	}

	// 最终安全校验：确保解析后的路径在 baseDir 内
	finalPath := filepath.Join(baseDir, dirName)
	cleanFinal := filepath.Clean(finalPath)
	cleanBase := filepath.Clean(baseDir)
	if !strings.HasPrefix(cleanFinal, cleanBase+string(filepath.Separator)) && cleanFinal != cleanBase {
		return "", "", false, fmt.Errorf("worktree path escapes base directory")
	}

	return finalPath, baseDirToPersist, persistRequested, nil
}

// sanitizeBranchName 将分支名称转换为安全的目录名称。
// 替换路径分隔符和特殊字符，防止路径遍历攻击。
func sanitizeBranchName(branch string) string {
	clean := strings.TrimSpace(branch)
	// 拒绝可能导致路径遍历的危险目录名
	if clean == "" || clean == "." || clean == ".." {
		return "_invalid_branch_"
	}

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
	result := replacer.Replace(clean)

	// 二次校验：如果结果仍包含 ".." 则拒绝
	if strings.Contains(result, "..") {
		return "_invalid_branch_"
	}
	return result
}

// isGlobalWorktreeBaseDir 判断 worktree 基础目录是否在项目目录外（即全局模式）。
func isGlobalWorktreeBaseDir(projectPath, baseDir string) bool {
	projectAbs, err := filepath.Abs(projectPath)
	if err != nil {
		return false
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(projectAbs, baseAbs)
	if err != nil {
		return false
	}
	if rel == "." {
		return false
	}
	return strings.HasPrefix(rel, "..")
}

// sanitizePathSegment 清理路径片段中的特殊字符。
func sanitizePathSegment(input string) string {
	trimmed := strings.TrimSpace(input)
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(trimmed)
}

// expandWorktreeDirNamePattern 展开 worktree 目录名模式。
// 支持的变量：{projectName}、{projectId}、{branch}
func expandWorktreeDirNamePattern(pattern string, project *model.Project, branchName string) (string, error) {
	rawProjectName := ""
	if project != nil {
		rawProjectName = project.Name
	}

	// 使用固定顺序替换以避免非确定性行为
	expanded := pattern
	expanded = strings.ReplaceAll(expanded, "{projectName}", sanitizePathSegment(rawProjectName))
	expanded = strings.ReplaceAll(expanded, "{projectId}", sanitizePathSegment(project.Id))
	expanded = strings.ReplaceAll(expanded, "{branch}", sanitizeBranchName(branchName))

	expanded = strings.TrimSpace(expanded)
	if expanded == "" {
		return "", fmt.Errorf("worktree dir name is empty after pattern expansion")
	}
	if strings.Contains(expanded, "..") {
		return "", fmt.Errorf("invalid worktree dir name: %s", expanded)
	}
	if strings.ContainsAny(expanded, "/\\") {
		return "", fmt.Errorf("invalid worktree dir name: %s", expanded)
	}

	return expanded, nil
}
