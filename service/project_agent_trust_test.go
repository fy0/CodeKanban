package service

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"code-kanban/model"
	"code-kanban/model/tables"
)

func seedAgentTrustProject(t *testing.T, path string) *tables.ProjectTable {
	t.Helper()
	project := &tables.ProjectTable{Name: "Agent Trust Test", Path: path}
	project.Init()
	if err := model.GetDB().Create(project).Error; err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return project
}

func TestProjectAgentTrustLifecycleAndPathChange(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	projectPath := t.TempDir()
	project := seedAgentTrustProject(t, projectPath)
	svc := NewProjectAgentTrustService()
	ctx := context.Background()

	status, err := svc.GetStatus(ctx, project.ID, ProjectAgentPi)
	if err != nil || status.Trusted {
		t.Fatalf("initial status = %#v, %v", status, err)
	}
	status, err = svc.Trust(ctx, project.ID, ProjectAgentPi)
	if err != nil || !status.Trusted || status.TrustedAt == nil {
		t.Fatalf("trusted status = %#v, %v", status, err)
	}
	expectedPath, err := CanonicalAgentTrustPath(projectPath)
	if err != nil || status.TrustedPath != expectedPath {
		t.Fatalf("trusted path = %q, want %q (%v)", status.TrustedPath, expectedPath, err)
	}
	if err := svc.EnsureTrustedPath(ctx, project.ID, ProjectAgentPi, projectPath); err != nil {
		t.Fatalf("EnsureTrustedPath(project) returned error: %v", err)
	}

	movedPath := t.TempDir()
	if err := model.GetDB().Model(&tables.ProjectTable{}).
		Where("id = ?", project.ID).
		Update("path", movedPath).Error; err != nil {
		t.Fatalf("move project path in database: %v", err)
	}
	status, err = svc.GetStatus(ctx, project.ID, ProjectAgentPi)
	if err != nil || status.Trusted {
		t.Fatalf("status after path change = %#v, %v", status, err)
	}
	if err := svc.EnsureTrustedPath(ctx, project.ID, ProjectAgentPi, movedPath); !errors.Is(err, ErrProjectAgentTrustRequired) {
		t.Fatalf("EnsureTrustedPath after path change = %v, want trust required", err)
	}

	status, err = svc.Trust(ctx, project.ID, ProjectAgentPi)
	if err != nil || !status.Trusted {
		t.Fatalf("re-trust after path change = %#v, %v", status, err)
	}
	status, err = svc.Revoke(ctx, project.ID, ProjectAgentPi)
	if err != nil || status.Trusted || status.RevokedAt == nil {
		t.Fatalf("revoked status = %#v, %v", status, err)
	}
}

func TestProjectAgentTrustAllowsOnlyManagedWorktrees(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	projectPath := t.TempDir()
	worktreePath := t.TempDir()
	unrelatedPath := t.TempDir()
	project := seedAgentTrustProject(t, projectPath)
	worktree := &tables.WorktreeTable{
		ProjectID:  project.ID,
		BranchName: "feature",
		Path:       worktreePath,
	}
	worktree.Init()
	if err := model.GetDB().Create(worktree).Error; err != nil {
		t.Fatalf("seed worktree: %v", err)
	}

	svc := NewProjectAgentTrustService()
	if _, err := svc.Trust(context.Background(), project.ID, ProjectAgentPi); err != nil {
		t.Fatalf("Trust returned error: %v", err)
	}
	if err := svc.EnsureTrustedPath(context.Background(), project.ID, ProjectAgentPi, worktreePath); err != nil {
		t.Fatalf("managed worktree rejected: %v", err)
	}
	if err := svc.EnsureTrustedPath(context.Background(), project.ID, ProjectAgentPi, unrelatedPath); !errors.Is(err, ErrProjectAgentPathNotAllowed) {
		t.Fatalf("unrelated path result = %v, want path not allowed", err)
	}
}

func TestDeleteProjectHardDeletesAgentTrust(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	project := seedAgentTrustProject(t, t.TempDir())
	svc := NewProjectAgentTrustService()
	if _, err := svc.Trust(context.Background(), project.ID, ProjectAgentPi); err != nil {
		t.Fatalf("Trust returned error: %v", err)
	}
	if err := model.NewProjectService().DeleteProject(context.Background(), project.ID); err != nil {
		t.Fatalf("DeleteProject returned error: %v", err)
	}
	var count int64
	if err := model.GetDB().Unscoped().Model(&tables.ProjectAgentTrustTable{}).
		Where("project_id = ?", project.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count trust rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("trust row count = %d, want 0", count)
	}
}

func TestCanonicalAgentTrustPathNormalizesRelativeSegments(t *testing.T) {
	root := t.TempDir()
	value := filepath.Join(root, "child", "..")
	got, err := CanonicalAgentTrustPath(value)
	if err != nil {
		t.Fatalf("CanonicalAgentTrustPath returned error: %v", err)
	}
	want, err := CanonicalAgentTrustPath(root)
	if err != nil || got != want {
		t.Fatalf("canonical path = %q, want %q (%v)", got, want, err)
	}
	if runtime.GOOS == "windows" {
		upper, err := CanonicalAgentTrustPath(strings.ToUpper(root))
		if err != nil || upper != want {
			t.Fatalf("case-normalized path = %q, want %q (%v)", upper, want, err)
		}
	}
}
