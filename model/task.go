package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"code-kanban/model/tables"

	"gorm.io/gorm"
)

var (
	// ErrTaskNotFound indicates the requested task does not exist.
	ErrTaskNotFound = errors.New("task not found")
	// ErrInvalidTaskStatus indicates the provided task status is not supported.
	ErrInvalidTaskStatus = errors.New("invalid task status")
)

var taskStatusSet = map[string]struct{}{
	"todo":        {},
	"in_progress": {},
	"done":        {},
	"archived":    {},
}

// TaskService coordinates CRUD operations for kanban tasks.
type TaskService struct{}

// CreateTaskRequest captures inputs required to create a task.
type CreateTaskRequest struct {
	ProjectID   string
	WorktreeID  *string
	Title       string
	Description string
	Status      string
	Priority    int
	Tags        tables.StringArray
	DueDate     *time.Time
}

// ListTasksRequest configures list filtering and pagination.
type ListTasksRequest struct {
	ProjectID  string
	Status     string
	WorktreeID string
	Priority   *int
	Keyword    string
	Page       int
	PageSize   int
}

// MoveTaskRequest defines updates when dragging tasks across columns.
type MoveTaskRequest struct {
	Status     string
	OrderIndex *float64
	WorktreeID *string
}

// CreateTask inserts a new task row after validating related entities.
func (s *TaskService) CreateTask(ctx context.Context, req *CreateTaskRequest) (*tables.TaskTable, error) {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("project id is required")
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, fmt.Errorf("task title is required")
	}

	status, err := normalizeTaskStatus(req.Status)
	if err != nil {
		return nil, err
	}

	if req.Priority < 0 {
		req.Priority = 0
	}

	var project tables.ProjectTable
	if err := dbCtx.First(&project, "id = ?", projectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}

	var worktreeID *string
	var branchName string
	if req.WorktreeID != nil && strings.TrimSpace(*req.WorktreeID) != "" {
		worktreeID = req.WorktreeID
		worktree, err := s.getWorktreeWithBranch(dbCtx, *worktreeID, projectID)
		if err != nil {
			return nil, err
		}
		branchName = worktree.BranchName
	}

	orderIndex, err := s.getNextOrderIndex(dbCtx, projectID, status)
	if err != nil {
		return nil, err
	}

	task := &tables.TaskTable{
		ProjectID:   projectID,
		WorktreeID:  worktreeID,
		BranchName:  branchName,
		Title:       title,
		Description: strings.TrimSpace(req.Description),
		Status:      status,
		Priority:    req.Priority,
		OrderIndex:  orderIndex,
		Tags:        sanitizeTags(req.Tags),
		DueDate:     req.DueDate,
	}

	if err := dbCtx.Create(task).Error; err != nil {
		return nil, err
	}

	if err := dbCtx.Preload("Worktree").First(task, "id = ?", task.ID).Error; err != nil {
		return nil, err
	}

	return task, nil
}

// ListTasks returns filtered paginated tasks for a project.
func (s *TaskService) ListTasks(ctx context.Context, req *ListTasksRequest) ([]tables.TaskTable, int64, error) {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return nil, 0, err
	}
	if req == nil {
		return nil, 0, fmt.Errorf("request is required")
	}
	if strings.TrimSpace(req.ProjectID) == "" {
		return nil, 0, fmt.Errorf("project id is required")
	}

	query := dbCtx.Model(&tables.TaskTable{}).
		Where("project_id = ?", req.ProjectID)

	if status := strings.TrimSpace(req.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	if req.WorktreeID != "" {
		query = query.Where("worktree_id = ?", req.WorktreeID)
	}
	if req.Priority != nil {
		query = query.Where("priority = ?", *req.Priority)
	}
	if keyword := strings.TrimSpace(req.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("title LIKE ? OR description LIKE ?", like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}
	if pageSize > 200 {
		pageSize = 200
	}
	offset := (page - 1) * pageSize

	var tasks []tables.TaskTable
	if err := query.
		Preload("Worktree").
		Order("status ASC").
		Order("order_index ASC").
		Offset(offset).
		Limit(pageSize).
		Find(&tasks).Error; err != nil {
		return nil, 0, err
	}

	req.Page = page
	req.PageSize = pageSize
	return tasks, total, nil
}

// GetTask loads a task by identifier.
func (s *TaskService) GetTask(ctx context.Context, id string) (*tables.TaskTable, error) {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return nil, err
	}

	var task tables.TaskTable
	if err := dbCtx.Preload("Worktree").First(&task, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	return &task, nil
}

// UpdateTask applies partial updates to a task record.
func (s *TaskService) UpdateTask(ctx context.Context, id string, updates map[string]interface{}) (*tables.TaskTable, error) {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return nil, err
	}
	if len(updates) == 0 {
		return s.GetTask(ctx, id)
	}

	task, err := s.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	if value, ok := updates["worktree_id"]; ok {
		if err := dbCtx.
			Model(&tables.TaskTable{}).
			Where("id = ?", id).
			Update("worktree_id", value).Error; err != nil {
			return nil, err
		}
		delete(updates, "worktree_id")
	}

	if len(updates) > 0 {
		if err := dbCtx.Model(task).Updates(updates).Error; err != nil {
			return nil, err
		}
	}

	return s.GetTask(ctx, id)
}

