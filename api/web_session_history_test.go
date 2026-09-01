package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestWebSessionHistoryResponseOmitsLegacyEvents(t *testing.T) {
	model.DBClose()
	if err := model.InitWithDSN(filepath.Join(t.TempDir(), "web-session-history.db"), 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}
	t.Cleanup(model.DBClose)

	project := &tables.ProjectTable{Name: "History API", Path: t.TempDir()}
	project.Init()
	if err := model.GetDB().Create(project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	manager, err := websession.NewManager(websession.Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	session, err := manager.CreateSession(context.Background(), websession.CreateParams{
		ProjectID: project.ID,
		Agent:     websession.AgentCodex,
		Title:     "History",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	for index, text := range []string{"first", "second", "third"} {
		item := &tables.WebSessionItemTable{
			WebSessionID: session.ID,
			OrderIndex:   int64(index+1) * 10,
			ItemKind:     "user",
			ItemType:     "user_message",
			Text:         text,
		}
		item.Init()
		if err := model.GetDB().Create(item).Error; err != nil {
			t.Fatalf("create history item: %v", err)
		}
	}

	app := fiber.New(fiber.Config{Immutable: true})
	_, group := h.NewAPI(app, &utils.AppConfig{})
	registerWebSessionRoutes(app, group, manager, zap.NewNop())
	basePath := "/api/v1/projects/" + project.ID + "/web-sessions/" + session.ID + "/history"

	backward := requestWebSessionHistoryItem(t, app, basePath+"?limit=2")
	assertWebSessionHistoryFields(t, backward, []string{"items", "hasMore", "beforeCursor", "total"})

	forward := requestWebSessionHistoryItem(t, app, basePath+"?afterCursor=0&limit=2")
	assertWebSessionHistoryFields(t, forward, []string{"items", "hasLater", "afterCursor", "total"})
}

func requestWebSessionHistoryItem(t *testing.T, app *fiber.App, target string) map[string]json.RawMessage {
	t.Helper()
	response, err := app.Test(httptest.NewRequest(http.MethodGet, target, nil))
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d: %s", target, response.StatusCode, payload)
	}
	var envelope struct {
		Item map[string]json.RawMessage `json:"item"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return envelope.Item
}

func assertWebSessionHistoryFields(
	t *testing.T,
	payload map[string]json.RawMessage,
	required []string,
) {
	t.Helper()
	if _, exists := payload["events"]; exists {
		t.Fatalf("history response contains removed events field: %#v", payload)
	}
	for _, key := range required {
		if _, exists := payload[key]; !exists {
			t.Fatalf("history response is missing %q: %#v", key, payload)
		}
	}
}
