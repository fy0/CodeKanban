package api

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gorilla/websocket"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"go.uber.org/zap"

	"code-kanban/api/h"
	"code-kanban/model"
	"code-kanban/service"
	"code-kanban/service/terminal"
	"code-kanban/utils"
	"code-kanban/utils/ai_assistant2"
)

const (
	terminalTag          = "terminal-session-终端会话"
	terminalWSPath       = "/api/v1/terminal/ws"
	terminalEventsWSPath = "/api/v1/terminals/events"
)

type terminalController struct {
	cfg         *utils.AppConfig
	manager     *terminal.Manager
	worktreeSvc *service.WorktreeService
	logger      *zap.Logger
	upgrader    websocket.Upgrader
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
			ReadBufferSize:    32 * 1024,
			WriteBufferSize:   32 * 1024,
			EnableCompression: true,
			CheckOrigin:       func(r *http.Request) bool { return true },
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

	huma.Post(group, "/projects/{projectId}/terminals/restore", func(
		ctx context.Context,
		input *struct {
			ProjectID string `path:"projectId"`
		},
	) (*h.ItemsResponse[terminalSessionView], error) {
		sessions, err := c.manager.RestoreProjectSessions(ctx, input.ProjectID)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to restore terminal sessions", err)
		}
		views := make([]terminalSessionView, 0, len(sessions))
		for _, snapshot := range sessions {
			views = append(views, c.viewFromSnapshot(snapshot))
		}
		resp := h.NewItemsResponse(views)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "terminal-session-restore"
		op.Summary = "恢复项目终端工作环境"
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

	huma.Post(group, "/projects/{projectId}/terminals/{sessionId}/move", func(
		ctx context.Context,
		input *terminalMoveInput,
	) (*h.ItemResponse[terminalSessionView], error) {
		session, err := c.manager.MoveSession(
			input.ProjectID,
			input.SessionID,
			input.Body.PreviousSessionID,
			input.Body.NextSessionID,
		)
		if err != nil {
			switch {
			case errors.Is(err, terminal.ErrSessionNotFound):
				return nil, huma.Error404NotFound(err.Error())
			case errors.Is(err, terminal.ErrInvalidSessionMoveTarget):
				return nil, huma.Error400BadRequest(err.Error())
			default:
				return nil, huma.Error500InternalServerError("failed to move session", err)
			}
		}
		view := c.viewFromSnapshot(session.Snapshot())
		resp := h.NewItemResponse(view)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "terminal-session-move"
		op.Summary = "调整终端标签顺序"
		op.Tags = []string{terminalTag}
	})

	huma.Get(group, "/terminals/{sessionId}/debug", func(
		ctx context.Context,
		input *struct {
			SessionID string `path:"sessionId"`
		},
	) (*h.ItemResponse[terminal.DebugInfo], error) {
		debugInfo, err := c.manager.GetSessionDebugInfo(input.SessionID)
		if err != nil {
			if errors.Is(err, terminal.ErrSessionNotFound) {
				return nil, huma.Error404NotFound(err.Error())
			}
			return nil, huma.Error500InternalServerError("failed to get debug info", err)
		}
		resp := h.NewItemResponse(*debugInfo)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "terminal-session-debug"
		op.Summary = "获取终端调试信息（包含完整输出内容）"
		op.Tags = []string{terminalTag}
		op.Description = "用于调试，返回终端的 scrollback 缓冲区内容、AI 助手状态、录制信息等"
	})

	huma.Get(group, "/terminals/{sessionId}/capture", func(
		ctx context.Context,
		input *struct {
			SessionID string `path:"sessionId"`
			Timeout   int    `query:"timeout" doc:"超时时间（秒），默认为 2 秒，0 表示使用默认值" minimum:"0" maximum:"10"`
		},
	) (*h.ItemResponse[terminal.CapturedChunk], error) {
		timeout := 2 * time.Second
		if input.Timeout > 0 {
			timeout = time.Duration(input.Timeout) * time.Second
		}

		chunk, err := c.manager.CaptureChunk(ctx, input.SessionID, timeout)
		if err != nil {
			if errors.Is(err, terminal.ErrSessionNotFound) {
				return nil, huma.Error404NotFound(err.Error())
			}
			return nil, huma.Error500InternalServerError("failed to capture chunk", err)
		}
		resp := h.NewItemResponse(*chunk)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "terminal-session-capture-chunk"
		op.Summary = "触发 resize 并捕获下一个输出 chunk"
		op.Tags = []string{terminalTag}
		op.Description = "发送一个 resize 命令给终端，然后捕获并返回接下来的第一个输出 chunk，用于调试和测试"
	})

}

func (c *terminalController) registerWebsocket(app *fiber.App) {
	handler := fasthttpadaptor.NewFastHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.serveWebsocket(w, r)
	}))
	eventHandler := fasthttpadaptor.NewFastHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.serveEventsWebsocket(w, r)
	}))
	app.Get(terminalWSPath, func(ctx *fiber.Ctx) error {
		handler(ctx.Context())
		return nil
	})
	app.Get(terminalEventsWSPath, func(ctx *fiber.Ctx) error {
		eventHandler(ctx.Context())
		return nil
	})
}

