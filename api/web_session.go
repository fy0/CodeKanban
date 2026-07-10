package api

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"code-kanban/api/h"
	"code-kanban/model"
	"code-kanban/service/websession"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gorilla/websocket"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	webSessionTag         = "web-session-会话"
	webSessionCommandPath = "/api/v1/web-sessions/ws"
	webSessionEventsPath  = "/api/v1/web-sessions/events"
)

type webSessionController struct {
	manager  *websession.Manager
	logger   *zap.Logger
	upgrader websocket.Upgrader
}

type webSessionCountsResponse struct {
	Status int `json:"-"`
	Body   struct {
		Counts map[string]int `json:"counts" doc:"项目ID到会话数量的映射"`
	} `json:"body"`
}

func registerWebSessionRoutes(app *fiber.App, group *huma.Group, manager *websession.Manager, logger *zap.Logger) {
	ctrl := &webSessionController{
		manager: manager,
		logger:  logger.Named("web-session-controller"),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  32 * 1024,
			WriteBufferSize: 32 * 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}

	ctrl.registerHTTP(app, group)
	ctrl.registerWebsocket(app)
}

func (c *webSessionController) registerHTTP(app *fiber.App, group *huma.Group) {
	huma.Get(group, "/projects/{projectId}/web-sessions", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
		},
	) (*h.ItemsResponse[websession.SessionSummary], error) {
		items, err := c.manager.ListSessions(ctx, input.ProjectID)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to list web sessions", err)
		}
		resp := h.NewItemsResponse(items)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-list"
		op.Summary = "获取会话列表"
		op.Tags = []string{webSessionTag}
	})

	huma.Get(group, "/web-sessions/counts", func(
		ctx context.Context,
		_ *struct{},
	) (*webSessionCountsResponse, error) {
		counts, err := c.manager.CountSessionsByProject(ctx)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to count web sessions", err)
		}
		resp := &webSessionCountsResponse{}
		resp.Status = http.StatusOK
		resp.Body.Counts = counts
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-counts"
		op.Summary = "获取项目会话数量"
		op.Tags = []string{webSessionTag}
	})

	huma.Get(group, "/projects/{projectId}/web-sessions/{sessionId}/snapshot", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
			Limit     int    `query:"limit" default:"80"`
		},
	) (*h.ItemResponse[websession.SessionSnapshot], error) {
		record, err := c.manager.GetSession(ctx, input.SessionID)
		if err != nil || record.ProjectID != input.ProjectID {
			return nil, huma.Error404NotFound("session not found")
		}
		item, err := c.manager.SnapshotWithAutoSync(ctx, input.SessionID, input.Limit)
		if err != nil {
			if errors.Is(err, websession.ErrSessionHistoryUnavailable) {
				return nil, huma.Error404NotFound("session history not found")
			}
			return nil, huma.Error400BadRequest(err.Error())
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-snapshot"
		op.Summary = "获取会话快照"
		op.Tags = []string{webSessionTag}
	})

	huma.Get(group, "/projects/{projectId}/web-sessions/{sessionId}/history", func(
		ctx context.Context,
		input *struct {
			ProjectID    string `path:"projectId"`
			SessionID    string `path:"sessionId"`
			BeforeCursor string `query:"beforeCursor"`
			Limit        int    `query:"limit" default:"80"`
		},
	) (*h.ItemResponse[websession.HistoryWindow], error) {
		record, err := c.manager.GetSession(ctx, input.SessionID)
		if err != nil || record.ProjectID != input.ProjectID {
			return nil, huma.Error404NotFound("session not found")
		}
		var beforeSeq *int64
		if strings.TrimSpace(input.BeforeCursor) != "" {
			value, parseErr := strconv.ParseInt(strings.TrimSpace(input.BeforeCursor), 10, 64)
			if parseErr != nil {
				return nil, huma.Error400BadRequest("invalid history cursor")
			}
			beforeSeq = &value
		}
		item, err := c.manager.History(ctx, input.SessionID, input.Limit, beforeSeq)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-history"
		op.Summary = "获取会话历史分页"
		op.Tags = []string{webSessionTag}
	})

	huma.Get(group, "/projects/{projectId}/web-sessions/import-sources", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
		},
	) (*h.ItemResponse[websession.ImportSourceList], error) {
		item, err := c.manager.ListCodexImportSources(ctx, input.ProjectID)
		if err != nil {
			switch {
			case errors.Is(err, model.ErrProjectNotFound):
				return nil, huma.Error404NotFound("project not found")
			default:
				return nil, huma.Error500InternalServerError("failed to list codex import sources", err)
			}
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-import-sources"
		op.Summary = "获取 Codex 导入源列表"
		op.Tags = []string{webSessionTag}
	})

	huma.Get(group, "/web-sessions/runtime-config", func(
		ctx context.Context,
		_ *struct{},
	) (*h.ItemResponse[websession.CodexRuntimeConfig], error) {
		resp := h.NewItemResponse(c.manager.GetCodexRuntimeConfigWithModels())
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-runtime-config"
		op.Summary = "获取网页会话运行时配置"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/projects/{projectId}/web-sessions", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			Body      struct {
				WorktreeID               string `json:"worktreeId"`
				Agent                    string `json:"agent"`
				ClaudeRuntime            string `json:"claudeRuntime"`
				Model                    string `json:"model"`
				ReasoningEffort          string `json:"reasoningEffort"`
				WorkflowMode             string `json:"workflowMode"`
				PermissionLevel          string `json:"permissionLevel"`
				ActiveCallTimeoutEnabled *bool  `json:"activeCallTimeoutEnabled,omitempty"`
				AutoRetryEnabled         bool   `json:"autoRetryEnabled"`
				AutoRetryScope           string `json:"autoRetryScope"`
				AutoRetryPreset          string `json:"autoRetryPreset"`
				PermissionMode           string `json:"permissionMode,omitempty"`
				Title                    string `json:"title"`
			}
		},
	) (*h.ItemResponse[websession.SessionSummary], error) {
		workflowMode := websession.WorkflowMode(input.Body.WorkflowMode)
		permissionLevel := websession.PermissionLevel(input.Body.PermissionLevel)
		if strings.TrimSpace(input.Body.PermissionMode) != "" {
			switch strings.ToLower(strings.TrimSpace(input.Body.PermissionMode)) {
			case "plan":
				if strings.TrimSpace(input.Body.WorkflowMode) == "" {
					workflowMode = websession.WorkflowModePlan
				}
				if strings.TrimSpace(input.Body.PermissionLevel) == "" {
					permissionLevel = websession.PermissionLevelElevated
				}
			case "yolo":
				if strings.TrimSpace(input.Body.WorkflowMode) == "" {
					workflowMode = websession.WorkflowModeDefault
				}
				if strings.TrimSpace(input.Body.PermissionLevel) == "" {
					permissionLevel = websession.PermissionLevelYolo
				}
			default:
				if strings.TrimSpace(input.Body.WorkflowMode) == "" {
					workflowMode = websession.WorkflowModeDefault
				}
				if strings.TrimSpace(input.Body.PermissionLevel) == "" {
					permissionLevel = websession.PermissionLevelElevated
				}
			}
		}
		item, err := c.manager.CreateSession(ctx, websession.CreateParams{
			ProjectID:                input.ProjectID,
			WorktreeID:               input.Body.WorktreeID,
			Agent:                    websession.Agent(input.Body.Agent),
			ClaudeRuntime:            websession.ClaudeRuntime(input.Body.ClaudeRuntime),
			Model:                    input.Body.Model,
			ReasoningEffort:          websession.ReasoningEffort(input.Body.ReasoningEffort),
			WorkflowMode:             workflowMode,
			PermissionLevel:          permissionLevel,
			ActiveCallTimeoutEnabled: input.Body.ActiveCallTimeoutEnabled,
			AutoRetryEnabled:         input.Body.AutoRetryEnabled,
			AutoRetryScope:           websession.AutoRetryScope(input.Body.AutoRetryScope),
			AutoRetryPreset:          websession.AutoRetryPreset(input.Body.AutoRetryPreset),
			Title:                    input.Body.Title,
		})
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusCreated
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-create"
		op.Summary = "创建会话"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/projects/{projectId}/web-sessions/import", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			Body      struct {
				AISessionID string `json:"aiSessionId"`
				SessionID   string `json:"sessionId,omitempty"`
				Mode        string `json:"mode,omitempty"`
			}
		},
	) (*h.ItemResponse[websession.ImportResult], error) {
		var (
			item websession.ImportResult
			err  error
		)
		if strings.TrimSpace(input.Body.SessionID) != "" {
			item, err = c.manager.ImportCodexSessionBySessionID(
				ctx,
				input.ProjectID,
				input.Body.SessionID,
				websession.SyncMode(input.Body.Mode),
			)
		} else {
			item, err = c.manager.ImportCodexSession(
				ctx,
				input.ProjectID,
				input.Body.AISessionID,
				websession.SyncMode(input.Body.Mode),
			)
		}
		if err != nil {
			switch {
			case errors.Is(err, model.ErrProjectNotFound):
				return nil, huma.Error404NotFound("project not found")
			case errors.Is(err, gorm.ErrRecordNotFound):
				return nil, huma.Error404NotFound("codex session not found")
			default:
				return nil, huma.Error400BadRequest(err.Error())
			}
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-import"
		op.Summary = "导入 Codex 历史会话"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/projects/{projectId}/web-sessions/{sessionId}/archive", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
		},
	) (*h.ItemResponse[websession.SessionSummary], error) {
		record, err := c.manager.GetSession(ctx, input.SessionID)
		if err != nil || record.ProjectID != input.ProjectID {
			return nil, huma.Error404NotFound("session not found")
		}
		item, err := c.manager.ArchiveSession(ctx, input.SessionID)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-archive"
		op.Summary = "归档会话"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/projects/{projectId}/web-sessions/{sessionId}/unarchive", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
		},
	) (*h.ItemResponse[websession.SessionSummary], error) {
		record, err := c.manager.GetSession(ctx, input.SessionID)
		if err != nil || record.ProjectID != input.ProjectID {
			return nil, huma.Error404NotFound("session not found")
		}
		item, err := c.manager.UnarchiveSession(ctx, input.SessionID)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-unarchive"
		op.Summary = "取消归档会话"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/projects/{projectId}/web-sessions/{sessionId}/rename", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
			Body      struct {
				Title string `json:"title"`
			}
		},
	) (*h.ItemResponse[websession.SessionSummary], error) {
		record, err := c.manager.GetSession(ctx, input.SessionID)
		if err != nil || record.ProjectID != input.ProjectID {
			return nil, huma.Error404NotFound("session not found")
		}
		item, err := c.manager.RenameSession(ctx, input.SessionID, input.Body.Title)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-rename"
		op.Summary = "重命名会话"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/projects/{projectId}/web-sessions/{sessionId}/close", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
		},
	) (*h.MessageResponse, error) {
		record, err := c.manager.GetSession(ctx, input.SessionID)
		if err != nil || record.ProjectID != input.ProjectID {
			return nil, huma.Error404NotFound("session not found")
		}
		if err := c.manager.AbortSession(input.SessionID); err != nil {
			return nil, huma.Error500InternalServerError("failed to abort session", err)
		}
		resp := h.NewMessageResponse("session aborted")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-close"
		op.Summary = "停止会话运行"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/projects/{projectId}/web-sessions/{sessionId}/sync", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
			Body      struct {
				Mode          string `json:"mode,omitempty"`
				ClearExisting bool   `json:"clearExisting,omitempty"`
			}
		},
	) (*h.ItemResponse[websession.SessionSnapshot], error) {
		record, err := c.manager.GetSession(ctx, input.SessionID)
		if err != nil || record.ProjectID != input.ProjectID {
			return nil, huma.Error404NotFound("session not found")
		}
		item, err := c.manager.SyncSessionWithMode(
			ctx,
			input.SessionID,
			websession.SyncMode(input.Body.Mode),
			input.Body.ClearExisting,
		)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-sync"
		op.Summary = "强制同步会话缓存"
		op.Tags = []string{webSessionTag}
	})

	huma.Delete(group, "/projects/{projectId}/web-sessions/{sessionId}", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
		},
	) (*h.MessageResponse, error) {
		record, err := c.manager.GetSession(ctx, input.SessionID)
		if err != nil || record.ProjectID != input.ProjectID {
			return nil, huma.Error404NotFound("session not found")
		}
		if err := c.manager.DeleteSession(ctx, input.SessionID); err != nil {
			return nil, huma.Error500InternalServerError("failed to delete session", err)
		}
		resp := h.NewMessageResponse("session deleted")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-delete"
		op.Summary = "删除会话"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/web-sessions/archived/query", func(
		ctx context.Context,
		input *struct {
			Body struct {
				ProjectIDs []string `json:"projectIds"`
				Offset     int      `json:"offset"`
				Limit      int      `json:"limit"`
			}
		},
	) (*h.ItemResponse[websession.ArchivedQueryResult], error) {
		item, err := c.manager.ListArchivedSessions(
			ctx,
			input.Body.ProjectIDs,
			input.Body.Limit,
			input.Body.Offset,
		)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to query archived sessions", err)
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-archived-query"
		op.Summary = "查询归档会话"
		op.Tags = []string{webSessionTag}
	})

	huma.Get(group, "/projects/{projectId}/web-sessions/{sessionId}/command-groups/{groupId}", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
			GroupID   string `path:"groupId"`
		},
	) (*h.ItemResponse[websession.CommandExecutionGroupDetail], error) {
		record, err := c.manager.GetSession(ctx, input.SessionID)
		if err != nil || record.ProjectID != input.ProjectID {
			return nil, huma.Error404NotFound("session not found")
		}

		item, err := c.manager.GetCommandExecutionGroup(ctx, input.SessionID, input.GroupID)
		if err != nil {
			if errors.Is(err, websession.ErrCommandExecutionGroupNotFound) {
				return nil, huma.Error404NotFound("tool group not found")
			}
			return nil, huma.Error500InternalServerError("failed to load tool group", err)
		}

		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-command-group-detail"
		op.Summary = "获取工具批次详情"
		op.Tags = []string{webSessionTag}
	})

	app.Post("/api/v1/projects/:projectId/web-sessions/attachments", func(ctx *fiber.Ctx) error {
		projectID := strings.TrimSpace(ctx.Params("projectId"))
		if projectID == "" {
			return fiber.NewError(http.StatusBadRequest, "projectId is required")
		}
		fileHeader, err := ctx.FormFile("file")
		if err != nil || fileHeader == nil {
			return fiber.NewError(http.StatusBadRequest, "file is required")
		}
		if !strings.HasPrefix(strings.ToLower(fileHeader.Header.Get("Content-Type")), "image/") {
			return fiber.NewError(http.StatusBadRequest, "only image attachments are supported")
		}
		attachment, err := c.manager.SaveAttachment(fileHeader)
		if err != nil {
			return fiber.NewError(http.StatusBadRequest, err.Error())
		}
		resp := h.NewItemResponse(attachment)
		resp.Status = http.StatusCreated
		return ctx.Status(http.StatusCreated).JSON(resp)
	})

	app.Get("/api/v1/web-sessions/image-view", c.serveImageViewPreview)

	app.Get("/api/v1/web-sessions/attachments/:attachmentId", func(ctx *fiber.Ctx) error {
		attachmentID := strings.TrimSpace(ctx.Params("attachmentId"))
		if attachmentID == "" {
			return fiber.NewError(http.StatusBadRequest, "attachmentId is required")
		}

		attachment, err := c.manager.GetAttachment(attachmentID)
		if err != nil {
			return fiber.NewError(http.StatusNotFound, "attachment not found")
		}
		if _, err := os.Stat(attachment.Path); err != nil {
			if os.IsNotExist(err) {
				return fiber.NewError(http.StatusNotFound, "attachment not found")
			}
			return fiber.NewError(http.StatusInternalServerError, "failed to read attachment")
		}

		if attachment.Mime != "" {
			ctx.Set(fiber.HeaderContentType, attachment.Mime)
		}
		ctx.Set(fiber.HeaderContentDisposition, "inline")
		return ctx.SendFile(attachment.Path, false)
	})
}

