package model

import (
	"context"
	"testing"

	"code-kanban/model/tables"
)

func TestTaskServiceLifecycle(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	ctx := context.Background()
	project := seedProject(t)
	worktree := seedWorktree(t, project.ID, "feature/one")

	service := &TaskService{}

	task, err := service.CreateTask(ctx, &CreateTaskRequest{
		ProjectID:   project.ID,
		WorktreeID:  &worktree.ID,
		Title:       "Implement kanban board",
		Description: "Step 3 task",
		Status:      "todo",
		Priority:    1,
		Tags:        tables.StringArray{"backend"},
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	if task.Status != "todo" {
		t.Fatalf("expected status todo, got %s", task.Status)
	}

	list, total, err := service.ListTasks(ctx, &ListTasksRequest{
		ProjectID: project.ID,
	})
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("expected single task, got total=%d len=%d", total, len(list))
	}

	orderIndex := task.OrderIndex + 500
	moved, err := service.MoveTask(ctx, task.ID, &MoveTaskRequest{
		Status:     "in_progress",
		OrderIndex: &orderIndex,
		WorktreeID: &worktree.ID,
	})
	if err != nil {
		t.Fatalf("MoveTask returned error: %v", err)
	}
	if moved.Status != "in_progress" {
		t.Fatalf("expected status in_progress, got %s", moved.Status)
	}
	if moved.OrderIndex != orderIndex {
		t.Fatalf("expected order index %.2f, got %.2f", orderIndex, moved.OrderIndex)
	}

	unbound, err := service.BindWorktree(ctx, task.ID, nil)
	if err != nil {
		t.Fatalf("BindWorktree returned error: %v", err)
	}
	var stored struct {
		WorktreeID *string
	}
	if err := db.Table("tasks").Select("worktree_id").Where("id = ?", task.ID).Scan(&stored).Error; err != nil {
		t.Fatalf("scan worktree_id failed: %v", err)
	}
	if stored.WorktreeID != nil {
		t.Fatalf("expected stored worktree_id to be null, got %q", *stored.WorktreeID)
	}

	if unbound.WorktreeID != nil {
		t.Fatalf("expected worktree to be nil after unbind, got %q", *unbound.WorktreeID)
	}

	if err := service.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("DeleteTask returned error: %v", err)
	}
}

func TestTaskCommentService(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	ctx := context.Background()
	project := seedProject(t)
	service := &TaskService{}

	task, err := service.CreateTask(ctx, &CreateTaskRequest{
		ProjectID: project.ID,
		Title:     "Comment me",
		Status:    "todo",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}

	commentSvc := NewTaskCommentService()

	comment, err := commentSvc.CreateComment(ctx, task.ID, "Looks good")
	if err != nil {
		t.Fatalf("CreateComment returned error: %v", err)
	}
	if comment.TaskID != task.ID {
		t.Fatalf("expected comment task id %s, got %s", task.ID, comment.TaskID)
	}

	comments, err := commentSvc.ListComments(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListComments returned error: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	if err := commentSvc.DeleteComment(ctx, comment.ID); err != nil {
		t.Fatalf("DeleteComment returned error: %v", err)
	}
}

func seedProject(t *testing.T) *tables.ProjectTable {
	t.Helper()
	project := &tables.ProjectTable{
		Name:          "Seed Project",
		Path:          t.TempDir(),
		DefaultBranch: "main",
	}
	if err := db.Create(project).Error; err != nil {
		t.Fatalf("seed project failed: %v", err)
	}
	return project
}

func seedWorktree(t *testing.T, projectID, branch string) *tables.WorktreeTable {
	t.Helper()
	worktree := &tables.WorktreeTable{
		ProjectID:  projectID,
		BranchName: branch,
		Path:       t.TempDir(),
		IsMain:     false,
		IsBare:     false,
	}
	if err := db.Create(worktree).Error; err != nil {
		t.Fatalf("seed worktree failed: %v", err)
	}
	return worktree
}
