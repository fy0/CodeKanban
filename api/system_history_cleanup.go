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
	huma.Get(group, "/system/web-session-storage-overview", func(
		ctx context.Context,
		_ *struct{},
	) (*h.ItemResponse[websession.HistoryCleanupStorageStats], error) {
		if manager == nil {
			return nil, huma.Error503ServiceUnavailable("web session manager is not available")
		}
		item, err := manager.HistoryStorageOverview(ctx)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to load web session storage overview", err)
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-web-session-storage-overview"
		op.Summary = "查看会话缓存存储概览"
		op.Description = "返回数据库文件、WAL、可复用空间和归档缓存的逻辑大小"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/web-session-history-archive/preview", func(
		ctx context.Context,
		input *struct {
			Body websession.HistoryArchiveParams `json:"body"`
		},
	) (*h.ItemResponse[websession.HistoryArchiveStats], error) {
		if manager == nil {
			return nil, huma.Error503ServiceUnavailable("web session manager is not available")
		}
		item, err := manager.PreviewHistoryArchive(ctx, input.Body)
		if err != nil {
			if errors.Is(err, websession.ErrInvalidHistoryArchive) {
				return nil, huma.Error400BadRequest("invalid history archive request")
			}
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to preview history archive", err)
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-web-session-history-archive-preview"
		op.Summary = "预览批量归档会话"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/web-session-history-archive/run", func(
		ctx context.Context,
		input *struct {
			Body websession.HistoryArchiveParams `json:"body"`
		},
	) (*h.ItemResponse[websession.HistoryArchiveResult], error) {
		if manager == nil {
			return nil, huma.Error503ServiceUnavailable("web session manager is not available")
		}
		item, err := manager.RunHistoryArchive(ctx, input.Body)
		if err != nil {
			if errors.Is(err, websession.ErrInvalidHistoryArchive) {
				return nil, huma.Error400BadRequest("invalid history archive request")
			}
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to archive web sessions", err)
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-web-session-history-archive-run"
		op.Summary = "执行批量归档会话"
		op.Tags = []string{systemTag}
	})

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
