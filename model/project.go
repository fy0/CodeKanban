package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"code-kanban/utils"
	"code-kanban/utils/git"

	"go.uber.org/zap"
)

var (
	// ErrInvalidProjectInput indicates required fields are missing.
	ErrInvalidProjectInput = errors.New("project name and path are required")
	// ErrInvalidProjectPath indicates the provided path is not accessible.
	ErrInvalidProjectPath = errors.New("project path is invalid")
	// ErrInvalidGitRepository indicates the provided path is not a git repository.
	ErrInvalidGitRepository = errors.New("not a valid git repository")
	// ErrProjectAlreadyExists indicates the path is already tracked.
	ErrProjectAlreadyExists = errors.New("project already exists")
	// ErrProjectNotFound indicates the requested project does not exist.
	ErrProjectNotFound = errors.New("project not found")
)

// CreateProjectParams contains inputs for creating a project record.
type CreateProjectParams struct {
	Name             string
	Path             string
	Description      string
	WorktreeBasePath string
	HidePath         bool
}

// UpdateProjectParams contains inputs for editing project metadata.
type UpdateProjectParams struct {
	Name        string
	Description string
	HidePath    bool
}

// ProjectService wraps project level behaviours.
type ProjectService struct {
	asyncWorktreeSync bool
}

// NewProjectService constructs a service with async worktree sync enabled.
func NewProjectService() *ProjectService {
	return &ProjectService{asyncWorktreeSync: true}
}

// CreateProject validates and persists a project record from filesystem metadata.
func (s *ProjectService) CreateProject(ctx context.Context, params CreateProjectParams) (*Project, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	q, err := resolveQueries(nil)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(params.Name)
	path := strings.TrimSpace(params.Path)
	if name == "" || path == "" {
		return nil, ErrInvalidProjectInput
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidProjectPath, err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidProjectPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: not a directory", ErrInvalidProjectPath)
	}
	cleanPath := filepath.Clean(absPath)

	var gitRepo *git.GitRepo
	if repo, detectErr := git.DetectRepository(cleanPath); detectErr == nil {
		gitRepo = repo
	} else {
		utils.Logger().Debug("project path is not a git repository",
			zap.String("path", cleanPath),
			zap.Error(detectErr),
		)
	}

	defaultBranch := ""
	if gitRepo != nil {
		if branch, branchErr := gitRepo.GetCurrentBranch(); branchErr == nil && strings.TrimSpace(branch) != "" {
			defaultBranch = branch
		}
	}
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	var remoteURLPtr *string
	if gitRepo != nil {
		remotes, remotesErr := gitRepo.GetRemotes()
		if remotesErr == nil && len(remotes) > 0 && strings.TrimSpace(remotes[0].URL) != "" {
			val := remotes[0].URL
			remoteURLPtr = &val
		}
	}

	worktreeBase := strings.TrimSpace(params.WorktreeBasePath)
	if worktreeBase == "" {
		if gitRepo != nil {
			worktreeBase = filepath.Join(gitRepo.Path, ".worktrees")
		} else {
			worktreeBase = filepath.Join(cleanPath, ".worktrees")
		}
	} else {
		worktreeBase = filepath.Clean(worktreeBase)
	}
	worktreeBasePointer := &worktreeBase

	description := strings.TrimSpace(params.Description)
	var descriptionPtr *string
	if description != "" {
		descriptionPtr = &description
	}

	now := time.Now()
	idVal := utils.NewID()
	project, err := q.ProjectCreate(ctx, &ProjectCreateParams{
		Id:               idVal,
		CreatedAt:        now,
		UpdatedAt:        now,
		Name:             name,
		Path:             cleanPath,
		Description:      descriptionPtr,
		DefaultBranch:    defaultBranch,
		WorktreeBasePath: worktreeBasePointer,
		RemoteUrl:        remoteURLPtr,
		HidePath:         params.HidePath,
		LastSyncAt:       nil,
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			return nil, ErrProjectAlreadyExists
		}
		return nil, err
	}

	// 如果是 git 仓库，同步 worktrees；否则创建一个虚拟的 main worktree
	if gitRepo != nil {
		s.dispatchWorktreeSync(ctx, project.Id, gitRepo)
	} else {
		// 为非 git 目录创建虚拟 worktree，使终端等功能可用
		s.createVirtualMainWorktree(ctx, project.Id, cleanPath, defaultBranch)
	}
	return project, nil
}

// GetProject loads a project by identifier.
func (s *ProjectService) GetProject(ctx context.Context, id string) (*Project, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	q, err := resolveQueries(nil)
	if err != nil {
		return nil, err
	}

	project, err := q.ProjectGetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}
	return project, nil
}

// ListProjects returns all projects ordered by creation timestamp descending.
func (s *ProjectService) ListProjects(ctx context.Context) ([]*Project, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	q, err := resolveQueries(nil)
	if err != nil {
		return nil, err
	}

	return q.ProjectList(ctx)
}

