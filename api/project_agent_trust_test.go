package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"code-kanban/api/h"
	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/service"
	"code-kanban/service/websession"
	"code-kanban/utils"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func TestProjectPiTrustRoutes(t *testing.T) {
	model.DBClose()
	if err := model.InitWithDSN(filepath.Join(t.TempDir(), "agent-trust.db"), 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}
	t.Cleanup(model.DBClose)

	project := &tables.ProjectTable{Name: "Trust API", Path: t.TempDir()}
	project.Init()
	if err := model.GetDB().Create(project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	manager, err := websession.NewManager(websession.Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	app := fiber.New(fiber.Config{Immutable: true})
	_, group := h.NewAPI(app, &utils.AppConfig{})
	registerWebSessionRoutes(app, group, manager, zap.NewNop())
	path := "/api/v1/projects/" + project.ID + "/agent-trust/pi"

	assertProjectPiTrustResponse(t, app, http.MethodGet, path, "", http.StatusOK, `"trusted":false`)
	assertProjectPiTrustResponse(
		t,
		app,
		http.MethodPost,
		path,
		`{"trusted":true,"path":"D:/forged"}`,
		http.StatusOK,
		`"trusted":true`,
	)
	var trust tables.ProjectAgentTrustTable
	if err := model.GetDB().Where("project_id = ? AND agent = ?", project.ID, "pi").First(&trust).Error; err != nil {
		t.Fatalf("load trust record: %v", err)
	}
	wantTrustedPath, err := service.CanonicalAgentTrustPath(project.Path)
	if err != nil {
		t.Fatalf("canonical project path: %v", err)
	}
	if trust.TrustedPath != wantTrustedPath {
		t.Fatalf("trusted path = %q, want server project path %q", trust.TrustedPath, wantTrustedPath)
	}
	assertProjectPiTrustResponse(t, app, http.MethodDelete, path, "", http.StatusOK, `"trusted":false`)
}

func assertProjectPiTrustResponse(
	t *testing.T,
	app *fiber.App,
	method string,
	path string,
	body string,
	wantStatus int,
	wantFragment string,
) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s status = %d, want %d: %s", method, path, response.StatusCode, wantStatus, payload)
	}
	if !strings.Contains(string(payload), wantFragment) {
		t.Fatalf("%s %s response %s does not contain %s", method, path, payload, wantFragment)
	}
}
