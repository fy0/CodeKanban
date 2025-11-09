package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gorilla/websocket"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"go.uber.org/zap"

	"go-template/api/h"
	"go-template/api/ptytest"
	"go-template/utils"
)

const (
	ptyTestTag    = "pty-test-终端测试"
	ptyTestWSPath = "/api/v1/pty-test/ws"
)

type ptyTestController struct {
	cfg      *utils.AppConfig
	manager  *ptytest.Manager
	logger   *zap.Logger
	upgrader websocket.Upgrader
}

func registerPtyTestRoutes(app *fiber.App, group *huma.Group, cfg *utils.AppConfig, manager *ptytest.Manager, logger *zap.Logger) {
	if manager == nil {
		return
	}
	ctrl := &ptyTestController{
		cfg:     cfg,
		manager: manager,
		logger:  logger.Named("pty-test-controller"),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  32 * 1024,
			WriteBufferSize: 32 * 1024,
			CheckOrigin:     func(*http.Request) bool { return true },
		},
	}

	ctrl.registerHTTP(group)
	ctrl.registerWebsocket(app)
}

func (c *ptyTestController) registerHTTP(group *huma.Group) {
	huma.Post(group, "/pty-test/sessions", func(
		ctx context.Context,
		input *ptyTestCreateInput,
	) (*h.ItemResponse[ptyTestSessionView], error) {
		session, err := c.manager.CreateSession(ctx, ptytest.CreateSessionParams{
			WorkingDir: input.Body.WorkingDir,
			Shell:      input.Body.Shell,
			Rows:       input.Body.Rows,
			Cols:       input.Body.Cols,
			Encoding:   input.Body.Encoding,
		})
		if err != nil {
			if errors.Is(err, ptytest.ErrInvalidEncoding) {
				return nil, huma.Error400BadRequest(err.Error())
			}
			return nil, huma.Error500InternalServerError("failed to create PTY test session", err)
		}
		view := c.viewFromSession(session)
		resp := h.NewItemResponse(view)
		resp.Status = http.StatusCreated
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "pty-test-session-create"
		op.Summary = "创建 PTY 测试会话"
		op.Tags = []string{ptyTestTag}
	})

	huma.Delete(group, "/pty-test/sessions/{sessionId}", func(
		_ context.Context,
		input *struct {
			SessionID string `path:"sessionId"`
		},
	) (*h.MessageResponse, error) {
		if err := c.manager.CloseSession(input.SessionID); err != nil {
			if err == ptytest.ErrSessionNotFound {
				return nil, huma.Error404NotFound(err.Error())
			}
			return nil, huma.Error500InternalServerError("failed to close PTY test session", err)
		}
		resp := h.NewMessageResponse("session closed")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "pty-test-session-close"
		op.Summary = "关闭 PTY 测试会话"
		op.Tags = []string{ptyTestTag}
	})
}

func (c *ptyTestController) registerWebsocket(app *fiber.App) {
	handler := fasthttpadaptor.NewFastHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.serveWebsocket(w, r)
	}))
	app.Get(ptyTestWSPath, func(ctx *fiber.Ctx) error {
		handler(ctx.Context())
		return nil
	})
}

func (c *ptyTestController) serveWebsocket(w http.ResponseWriter, r *http.Request) {
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
	defer session.Close()

	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		c.logger.Warn("upgrade PTY test websocket failed", zap.Error(err))
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	sendMu := &sync.Mutex{}
	send := func(msg wsMessage) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		return conn.WriteJSON(msg)
	}

	_ = send(wsMessage{Type: "ready"})

	go c.forwardPTY(ctx, session, send)
	c.consumeClient(ctx, session, conn, send)
}

func (c *ptyTestController) forwardPTY(ctx context.Context, session *ptytest.Session, send func(wsMessage) error) {
	reader := session.Reader()
	buffer := make([]byte, 32*1024)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, err := reader.Read(buffer)
			if n > 0 {
				session.Touch()
				normalized := session.NormalizeOutput(buffer[:n])
				if len(normalized) == 0 {
					continue
				}
				chunk := base64.StdEncoding.EncodeToString(normalized)
				if writeErr := send(wsMessage{Type: "data", Data: chunk}); writeErr != nil {
					return
				}
			}
			if err != nil {
				if errMsg := err.Error(); errMsg != "" {
					_ = send(wsMessage{Type: "exit", Data: errMsg})
				} else {
					_ = send(wsMessage{Type: "exit"})
				}
				return
			}
		}
	}
}

func (c *ptyTestController) consumeClient(ctx context.Context, session *ptytest.Session, conn *websocket.Conn, send func(wsMessage) error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, payload, err := conn.ReadMessage()
			if err != nil {
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
				if _, err := session.Write([]byte(msg.Data)); err != nil {
					_ = send(wsMessage{Type: "error", Data: err.Error()})
					return
				}
			case "resize":
				_ = session.Resize(msg.Cols, msg.Rows)
			case "close":
				_ = session.Close()
				_ = send(wsMessage{Type: "exit", Data: "closed"})
				return
			}
		}
	}
}

func (c *ptyTestController) viewFromSession(session *ptytest.Session) ptyTestSessionView {
	wsPath := fmt.Sprintf("%s?sessionId=%s", ptyTestWSPath, session.ID())
	return ptyTestSessionView{
		ID:         session.ID(),
		WorkingDir: session.WorkingDir(),
		Shell:      session.Shell(),
		Rows:       session.Rows(),
		Cols:       session.Cols(),
		CreatedAt:  session.CreatedAt(),
		Encoding:   session.Encoding(),
		WsPath:     wsPath,
		WsURL:      buildWSURL(c.cfg, wsPath),
	}
}

type ptyTestCreateInput struct {
	Body struct {
		WorkingDir string `json:"workingDir" doc:"工作目录，可选"`
		Shell      string `json:"shell" doc:"自定义 shell 命令，留空使用默认配置"`
		Rows       int    `json:"rows" doc:"终端行数"`
		Cols       int    `json:"cols" doc:"终端列数"`
		Encoding   string `json:"encoding" doc:"xpty 输出编码，默认 utf-8"`
	} `json:"body"`
}

type ptyTestSessionView struct {
	ID         string    `json:"id"`
	WorkingDir string    `json:"workingDir"`
	Shell      []string  `json:"shell"`
	Rows       int       `json:"rows"`
	Cols       int       `json:"cols"`
	CreatedAt  time.Time `json:"createdAt"`
	Encoding   string    `json:"encoding"`
	WsPath     string    `json:"wsPath"`
	WsURL      string    `json:"wsUrl"`
}
