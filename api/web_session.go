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
	"code-kanban/service"
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
	manager      *websession.Manager
	logger       *zap.Logger
	upgrader     websocket.Upgrader
	openExplorer func(string) error
}

type webSessionCountsResponse struct {
	Status int `json:"-"`
	Body   struct {
		Counts map[string]int `json:"counts" doc:"项目ID到会话数量的映射"`
	} `json:"body"`
}

type piTreeNavigateBody struct {
	TargetID  string `json:"targetId" minLength:"1"`
	Revision  string `json:"revision" minLength:"1"`
	Summarize *bool  `json:"summarize,omitempty"`
}

func registerWebSessionRoutes(app *fiber.App, group *huma.Group, manager *websession.Manager, logger *zap.Logger) {
	ctrl := &webSessionController{
		manager: manager,
		logger:  logger.Named("web-session-controller"),
		upgrader: websocket.Upgrader{
			ReadBufferSize:    32 * 1024,
			WriteBufferSize:   32 * 1024,
			EnableCompression: true,
			CheckOrigin:       func(r *http.Request) bool { return true },
		},
	}

	ctrl.registerHTTP(app, group)
	ctrl.registerWebsocket(app)
}

func (c *webSessionController) registerHTTP(app *fiber.App, group *huma.Group) {
	huma.Get(group, "/projects/{projectId}/agent-trust/pi", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
		},
	) (*h.ItemResponse[service.ProjectAgentTrustStatus], error) {
		item, err := c.manager.GetProjectPiTrust(ctx, input.ProjectID)
		if err != nil {
			switch {
			case errors.Is(err, model.ErrProjectNotFound):
				return nil, huma.Error404NotFound("project not found")
			case errors.Is(err, model.ErrDBNotInitialized):
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			default:
				return nil, huma.Error500InternalServerError("failed to load Pi project trust", err)
			}
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "project-agent-trust-pi-get"
		op.Summary = "获取项目 Pi 授权状态"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/projects/{projectId}/agent-trust/pi", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
		},
	) (*h.ItemResponse[service.ProjectAgentTrustStatus], error) {
		item, err := c.manager.TrustProjectForPi(ctx, input.ProjectID)
		if err != nil {
			switch {
			case errors.Is(err, model.ErrProjectNotFound):
				return nil, huma.Error404NotFound("project not found")
			case errors.Is(err, model.ErrDBNotInitialized):
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			default:
				return nil, huma.Error400BadRequest(err.Error())
			}
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "project-agent-trust-pi-create"
		op.Summary = "授权项目加载 Pi 本地资源"
		op.Tags = []string{webSessionTag}
	})

	huma.Delete(group, "/projects/{projectId}/agent-trust/pi", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
		},
	) (*h.ItemResponse[service.ProjectAgentTrustStatus], error) {
		item, err := c.manager.RevokeProjectPiTrust(ctx, input.ProjectID)
		if err != nil {
			switch {
			case errors.Is(err, model.ErrProjectNotFound):
				return nil, huma.Error404NotFound("project not found")
			case errors.Is(err, model.ErrDBNotInitialized):
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			default:
				return nil, huma.Error500InternalServerError("failed to revoke Pi project trust", err)
			}
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "project-agent-trust-pi-delete"
		op.Summary = "撤销项目 Pi 授权"
		op.Tags = []string{webSessionTag}
	})

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
			ProjectID     string `path:"projectId"`
			SessionID     string `path:"sessionId"`
			Limit         int    `query:"limit" default:"80"`
			KnownRevision string `query:"knownRevision"`
		},
	) (*h.ItemResponse[websession.SessionSnapshotResponse], error) {
		record, err := c.manager.GetSession(ctx, input.SessionID)
		if err != nil || record.ProjectID != input.ProjectID {
			return nil, huma.Error404NotFound("session not found")
		}
		item, err := c.manager.SnapshotWithAutoSyncIfChanged(
			ctx,
			input.SessionID,
			input.Limit,
			input.KnownRevision,
		)
		if err != nil {
			if errors.Is(err, websession.ErrSessionHistoryUnavailable) {
				return nil, huma.Error404NotFound("session history not found")
			}
			return nil, huma.Error400BadRequest(err.Error())
		}
		item = websession.SessionSnapshotResponseForTransport(item)
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-snapshot"
		op.Summary = "获取会话快照"
		op.Tags = []string{webSessionTag}
	})

	huma.Get(group, "/projects/{projectId}/web-sessions/{sessionId}/tree", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
		},
	) (*h.ItemResponse[websession.PiTreeSnapshot], error) {
		if err := c.requireProjectSession(ctx, input.ProjectID, input.SessionID); err != nil {
			return nil, err
		}
		item, err := c.manager.GetPiSessionTree(ctx, input.SessionID)
		if err != nil {
			return nil, piTreeHTTPError(err)
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-tree-get"
		op.Summary = "获取 Pi 会话历史树"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/projects/{projectId}/web-sessions/{sessionId}/tree/navigate", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
			Body      piTreeNavigateBody
		},
	) (*h.ItemResponse[websession.PiTreeNavigateResult], error) {
		if err := c.requireProjectSession(ctx, input.ProjectID, input.SessionID); err != nil {
			return nil, err
		}
		item, err := c.manager.NavigatePiSessionTree(ctx, input.SessionID, websession.PiTreeNavigateInput{
			TargetID: input.Body.TargetID, Revision: input.Body.Revision,
			Summarize: input.Body.Summarize != nil && *input.Body.Summarize,
		})
		if err != nil {
			return nil, piTreeHTTPError(err)
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-tree-navigate"
		op.Summary = "切换 Pi 会话历史分支"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/projects/{projectId}/web-sessions/{sessionId}/tree/fork", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
			Body      websession.PiTreeForkInput
		},
	) (*h.ItemResponse[websession.PiTreeCreateResult], error) {
		if err := c.requireProjectSession(ctx, input.ProjectID, input.SessionID); err != nil {
			return nil, err
		}
		item, err := c.manager.ForkPiSessionTree(ctx, input.SessionID, input.Body)
		if err != nil {
			return nil, piTreeHTTPError(err)
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusCreated
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-tree-fork"
		op.Summary = "从 Pi 历史节点创建新会话"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/projects/{projectId}/web-sessions/{sessionId}/tree/clone", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
			Body      websession.PiTreeCloneInput
		},
	) (*h.ItemResponse[websession.PiTreeCreateResult], error) {
		if err := c.requireProjectSession(ctx, input.ProjectID, input.SessionID); err != nil {
			return nil, err
		}
		item, err := c.manager.ClonePiSessionTree(ctx, input.SessionID, input.Body)
		if err != nil {
			return nil, piTreeHTTPError(err)
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusCreated
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-tree-clone"
		op.Summary = "克隆当前 Pi 会话分支"
		op.Tags = []string{webSessionTag}
	})

	huma.Get(group, "/projects/{projectId}/web-sessions/{sessionId}/history", func(
		ctx context.Context,
		input *struct {
			ProjectID    string `path:"projectId"`
			SessionID    string `path:"sessionId"`
			BeforeCursor string `query:"beforeCursor"`
			AfterCursor  string `query:"afterCursor"`
			Limit        int    `query:"limit" default:"80"`
		},
	) (*h.ItemResponse[websession.HistoryWindow], error) {
		record, err := c.manager.GetSession(ctx, input.SessionID)
		if err != nil || record.ProjectID != input.ProjectID {
			return nil, huma.Error404NotFound("session not found")
		}
		beforeCursor := strings.TrimSpace(input.BeforeCursor)
		afterCursor := strings.TrimSpace(input.AfterCursor)
		if beforeCursor != "" && afterCursor != "" {
			return nil, huma.Error400BadRequest("history cursors are mutually exclusive")
		}
		var beforeSeq *int64
		if beforeCursor != "" {
			value, parseErr := strconv.ParseInt(beforeCursor, 10, 64)
			if parseErr != nil {
				return nil, huma.Error400BadRequest("invalid history cursor")
			}
			beforeSeq = &value
		}
		var item websession.HistoryWindow
		if afterCursor != "" {
			afterSeq, parseErr := strconv.ParseInt(afterCursor, 10, 64)
			if parseErr != nil {
				return nil, huma.Error400BadRequest("invalid history cursor")
			}
			item, err = c.manager.HistoryAfter(ctx, input.SessionID, input.Limit, afterSeq)
		} else {
			item, err = c.manager.History(ctx, input.SessionID, input.Limit, beforeSeq)
		}
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		item = websession.HistoryWindowForTransport(item)
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-history"
		op.Summary = "获取会话历史分页"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/projects/{projectId}/web-sessions/{sessionId}/work-timing/calculate", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
		},
	) (*h.ItemResponse[websession.WorkTimingCalculationResult], error) {
		record, err := c.manager.GetSession(ctx, input.SessionID)
		if err != nil || record.ProjectID != input.ProjectID {
			return nil, huma.Error404NotFound("session not found")
		}
		item, err := c.manager.CalculateSessionWorkTiming(ctx, input.SessionID)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to calculate session work timing", err)
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-work-timing-calculate"
		op.Summary = "按需补算会话工作时间"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/projects/{projectId}/web-sessions/{sessionId}/search", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
			Body      struct {
				Query            string `json:"query"`
				IncludeUser      bool   `json:"includeUser"`
				IncludeAssistant bool   `json:"includeAssistant"`
				IncludeTools     bool   `json:"includeTools"`
				IncludeSystem    bool   `json:"includeSystem"`
				SourceThreadID   string `json:"sourceThreadId,omitempty"`
				Cursor           string `json:"cursor,omitempty"`
				Limit            int    `json:"limit,omitempty"`
			}
		},
	) (*h.ItemResponse[websession.SessionConversationSearchResult], error) {
		record, err := c.manager.GetSession(ctx, input.SessionID)
		if err != nil || record.ProjectID != input.ProjectID {
			return nil, huma.Error404NotFound("session not found")
		}
		item, err := c.manager.SearchSessionConversation(
			ctx,
			input.SessionID,
			input.Body.Query,
			input.Body.IncludeUser,
			input.Body.IncludeAssistant,
			input.Body.IncludeTools,
			input.Body.IncludeSystem,
			input.Body.SourceThreadID,
			input.Body.Cursor,
			input.Body.Limit,
		)
		if err != nil {
			if errors.Is(err, websession.ErrInvalidSessionConversationSearchCursor) {
				return nil, huma.Error400BadRequest(err.Error())
			}
			return nil, huma.Error500InternalServerError("failed to search session conversation", err)
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-conversation-search"
		op.Summary = "搜索当前会话的用户、助手、工具和系统内容"
		op.Tags = []string{webSessionTag}
	})

	huma.Get(group, "/projects/{projectId}/web-sessions/import-sources", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
		},
	) (*h.ItemResponse[websession.ImportSourceList], error) {
		item, err := c.manager.ListImportSources(ctx, input.ProjectID)
		if err != nil {
			switch {
			case errors.Is(err, model.ErrProjectNotFound):
				return nil, huma.Error404NotFound("project not found")
			default:
				return nil, huma.Error500InternalServerError("failed to list import sources", err)
			}
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-import-sources"
		op.Summary = "获取 AI 会话导入源列表"
		op.Tags = []string{webSessionTag}
	})

	huma.Get(group, "/web-sessions/runtime-config", func(
		ctx context.Context,
		input *struct {
			Refresh bool `query:"refresh" default:"false" doc:"强制刷新运行时能力"`
		},
	) (*h.ItemResponse[websession.WebSessionRuntimeConfig], error) {
		var config websession.WebSessionRuntimeConfig
		if input.Refresh {
			config = c.manager.RefreshWebSessionRuntimeConfigWithModels()
		} else {
			config = c.manager.GetWebSessionRuntimeConfigWithModels()
		}
		resp := h.NewItemResponse(config)
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
				WorktreeID                        string  `json:"worktreeId"`
				Agent                             string  `json:"agent"`
				ClaudeRuntime                     string  `json:"claudeRuntime"`
				Model                             string  `json:"model"`
				ReasoningEffort                   string  `json:"reasoningEffort"`
				WorkflowMode                      string  `json:"workflowMode"`
				PermissionLevel                   string  `json:"permissionLevel"`
				ActiveCallTimeoutEnabled          *bool   `json:"activeCallTimeoutEnabled,omitempty"`
				AutoRetryEnabled                  bool    `json:"autoRetryEnabled"`
				AutoRetryPolicyMode               *string `json:"autoRetryPolicyMode,omitempty"`
				AutoRetryScope                    *string `json:"autoRetryScope,omitempty"`
				AutoRetryPreset                   *string `json:"autoRetryPreset,omitempty"`
				AutoRetryMaxAttempts              *int    `json:"autoRetryMaxAttempts,omitempty"`
				AutoRetryDispatchPendingOnFailure *bool   `json:"autoRetryDispatchPendingOnFailure,omitempty"`
				PermissionMode                    string  `json:"permissionMode,omitempty"`
				Title                             string  `json:"title"`
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
			ProjectID:                         input.ProjectID,
			WorktreeID:                        input.Body.WorktreeID,
			Agent:                             websession.Agent(input.Body.Agent),
			ClaudeRuntime:                     websession.ClaudeRuntime(input.Body.ClaudeRuntime),
			Model:                             input.Body.Model,
			ReasoningEffort:                   websession.ReasoningEffort(input.Body.ReasoningEffort),
			WorkflowMode:                      workflowMode,
			PermissionLevel:                   permissionLevel,
			ActiveCallTimeoutEnabled:          input.Body.ActiveCallTimeoutEnabled,
			AutoRetryEnabled:                  input.Body.AutoRetryEnabled,
			AutoRetryPolicyMode:               mapOptionalString[websession.AutoRetryPolicyMode](input.Body.AutoRetryPolicyMode),
			AutoRetryScope:                    mapOptionalString[websession.AutoRetryScope](input.Body.AutoRetryScope),
			AutoRetryPreset:                   mapOptionalString[websession.AutoRetryPreset](input.Body.AutoRetryPreset),
			AutoRetryMaxAttempts:              input.Body.AutoRetryMaxAttempts,
			AutoRetryDispatchPendingOnFailure: input.Body.AutoRetryDispatchPendingOnFailure,
			Title:                             input.Body.Title,
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

	huma.Post(group, "/projects/{projectId}/web-sessions/{sessionId}/messages/{itemId}/edit", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
			ItemID    string `path:"itemId"`
			Body      struct {
				Text string `json:"text"`
			}
		},
	) (*h.ItemResponse[websession.SessionHydrationTarget], error) {
		record, err := c.manager.GetSession(ctx, input.SessionID)
		if err != nil || record.ProjectID != input.ProjectID {
			return nil, huma.Error404NotFound("session not found")
		}
		item, err := c.manager.EditUserMessage(ctx, input.SessionID, input.ItemID, input.Body.Text)
		if err != nil {
			switch {
			case errors.Is(err, websession.ErrMessageEditTargetNotFound):
				return nil, huma.Error404NotFound(err.Error())
			case errors.Is(err, websession.ErrMessageEditSessionActive),
				errors.Is(err, websession.ErrMessageEditHistoryConflict):
				return nil, huma.Error409Conflict(err.Error())
			case errors.Is(err, websession.ErrMessageEditUnsupported),
				errors.Is(err, websession.ErrMessageEditForkUnavailable),
				errors.Is(err, websession.ErrMessageEditSteeredMessage),
				errors.Is(err, websession.ErrMessageEditEmpty),
				errors.Is(err, websession.ErrCodexWebSessionUnavailable):
				return nil, huma.Error400BadRequest(err.Error())
			default:
				return nil, huma.Error500InternalServerError("failed to edit user message", err)
			}
		}
		resp := h.NewItemResponse(websession.NewSessionHydrationTarget(item.Session))
		resp.Status = http.StatusCreated
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-edit-user-message"
		op.Summary = "编辑用户消息并创建 Codex 分支"
		op.Tags = []string{webSessionTag}
	})

	huma.Post(group, "/projects/{projectId}/web-sessions/import", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			Body      struct {
				Agent       string `json:"agent,omitempty"`
				AISessionID string `json:"aiSessionId"`
				SessionID   string `json:"sessionId,omitempty"`
				Mode        string `json:"mode,omitempty"`
			}
		},
	) (*h.ItemResponse[websession.ImportHydrationTarget], error) {
		var (
			item websession.ImportResult
			err  error
		)
		agent := websession.Agent(strings.ToLower(strings.TrimSpace(input.Body.Agent)))
		if agent == "" {
			agent = websession.AgentCodex
		}
		switch agent {
		case websession.AgentPi:
			if strings.TrimSpace(input.Body.SessionID) != "" {
				item, err = c.manager.ImportPiSessionBySessionID(ctx, input.ProjectID, input.Body.SessionID)
			} else {
				item, err = c.manager.ImportPiSession(ctx, input.ProjectID, input.Body.AISessionID)
			}
		case websession.AgentCodex:
			if strings.TrimSpace(input.Body.SessionID) != "" {
				item, err = c.manager.ImportCodexSessionBySessionID(
					ctx, input.ProjectID, input.Body.SessionID, websession.SyncMode(input.Body.Mode),
				)
			} else {
				item, err = c.manager.ImportCodexSession(
					ctx, input.ProjectID, input.Body.AISessionID, websession.SyncMode(input.Body.Mode),
				)
			}
		default:
			return nil, huma.Error400BadRequest("unsupported import agent")
		}
		if err != nil {
			switch {
			case errors.Is(err, model.ErrProjectNotFound):
				return nil, huma.Error404NotFound("project not found")
			case errors.Is(err, gorm.ErrRecordNotFound):
				return nil, huma.Error404NotFound("agent session not found")
			default:
				return nil, huma.Error400BadRequest(err.Error())
			}
		}
		resp := h.NewItemResponse(websession.NewImportHydrationTarget(item))
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-import"
		op.Summary = "导入 Agent 历史会话"
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
		if err := c.manager.AbortSessionForUser(input.SessionID); err != nil {
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

	huma.Post(group, "/projects/{projectId}/web-sessions/{sessionId}/app-server/terminate", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
		},
	) (*h.ItemResponse[websession.CodexAppServerTermination], error) {
		item, err := c.manager.ForceTerminateCodexAppServer(input.ProjectID, input.SessionID)
		if err != nil {
			switch {
			case errors.Is(err, websession.ErrCodexAppServerProjectMismatch):
				return nil, huma.Error404NotFound("session not found")
			case errors.Is(err, websession.ErrCodexAppServerNotActive):
				return nil, huma.Error409Conflict(err.Error())
			default:
				return nil, huma.Error500InternalServerError("failed to terminate Codex app-server", err)
			}
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-codex-app-server-terminate"
		op.Summary = "强制终止会话的 Codex app-server 进程树"
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
	) (*h.ItemResponse[websession.SessionHydrationTarget], error) {
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
		resp := h.NewItemResponse(websession.NewSessionHydrationTarget(item.Session))
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
				Query      string   `json:"query,omitempty"`
				Offset     int      `json:"offset"`
				Limit      int      `json:"limit"`
			}
		},
	) (*h.ItemResponse[websession.ArchivedQueryResult], error) {
		item, err := c.manager.ListArchivedSessions(
			ctx,
			input.Body.ProjectIDs,
			input.Body.Query,
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

	huma.Post(group, "/web-sessions/search", func(
		ctx context.Context,
		input *struct {
			Body struct {
				ProjectIDs      []string `json:"projectIds"`
				Query           string   `json:"query"`
				IncludeArchived bool     `json:"includeArchived"`
				IncludeBody     *bool    `json:"includeBody,omitempty"`
				Cursor          string   `json:"cursor,omitempty"`
				ScanLimit       int      `json:"scanLimit,omitempty"`
			}
		},
	) (*h.ItemResponse[websession.SessionSearchChunkResult], error) {
		includeBody := input.Body.IncludeBody == nil || *input.Body.IncludeBody
		item, err := c.manager.SearchSessionsChunk(
			ctx,
			input.Body.ProjectIDs,
			input.Body.Query,
			input.Body.IncludeArchived,
			includeBody,
			input.Body.Cursor,
			input.Body.ScanLimit,
		)
		if err != nil {
			if errors.Is(err, websession.ErrInvalidSessionSearchCursor) {
				return nil, huma.Error400BadRequest(err.Error())
			}
			return nil, huma.Error500InternalServerError("failed to search web sessions", err)
		}
		resp := h.NewItemResponse(item)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-search"
		op.Summary = "渐进搜索会话标题、预览和可选正文"
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

	huma.Post(group, "/projects/{projectId}/web-sessions/attachments/import-url", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			Body      struct {
				URL string `json:"url" minLength:"1" maxLength:"4096"`
			}
		},
	) (*h.ItemResponse[websession.Attachment], error) {
		if strings.TrimSpace(input.ProjectID) == "" {
			return nil, huma.Error400BadRequest("projectId is required")
		}
		attachment, err := c.manager.ImportRemoteAttachment(ctx, input.Body.URL)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		resp := h.NewItemResponse(attachment)
		resp.Status = http.StatusCreated
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "web-session-attachment-import-url"
		op.Summary = "下载远程图片为会话附件"
		op.Tags = []string{webSessionTag}
	})

	app.Post("/api/v1/projects/:projectId/web-sessions/attachments/import-clipboard", func(ctx *fiber.Ctx) error {
		if remoteIP := ctx.Context().RemoteIP(); remoteIP == nil || !remoteIP.IsLoopback() {
			return fiber.NewError(http.StatusForbidden, "local clipboard import requires a loopback connection")
		}
		projectID := strings.TrimSpace(ctx.Params("projectId"))
		if projectID == "" {
			return fiber.NewError(http.StatusBadRequest, "projectId is required")
		}
		input := struct {
			Source string `json:"source"`
		}{}
		if err := ctx.BodyParser(&input); err != nil {
			return fiber.NewError(http.StatusBadRequest, "invalid clipboard import request")
		}
		input.Source = strings.TrimSpace(input.Source)
		if input.Source == "" || len(input.Source) > 4096 {
			return fiber.NewError(http.StatusBadRequest, "clipboard image source is required")
		}
		attachment, err := c.manager.ImportLocalClipboardAttachment(ctx.UserContext(), input.Source)
		if err != nil {
			return fiber.NewError(http.StatusBadRequest, err.Error())
		}
		resp := h.NewItemResponse(attachment)
		resp.Status = http.StatusCreated
		return ctx.Status(http.StatusCreated).JSON(resp.Body)
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
	c.registerLocalFileRoutes(app)

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

func mapOptionalString[T ~string](value *string) *T {
	if value == nil {
		return nil
	}
	mapped := T(*value)
	return &mapped
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
	return sendWebSessionFileStream(ctx, resolvedPath, info.Size())
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

func (c *webSessionController) requireProjectSession(ctx context.Context, projectID, sessionID string) error {
	record, err := c.manager.GetSession(ctx, sessionID)
	if err != nil || record.ProjectID != projectID {
		return huma.Error404NotFound("session not found")
	}
	if !c.manager.SupportsPiSessionTree() {
		return huma.Error403Forbidden("Pi session tree is not supported")
	}
	return nil
}

func piTreeHTTPError(err error) error {
	if errors.Is(err, model.ErrDBNotInitialized) {
		return huma.Error503ServiceUnavailable("database is not available")
	}
	publicErr := websession.ClassifyPiTreeError(err)
	switch publicErr.Code {
	case "conflict", "invalid_state":
		return huma.Error409Conflict(publicErr.Message)
	case "bad_req":
		return huma.Error400BadRequest(publicErr.Message)
	case "forbidden":
		return huma.Error403Forbidden(publicErr.Message)
	default:
		return huma.Error500InternalServerError(publicErr.Message)
	}
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
		if err := c.manager.EnqueueCommand(client, payload); err != nil {
			c.logger.Debug("failed to enqueue web session command", zap.Error(err))
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
