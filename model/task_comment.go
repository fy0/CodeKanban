package model

import (
	"context"
	"errors"
	"strings"

	"code-kanban/model/tables"

	"gorm.io/gorm"
)

var (
	// ErrTaskCommentNotFound indicates the requested task comment does not exist.
	ErrTaskCommentNotFound = errors.New("task comment not found")
)

// TaskCommentService coordinates CRUD operations for task comments.
type TaskCommentService struct {
	taskSvc *TaskService
}

// NewTaskCommentService constructs a task comment service with a task dependency.
func NewTaskCommentService() *TaskCommentService {
	return &TaskCommentService{taskSvc: &TaskService{}}
}

// CreateComment inserts a comment for the given task.
func (s *TaskCommentService) CreateComment(ctx context.Context, taskID, content string) (*tables.TaskCommentTable, error) {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return nil, err
	}

	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, ErrTaskNotFound
	}

	body := strings.TrimSpace(content)
	if body == "" {
		return nil, errors.New("comment content is required")
	}

	if s.taskSvc != nil {
		if _, err := s.taskSvc.GetTask(ctx, taskID); err != nil {
			return nil, err
		}
	}

	comment := &tables.TaskCommentTable{
		TaskID:  taskID,
		Content: body,
	}

	if err := dbCtx.Create(comment).Error; err != nil {
		return nil, err
	}
	return comment, nil
}

// ListComments fetches comments for a task ordered by creation time.
func (s *TaskCommentService) ListComments(ctx context.Context, taskID string) ([]tables.TaskCommentTable, error) {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return nil, err
	}

	var comments []tables.TaskCommentTable
	if err := dbCtx.
		Where("task_id = ?", taskID).
		Order("created_at ASC").
		Find(&comments).Error; err != nil {
		return nil, err
	}
	return comments, nil
}

// DeleteComment removes a comment by identifier.
func (s *TaskCommentService) DeleteComment(ctx context.Context, id string) error {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return err
	}

	result := dbCtx.Delete(&tables.TaskCommentTable{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTaskCommentNotFound
	}
	return nil
}

func (s *TaskCommentService) dbWithContext(ctx context.Context) (*gorm.DB, error) {
	if db == nil {
		return nil, ErrDBNotInitialized
	}
	return db.WithContext(ensureContext(ctx)), nil
}
