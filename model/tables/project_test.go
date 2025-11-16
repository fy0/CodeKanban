package tables

import (
	"testing"
)

func TestProjectTableCRUD(t *testing.T) {
	db := setupTestDB(t)

	project := &ProjectTable{
		Name:             "Sample Project",
		Path:             "/tmp/sample-project",
		Description:      "Test project record",
		DefaultBranch:    "main",
		WorktreeBasePath: "/tmp/sample-project/.worktrees",
		RemoteURL:        "git@example.com:sample/project.git",
	}

	if err := db.Create(project).Error; err != nil {
		t.Fatalf("create project failed: %v", err)
	}
	if project.ID == "" {
		t.Fatalf("expected project to have an ID after creation")
	}

	var found ProjectTable
	if err := db.First(&found, "id = ?", project.ID).Error; err != nil {
		t.Fatalf("failed to load project: %v", err)
	}

	if found.Name != project.Name {
		t.Fatalf("unexpected project name, want %q got %q", project.Name, found.Name)
	}
	if found.RemoteURL != project.RemoteURL {
		t.Fatalf("unexpected remote url, want %q got %q", project.RemoteURL, found.RemoteURL)
	}
}