type terminalSessionsEventView struct {
	Type      string                `json:"type"`
	ProjectID string                `json:"projectId"`
	Sessions  []terminalSessionView `json:"sessions"`
}

func (c *terminalController) serveEventsWebsocket(w http.ResponseWriter, r *http.Request) {
	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		c.logger.Warn("upgrade terminal events websocket failed", zap.Error(err))
		return
	}
	defer conn.Close()

	events, unsubscribe := c.manager.SubscribeSessionListEvents()
	defer unsubscribe()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			views := make([]terminalSessionView, 0, len(event.Sessions))
			for _, snapshot := range event.Sessions {
				views = append(views, c.viewFromSnapshot(snapshot))
			}
			if err := conn.WriteJSON(terminalSessionsEventView{
				Type:      event.Type,
				ProjectID: event.ProjectID,
				Sessions:  views,
			}); err != nil {
				c.logger.Debug("terminal events websocket write failed", zap.Error(err))
				return
			}
		}
	}
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
		ProjectID:            input.ProjectID,
		WorktreeID:           input.WorktreeID,
		WorkingDir:           workingDir,
		Title:                title,
		Rows:                 rows,
		Cols:                 cols,
		InsertAfterSessionID: input.Body.InsertAfterSessionID,
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
	conn.EnableWriteCompression(true)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	renderState := newTerminalConnectionRenderState()
	mirrorState := newTerminalMirrorSenderState()
	writeMu := &sync.Mutex{}
	send := func(msg wsMessage) error {
		payload, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			return err
		}
		session.RecordTraffic(0, len(payload))
		return nil
	}
	sendSnapshot := func(snapshot *terminal.TerminalMirrorSnapshot, force bool) error {
		if snapshot == nil || renderState.Mode() != terminalRenderModeSnapshot {
			return nil
		}
		_, _, compressionEnabled := renderState.SnapshotConfig()
		payload, shouldSend, err := mirrorState.EncodeFrame(snapshot, force, compressionEnabled)
		if err != nil {
			return err
		}
		if !shouldSend || len(payload) == 0 {
			return nil
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
			return err
		}
		session.RecordTraffic(0, len(payload))
		return nil
	}
	initializeTerminalConnectionRenderState(renderState, r)

	status := session.Status()
	var initialSnapshot *terminal.TerminalMirrorSnapshot
	if renderState.Mode() == terminalRenderModeSnapshot {
		initialSnapshot = session.TerminalMirrorSnapshot()
	}
	if renderState.Mode() == terminalRenderModeSnapshot && initialSnapshot == nil {
		_, interval, compressionEnabled := renderState.SnapshotConfig()
		renderState.Update(
			string(terminalRenderModeLive),
			snapshotIntervalMilliseconds(interval),
			compressionEnabled,
		)
	}

	if err := send(wsMessage{
		Type: "ready",
		Data: string(status),
	}); err != nil {
		return
	}
	if initialSnapshot == nil && renderState.Mode() == terminalRenderModeLive {
		if err := c.sendRenderModeAck(renderState, mirrorState, send); err != nil {
			return
		}
	}

	scrollback := session.Scrollback()
	if renderState.Mode() == terminalRenderModeSnapshot {
		if initialSnapshot != nil {
			if err := sendSnapshot(initialSnapshot, true); err != nil {
				return
			}
			scrollback = session.ScrollbackSince(initialSnapshot.CapturedAt)
		}
	}
	for _, chunk := range scrollback {
		if len(chunk) == 0 {
			continue
		}
		encoded := base64.StdEncoding.EncodeToString(chunk)
		if err := send(wsMessage{Type: "data", Data: encoded}); err != nil {
			return
		}
	}
	if renderState.Mode() == terminalRenderModeSnapshot {
		if prefix := terminal.BuildTerminalModesReplayPrefix(session.TerminalModesSnapshot(), false); len(prefix) > 0 {
			encoded := base64.StdEncoding.EncodeToString(prefix)
			if err := send(wsMessage{Type: "mode-prefix", Data: encoded}); err != nil {
				return
			}
		}
	}
	if err := send(wsMessage{Type: "replay-complete"}); err != nil {
		return
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

	go c.forwardPTY(ctx, cancel, session, stream, renderState, send)
	go c.forwardSnapshots(ctx, cancel, session, renderState, sendSnapshot)
	// A shell can emit its first prompt before the websocket replay finishes.
	// Once the live stream is attached, force one same-size redraw so Unix
	// shells repaint that prompt instead of waiting for the next user input.
	if err := session.ForceRedraw(); err != nil {
		c.logger.Debug("initial terminal redraw failed", zap.Error(err), zap.String("sessionId", session.ID()))
	}
	c.consumeClient(ctx, cancel, session, renderState, mirrorState, conn, send, sendSnapshot)
}