// DeleteTask removes a task softly.
func (s *TaskService) DeleteTask(ctx context.Context, id string) error {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return err
	}

	result := dbCtx.Delete(&tables.TaskTable{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// MoveTask updates the task status/order/worktree when dragged.
func (s *TaskService) MoveTask(ctx context.Context, id string, req *MoveTaskRequest) (*tables.TaskTable, error) {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return nil, err
	}
	ctx = ensureContext(ctx)
	if req == nil {
		req = &MoveTaskRequest{}
	}

	task, err := s.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}

	if status := strings.TrimSpace(req.Status); status != "" && status != task.Status {
		if _, err := normalizeTaskStatus(status); err != nil {
			return nil, err
		}
		updates["status"] = status
		task.Status = status
	}

	if req.OrderIndex != nil {
		updates["order_index"] = *req.OrderIndex
	} else {
		targetStatus := task.Status
		if strings.TrimSpace(req.Status) != "" {
			targetStatus = strings.TrimSpace(req.Status)
		}
		orderIndex, err := s.getNextOrderIndex(dbCtx, task.ProjectID, targetStatus)
		if err != nil {
			return nil, err
		}
		updates["order_index"] = orderIndex
	}

	if req.WorktreeID != nil {
		if *req.WorktreeID == "" {
			updates["worktree_id"] = nil
			updates["branch_name"] = ""
		} else {
			worktree, err := s.getWorktreeWithBranch(dbCtx, *req.WorktreeID, task.ProjectID)
			if err != nil {
				return nil, err
			}
			updates["worktree_id"] = *req.WorktreeID
			updates["branch_name"] = worktree.BranchName
		}
	}

	// 当任务从完成状态改回其他状态时，验证关联的分支是否还存在
	// 注意：task.Status 已经在上面被更新为新状态（如果有的话）
	originalStatus := task.Status
	if _, exists := updates["status"]; exists {
		// 如果状态被更新了，需要检查原始状态
		var origTask tables.TaskTable
		if err := dbCtx.First(&origTask, "id = ?", id).Error; err == nil {
			originalStatus = origTask.Status
		}
	}

	newStatus := task.Status
	if originalStatus == "done" && newStatus != "" && newStatus != "done" {
		if task.WorktreeID != nil && task.BranchName != "" {
			// 尝试获取worktree，如果不存在则清除绑定
			_, err := s.getWorktreeWithBranch(dbCtx, *task.WorktreeID, task.ProjectID)
			if err != nil {
				// worktree已被删除，清除绑定
				updates["worktree_id"] = nil
				updates["branch_name"] = ""
			}
		}
	}

	return s.UpdateTask(ctx, id, updates)
}

// BindWorktree associates or detaches a worktree from a task.
func (s *TaskService) BindWorktree(ctx context.Context, id string, worktreeID *string) (*tables.TaskTable, error) {
	dbCtx, err := s.dbWithContext(ctx)
	if err != nil {
		return nil, err
	}
	ctx = ensureContext(ctx)

	task, err := s.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if worktreeID == nil || strings.TrimSpace(*worktreeID) == "" {
		updates["worktree_id"] = nil
		updates["branch_name"] = ""
	} else {
		worktree, err := s.getWorktreeWithBranch(dbCtx, *worktreeID, task.ProjectID)
		if err != nil {
			return nil, err
		}
		updates["worktree_id"] = *worktreeID
		updates["branch_name"] = worktree.BranchName
	}

	return s.UpdateTask(ctx, id, updates)
}

func (s *TaskService) dbWithContext(ctx context.Context) (*gorm.DB, error) {
	if db == nil {
		return nil, ErrDBNotInitialized
	}
	return db.WithContext(ensureContext(ctx)), nil
}

func ensureContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func normalizeTaskStatus(status string) (string, error) {
	st := strings.TrimSpace(status)
	if st == "" {
		st = "todo"
	}
	if _, ok := taskStatusSet[st]; !ok {
		return "", ErrInvalidTaskStatus
	}
	return st, nil
}

func sanitizeTags(tags tables.StringArray) tables.StringArray {
	if len(tags) == 0 {
		return tables.StringArray{}
	}
	result := make(tables.StringArray, 0, len(tags))
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func (s *TaskService) getNextOrderIndex(dbCtx *gorm.DB, projectID, status string) (float64, error) {
	var maxOrder float64
	if err := dbCtx.
		Model(&tables.TaskTable{}).
		Where("project_id = ? AND status = ?", projectID, status).
		Select("COALESCE(MAX(order_index), 0)").
		Scan(&maxOrder).Error; err != nil {
		return 0, err
	}
	return maxOrder + 1000, nil
}

func (s *TaskService) ensureWorktreeBelongsToProject(dbCtx *gorm.DB, worktreeID, projectID string) error {
	var worktree tables.WorktreeTable
	if err := dbCtx.First(&worktree, "id = ?", worktreeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrWorktreeNotFound
		}
		return err
	}
	if worktree.ProjectID != projectID {
		return fmt.Errorf("worktree does not belong to project")
	}
	return nil
}

// getWorktreeWithBranch 获取 worktree 并验证其归属，返回 worktree 以便获取分支名称
func (s *TaskService) getWorktreeWithBranch(dbCtx *gorm.DB, worktreeID, projectID string) (*tables.WorktreeTable, error) {
	var worktree tables.WorktreeTable
	if err := dbCtx.First(&worktree, "id = ?", worktreeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWorktreeNotFound
		}
		return nil, err
	}
	if worktree.ProjectID != projectID {
		return nil, fmt.Errorf("worktree does not belong to project")
	}
	return &worktree, nil
}
