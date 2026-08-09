package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"code-kanban/utils"
)

type webSessionQuickInputResponse struct {
	Item utils.WebSessionQuickInputConfig `json:"item"`
}

func TestSystemWebSessionQuickInputUpdateNormalizesProjectHistory(t *testing.T) {
	cfg, configPath := loadSystemDailyTipTestConfig(t, `
ui:
  webSessionQuickInput:
    pinned: [continue]
    recent: [global one]
    recentByProject:
      project-1: [project one]
`)
	app := newSystemDailyTipTestApp(t, cfg)

	recent := make([]string, 0, 31)
	for index := 1; index <= 31; index++ {
		recent = append(recent, fmt.Sprintf("project prompt %02d", index))
	}
	body, err := json.Marshal(utils.WebSessionQuickInputConfig{
		Pinned: []string{" Plan "},
		Recent: []string{" global two "},
		RecentByProject: map[string][]string{
			" project-2 ": recent,
		},
	})
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}

	resp := mustSystemDailyTipTestRequest(
		t,
		app,
		http.MethodPost,
		"/api/v1/system/web-session-quick-input/update",
		bytes.NewBuffer(body),
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var payload webSessionQuickInputResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if got := payload.Item.RecentByProject["project-2"]; len(got) != 30 {
		t.Fatalf("project recent length = %d, want 30", len(got))
	}
	if got := payload.Item.RecentByProject["project-2"][0]; got != recent[0] {
		t.Fatalf("project recent first item = %q, want %q", got, recent[0])
	}
	if got := cfg.UI.WebSessionQuickInput.Pinned; len(got) != 1 || got[0] != "Plan" {
		t.Fatalf("unexpected pinned items: %#v", got)
	}

	legacyUpdate := mustSystemDailyTipTestRequest(
		t,
		app,
		http.MethodPost,
		"/api/v1/system/web-session-quick-input/update",
		bytes.NewBufferString(`{"pinned":["Continue"],"recent":["legacy"]}`),
	)
	if legacyUpdate.StatusCode != http.StatusOK {
		t.Fatalf("legacy update status = %d, want %d", legacyUpdate.StatusCode, http.StatusOK)
	}
	if _, ok := cfg.UI.WebSessionQuickInput.RecentByProject["project-2"]; !ok {
		t.Fatal("legacy update removed project history")
	}

	persisted, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	if !strings.Contains(string(persisted), "recentByProject") {
		t.Fatalf("expected project history to be persisted, got:\n%s", string(persisted))
	}
}
