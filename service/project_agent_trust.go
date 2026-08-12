package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"

	"gorm.io/gorm"
)

type ProjectAgent string

const ProjectAgentPi ProjectAgent = "pi"

var (
	ErrUnsupportedProjectAgent    = errors.New("unsupported project agent")
	ErrProjectAgentTrustRequired  = errors.New("project agent trust is required")
	ErrProjectAgentPathNotAllowed = errors.New("path is not managed by the project")
)

type ProjectAgentTrustStatus struct {
	ProjectID   string     `json:"projectId"`
	Agent       string     `json:"agent"`
	ProjectPath string     `json:"projectPath"`
	TrustedPath string     `json:"trustedPath,omitempty"`
	Trusted     bool       `json:"trusted"`
	TrustedAt   *time.Time `json:"trustedAt,omitempty"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
}

type ProjectAgentTrustService struct {
	projectSvc  *model.ProjectService
	worktreeSvc *WorktreeService
}

func NewProjectAgentTrustService() *ProjectAgentTrustService {
	return &ProjectAgentTrustService{
		projectSvc:  model.NewProjectService(),
		worktreeSvc: NewWorktreeService(),
	}
}

func normalizeProjectAgent(agent ProjectAgent) (ProjectAgent, error) {
	normalized := ProjectAgent(strings.ToLower(strings.TrimSpace(string(agent))))
	if normalized != ProjectAgentPi {
		return "", ErrUnsupportedProjectAgent
	}
	return normalized, nil
}

func CanonicalAgentTrustPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("project path is empty")
	}
	absolute, err := filepath.Abs(filepath.FromSlash(value))
	if err != nil {
		return "", fmt.Errorf("resolve absolute project path: %w", err)
	}
	canonical := filepath.Clean(absolute)
	if resolved, resolveErr := filepath.EvalSymlinks(canonical); resolveErr == nil && strings.TrimSpace(resolved) != "" {
		canonical = filepath.Clean(resolved)
	}
	if runtime.GOOS == "windows" {
		canonical = strings.ToLower(canonical)
	}
	return canonical, nil
}

func (s *ProjectAgentTrustService) GetStatus(
	ctx context.Context,
	projectID string,
	agent ProjectAgent,
) (ProjectAgentTrustStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	normalizedAgent, err := normalizeProjectAgent(agent)
	if err != nil {
		return ProjectAgentTrustStatus{}, err
	}
	project, err := s.projectSvc.GetProject(ctx, strings.TrimSpace(projectID))
	if err != nil {
		return ProjectAgentTrustStatus{}, err
	}
	projectPath, err := CanonicalAgentTrustPath(project.Path)
	if err != nil {
		return ProjectAgentTrustStatus{}, err
	}
	status := ProjectAgentTrustStatus{
		ProjectID:   project.Id,
		Agent:       string(normalizedAgent),
		ProjectPath: projectPath,
	}

	db := model.GetDB()
	if db == nil {
		return ProjectAgentTrustStatus{}, model.ErrDBNotInitialized
	}
	var record tables.ProjectAgentTrustTable
	err = db.WithContext(ctx).
		Where("project_id = ? AND agent = ?", project.Id, string(normalizedAgent)).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return status, nil
	}
	if err != nil {
		return ProjectAgentTrustStatus{}, err
	}
	status.TrustedPath = record.TrustedPath
	trustedAt := record.TrustedAt
	status.TrustedAt = &trustedAt
	status.RevokedAt = record.RevokedAt
	trustedPath, pathErr := CanonicalAgentTrustPath(record.TrustedPath)
	status.Trusted = pathErr == nil && record.RevokedAt == nil && trustedPath == projectPath
	return status, nil
}

func (s *ProjectAgentTrustService) Trust(
	ctx context.Context,
	projectID string,
	agent ProjectAgent,
) (ProjectAgentTrustStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	normalizedAgent, err := normalizeProjectAgent(agent)
	if err != nil {
		return ProjectAgentTrustStatus{}, err
	}
	project, err := s.projectSvc.GetProject(ctx, strings.TrimSpace(projectID))
	if err != nil {
		return ProjectAgentTrustStatus{}, err
	}
	info, err := os.Stat(project.Path)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = fmt.Errorf("project path is not a directory")
		}
		return ProjectAgentTrustStatus{}, fmt.Errorf("project path is unavailable: %w", err)
	}
	trustedPath, err := CanonicalAgentTrustPath(project.Path)
	if err != nil {
		return ProjectAgentTrustStatus{}, err
	}
	db := model.GetDB()
	if db == nil {
		return ProjectAgentTrustStatus{}, model.ErrDBNotInitialized
	}
	now := time.Now()
	var record tables.ProjectAgentTrustTable
	err = db.WithContext(ctx).Unscoped().
		Where("project_id = ? AND agent = ?", project.Id, string(normalizedAgent)).
		First(&record).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		record = tables.ProjectAgentTrustTable{
			ProjectID:   project.Id,
			Agent:       string(normalizedAgent),
			TrustedPath: trustedPath,
			TrustedAt:   now,
		}
		if err := db.WithContext(ctx).Create(&record).Error; err != nil {
			return ProjectAgentTrustStatus{}, err
		}
	case err != nil:
		return ProjectAgentTrustStatus{}, err
	default:
		if err := db.WithContext(ctx).Unscoped().Model(&record).Updates(map[string]any{
			"trusted_path": trustedPath,
			"trusted_at":   now,
			"revoked_at":   nil,
			"deleted_at":   nil,
		}).Error; err != nil {
			return ProjectAgentTrustStatus{}, err
		}
	}
	return s.GetStatus(ctx, project.Id, normalizedAgent)
}

func (s *ProjectAgentTrustService) Revoke(
	ctx context.Context,
	projectID string,
	agent ProjectAgent,
) (ProjectAgentTrustStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	normalizedAgent, err := normalizeProjectAgent(agent)
	if err != nil {
		return ProjectAgentTrustStatus{}, err
	}
	project, err := s.projectSvc.GetProject(ctx, strings.TrimSpace(projectID))
	if err != nil {
		return ProjectAgentTrustStatus{}, err
	}
	db := model.GetDB()
	if db == nil {
		return ProjectAgentTrustStatus{}, model.ErrDBNotInitialized
	}
	now := time.Now()
	if err := db.WithContext(ctx).Model(&tables.ProjectAgentTrustTable{}).
		Where("project_id = ? AND agent = ?", project.Id, string(normalizedAgent)).
		Updates(map[string]any{"revoked_at": now, "updated_at": now}).Error; err != nil {
		return ProjectAgentTrustStatus{}, err
	}
	return s.GetStatus(ctx, project.Id, normalizedAgent)
}

func (s *ProjectAgentTrustService) EnsureTrustedPath(
	ctx context.Context,
	projectID string,
	agent ProjectAgent,
	cwd string,
) error {
	status, err := s.GetStatus(ctx, projectID, agent)
	if err != nil {
		return err
	}
	if !status.Trusted {
		return ErrProjectAgentTrustRequired
	}
	candidate, err := CanonicalAgentTrustPath(cwd)
	if err != nil {
		return ErrProjectAgentPathNotAllowed
	}
	if candidate == status.ProjectPath {
		return nil
	}
	worktrees, err := s.worktreeSvc.ListWorktrees(ctx, status.ProjectID)
	if err != nil {
		return err
	}
	for _, worktree := range worktrees {
		if worktree == nil || worktree.ProjectId != status.ProjectID {
			continue
		}
		worktreePath, pathErr := CanonicalAgentTrustPath(worktree.Path)
		if pathErr == nil && candidate == worktreePath {
			return nil
		}
	}
	return ErrProjectAgentPathNotAllowed
}
