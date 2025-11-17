package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"code-kanban/api/h"
	"code-kanban/model"
	"code-kanban/model/tables"
)

const (
	taskTag        = "task-任务看板"
	taskCommentTag = "task-comment-任务评论"
)

type createTaskBody struct {
	Title       string     `json:"title" minLength:"1" doc:"任务标题"`
	Description string     `json:"description" doc:"任务描述"`
	Status      string     `json:"status" enum:"todo,in_progress,done,archived" default:"todo" doc:"任务状态"`
	Priority    int        `json:"priority" minimum:"0" maximum:"3" default:"0" doc:"优先级"`
	Tags        []string   `json:"tags" doc:"标签"`
	WorktreeID  *string    `json:"worktreeId" doc:"关联的 Worktree"`
	DueDate     *time.Time `json:"dueDate" doc:"截止日期"`
}

type updateTaskBody struct {
	Title       *string    `json:"title,omitempty" doc:"任务标题"`
	Description *string    `json:"description,omitempty" doc:"任务描述"`
	Priority    *int       `json:"priority,omitempty" doc:"优先级"`
	Tags        *[]string  `json:"tags,omitempty" doc:"标签"`
	DueDate     *time.Time `json:"dueDate,omitempty" doc:"截止日期"`
}

type moveTaskBody struct {
	Status     string   `json:"status,omitempty" doc:"新状态"`
	OrderIndex *float64 `json:"orderIndex,omitempty" doc:"排序索引"`
	WorktreeID *string  `json:"worktreeId,omitempty" doc:"关联 Worktree"`
}

type bindWorktreeBody struct {
	WorktreeID *string `json:"worktreeId,omitempty" doc:"Worktree ID（null 表示解绑）"`
}

type createCommentBody struct {
	Content string `json:"content" minLength:"1" doc:"评论内容"`
}

