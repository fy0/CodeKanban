package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"code-kanban/api/h"
	"code-kanban/utils"
)

type dailyTipSettingsResponse struct {
	Body struct {
		Item struct {
			Enabled bool `json:"enabled"`
		} `json:"item"`
	} `json:"body"`
}

func TestSystemDailyTipSettingsGetReturnsServerValue(t *testing.T) {
	cfg, _ := loadSystemDailyTipTestConfig(t, `
ui:
  dailyTipEnabled: false
`)
	app := newSystemDailyTipTestApp(t, cfg)

	resp := mustSystemDailyTipTestRequest(t, app, http.MethodGet, "/api/v1/system/daily-tip-settings", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var payload dailyTipSettingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload.Body.Item.Enabled {
		t.Fatal("expected daily tip setting to be disabled")
	}
}

func TestSystemDailyTipSettingsUpdatePersistsConfig(t *testing.T) {
	cfg, configPath := loadSystemDailyTipTestConfig(t, `
ui:
  dailyTipEnabled: true
`)
	app := newSystemDailyTipTestApp(t, cfg)

	resp := mustSystemDailyTipTestRequest(
		t,
		app,
		http.MethodPost,
		"/api/v1/system/daily-tip-settings/update",
		bytes.NewBufferString(`{"enabled":false}`),
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var payload dailyTipSettingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload.Body.Item.Enabled {
		t.Fatal("expected daily tip update response to return disabled")
	}
	if cfg.UI.DailyTipEnabled {
		t.Fatal("expected in-memory config to be updated")
	}

	rewritten, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	if !strings.Contains(string(rewritten), "dailyTipEnabled: false") {
		t.Fatalf("expected config file to persist dailyTipEnabled=false, got:\n%s", string(rewritten))
	}
}

func loadSystemDailyTipTestConfig(t *testing.T, configYAML string) (*utils.AppConfig, string) {
	t.Helper()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := strings.TrimSpace(configYAML)
	if content == "" {
		content = "ui:\n  dailyTipEnabled: true"
	}
	if err := os.WriteFile(configPath, []byte(content+"\n"), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	return utils.ReadConfig(), configPath
}

func newSystemDailyTipTestApp(t *testing.T, cfg *utils.AppConfig) *fiber.App {
	t.Helper()

	app := fiber.New()
	_, v1 := h.NewAPI(app, cfg)
	registerSystemRoutes(v1, cfg, nil, nil)
	return app
}

func mustSystemDailyTipTestRequest(
	t *testing.T,
	app *fiber.App,
	method string,
	target string,
	body *bytes.Buffer,
) *http.Response {
	t.Helper()

	var payload *bytes.Buffer
	if body != nil {
		payload = body
	} else {
		payload = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, target, payload)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}
