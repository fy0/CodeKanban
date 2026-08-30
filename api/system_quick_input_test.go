package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"code-kanban/utils"
)

type webSessionQuickInputResponse struct {
	Item utils.WebSessionQuickInputView `json:"item"`
}

func TestSystemWebSessionQuickInputUsesScopedIncrementalRoutes(t *testing.T) {
	cfg, _ := loadSystemDailyTipTestConfig(t, `
ui:
  webSessionQuickInput:
    pinned: [continue]
    recent: [global one]
    recentByProject:
      project-1: [project one]
`)
	app := newSystemDailyTipTestApp(t, cfg)

	initial := requestQuickInputView(t, app, http.MethodGet, "/api/v1/system/web-session-quick-input?projectId=project-1", nil)
	if !reflect.DeepEqual(initial.Item.Pinned, []string{"continue"}) {
		t.Fatalf("initial pinned items = %#v", initial.Item.Pinned)
	}
	if !reflect.DeepEqual(initial.Item.GlobalRecent, []string{"global one"}) {
		t.Fatalf("initial global history = %#v", initial.Item.GlobalRecent)
	}
	if !reflect.DeepEqual(initial.Item.ProjectRecent, []string{"project one"}) {
		t.Fatalf("initial project history = %#v", initial.Item.ProjectRecent)
	}

	recorded := requestQuickInputView(
		t,
		app,
		http.MethodPost,
		"/api/v1/system/web-session-quick-input/recent",
		bytes.NewBufferString(`{"text":"  project two  ","projectId":" project-1 "}`),
	)
	if recorded.Item.ProjectID != "project-1" {
		t.Fatalf("normalized project ID = %q", recorded.Item.ProjectID)
	}
	if !reflect.DeepEqual(recorded.Item.GlobalRecent, []string{"project two", "global one"}) {
		t.Fatalf("project prompt was not added to global history: %#v", recorded.Item.GlobalRecent)
	}
	if !reflect.DeepEqual(recorded.Item.ProjectRecent, []string{"project two", "project one"}) {
		t.Fatalf("project history = %#v", recorded.Item.ProjectRecent)
	}

	deduplicated := requestQuickInputView(
		t,
		app,
		http.MethodPost,
		"/api/v1/system/web-session-quick-input/recent",
		bytes.NewBufferString(`{"text":"project one","projectId":"project-1"}`),
	)
	if !reflect.DeepEqual(deduplicated.Item.GlobalRecent, []string{"project one", "project two", "global one"}) {
		t.Fatalf("deduplicated global history = %#v", deduplicated.Item.GlobalRecent)
	}
	if !reflect.DeepEqual(deduplicated.Item.ProjectRecent, []string{"project one", "project two"}) {
		t.Fatalf("deduplicated project history = %#v", deduplicated.Item.ProjectRecent)
	}

	pinned := requestQuickInputView(
		t,
		app,
		http.MethodPost,
		"/api/v1/system/web-session-quick-input/pinned/update",
		bytes.NewBufferString(`{"items":[" Plan ","","Plan","Ship"]}`),
	)
	if !reflect.DeepEqual(pinned.Item.Pinned, []string{"Plan", "Ship"}) {
		t.Fatalf("normalized pinned items = %#v", pinned.Item.Pinned)
	}
	if got := cfg.UI.WebSessionQuickInput.Pinned; !reflect.DeepEqual(got, []string{"Plan", "Ship"}) {
		t.Fatalf("in-memory pinned items = %#v", got)
	}

	removedRoute := mustSystemDailyTipTestRequest(
		t,
		app,
		http.MethodPost,
		"/api/v1/system/web-session-quick-input/update",
		bytes.NewBufferString(`{"pinned":["legacy"]}`),
	)
	if removedRoute.StatusCode != http.StatusNotFound {
		t.Fatalf("removed bulk route status = %d, want %d", removedRoute.StatusCode, http.StatusNotFound)
	}
}

func requestQuickInputView(
	t *testing.T,
	app interface {
		Test(*http.Request, ...int) (*http.Response, error)
	},
	method string,
	target string,
	body *bytes.Buffer,
) webSessionQuickInputResponse {
	t.Helper()
	var payload *bytes.Buffer
	if body == nil {
		payload = bytes.NewBuffer(nil)
	} else {
		payload = body
	}
	req, err := http.NewRequest(method, target, payload)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s %s status = %d, want %d", method, target, resp.StatusCode, http.StatusOK)
	}
	var result webSessionQuickInputResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}
