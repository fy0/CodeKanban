package api

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code-kanban/api/h"
	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/service"
	"code-kanban/service/websession"
	"code-kanban/utils"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func TestWebSessionTreeRoutesFailClosedAndHideCrossProjectSessions(t *testing.T) {
	model.DBClose()
	if err := model.InitWithDSN(filepath.Join(t.TempDir(), "web-session-tree.db"), 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}
	t.Cleanup(model.DBClose)

	session := tables.WebSessionTable{
		ProjectID: "project-tree", Agent: "pi", Backend: "pi_rpc", Title: "Tree source",
		WorkflowMode: "default", PermissionLevel: "yolo", Cwd: t.TempDir(), Status: "idle",
		ActivityAt: time.Now(), NativeSessionID: stringPointer("native-tree"), ThreadPath: stringPointer(filepath.Join(t.TempDir(), "native-tree.jsonl")),
	}
	session.Init()
	if err := model.GetDB().Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	manager, err := websession.NewManager(websession.Config{DataDir: t.TempDir(), PiPath: filepath.Join(t.TempDir(), "missing-pi")}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	app := fiber.New(fiber.Config{Immutable: true})
	_, group := h.NewAPI(app, &utils.AppConfig{})
	registerWebSessionRoutes(app, group, manager, zap.NewNop())

	base := "/api/v1/projects/project-tree/web-sessions/" + session.ID + "/tree"
	for _, request := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, base, ""},
		{http.MethodPost, base + "/navigate", `{"targetId":"item","revision":"rev"}`},
		{http.MethodPost, base + "/fork", `{"targetId":"item","revision":"rev"}`},
		{http.MethodPost, base + "/clone", `{"revision":"rev"}`},
	} {
		response, payload := requestWebSessionTree(t, app, request.method, request.path, request.body)
		if response.StatusCode != http.StatusForbidden {
			response.Body.Close()
			t.Fatalf("%s %s status = %d, want 403: %s", request.method, request.path, response.StatusCode, payload)
		}
		response.Body.Close()
	}

	crossProject := strings.Replace(base, "/projects/project-tree/", "/projects/project-other/", 1)
	response, payload := requestWebSessionTree(t, app, http.MethodGet, crossProject, "")
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-project status = %d, want 404: %s", response.StatusCode, payload)
	}
}

func TestPiTreeHTTPErrorMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"revision", websession.ErrPiTreeRevisionConflict, http.StatusConflict},
		{"active", errors.New("cannot navigate an active Pi web session"), http.StatusConflict},
		{"pending", errors.New("cannot navigate while messages are pending"), http.StatusConflict},
		{"input", errors.New("Pi tree revision is required"), http.StatusBadRequest},
		{"target", errors.New("Pi fork target is not a user message"), http.StatusBadRequest},
		{"trust", service.ErrProjectAgentTrustRequired, http.StatusForbidden},
		{"db", model.ErrDBNotInitialized, http.StatusServiceUnavailable},
		{"integrity", errors.New("Pi session tree contains a duplicate node id"), http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var statusErr huma.StatusError
			if !errors.As(piTreeHTTPError(test.err), &statusErr) {
				t.Fatalf("error does not implement huma.StatusError")
			}
			if statusErr.GetStatus() != test.want {
				t.Fatalf("status = %d, want %d", statusErr.GetStatus(), test.want)
			}
			if strings.Contains(piTreeHTTPError(test.err).Error(), "duplicate node id") {
				t.Fatal("internal Pi tree detail leaked through HTTP error")
			}
		})
	}
}

func requestWebSessionTree(t *testing.T, app *fiber.App, method, target, body string) (*http.Response, string) {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		response.Body.Close()
		t.Fatalf("read response: %v", err)
	}
	response.Body.Close()
	response.Body = io.NopCloser(strings.NewReader(string(payload)))
	return response, string(payload)
}

func stringPointer(value string) *string { return &value }