func (c *webSessionController) serveImageViewPreview(ctx *fiber.Ctx) error {
	resolvedPath, err := resolveWebSessionImageViewPath(ctx.Query("path"), ctx.Query("cwd"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, err.Error())
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fiber.NewError(http.StatusNotFound, "image not found")
		}
		return fiber.NewError(http.StatusInternalServerError, "failed to read image")
	}
	if !info.Mode().IsRegular() {
		return fiber.NewError(http.StatusBadRequest, "path is not a regular file")
	}

	mimeType := detectWebSessionImagePreviewMimeType(resolvedPath)
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/") {
		return fiber.NewError(http.StatusBadRequest, "path is not an image")
	}

	ctx.Set(fiber.HeaderContentDisposition, "inline")
	ctx.Set(fiber.HeaderCacheControl, "no-store")
	ctx.Set(fiber.HeaderContentType, mimeType)
	return ctx.SendFile(resolvedPath, false)
}

func resolveWebSessionImageViewPath(rawPath string, rawCwd string) (string, error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return "", errors.New("path is required")
	}
	if filepath.IsAbs(path) || looksLikeWindowsAbsolutePath(path) {
		return filepath.Clean(path), nil
	}

	cwd := strings.TrimSpace(rawCwd)
	if cwd == "" {
		return "", errors.New("cwd is required for relative paths")
	}
	if !filepath.IsAbs(cwd) && !looksLikeWindowsAbsolutePath(cwd) {
		return "", errors.New("cwd must be absolute")
	}
	return filepath.Clean(filepath.Join(cwd, path)), nil
}

