package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gorilla/websocket"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"go.uber.org/zap"

	"code-kanban/api/h"
	"code-kanban/api/terminal"
	"code-kanban/model"
	"code-kanban/service"
	"code-kanban/utils"
)

const (
	terminalTag    = "terminal-session-终端会话"
	terminalWSPath = "/api/v1/terminal/ws"
)

type terminalController struct {
	cfg            *utils.AppConfig
	manager        *terminal.Manager
	worktreeSvc    *service.WorktreeService
	logger         *zap.Logger
	upgrader       websocket.Upgrader
	wsPathTemplate string
}

func registerTerminalRoutes(app *fiber.App, group *huma.Group, cfg *utils.AppConfig, manager *terminal.Manager, logger *zap.Logger) {
	if manager == nil {
		return
	}
	ctrl := &terminalController{
		cfg:         cfg,
		manager:     manager,
		worktreeSvc: service.NewWorktreeService(),
		logger:      logger.Named("terminal-controller"),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  32 * 1024,
			WriteBufferSize: 32 * 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}

	ctrl.registerHTTP(group)
	ctrl.registerWebsocket(app)
}

func (c *terminalController) registerHTTP(group *huma.Group) {
	huma.Post(group, "/projects/{projectId}/worktrees/{worktreeId}/terminals", func(
		ctx context.Context,
		input *terminalCreateInput,
	) (*h.ItemResponse[terminalSessionView], error) {
		session, err := c.handleCreate(ctx, input)
		if err != nil {
			return nil, err
		}
		resp := h.NewItemResponse(*session)
		resp.Status = http.StatusCreated
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "terminal-session-create"
		op.Summary = "创建终端会话"
		op.Tags = []string{terminalTag}
	})

	huma.Get(group, "/projects/{projectId}/terminals", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
		},
	) (*h.ItemsResponse[terminalSessionView], error) {
		sessions := c.manager.ListSessions(input.ProjectID)
		views := make([]terminalSessionView, 0, len(sessions))
		for _, snapshot := range sessions {
			views = append(views, c.viewFromSnapshot(snapshot))
		}
		resp := h.NewItemsResponse(views)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "terminal-session-list"
		op.Summary = "获取终端会话列表"
		op.Tags = []string{terminalTag}
	})

	huma.Get(group, "/terminals/counts", func(
		ctx context.Context,
		input *struct{},
	) (*terminalCountsResponse, error) {
		sessions := c.manager.ListSessions("")
		counts := make(map[string]int)
		for _, snapshot := range sessions {
			counts[snapshot.ProjectID]++
		}
		resp := &terminalCountsResponse{
			Status: http.StatusOK,
		}
		resp.Body.Counts = counts
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "terminal-counts"
		op.Summary = "获取所有项目的终端数量统计"
		op.Tags = []string{terminalTag}
	})

	huma.Post(group, "/projects/{projectId}/terminals/{sessionId}/close", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
			SessionID string `path:"sessionId"`
		},
	) (*h.MessageResponse, error) {
		if err := c.manager.CloseSession(input.SessionID); err != nil {
			if errors.Is(err, terminal.ErrSessionNotFound) {
				return nil, huma.Error404NotFound(err.Error())
			}
			return nil, huma.Error500InternalServerError("failed to close session", err)
		}
		resp := h.NewMessageResponse("session closed")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "terminal-session-close"
		op.Summary = "关闭终端会话"
		op.Tags = []string{terminalTag}
	})

	huma.Post(group, "/projects/{projectId}/terminals/{sessionId}/rename", func(
		ctx context.Context,
		input *terminalRenameInput,
	) (*h.ItemResponse[terminalSessionView], error) {
		session, err := c.manager.RenameSession(input.ProjectID, input.SessionID, input.Body.Title)
		if err != nil {
			switch {
			case errors.Is(err, terminal.ErrSessionNotFound):
				return nil, huma.Error404NotFound(err.Error())
			case errors.Is(err, terminal.ErrInvalidSessionTitle):
				return nil, huma.Error400BadRequest(err.Error())
			default:
				return nil, huma.Error500InternalServerError("failed to rename session", err)
			}
		}
		view := c.viewFromSnapshot(session.Snapshot())
		resp := h.NewItemResponse(view)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "terminal-session-rename"
		op.Summary = "终端标签重命名"
		op.Tags = []string{terminalTag}
	})
}

