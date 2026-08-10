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

func registerSystemWorkTimingBackfillRoutes(group *huma.Group, manager *websession.Manager) {
	huma.Get(group, "/system/web-session-work-timing-backfill/status", func(
		ctx context.Context,
		input *struct{},
	) (*h.ItemResponse[websession.WorkTimingBackfillStatus], error) {
		if manager == nil {
			return nil, huma.Error503ServiceUnavailable("web session manager is not available")
		}
		item, err := manager.WorkTimingBackfillStatus(ctx)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to load work timing backfill status", err)
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-web-session-work-timing-backfill-status"
		op.Summary = "获取会话工作时间补算状态"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/web-session-work-timing-backfill/run", func(
		ctx context.Context,
		input *struct {
			Body websession.WorkTimingBackfillParams `json:"body"`
		},
	) (*h.ItemResponse[websession.WorkTimingBackfillResult], error) {
		if manager == nil {
			return nil, huma.Error503ServiceUnavailable("web session manager is not available")
		}
		item, err := manager.RunWorkTimingBackfill(ctx, input.Body)
		if err != nil {
			switch {
			case errors.Is(err, websession.ErrInvalidWorkTimingBackfill):
				return nil, huma.Error400BadRequest("invalid work timing backfill request")
			case errors.Is(err, websession.ErrWorkTimingBackfillBusy):
				return nil, huma.Error409Conflict("work timing backfill is already running")
			case errors.Is(err, model.ErrDBNotInitialized):
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			default:
				return nil, huma.Error500InternalServerError("failed to run work timing backfill", err)
			}
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-web-session-work-timing-backfill-run"
		op.Summary = "执行会话工作时间补算"
		op.Tags = []string{systemTag}
	})
}
