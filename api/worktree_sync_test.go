package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"code-kanban/api/h"
	"code-kanban/model"
	"code-kanban/utils"

	"github.com/gofiber/fiber/v2"
)

func TestSyncWorktreesResponseIncludesMessageAndItems(t *testing.T) {
	model.DBClose()
	if err := model.InitWithDSN(filepath.Join(t.TempDir(), "worktree-sync.db"), 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}
	t.Cleanup(model.DBClose)

	project, err := (&model.ProjectService{}).CreateProject(t.Context(), model.CreateProjectParams{
		Name: "Worktree Sync API",
		Path: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	app := fiber.New(fiber.Config{Immutable: true})
	_, group := h.NewAPI(app, &utils.AppConfig{})
	registerWorktreeRoutes(group, &utils.AppConfig{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.Id+"/sync-worktrees", nil)
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("sync worktrees request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var payload struct {
		Message string            `json:"message"`
		Items   []json.RawMessage `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Message == "" {
		t.Fatal("sync response message is empty")
	}
	if len(payload.Items) != 1 {
		t.Fatalf("sync response item count = %d, want 1", len(payload.Items))
	}
}