func registerTaskRoutes(group *huma.Group) {
	taskService := &model.TaskService{}
	commentService := model.NewTaskCommentService()

	huma.Post(group, "/projects/{projectId}/tasks/create", func(ctx context.Context, input *struct {
		ProjectID string `path:"projectId"`
		Body      createTaskBody
	}) (*h.ItemResponse[tables.TaskTable], error) {
		task, err := taskService.CreateTask(ctx, &model.CreateTaskRequest{
			ProjectID:   input.ProjectID,
			Title:       input.Body.Title,
			Description: input.Body.Description,
			Status:      input.Body.Status,
			Priority:    input.Body.Priority,
			Tags:        tables.StringArray(input.Body.Tags),
			WorktreeID:  input.Body.WorktreeID,
			DueDate:     input.Body.DueDate,
		})
		if err != nil {
			return nil, mapTaskError(err)
		}

		resp := h.NewItemResponse(*task)
		resp.Status = http.StatusCreated
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "task-create"
		op.Summary = "创建任务"
		op.Tags = []string{taskTag}
	})

	huma.Get(group, "/projects/{projectId}/tasks", func(ctx context.Context, input *struct {
		ProjectID  string `path:"projectId"`
		Status     string `query:"status"`
		WorktreeID string `query:"worktreeId"`
		Priority   string `query:"priority"`
		Keyword    string `query:"keyword"`
		Page       int    `query:"page" default:"1"`
		PageSize   int    `query:"pageSize" default:"100"`
	}) (*h.PaginatedResponse[tables.TaskTable], error) {
		var priorityPtr *int
		if strings.TrimSpace(input.Priority) != "" {
			value, err := strconv.Atoi(input.Priority)
			if err != nil {
				return nil, huma.Error400BadRequest("invalid priority value")
			}
			priorityPtr = &value
		}

		req := &model.ListTasksRequest{
			ProjectID:  input.ProjectID,
			Status:     input.Status,
			WorktreeID: input.WorktreeID,
			Priority:   priorityPtr,
			Keyword:    input.Keyword,
			Page:       input.Page,
			PageSize:   input.PageSize,
		}

		tasks, total, err := taskService.ListTasks(ctx, req)
		if err != nil {
			return nil, mapTaskError(err)
		}

		resp := h.NewPaginatedResponse(tasks, total, req.Page, req.PageSize)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "task-list"
		op.Summary = "任务列表"
		op.Tags = []string{taskTag}
	})

	huma.Get(group, "/tasks/{id}", func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*h.ItemResponse[tables.TaskTable], error) {
		task, err := taskService.GetTask(ctx, input.ID)
		if err != nil {
			return nil, mapTaskError(err)
		}

		resp := h.NewItemResponse(*task)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "task-get-by-id"
		op.Summary = "任务详情"
		op.Tags = []string{taskTag}
	})

	huma.Post(group, "/tasks/{id}/update", func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body updateTaskBody
	}) (*h.ItemResponse[tables.TaskTable], error) {
		updates := map[string]interface{}{}
		if input.Body.Title != nil {
			updates["title"] = *input.Body.Title
		}
		if input.Body.Description != nil {
			updates["description"] = *input.Body.Description
		}
		if input.Body.Priority != nil {
			updates["priority"] = *input.Body.Priority
		}
		if input.Body.Tags != nil {
			updates["tags"] = tables.StringArray(*input.Body.Tags)
		}
		if input.Body.DueDate != nil {
			updates["due_date"] = *input.Body.DueDate
		}

		task, err := taskService.UpdateTask(ctx, input.ID, updates)
		if err != nil {
			return nil, mapTaskError(err)
		}

		resp := h.NewItemResponse(*task)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "task-update"
		op.Summary = "更新任务"
		op.Tags = []string{taskTag}
	})

	huma.Post(group, "/tasks/{id}/delete", func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*h.MessageResponse, error) {
		if err := taskService.DeleteTask(ctx, input.ID); err != nil {
			return nil, mapTaskError(err)
		}

		resp := h.NewMessageResponse("Task deleted successfully")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "task-delete"
		op.Summary = "删除任务"
		op.Tags = []string{taskTag}
	})

	huma.Post(group, "/tasks/{id}/move", func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body moveTaskBody
	}) (*h.ItemResponse[tables.TaskTable], error) {
		task, err := taskService.MoveTask(ctx, input.ID, &model.MoveTaskRequest{
			Status:     input.Body.Status,
			OrderIndex: input.Body.OrderIndex,
			WorktreeID: input.Body.WorktreeID,
		})
		if err != nil {
			return nil, mapTaskError(err)
		}

		resp := h.NewItemResponse(*task)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "task-move"
		op.Summary = "移动任务"
		op.Tags = []string{taskTag}
	})

	huma.Post(group, "/tasks/{id}/bind-worktree", func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body bindWorktreeBody
	}) (*h.ItemResponse[tables.TaskTable], error) {
		task, err := taskService.BindWorktree(ctx, input.ID, input.Body.WorktreeID)
		if err != nil {
			return nil, mapTaskError(err)
		}

		resp := h.NewItemResponse(*task)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "task-bind-worktree"
		op.Summary = "绑定/解绑 Worktree"
		op.Tags = []string{taskTag}
	})

	huma.Get(group, "/tasks/{id}/comments", func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*h.ItemsResponse[tables.TaskCommentTable], error) {
		items, err := commentService.ListComments(ctx, input.ID)
		if err != nil {
			return nil, mapTaskError(err)
		}
		resp := h.NewItemsResponse(items)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "task-comment-list"
		op.Summary = "评论列表"
		op.Tags = []string{taskCommentTag}
	})

	huma.Post(group, "/tasks/{id}/comments/create", func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body createCommentBody
	}) (*h.ItemResponse[tables.TaskCommentTable], error) {
		comment, err := commentService.CreateComment(ctx, input.ID, input.Body.Content)
		if err != nil {
			return nil, mapTaskError(err)
		}

		resp := h.NewItemResponse(*comment)
		resp.Status = http.StatusCreated
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "task-comment-create"
		op.Summary = "新增评论"
		op.Tags = []string{taskCommentTag}
	})

	huma.Post(group, "/task-comments/{id}", func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*h.MessageResponse, error) {
		if err := commentService.DeleteComment(ctx, input.ID); err != nil {
			return nil, mapTaskError(err)
		}
		resp := h.NewMessageResponse("Comment deleted successfully")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "task-comment-delete"
		op.Summary = "删除评论"
		op.Tags = []string{taskCommentTag}
	})
}

func mapTaskError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, model.ErrDBNotInitialized):
		return huma.Error503ServiceUnavailable("database is not initialized")
	case errors.Is(err, model.ErrProjectNotFound),
		errors.Is(err, model.ErrTaskNotFound),
		errors.Is(err, model.ErrWorktreeNotFound),
		errors.Is(err, model.ErrTaskCommentNotFound):
		return huma.Error404NotFound(err.Error())
	case errors.Is(err, model.ErrInvalidTaskStatus),
		errors.Is(err, model.ErrInvalidProjectInput):
		return huma.Error400BadRequest(err.Error())
	default:
		return huma.Error500InternalServerError(err.Error())
	}
}
