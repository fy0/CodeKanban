package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"code-kanban/api/h"
	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/service/websession"
	"code-kanban/utils"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func TestWebSessionPromptCacheRenewalRoutes(t *testing.T) {
	model.DBClose()
	if err := model.InitWithDSN(filepath.Join(t.TempDir(), "renewal.db"), 0, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(model.DBClose)
	session := tables.WebSessionTable{ProjectID: "renewal-project", Agent: "codex", Backend: "codex_app_server", Cwd: t.TempDir()}
	session.Init()
	if err := model.GetDB().Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	manager, err := websession.NewManager(websession.Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New(fiber.Config{Immutable: true})
	_, group := h.NewAPI(app, &utils.AppConfig{})
	registerWebSessionRoutes(app, group, manager, zap.NewNop())
	for _, test := range []struct {
		method, projectID, body string
		status                  int
	}{
		{http.MethodGet, session.ProjectID, "", http.StatusOK},
		{http.MethodPost, session.ProjectID, `{"action":"cycle"}`, http.StatusOK},
		{http.MethodPost, session.ProjectID, `{"action":"unknown"}`, http.StatusUnprocessableEntity},
		{http.MethodGet, "other-project", "", http.StatusNotFound},
	} {
		path := "/api/v1/projects/" + test.projectID + "/web-sessions/" + session.ID + "/prompt-cache-renewal"
		response, body := requestWebSessionTree(t, app, test.method, path, test.body)
		response.Body.Close()
		if response.StatusCode != test.status {
			t.Fatalf("%s %s: status %d, want %d: %s", test.method, path, response.StatusCode, test.status, body)
		}
		if test.status == http.StatusOK {
			var envelope struct {
				Item websession.PromptCacheRenewalState `json:"item"`
			}
			if err := json.Unmarshal([]byte(body), &envelope); err != nil || envelope.Item.Availability != "unavailable" || envelope.Item.Infinite || envelope.Item.Remaining != 0 {
				t.Fatalf("renewal must be available by default but wait for a completed turn: %s", body)
			}
		}
	}
}