func initializeTerminalConnectionRenderState(state *terminalConnectionRenderState, r *http.Request) {
	if state == nil || r == nil {
		return
	}

	query := r.URL.Query()
	mode := query.Get("renderMode")
	if mode == "" {
		mode = string(terminalRenderModeLive)
	}
	interval := 0
	if rawInterval := query.Get("snapshotIntervalMs"); rawInterval != "" {
		interval, _ = strconv.Atoi(rawInterval)
	}
	compressionEnabled := true
	if rawCompression := query.Get("snapshotCompression"); rawCompression != "" {
		compressionEnabled = normalizeSnapshotCompression(rawCompression)
	}
	state.Update(mode, interval, compressionEnabled)
}

func (c *terminalController) forwardPTY(
	ctx context.Context,
	cancel context.CancelFunc,
	session *terminal.Session,
	stream *terminal.SessionStream,
	renderState *terminalConnectionRenderState,
	send func(wsMessage) error,
) {
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
				if renderState != nil && renderState.Mode() == terminalRenderModeSnapshot {
					continue
				}
				if len(event.Data) == 0 {
					continue
				}
				chunk := base64.StdEncoding.EncodeToString(event.Data)
				if writeErr := send(wsMessage{Type: "data", Data: chunk}); writeErr != nil {
					if cancel != nil {
						cancel()
					}
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
				if cancel != nil {
					cancel()
				}
				return
			case terminal.StreamEventMetadata:
				if event.Metadata != nil {
					if writeErr := send(wsMessage{Type: "metadata", Metadata: event.Metadata}); writeErr != nil {
						if cancel != nil {
							cancel()
						}
						return
					}
				}
			case terminal.StreamEventModes:
				if renderState == nil || renderState.Mode() != terminalRenderModeSnapshot {
					continue
				}
				prefix := terminal.BuildTerminalModesReplayPrefix(event.Modes, true)
				if len(prefix) == 0 {
					continue
				}
				encoded := base64.StdEncoding.EncodeToString(prefix)
				if writeErr := send(wsMessage{Type: "mode-prefix", Data: encoded}); writeErr != nil {
					return
				}
			default:
				continue
			}
		}
	}
}

