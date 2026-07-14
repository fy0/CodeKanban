package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"code-kanban/utils"

	"gopkg.in/yaml.v3"
)

type pageTitleSettingsResponse struct {
	Item pageTitleSettings `json:"item"`
}

func TestSystemPageTitleSettingsGetReturnsServerValue(t *testing.T) {
	cfg, _ := loadSystemDailyTipTestConfig(t, `
ui:
  pageTitle: Staging Board
`)
	if got := cfg.UI.PageTitle; got != "Staging Board" {
		t.Fatalf("configured title = %q, want %q", got, "Staging Board")
	}
	app := newSystemDailyTipTestApp(t, cfg)

	resp := mustSystemDailyTipTestRequest(t, app, http.MethodGet, "/api/v1/system/page-title-settings", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response failed: %v", err)
	}
	var payload pageTitleSettingsResponse
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		t.Fatalf("decode response failed: %v; body=%s", err, string(rawBody))
	}
	if got := payload.Item.Title; got != "Staging Board" {
		t.Fatalf("title = %q, want %q; body=%s", got, "Staging Board", string(rawBody))
	}
}

func TestSystemPageTitleSettingsUpdateNormalizesAndPersistsConfig(t *testing.T) {
	cfg, configPath := loadSystemDailyTipTestConfig(t, `
ui:
  pageTitle: Code Kanban
`)
	app := newSystemDailyTipTestApp(t, cfg)

	body, err := json.Marshal(pageTitleSettings{Title: "  工作实例 🚀  "})
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}
	resp := mustSystemDailyTipTestRequest(
		t,
		app,
		http.MethodPost,
		"/api/v1/system/page-title-settings/update",
		bytes.NewBuffer(body),
	)
	if resp.StatusCode != http.StatusOK {
		rawBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body=%s", resp.StatusCode, http.StatusOK, string(rawBody))
	}
	if got := cfg.UI.PageTitle; got != "工作实例 🚀" {
		t.Fatalf("in-memory title = %q, want %q", got, "工作实例 🚀")
	}

	rewritten, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	var persisted utils.AppConfig
	if err := yaml.Unmarshal(rewritten, &persisted); err != nil {
		t.Fatalf("parse persisted config failed: %v", err)
	}
	if got := persisted.UI.PageTitle; got != "工作实例 🚀" {
		t.Fatalf("persisted title = %q, want %q", got, "工作实例 🚀")
	}
}

func TestSystemPageTitleSettingsUpdateBlankRestoresDefault(t *testing.T) {
	cfg, _ := loadSystemDailyTipTestConfig(t, `
ui:
  pageTitle: Staging Board
`)
	app := newSystemDailyTipTestApp(t, cfg)

	resp := mustSystemDailyTipTestRequest(
		t,
		app,
		http.MethodPost,
		"/api/v1/system/page-title-settings/update",
		bytes.NewBufferString(`{"title":"   "}`),
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := cfg.UI.PageTitle; got != utils.DefaultPageTitle {
		t.Fatalf("title = %q, want %q", got, utils.DefaultPageTitle)
	}
}

func TestSystemPageTitleSettingsUpdateRejectsInvalidTitles(t *testing.T) {
	tests := []struct {
		name  string
		title string
	}{
		{name: "control character", title: "Bad\nTitle"},
		{name: "too long", title: strings.Repeat("界", utils.MaxPageTitleRunes+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := loadSystemDailyTipTestConfig(t, `
ui:
  pageTitle: Original
`)
			app := newSystemDailyTipTestApp(t, cfg)
			body, err := json.Marshal(pageTitleSettings{Title: tt.title})
			if err != nil {
				t.Fatalf("marshal request failed: %v", err)
			}

			resp := mustSystemDailyTipTestRequest(
				t,
				app,
				http.MethodPost,
				"/api/v1/system/page-title-settings/update",
				bytes.NewBuffer(body),
			)
			if resp.StatusCode != http.StatusBadRequest {
				rawBody, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, want %d; body=%s", resp.StatusCode, http.StatusBadRequest, string(rawBody))
			}
			if got := cfg.UI.PageTitle; got != "Original" {
				t.Fatalf("invalid update changed title to %q", got)
			}
		})
	}
}

func TestPageTitleSettingsReadIsAnonymousButUpdateIsProtected(t *testing.T) {
	cfg := newAuthTestConfig()
	if !isAnonymousPath("/api/v1/system/page-title-settings", cfg) {
		t.Fatal("expected page title read endpoint to allow anonymous access")
	}
	if isAnonymousPath("/api/v1/system/page-title-settings/update", cfg) {
		t.Fatal("expected page title update endpoint to require protected access")
	}
}