func (c *terminalController) registerWebsocket(app *fiber.App) {
	handler := fasthttpadaptor.NewFastHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.serveWebsocket(w, r)
	}))
	app.Get(terminalWSPath, func(ctx *fiber.Ctx) error {
		handler(ctx.Context())
		return nil
	})
}

func (c *terminalController) handleCreate(ctx context.Context, input *terminalCreateInput) (*terminalSessionView, error) {
	worktree, err := c.worktreeSvc.GetWorktree(ctx, input.WorktreeID)
	if err != nil {
		if errors.Is(err, model.ErrWorktreeNotFound) {
			return nil, huma.Error404NotFound("worktree not found")
		}
		return nil, huma.Error500InternalServerError("failed to fetch worktree", err)
	}
	if worktree.ProjectId != input.ProjectID {
		return nil, huma.Error404NotFound("worktree does not belong to project")
	}

	workingDir, err := c.resolveWorkingDir(worktree.Path, strings.TrimSpace(input.Body.WorkingDir))
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	title := strings.TrimSpace(input.Body.Title)
	if title == "" {
		title = fmt.Sprintf("%s 终端", worktree.BranchName)
	}

	rows := input.Body.Rows
	if rows <= 0 {
		rows = 24
	}
	cols := input.Body.Cols
	if cols <= 0 {
		cols = 80
	}

	session, err := c.manager.CreateSession(ctx, terminal.CreateSessionParams{
		ProjectID:  input.ProjectID,
		WorktreeID: input.WorktreeID,
		WorkingDir: workingDir,
		Title:      title,
		Rows:       rows,
		Cols:       cols,
	})
	if err != nil {
		switch {
		case errors.Is(err, terminal.ErrSessionLimitReached):
			return nil, huma.Error429TooManyRequests(err.Error())
		default:
			return nil, huma.Error500InternalServerError("failed to create terminal session", err)
		}
	}

	view := c.viewFromSnapshot(session.Snapshot())
	return &view, nil
}

func (c *terminalController) serveWebsocket(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	session, err := c.manager.GetSession(sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		c.logger.Warn("upgrade websocket failed", zap.Error(err))
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	writeMu := &sync.Mutex{}
	send := func(msg wsMessage) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(msg)
	}

	status := session.Status()

	if err := send(wsMessage{
		Type: "ready",
		Data: string(status),
	}); err != nil {
		return
	}

	scrollback := session.Scrollback()
	for _, chunk := range scrollback {
		if len(chunk) == 0 {
			continue
		}
		encoded := base64.StdEncoding.EncodeToString(chunk)
		if err := send(wsMessage{Type: "data", Data: encoded}); err != nil {
			return
		}
	}

	if status == terminal.SessionStatusClosed || status == terminal.SessionStatusError {
		message := "session closed"
		if err := session.Err(); err != nil {
			message = err.Error()
		}
		_ = send(wsMessage{Type: "exit", Data: message})
		return
	}

	stream, err := session.Subscribe(ctx)
	if err != nil {
		c.logger.Warn("failed to subscribe session stream", zap.Error(err))
		_ = send(wsMessage{Type: "error", Data: "failed to attach terminal stream"})
		return
	}

	go c.forwardPTY(ctx, session, stream, send)
	c.consumeClient(ctx, session, conn, send)
}

func (c *terminalController) forwardPTY(ctx context.Context, session *terminal.Session, stream *terminal.SessionStream, send func(wsMessage) error) {
	if stream == nil {
		return
	}
	defer stream.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-stream.Events():
			if !ok {
				return
			}
			switch event.Type {
			case terminal.StreamEventData:
				if len(event.Data) == 0 {
					continue
				}
				chunk := base64.StdEncoding.EncodeToString(event.Data)
				if writeErr := send(wsMessage{Type: "data", Data: chunk}); writeErr != nil {
					return
				}
			case terminal.StreamEventExit:
				message := "session closed"
				if event.Err != nil {
					message = event.Err.Error()
				} else if err := session.Err(); err != nil {
					message = err.Error()
				}
				_ = send(wsMessage{Type: "exit", Data: message})
				return
			default:
				continue
			}
		}
	}
}

