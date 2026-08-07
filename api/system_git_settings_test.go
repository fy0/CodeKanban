package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	gitutil "code-kanban/utils/git"
)

type gitSettingsResponseEnvelope struct {
	Item gitSettingsResult `json:"item"`
}

func TestSystemGitSettingsUpdatePersistsAndApplies(t *testing.T) {
	previous := gitutil.CurrentEngineSettings()
	t.Cleanup(func() { gitutil.ConfigureEngines(previous) })
	cfg, configPath := loadSystemDailyTipTestConfig(t, `
git:
  readEngine: auto
  writeEngine: auto
`)
	app := newSystemDailyTipTestApp(t, cfg)

	resp := mustSystemDailyTipTestRequest(
		t,
		app,
		http.MethodPost,
		"/api/v1/system/git-settings/update",
		bytes.NewBufferString(`{"readEngine":"builtin","writeEngine":"system","executable":""}`),
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var payload gitSettingsResponseEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Item.ReadEngine != "builtin" || payload.Item.WriteEngine != "system" {
		t.Fatalf("unexpected response: %#v", payload.Item)
	}
	runtimeSettings := gitutil.CurrentEngineSettings()
	if runtimeSettings.Read != gitutil.EnginePreferenceBuiltin || runtimeSettings.Write != gitutil.EnginePreferenceSystem {
		t.Fatalf("runtime settings were not updated: %#v", runtimeSettings)
	}
	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(written)
	if !strings.Contains(text, "readEngine: builtin") || !strings.Contains(text, "writeEngine: system") {
		t.Fatalf("Git settings were not persisted:\n%s", text)
	}
}

func TestSystemGitSettingsRejectsUnknownEngine(t *testing.T) {
	previous := gitutil.CurrentEngineSettings()
	t.Cleanup(func() { gitutil.ConfigureEngines(previous) })
	cfg, _ := loadSystemDailyTipTestConfig(t, `
git:
  readEngine: auto
  writeEngine: auto
`)
	app := newSystemDailyTipTestApp(t, cfg)

	resp := mustSystemDailyTipTestRequest(
		t,
		app,
		http.MethodPost,
		"/api/v1/system/git-settings/update",
		bytes.NewBufferString(`{"readEngine":"unknown","writeEngine":"system","executable":""}`),
	)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnprocessableEntity)
	}
	settings := gitutil.CurrentEngineSettings()
	if settings.Read != gitutil.EnginePreferenceAuto || settings.Write != gitutil.EnginePreferenceAuto {
		t.Fatalf("invalid request changed runtime settings: %#v", settings)
	}
}