func (c *terminalController) forwardSnapshots(
	ctx context.Context,
	cancel context.CancelFunc,
	session *terminal.Session,
	renderState *terminalConnectionRenderState,
	send func(*terminal.TerminalMirrorSnapshot, bool) error,
) {
	if renderState == nil {
		return
	}

	var ticker *time.Ticker
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()

	resetTicker := func() {
		if ticker != nil {
			ticker.Stop()
			ticker = nil
		}
		mode, interval, _ := renderState.SnapshotConfig()
		if mode == terminalRenderModeSnapshot {
			ticker = time.NewTicker(interval)
		}
	}

	resetTicker()

	for {
		var tickC <-chan time.Time
		if ticker != nil {
			tickC = ticker.C
		}

		select {
		case <-ctx.Done():
			return
		case <-renderState.NotifyC():
			resetTicker()
		case <-tickC:
			if err := c.sendTerminalSnapshot(session, send, false); err != nil {
				if cancel != nil {
					cancel()
				}
				return
			}
		}
	}
}

func (c *terminalController) consumeClient(
	ctx context.Context,
	cancel context.CancelFunc,
	session *terminal.Session,
	renderState *terminalConnectionRenderState,
	mirrorState *terminalMirrorSenderState,
	conn *websocket.Conn,
	send func(wsMessage) error,
	sendSnapshot func(*terminal.TerminalMirrorSnapshot, bool) error,
) {
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
			session.RecordTraffic(len(payload), 0)

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
					if cancel != nil {
						cancel()
					}
					return
				}
			case "resize":
				if err := session.Resize(msg.Cols, msg.Rows); err != nil {
					_ = send(wsMessage{Type: "error", Data: err.Error()})
					continue
				}
				if renderState != nil && renderState.Mode() == terminalRenderModeSnapshot {
					if err := session.ForceRedraw(); err != nil {
						_ = send(wsMessage{Type: "error", Data: err.Error()})
						continue
					}
					if err := c.sendTerminalSnapshot(session, sendSnapshot, true); err != nil {
						_ = send(wsMessage{Type: "error", Data: err.Error()})
						continue
					}
					c.sendTerminalSnapshotAfterDelay(
						ctx,
						session,
						renderState,
						sendSnapshot,
						60*time.Millisecond,
					)
				}
			case "render-mode":
				if err := c.handleRenderModeMessage(session, renderState, mirrorState, msg, send, sendSnapshot); err != nil {
					_ = send(wsMessage{Type: "error", Data: err.Error()})
					continue
				}
			case "snapshot-request":
				if renderState == nil || renderState.Mode() != terminalRenderModeSnapshot {
					continue
				}
				// A same-size resize is not guaranteed to generate SIGWINCH on
				// Unix. Nudge the PTY before capturing the snapshot so a prompt or
				// full-screen program is painted immediately after attachment.
				if err := session.ForceRedraw(); err != nil {
					c.logger.Debug("terminal redraw before snapshot failed", zap.Error(err), zap.String("sessionId", session.ID()))
				}
				if err := c.sendTerminalSnapshot(session, sendSnapshot, true); err != nil {
					_ = send(wsMessage{Type: "error", Data: err.Error()})
					continue
				}
			case "close":
				_ = session.Close()
				if cancel != nil {
					cancel()
				}
				return
			default:
				continue
			}
		}
	}
}

func (c *terminalController) sendRenderModeAck(
	renderState *terminalConnectionRenderState,
	mirrorState *terminalMirrorSenderState,
	send func(wsMessage) error,
) error {
	if renderState == nil {
		return nil
	}
	mode, interval, compressionEnabled := renderState.SnapshotConfig()
	return send(wsMessage{
		Type:                       "render-mode",
		Mode:                       string(mode),
		SnapshotIntervalMs:         snapshotIntervalMilliseconds(interval),
		SnapshotCompressionEnabled: compressionEnabled,
		SnapshotIncrementalEnabled: mirrorState == nil || mirrorState.IncrementalEnabled(),
	})
}