func detectWebSessionImagePreviewMimeType(filePath string) string {
	extMimeType := strings.TrimSpace(mime.TypeByExtension(strings.ToLower(filepath.Ext(filePath))))
	if strings.HasPrefix(extMimeType, "image/") {
		return extMimeType
	}

	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	var header [512]byte
	readBytes, err := file.Read(header[:])
	if err != nil || readBytes <= 0 {
		return extMimeType
	}

	detected := strings.TrimSpace(http.DetectContentType(header[:readBytes]))
	if strings.HasPrefix(detected, "image/") {
		return detected
	}
	return extMimeType
}

func looksLikeWindowsAbsolutePath(value string) bool {
	if len(value) < 3 {
		return false
	}
	if value[1] != ':' {
		return false
	}
	if value[2] != '\\' && value[2] != '/' {
		return false
	}
	first := value[0]
	return (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z')
}

func (c *webSessionController) registerWebsocket(app *fiber.App) {
	commandHandler := fasthttpadaptor.NewFastHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.serveCommandWebsocket(w, r)
	}))
	eventHandler := fasthttpadaptor.NewFastHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.serveEventWebsocket(w, r)
	}))
	app.Get(webSessionCommandPath, func(ctx *fiber.Ctx) error {
		commandHandler(ctx.Context())
		return nil
	})
	app.Get(webSessionEventsPath, func(ctx *fiber.Ctx) error {
		eventHandler(ctx.Context())
		return nil
	})
}

