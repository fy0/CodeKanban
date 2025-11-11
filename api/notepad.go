package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"go-template/api/h"
	"go-template/model"
	"go-template/model/tables"
)

const (
	notepadTag = "notepad-记事板"
)

type createNotePadBody struct {
	ProjectID *string `json:"projectId,omitempty" doc:"项目ID（为空表示全局笔记）"`
	Name      string  `json:"name" doc:"标签页名称"`
	Content   string  `json:"content" doc:"内容"`
}

type updateNotePadBody struct {
	Name    *string `json:"name,omitempty" doc:"标签页名称"`
	Content *string `json:"content,omitempty" doc:"内容"`
}

type moveNotePadBody struct {
	OrderIndex float64 `json:"orderIndex" doc:"排序索引"`
}

func registerNotePadRoutes(group *huma.Group) {
	service := &model.NotePadService{}

	huma.Post(group, "/notepads", func(ctx context.Context, input *struct {
		Body createNotePadBody
	}) (*h.ItemResponse[tables.NotePadTable], error) {
		notepad, err := service.CreateNotePad(ctx, &model.CreateNotePadRequest{
			ProjectID: input.Body.ProjectID,
			Name:      input.Body.Name,
			Content:   input.Body.Content,
		})
		if err != nil {
			return nil, mapNotePadError(err)
		}

		resp := h.NewItemResponse(*notepad)
		resp.Status = http.StatusCreated
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "notepad-create"
		op.Summary = "创建记事板标签"
		op.Tags = []string{notepadTag}
	})

	huma.Get(group, "/notepads", func(ctx context.Context, input *struct {
		ProjectID string `query:"projectId,omitempty" doc:"项目ID（为空表示全局笔记）"`
	}) (*h.ItemsResponse[tables.NotePadTable], error) {
		var projectID *string
		// 如果提供了projectId且不为空，则查询项目笔记；否则查询全局笔记
		if input.ProjectID != "" {
			projectID = &input.ProjectID
		}

		notepads, err := service.ListNotePads(ctx, projectID)
		if err != nil {
			return nil, mapNotePadError(err)
		}

		resp := h.NewItemsResponse(notepads)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "notepad-list"
		op.Summary = "获取记事板标签"
		op.Tags = []string{notepadTag}
	})

	huma.Get(group, "/notepads/{id}", func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*h.ItemResponse[tables.NotePadTable], error) {
		notepad, err := service.GetNotePad(ctx, input.ID)
		if err != nil {
			return nil, mapNotePadError(err)
		}

		resp := h.NewItemResponse(*notepad)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "notepad-get"
		op.Summary = "获取记事板标签详情"
		op.Tags = []string{notepadTag}
	})

	huma.Patch(group, "/notepads/{id}", func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body updateNotePadBody
	}) (*h.ItemResponse[tables.NotePadTable], error) {
		notepad, err := service.UpdateNotePad(ctx, input.ID, &model.UpdateNotePadRequest{
			Name:    input.Body.Name,
			Content: input.Body.Content,
		})
		if err != nil {
			return nil, mapNotePadError(err)
		}

		resp := h.NewItemResponse(*notepad)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "notepad-update"
		op.Summary = "更新记事板标签"
		op.Tags = []string{notepadTag}
	})

	huma.Delete(group, "/notepads/{id}", func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*h.MessageResponse, error) {
		if err := service.DeleteNotePad(ctx, input.ID); err != nil {
			return nil, mapNotePadError(err)
		}

		resp := h.NewMessageResponse("notepad deleted successfully")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "notepad-delete"
		op.Summary = "删除记事板标签"
		op.Tags = []string{notepadTag}
	})

	huma.Post(group, "/notepads/{id}/move", func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body moveNotePadBody
	}) (*h.ItemResponse[tables.NotePadTable], error) {
		notepad, err := service.MoveNotePad(ctx, input.ID, input.Body.OrderIndex)
		if err != nil {
			return nil, mapNotePadError(err)
		}

		resp := h.NewItemResponse(*notepad)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "notepad-move"
		op.Summary = "移动记事板标签顺序"
		op.Tags = []string{notepadTag}
	})
}

func mapNotePadError(err error) error {
	switch {
	case errors.Is(err, model.ErrDBNotInitialized):
		return huma.Error503ServiceUnavailable("database not initialized")
	case errors.Is(err, model.ErrNotePadNotFound):
		return huma.Error404NotFound("notepad not found")
	default:
		return huma.Error500InternalServerError("internal server error", err)
	}
}