func (c *terminalController) sendTerminalSnapshot(
	session *terminal.Session,
	send func(*terminal.TerminalMirrorSnapshot, bool) error,
	force bool,
) error {
	if session == nil {
		return nil
	}
	snapshot := session.TerminalMirrorSnapshot()
	if snapshot == nil {
		return nil
	}
	return send(snapshot, force)
}

func (c *terminalController) sendTerminalSnapshotAfterDelay(
	ctx context.Context,
	session *terminal.Session,
	renderState *terminalConnectionRenderState,
	send func(*terminal.TerminalMirrorSnapshot, bool) error,
	delay time.Duration,
) {
	if session == nil || send == nil || delay <= 0 {
		return
	}

	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		if renderState != nil && renderState.Mode() != terminalRenderModeSnapshot {
			return
		}
		_ = c.sendTerminalSnapshot(session, send, true)
	}()
}

func (c *terminalController) handleRenderModeMessage(
	session *terminal.Session,
	renderState *terminalConnectionRenderState,
	mirrorState *terminalMirrorSenderState,
	msg wsMessage,
	send func(wsMessage) error,
	sendSnapshot func(*terminal.TerminalMirrorSnapshot, bool) error,
) error {
	requestedMode := normalizeTerminalRenderMode(msg.Mode)
	if requestedMode == terminalRenderModeSnapshot && session.TerminalMirrorSnapshot() == nil {
		if _, currentMode, interval, compressionEnabled, _ := renderState.Update(
			string(terminalRenderModeLive),
			msg.SnapshotIntervalMs,
			msg.SnapshotCompressionEnabled,
		); currentMode != "" {
			if mirrorState != nil {
				mirrorState.SetIncrementalEnabled(msg.SnapshotIncrementalEnabled)
			}
			if err := send(wsMessage{
				Type:                       "render-mode",
				Mode:                       string(currentMode),
				SnapshotIntervalMs:         snapshotIntervalMilliseconds(interval),
				SnapshotCompressionEnabled: compressionEnabled,
				SnapshotIncrementalEnabled: mirrorState == nil || mirrorState.IncrementalEnabled(),
			}); err != nil {
				return err
			}
		}
		if err := session.ForceRedraw(); err != nil {
			return err
		}
		// Snapshot support is optional per session. Keep the user's preference
		// intact on the client while this connection operates as live PTY.
		return nil
	}

	previousMode, currentMode, interval, compressionEnabled, changed := renderState.Update(
		msg.Mode,
		msg.SnapshotIntervalMs,
		msg.SnapshotCompressionEnabled,
	)
	if mirrorState != nil {
		mirrorState.SetIncrementalEnabled(msg.SnapshotIncrementalEnabled)
	}
	if err := send(wsMessage{
		Type:                       "render-mode",
		Mode:                       string(currentMode),
		SnapshotIntervalMs:         snapshotIntervalMilliseconds(interval),
		SnapshotCompressionEnabled: compressionEnabled,
		SnapshotIncrementalEnabled: mirrorState == nil || mirrorState.IncrementalEnabled(),
	}); err != nil {
		return err
	}

	if currentMode == terminalRenderModeSnapshot && changed {
		if err := c.sendTerminalSnapshot(session, sendSnapshot, true); err != nil {
			return err
		}
	}
	if currentMode == terminalRenderModeLive && changed && previousMode == terminalRenderModeSnapshot {
		if err := session.ForceRedraw(); err != nil {
			return err
		}
	}

	return nil
}

func compressSnapshotPayload(input []byte) ([]byte, bool) {
	if len(input) == 0 {
		return input, false
	}

	var buffer bytes.Buffer
	writer, err := zlib.NewWriterLevel(&buffer, zlib.BestSpeed)
	if err != nil {
		return input, false
	}
	if _, err := writer.Write(input); err != nil {
		_ = writer.Close()
		return input, false
	}
	if err := writer.Close(); err != nil {
		return input, false
	}
	if buffer.Len() >= len(input) {
		return input, false
	}
	return buffer.Bytes(), true
}