// DeleteProject removes a project and cascades to related entities.
func (s *ProjectService) DeleteProject(ctx context.Context, id string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	q, err := resolveQueries(nil)
	if err != nil {
		return err
	}

	now := time.Now()
	affected, err := q.ProjectSoftDelete(ctx, &ProjectSoftDeleteParams{
		DeletedAt: &now,
		UpdatedAt: now,
		Id:        id,
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrProjectNotFound
	}
	return nil
}

// UpdateProject modifies project metadata such as name and description.
func (s *ProjectService) UpdateProject(ctx context.Context, id string, params UpdateProjectParams) (*Project, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	q, err := resolveQueries(nil)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(params.Name)
	if name == "" {
		return nil, ErrInvalidProjectInput
	}

	description := strings.TrimSpace(params.Description)
	var descriptionPtr *string
	if description != "" {
		descriptionPtr = &description
	}

	project, err := q.ProjectUpdate(ctx, &ProjectUpdateParams{
		UpdatedAt:   time.Now(),
		Name:        name,
		Description: descriptionPtr,
		HidePath:    params.HidePath,
		Id:          id,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}

	return project, nil
}

func (s *ProjectService) dispatchWorktreeSync(ctx context.Context, projectID string, repo *git.GitRepo) {
	if repo == nil {
		return
	}

	if ctx == nil {
		ctx = context.Background()
	}

	if s != nil && s.asyncWorktreeSync {
		go s.syncWorktrees(context.Background(), projectID, repo)
	} else {
		s.syncWorktrees(ctx, projectID, repo)
	}
}

func (s *ProjectService) syncWorktrees(ctx context.Context, projectID string, repo *git.GitRepo) {
	if repo == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	q, err := resolveQueries(nil)
	if err != nil {
		return
	}

	logger := utils.Logger()
	gitWorktrees, err := repo.ListWorktrees()
	if err != nil {
		logger.Warn("failed to list worktrees", zap.Error(err))
		return
	}

	dbWorktrees, err := q.WorktreeListByProject(ctx, projectID)
	if err != nil {
		logger.Warn("failed to list worktrees from database", zap.Error(err))
		return
	}

	gitByPath := make(map[string]git.WorktreeInfo, len(gitWorktrees))
	for _, wt := range gitWorktrees {
		gitByPath[NormalizePathCase(wt.Path)] = wt
	}

	dbByPath := make(map[string]*Worktree, len(dbWorktrees))
	for _, wt := range dbWorktrees {
		dbByPath[NormalizePathCase(wt.Path)] = wt
	}

	now := time.Now()
	for normPath, gitWT := range gitByPath {
		if existing, ok := dbByPath[normPath]; ok {
			var headPtr *string
			if gitWT.HeadCommit != "" {
				commit := gitWT.HeadCommit
				headPtr = &commit
			}
			err := q.WorktreeUpdateMetadata(ctx, &WorktreeUpdateMetadataParams{
				UpdatedAt:  now,
				BranchName: gitWT.Branch,
				HeadCommit: headPtr,
				IsMain:     gitWT.IsMain,
				IsBare:     gitWT.IsBare,
				Id:         existing.Id,
			})
			if err != nil {
				logger.Warn("failed to update worktree metadata",
					zap.Error(err),
					zap.String("projectId", projectID),
					zap.String("path", gitWT.Path),
				)
			}
			continue
		}

		var headPtr *string
		if gitWT.HeadCommit != "" {
			commit := gitWT.HeadCommit
			headPtr = &commit
		}
		idVal := utils.NewID()
		zeroVal := int64(0)
		_, err := q.WorktreeCreate(ctx, &WorktreeCreateParams{
			Id:              idVal,
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
		})
		if err != nil {
			logger.Warn("failed to insert worktree from git",
				zap.Error(err),
				zap.String("projectId", projectID),
				zap.String("path", gitWT.Path),
			)
		}
	}

	for normPath, dbWT := range dbByPath {
		if _, ok := gitByPath[normPath]; ok {
			continue
		}
		if _, err := q.WorktreeSoftDelete(ctx, &WorktreeSoftDeleteParams{
			DeletedAt: &now,
			UpdatedAt: now,
			Id:        dbWT.Id,
		}); err != nil {
			worktreeID := dbWT.Id
			logger.Warn("failed to soft delete orphan worktree",
				zap.Error(err),
				zap.String("worktreeId", worktreeID),
			)
		}
	}
}

// createVirtualMainWorktree 为非 git 目录创建一个虚拟的 main worktree，使终端等功能可用
func (s *ProjectService) createVirtualMainWorktree(ctx context.Context, projectID, projectPath, branchName string) {
	if ctx == nil {
		ctx = context.Background()
	}

	q, err := resolveQueries(nil)
	if err != nil {
		return
	}

	logger := utils.Logger()

	// 检查是否已经存在 worktree
	existingWorktrees, err := q.WorktreeListByProject(ctx, projectID)
	if err != nil {
		logger.Warn("failed to check existing worktrees",
			zap.Error(err),
			zap.String("projectId", projectID),
		)
		return
	}

	// 如果已经有 worktree，不创建
	if len(existingWorktrees) > 0 {
		return
	}

	// 创建虚拟 main worktree
	now := time.Now()
	zeroVal := int64(0)
	_, err = q.WorktreeCreate(ctx, &WorktreeCreateParams{
		Id:              utils.NewID(),
		CreatedAt:       now,
		UpdatedAt:       now,
		ProjectId:       projectID,
		BranchName:      branchName,
		Path:            filepath.Clean(projectPath),
		IsMain:          true,
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
		logger.Warn("failed to create virtual main worktree for non-git project",
			zap.Error(err),
			zap.String("projectId", projectID),
			zap.String("path", projectPath),
		)
	} else {
		logger.Debug("created virtual main worktree for non-git project",
			zap.String("projectId", projectID),
			zap.String("path", projectPath),
		)
	}
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