func (c *terminalController) consumeClient(ctx context.Context, session *terminal.Session, conn *websocket.Conn, send func(wsMessage) error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, payload, err := conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					c.logger.Debug("websocket read error", zap.Error(err))
				}
				return
			}

			var msg wsMessage
			if err := json.Unmarshal(payload, &msg); err != nil {
				continue
			}

			switch msg.Type {
			case "input":
				if msg.Data == "" {
					continue
				}
				if _, writeErr := session.Write([]byte(msg.Data)); writeErr != nil {
					_ = send(wsMessage{Type: "error", Data: writeErr.Error()})
					return
				}
			case "resize":
				_ = session.Resize(msg.Cols, msg.Rows)
			case "close":
				_ = session.Close()
				return
			default:
				continue
			}
		}
	}
}

func (c *terminalController) viewFromSnapshot(snapshot terminal.SessionSnapshot) terminalSessionView {
	wsPath := fmt.Sprintf("%s?sessionId=%s", terminalWSPath, snapshot.ID)
	return terminalSessionView{
		ID:         snapshot.ID,
		ProjectID:  snapshot.ProjectID,
		WorktreeID: snapshot.WorktreeID,
		WorkingDir: snapshot.WorkingDir,
		Title:      snapshot.Title,
		CreatedAt:  snapshot.CreatedAt,
		LastActive: snapshot.LastActive,
		Status:     string(snapshot.Status),
		WsPath:     wsPath,
		WsURL:      c.buildWSURL(wsPath),
		Rows:       snapshot.Rows,
		Cols:       snapshot.Cols,
		Encoding:   snapshot.Encoding,
	}
}

func (c *terminalController) resolveWorkingDir(root, user string) (string, error) {
	base := filepath.Clean(root)
	if base == "" {
		return "", fmt.Errorf("invalid worktree path")
	}
	target := user
	if target == "" {
		target = base
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(base, target)
	}
	target = filepath.Clean(target)

	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("working directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("working directory must be a folder")
	}

	if !isSubPath(base, target) {
		return "", fmt.Errorf("working directory escapes the worktree root")
	}
	return target, nil
}

func (c *terminalController) buildWSURL(path string) string {
	return buildWSURL(c.cfg, path)
}

func isSubPath(root, target string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, "..")
}

type terminalCreateInput struct {
	ProjectID  string `path:"projectId"`
	WorktreeID string `path:"worktreeId"`
	Body       struct {
		WorkingDir string `json:"workingDir" doc:"工作目录"`
		Title      string `json:"title" doc:"终端标题"`
		Rows       int    `json:"rows" doc:"终端行数"`
		Cols       int    `json:"cols" doc:"终端列数"`
	} `json:"body"`
}

type terminalRenameInput struct {
	ProjectID string `path:"projectId"`
	SessionID string `path:"sessionId"`
	Body      struct {
		Title string `json:"title" doc:"新的终端标签名"`
	} `json:"body"`
}

type terminalSessionView struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"projectId"`
	WorktreeID string    `json:"worktreeId"`
	WorkingDir string    `json:"workingDir"`
	Title      string    `json:"title"`
	CreatedAt  time.Time `json:"createdAt"`
	LastActive time.Time `json:"lastActive"`
	Status     string    `json:"status"`
	WsPath     string    `json:"wsPath"`
	WsURL      string    `json:"wsUrl"`
	Rows       int       `json:"rows"`
	Cols       int       `json:"cols"`
	Encoding   string    `json:"encoding"`
}

type terminalCountsResponse struct {
	Status int `json:"-"`
	Body   struct {
		Counts map[string]int `json:"counts" doc:"项目ID到终端数量的映射"`
	} `json:"body"`
}
