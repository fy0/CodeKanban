package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"code-kanban/api/h"
	"code-kanban/model"
	"code-kanban/service/websession"
	"code-kanban/utils"
)

type databaseHealthResult struct {
	Database model.DatabaseHealth                   `json:"database"`
	Sessions []websession.DatabaseSessionDiagnostic `json:"sessions,omitempty"`
}

func registerSystemDatabaseRoutes(group *huma.Group, cfg *utils.AppConfig, manager *websession.Manager) {
	huma.Get(group, "/system/database-health", func(
		ctx context.Context,
		_ *struct{},
	) (*h.ItemResponse[databaseHealthResult], error) {
		if cfg == nil {
			return nil, huma.Error503ServiceUnavailable("application config is not available")
		}
		health, err := model.InspectDatabase(ctx, cfg.DSN)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to inspect database", err)
		}
		result := databaseHealthResult{Database: health}
		if manager != nil {
			result.Sessions = manager.DatabaseSessionDiagnostics()
		}
		resp := h.NewItemResponse(result)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-database-health"
		op.Summary = "检查数据库健康状态"
		op.Description = "返回 SQLite 连接池、锁等待、存储空间和会话投影队列状态"
		op.Tags = []string{systemTag}
	})
}