func (c *webSessionController) serveCommandWebsocket(w http.ResponseWriter, r *http.Request) {
	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		c.logger.Debug("failed to upgrade web session ws", zap.Error(err))
		return
	}
	defer conn.Close()

	client := c.manager.RegisterCommandClient(conn)
	defer c.manager.UnregisterClient(client)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) &&
				!errors.Is(err, context.Canceled) {
				c.logger.Debug("web session ws read failed", zap.Error(err))
			}
			return
		}
		client.MarkSeen()
		handled, heartbeatErr := c.manager.HandleHeartbeatPayload(client, payload)
		if handled {
			if heartbeatErr != nil {
				c.logger.Debug("failed to handle web session heartbeat", zap.Error(heartbeatErr))
				return
			}
			continue
		}
		if err := c.manager.HandleCommand(ctx, client, payload); err != nil {
			c.logger.Debug("failed to handle web session command", zap.Error(err))
		}
	}
}

func (c *webSessionController) serveEventWebsocket(w http.ResponseWriter, r *http.Request) {
	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		c.logger.Debug("failed to upgrade web session event ws", zap.Error(err))
		return
	}
	defer conn.Close()

	client := c.manager.RegisterEventClient(conn)
	defer c.manager.UnregisterClient(client)

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) &&
				!errors.Is(err, context.Canceled) {
				c.logger.Debug("web session event ws read failed", zap.Error(err))
			}
			return
		}
		client.MarkSeen()
		handled, heartbeatErr := c.manager.HandleHeartbeatPayload(client, payload)
		if handled {
			if heartbeatErr != nil {
				c.logger.Debug("failed to handle web session event heartbeat", zap.Error(heartbeatErr))
				return
			}
			continue
		}
	}
}
