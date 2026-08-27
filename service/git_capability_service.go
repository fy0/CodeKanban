package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"code-kanban/model"
	"code-kanban/utils/git"
)

type GitCapabilityService struct{}

func NewGitCapabilityService() *GitCapabilityService {
	return &GitCapabilityService{}
}

func (s *GitCapabilityService) GetProjectCapabilities(ctx context.Context, projectID string) (*model.GitCapabilityResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(projectID) == "" {
		return nil, errors.New("project id is required")
	}
	queries, err := model.ResolveReadQueries(nil)
	if err != nil {
		return nil, err
	}
	project, err := queries.ProjectGetByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrProjectNotFound
		}
		return nil, err
	}

	repository, err := git.DetectRepository(project.Path)
	if err != nil {
		code := git.ErrorCode(err)
		if code == "" {
			code = git.ErrorCodeNotRepository
		}
		return &model.GitCapabilityResult{
			Repository: git.HasRepositoryStructure(project.Path),
			Mode:       git.CapabilityModeUnavailable,
			Operations: git.OperationCapabilities{},
			Engines:    git.OperationEngines{},
			Reasons: []git.CapabilityReason{{
				Code:   code,
				Detail: err.Error(),
			}},
			Worktrees: []model.GitWorktreeCapabilityResult{},
		}, nil
	}
	defer repository.Close()

	base := repository.Capabilities(project.Path)
	result := &model.GitCapabilityResult{
		Repository: base.Repository,
		Mode:       base.Mode,
		Operations: base.Operations,
		Engines:    base.Engines,
		Reasons:    base.Reasons,
		Worktrees:  []model.GitWorktreeCapabilityResult{},
	}
	worktrees, err := queries.WorktreeListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	result.Worktrees = make([]model.GitWorktreeCapabilityResult, 0, len(worktrees))
	for _, worktree := range worktrees {
		report := repository.Capabilities(worktree.Path)
		result.Worktrees = append(result.Worktrees, model.GitWorktreeCapabilityResult{
			ID:         worktree.Id,
			Operations: report.Operations,
			Engines:    report.Engines,
			Reasons:    report.Reasons,
		})
	}
	return result, nil
}
