package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"code-kanban/api/h"
	"code-kanban/model"
	"code-kanban/service"
)

const aiSessionTag = "ai-session-AI会话"

func registerAISessionRoutes(app *fiber.App, group *huma.Group) {
	svc := service.NewAISessionService()

	app.Get("/api/v1/ai-sessions/by-session-id/:sessionId/conversation/images/:attachmentId", func(c *fiber.Ctx) error {
		preview, err := svc.GetConversationImagePreviewBySessionID(c.UserContext(), c.Params("sessionId"), c.Params("attachmentId"))
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return fiber.NewError(http.StatusServiceUnavailable, "database is not initialized")
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fiber.ErrNotFound
			}
			return fiber.NewError(http.StatusNotFound, "image preview not found")
		}

		c.Set(fiber.HeaderContentDisposition, "inline")
		if preview.MimeType != "" {
			c.Set(fiber.HeaderContentType, preview.MimeType)
		}
		if preview.FilePath != "" {
			return c.SendFile(preview.FilePath)
		}
		return c.Send(preview.Data)
	})

	// Get AI sessions for a project
	huma.Get(group, "/projects/{id}/ai-sessions", func(ctx context.Context, input *struct {
		ID string `path:"id" doc:"项目ID"`
	}) (*h.ItemResponse[service.ProjectAISessions], error) {
		// First get the project to get its path
		projectService := model.NewProjectService()
		project, err := projectService.GetProject(ctx, input.ID)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			if errors.Is(err, model.ErrProjectNotFound) {
				return nil, huma.Error404NotFound("project not found")
			}
			return nil, huma.Error500InternalServerError("failed to get project", err)
		}

		sessions, err := svc.GetProjectAISessions(ctx, project.Path)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to get AI sessions", err)
		}

		resp := h.NewItemResponse(*sessions)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "ai-session-list-by-project"
		op.Summary = "获取项目的AI助手会话列表"
		op.Description = "返回指定项目目录下的 Claude Code 和 Codex 会话信息"
		op.Tags = []string{aiSessionTag}
	})

	// Get AI sessions by path (for cases where we don't have a project ID)
	huma.Post(group, "/ai-sessions/by-path", func(ctx context.Context, input *struct {
		Body struct {
			Path string `json:"path" minLength:"1" doc:"项目目录路径"`
		}
	}) (*h.ItemResponse[service.ProjectAISessions], error) {
		sessions, err := svc.GetProjectAISessions(ctx, input.Body.Path)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to get AI sessions", err)
		}

		resp := h.NewItemResponse(*sessions)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "ai-session-list-by-path"
		op.Summary = "通过路径获取AI助手会话列表"
		op.Description = "根据目录路径返回 Claude Code 和 Codex 会话信息"
		op.Tags = []string{aiSessionTag}
	})

	// Get conversation for a session by database ID
	huma.Get(group, "/ai-sessions/{id}/conversation", func(ctx context.Context, input *struct {
		ID string `path:"id" doc:"会话ID（数据库ID）"`
	}) (*h.ItemResponse[service.ConversationResponse], error) {
		conversation, err := svc.GetSessionConversation(ctx, input.ID)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error404NotFound("session not found or failed to load conversation")
		}

		resp := h.NewItemResponse(*conversation)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "ai-session-get-conversation"
		op.Summary = "获取AI会话的对话内容"
		op.Description = "返回指定会话的完整对话记录（用户消息和助手回复），使用数据库ID"
		op.Tags = []string{aiSessionTag}
	})

	huma.Get(group, "/ai-sessions/{id}/conversation/window", func(ctx context.Context, input *struct {
		ID           string `path:"id" doc:"会话ID（数据库ID）"`
		BeforeCursor string `query:"beforeCursor"`
		Limit        int    `query:"limit" default:"80"`
	}) (*h.ItemResponse[service.ConversationWindowResponse], error) {
		conversation, err := svc.GetSessionConversationWindow(ctx, input.ID, input.BeforeCursor, input.Limit)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error404NotFound("session not found or failed to load conversation")
		}

		resp := h.NewItemResponse(*conversation)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "ai-session-get-conversation-window"
		op.Summary = "获取 AI 会话对话窗口"
		op.Description = "返回指定会话的分页对话窗口，使用数据库ID"
		op.Tags = []string{aiSessionTag}
	})

	// Get a (possibly large) tool_result content on demand (by database ID).
	huma.Get(group, "/ai-sessions/{id}/conversation/tool-results/{toolUseId}", func(ctx context.Context, input *struct {
		ID        string `path:"id" doc:"会话ID（数据库ID）"`
		ToolUseID string `path:"toolUseId" doc:"tool_use_id"`
	}) (*h.ItemResponse[service.ToolResultResponse], error) {
		result, err := svc.GetClaudeToolResult(ctx, input.ID, input.ToolUseID)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error404NotFound("tool result not found or failed to load")
		}

		resp := h.NewItemResponse(*result)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "ai-session-get-tool-result"
		op.Summary = "按需获取 Claude tool_result 内容"
		op.Description = "默认对话返回 tool_result 的折叠预览，本接口用于展开时按需拉取原始内容"
		op.Tags = []string{aiSessionTag}
	})

	// Get conversation for a session by session ID (UUID)
	huma.Get(group, "/ai-sessions/by-session-id/{sessionId}/conversation", func(ctx context.Context, input *struct {
		SessionID string `path:"sessionId" doc:"会话ID（AI助手生成的UUID）"`
	}) (*h.ItemResponse[service.ConversationResponse], error) {
		conversation, err := svc.GetSessionConversationBySessionID(ctx, input.SessionID)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error404NotFound("session not found or failed to load conversation")
		}

		resp := h.NewItemResponse(*conversation)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "ai-session-get-conversation-by-session-id"
		op.Summary = "通过Session ID获取AI会话的对话内容"
		op.Description = "返回指定会话的完整对话记录（用户消息和助手回复），使用AI助手生成的Session ID"
		op.Tags = []string{aiSessionTag}
	})

	huma.Get(group, "/ai-sessions/by-session-id/{sessionId}/conversation/window", func(ctx context.Context, input *struct {
		SessionID    string `path:"sessionId" doc:"会话ID（AI助手生成的UUID）"`
		BeforeCursor string `query:"beforeCursor"`
		Limit        int    `query:"limit" default:"80"`
	}) (*h.ItemResponse[service.ConversationWindowResponse], error) {
		conversation, err := svc.GetSessionConversationWindowBySessionID(
			ctx,
			input.SessionID,
			input.BeforeCursor,
			input.Limit,
		)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error404NotFound("session not found or failed to load conversation")
		}

		resp := h.NewItemResponse(*conversation)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "ai-session-get-conversation-window-by-session-id"
		op.Summary = "通过 Session ID 获取 AI 会话对话窗口"
		op.Description = "返回指定会话的分页对话窗口，使用AI助手生成的Session ID"
		op.Tags = []string{aiSessionTag}
	})

	// Get a (possibly large) tool_result content on demand (by session ID).
	huma.Get(group, "/ai-sessions/by-session-id/{sessionId}/conversation/tool-results/{toolUseId}", func(ctx context.Context, input *struct {
		SessionID string `path:"sessionId" doc:"会话ID（AI助手生成的UUID）"`
		ToolUseID string `path:"toolUseId" doc:"tool_use_id"`
	}) (*h.ItemResponse[service.ToolResultResponse], error) {
		result, err := svc.GetClaudeToolResultBySessionID(ctx, input.SessionID, input.ToolUseID)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error404NotFound("tool result not found or failed to load")
		}

		resp := h.NewItemResponse(*result)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "ai-session-get-tool-result-by-session-id"
		op.Summary = "按需获取 Claude tool_result 内容（SessionID）"
		op.Description = "默认对话返回 tool_result 的折叠预览，本接口用于展开时按需拉取原始内容"
		op.Tags = []string{aiSessionTag}
	})

	// Cleanup stale sessions
	huma.Post(group, "/ai-sessions/cleanup", func(ctx context.Context, _ *struct{}) (*h.ItemResponse[struct {
		DeletedCount int64 `json:"deletedCount"`
	}], error) {
		count, err := svc.CleanupStaleSessions(ctx)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to cleanup sessions", err)
		}

		resp := h.NewItemResponse(struct {
			DeletedCount int64 `json:"deletedCount"`
		}{
			DeletedCount: count,
		})
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "ai-session-cleanup"
		op.Summary = "清理过期的AI会话缓存"
		op.Description = "删除已不存在的会话文件对应的缓存记录"
		op.Tags = []string{aiSessionTag}
	})

	// Refresh session - clear cache and re-parse
	huma.Post(group, "/ai-sessions/{id}/refresh", func(ctx context.Context, input *struct {
		ID string `path:"id" doc:"会话ID（数据库ID）"`
	}) (*h.ItemResponse[service.ConversationResponse], error) {
		conversation, err := svc.RefreshSessionAndGetConversation(ctx, input.ID)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error404NotFound("session not found or failed to refresh")
		}

		resp := h.NewItemResponse(*conversation)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "ai-session-refresh"
		op.Summary = "刷新AI会话缓存"
		op.Description = "清除会话的数据库缓存，重新解析会话文件并返回对话内容"
		op.Tags = []string{aiSessionTag}
	})

	huma.Post(group, "/ai-sessions/{id}/refresh-window", func(ctx context.Context, input *struct {
		ID    string `path:"id" doc:"会话ID（数据库ID）"`
		Limit int    `query:"limit" default:"80"`
	}) (*h.ItemResponse[service.ConversationWindowResponse], error) {
		conversation, err := svc.RefreshSessionAndGetConversationWindow(ctx, input.ID, input.Limit)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error404NotFound("session not found or failed to refresh")
		}

		resp := h.NewItemResponse(*conversation)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "ai-session-refresh-window"
		op.Summary = "刷新 AI 会话缓存并返回对话窗口"
		op.Description = "清除会话的数据库缓存，重新解析会话文件并返回分页对话窗口"
		op.Tags = []string{aiSessionTag}
	})

	// Task-AI Session linking routes
	taskAISessionSvc := &model.TaskAISessionService{}

	// Get AI sessions linked to a task
	huma.Get(group, "/tasks/{taskId}/ai-sessions", func(ctx context.Context, input *struct {
		TaskID string `path:"taskId" doc:"任务ID"`
	}) (*h.ItemsResponse[model.TaskAISessionWithDetails], error) {
		sessions, err := taskAISessionSvc.GetAISessionsForTask(ctx, input.TaskID)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			return nil, huma.Error500InternalServerError("failed to get linked AI sessions", err)
		}

		resp := h.NewItemsResponse(sessions)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "task-ai-session-list"
		op.Summary = "获取任务关联的AI会话列表"
		op.Description = "返回指定任务关联的所有AI会话信息"
		op.Tags = []string{aiSessionTag}
	})

	// Link AI session to task
	huma.Post(group, "/tasks/{taskId}/ai-sessions/link", func(ctx context.Context, input *struct {
		TaskID string `path:"taskId" doc:"任务ID"`
		Body   struct {
			AISessionID string `json:"aiSessionId" minLength:"1" doc:"AI会话的数据库ID"`
		}
	}) (*h.MessageResponse, error) {
		_, err := taskAISessionSvc.LinkTaskToAISession(ctx, input.TaskID, input.Body.AISessionID)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			if errors.Is(err, model.ErrTaskNotFound) {
				return nil, huma.Error404NotFound("task not found")
			}
			if errors.Is(err, model.ErrAISessionNotFound) {
				return nil, huma.Error404NotFound("AI session not found")
			}
			if errors.Is(err, model.ErrTaskAISessionExists) {
				return nil, huma.Error409Conflict("AI session already linked to this task")
			}
			return nil, huma.Error500InternalServerError("failed to link AI session", err)
		}

		resp := h.NewMessageResponse("AI session linked successfully")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "task-ai-session-link"
		op.Summary = "关联AI会话到任务"
		op.Description = "将指定的AI会话关联到任务"
		op.Tags = []string{aiSessionTag}
	})

	// Unlink AI session from task
	huma.Post(group, "/tasks/{taskId}/ai-sessions/unlink", func(ctx context.Context, input *struct {
		TaskID string `path:"taskId" doc:"任务ID"`
		Body   struct {
			AISessionID string `json:"aiSessionId" minLength:"1" doc:"AI会话的数据库ID"`
		}
	}) (*h.MessageResponse, error) {
		err := taskAISessionSvc.UnlinkTaskFromAISession(ctx, input.TaskID, input.Body.AISessionID)
		if err != nil {
			if errors.Is(err, model.ErrDBNotInitialized) {
				return nil, huma.Error503ServiceUnavailable("database is not initialized")
			}
			if errors.Is(err, model.ErrTaskAISessionNotFound) {
				return nil, huma.Error404NotFound("link not found")
			}
			return nil, huma.Error500InternalServerError("failed to unlink AI session", err)
		}

		resp := h.NewMessageResponse("AI session unlinked successfully")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "task-ai-session-unlink"
		op.Summary = "解除任务与AI会话的关联"
		op.Description = "解除指定AI会话与任务的关联关系"
		op.Tags = []string{aiSessionTag}
	})
}
