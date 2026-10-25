package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"code-kanban/utils"
)

func TestSystemRecentProjectsSettingsGet(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config string
		want   int
	}{
		{name: "default", config: "ui:\n  dailyTipEnabled: false\n", want: utils.DefaultRecentProjectsLimit},
		{name: "configured", config: "ui:\n  recentProjectsLimit: 7\n", want: 7},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := loadSystemDailyTipTestConfig(t, tt.config)
			app := newSystemDailyTipTestApp(t, cfg)
			resp := mustSystemDailyTipTestRequest(t, app, http.MethodGet, "/api/v1/system/recent-projects-settings", nil)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			var payload struct {
				Item recentProjectsSettings `json:"item"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Item.Limit != tt.want {
				t.Fatalf("limit = %d, want %d", payload.Item.Limit, tt.want)
			}
		})
	}
}

func TestSystemRecentProjectsSettingsUpdateSurvivesRestart(t *testing.T) {
	cfg, _ := loadSystemDailyTipTestConfig(t, "ui:\n  recentProjectsLimit: 10\n")
	app := newSystemDailyTipTestApp(t, cfg)
	resp := mustSystemDailyTipTestRequest(t, app, http.MethodPost,
		"/api/v1/system/recent-projects-settings/update", bytes.NewBufferString(`{"limit":4}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if cfg.UI.RecentProjectsLimit != 4 {
		t.Fatalf("in-memory limit = %d, want 4", cfg.UI.RecentProjectsLimit)
	}
	if err := utils.CurrentConfigDatabase().Close(); err != nil {
		t.Fatal(err)
	}

	restarted := utils.ReadConfig()
	store, err := utils.InitConfigDatabase(restarted)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	app = newSystemDailyTipTestApp(t, restarted)
	resp = mustSystemDailyTipTestRequest(t, app, http.MethodGet, "/api/v1/system/recent-projects-settings", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restarted status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var payload struct {
		Item recentProjectsSettings `json:"item"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Item.Limit != 4 {
		t.Fatalf("restarted limit = %d, want 4", payload.Item.Limit)
	}
}

func TestSystemRecentProjectsSettingsRejectsInvalidLimit(t *testing.T) {
	for _, body := range []string{`{}`, `{"limit":0}`, `{"limit":-1}`, `{"limit":21}`, `{"limit":1.5}`} {
		t.Run(body, func(t *testing.T) {
			cfg, _ := loadSystemDailyTipTestConfig(t, "ui:\n  recentProjectsLimit: 7\n")
			app := newSystemDailyTipTestApp(t, cfg)
			resp := mustSystemDailyTipTestRequest(t, app, http.MethodPost,
				"/api/v1/system/recent-projects-settings/update", bytes.NewBufferString(body))
			if resp.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnprocessableEntity)
			}
			if cfg.UI.RecentProjectsLimit != 7 {
				t.Fatalf("invalid update changed limit to %d", cfg.UI.RecentProjectsLimit)
			}
		})
	}
}