func (c *terminalController) viewFromSnapshot(snapshot terminal.SessionSnapshot) terminalSessionView {
	wsPath := fmt.Sprintf("%s?sessionId=%s", terminalWSPath, snapshot.ID)
	return terminalSessionView{
		ID:            snapshot.ID,
		ProjectID:     snapshot.ProjectID,
		WorktreeID:    snapshot.WorktreeID,
		WorkingDir:    snapshot.WorkingDir,
		Title:         snapshot.Title,
		OrderIndex:    snapshot.OrderIndex,
		CreatedAt:     snapshot.CreatedAt,
		LastActive:    snapshot.LastActive,
		Status:        string(snapshot.Status),
		WsPath:        wsPath,
		WsURL:         wsPath,
		Rows:          snapshot.Rows,
		Cols:          snapshot.Cols,
		Encoding:      snapshot.Encoding,
		TerminalModes: snapshot.TerminalModes,
		// Process information
		ProcessPID:         snapshot.ProcessPID,
		ProcessStatus:      snapshot.ProcessStatus,
		ProcessHasChildren: snapshot.ProcessHasChildren,
		RunningCommand:     snapshot.RunningCommand,
		MetadataCapturedAt: terminalMetadataCapturedAt(snapshot.MetadataCapturedAt),
		AIAssistant:        snapshot.AIAssistant,
		Traffic:            snapshot.Traffic,
	}
}

func terminalMetadataCapturedAt(capturedAt time.Time) *time.Time {
	if capturedAt.IsZero() {
		return nil
	}
	return &capturedAt
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
		WorkingDir           string `json:"workingDir" doc:"工作目录"`
		Title                string `json:"title" doc:"终端标题"`
		Rows                 int    `json:"rows" doc:"终端行数"`
		Cols                 int    `json:"cols" doc:"终端列数"`
		InsertAfterSessionID string `json:"insertAfterSessionId,omitempty" doc:"插入到指定终端会话之后"`
	} `json:"body"`
}

type terminalRenameInput struct {
	ProjectID string `path:"projectId"`
	SessionID string `path:"sessionId"`
	Body      struct {
		Title string `json:"title" doc:"新的终端标签名"`
	} `json:"body"`
}

type terminalMoveInput struct {
	ProjectID string `path:"projectId"`
	SessionID string `path:"sessionId"`
	Body      struct {
		PreviousSessionID string `json:"previousSessionId" doc:"前一个终端会话ID"`
		NextSessionID     string `json:"nextSessionId" doc:"后一个终端会话ID"`
	} `json:"body"`
}

type terminalSessionView struct {
	ID            string                          `json:"id"`
	ProjectID     string                          `json:"projectId"`
	WorktreeID    string                          `json:"worktreeId"`
	WorkingDir    string                          `json:"workingDir"`
	Title         string                          `json:"title"`
	OrderIndex    float64                         `json:"orderIndex"`
	CreatedAt     time.Time                       `json:"createdAt"`
	LastActive    time.Time                       `json:"lastActive"`
	Status        string                          `json:"status"`
	WsPath        string                          `json:"wsPath"`
	WsURL         string                          `json:"wsUrl"`
	Rows          int                             `json:"rows"`
	Cols          int                             `json:"cols"`
	Encoding      string                          `json:"encoding"`
	TerminalModes *terminal.TerminalModesSnapshot `json:"terminalModes,omitempty"`
	// Process information
	ProcessPID         int32                          `json:"processPid,omitempty"`
	ProcessStatus      string                         `json:"processStatus,omitempty"`
	ProcessHasChildren bool                           `json:"processHasChildren,omitempty"`
	RunningCommand     string                         `json:"runningCommand,omitempty"`
	MetadataCapturedAt *time.Time                     `json:"metadataCapturedAt,omitempty"`
	AIAssistant        *ai_assistant2.AIAssistantInfo `json:"aiAssistant,omitempty"`
	Traffic            *terminal.SessionTrafficStats  `json:"traffic,omitempty"`
}

type terminalCountsResponse struct {
	Status int `json:"-"`
	Body   struct {
		Counts map[string]int `json:"counts" doc:"项目ID到终端数量的映射"`
	} `json:"body"`
}
