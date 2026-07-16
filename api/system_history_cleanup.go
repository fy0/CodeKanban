package api

import (
	"context"
	"errors"
	"net/http"

	"code-kanban/api/h"
	"code-kanban/model"
	"code-kanban/service/websession"

	"github.com/danielgtaylor/huma/v2"
)

func registerSystemHistoryCleanupRoutes(group *huma.Group, manager *websession.Manager) {
	huma.Post(group, "/system/web-session-history-cleanup/preview", func(
		ctx context.Context,
		input *struct {
			Body websession.HistoryCleanupParams `json:"body"`
		},
	) (*h.ItemResponse[websession.HistoryCleanupStats], error) {
		if manager == nil {
			return nil, huma.Error503ServiceUnavailable("web session manager is not available")
		}
		item, err := manager.PreviewHistoryCleanup(ctx, input.Body)
		if err != nil {
			if errors.Is(err, websession.ErrInvalidHistoryCleanup) {
				return nil, huma.Error400BadRequest("invalid history cleanup request")
			}
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to preview history cleanup", err)
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-web-session-history-cleanup-preview"
		op.Summary = "预览会话聊天缓存清理"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/web-session-history-cleanup/run", func(
		ctx context.Context,
		input *struct {
			Body websession.HistoryCleanupParams `json:"body"`
		},
	) (*h.ItemResponse[websession.HistoryCleanupResult], error) {
		if manager == nil {
			return nil, huma.Error503ServiceUnavailable("web session manager is not available")
		}
		item, err := manager.RunHistoryCleanup(ctx, input.Body)
		if err != nil {
			if errors.Is(err, websession.ErrInvalidHistoryCleanup) {
				return nil, huma.Error400BadRequest("invalid history cleanup request")
			}
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to run history cleanup", err)
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-web-session-history-cleanup-run"
		op.Summary = "执行会话聊天缓存清理"
		op.Tags = []string{systemTag}
	})
}
