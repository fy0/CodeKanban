package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"

	"code-kanban/api/h"
	"code-kanban/utils"
	gitutil "code-kanban/utils/git"
)

func TestGitOpenAPIContract(t *testing.T) {
	previous := gitutil.CurrentEngineSettings()
	t.Cleanup(func() { gitutil.ConfigureEngines(previous) })
	app := fiber.New()
	cfg := &utils.AppConfig{OpenAPIEnabled: true}
	humaAPI, group := h.NewAPI(app, cfg)
	humaTypesRegister()
	registerWorktreeRoutes(group, cfg)
	registerBranchRoutes(group)
	registerSystemRoutes(group, cfg, nil, nil)

	raw, err := json.Marshal(humaAPI.OpenAPI())
	if err != nil {
		t.Fatalf("marshal OpenAPI: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode OpenAPI: %v", err)
	}
	paths := document["paths"].(map[string]any)
	if _, ok := paths["/api/v1/projects/{projectId}/git-capabilities"]; !ok {
		t.Fatal("Git capability endpoint is missing from OpenAPI")
	}
	if _, ok := paths["/api/v1/system/git-settings"]; !ok {
		t.Fatal("Git settings endpoint is missing from OpenAPI")
	}
	if _, ok := paths["/api/v1/system/git-settings/update"]; !ok {
		t.Fatal("Git settings update endpoint is missing from OpenAPI")
	}
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	mergeSchema := schemas["MergeBranchBody"].(map[string]any)
	properties := mergeSchema["properties"].(map[string]any)
	if _, ok := properties["commit"]; !ok {
		t.Fatal("merge schema does not expose squash commit control")
	}
	if _, ok := properties["commitMessage"]; !ok {
		t.Fatal("merge schema does not expose squash commit message")
	}
	strategy := properties["strategy"].(map[string]any)
	values := strategy["enum"].([]any)
	if len(values) != 3 {
		t.Fatalf("merge strategy enum = %#v, want merge/rebase/squash", values)
	}
	capabilitySchema, ok := schemas["GitCapabilityResult"].(map[string]any)
	if !ok {
		t.Fatal("GitCapabilityResult schema is missing")
	}
	capabilityProperties := capabilitySchema["properties"].(map[string]any)
	if _, ok := capabilityProperties["engines"]; !ok {
		t.Fatal("GitCapabilityResult schema does not expose selected engines")
	}
	operationSchema := schemas["OperationCapabilities"].(map[string]any)
	operationProperties := operationSchema["properties"].(map[string]any)
	for _, name := range []string{"merge", "rebase", "squash"} {
		if _, ok := operationProperties[name]; !ok {
			t.Fatalf("OperationCapabilities schema does not expose %s", name)
		}
	}
	if _, ok := schemas["OperationEngines"]; !ok {
		t.Fatal("OperationEngines schema is missing")
	}
	gitConfigSchema := schemas["GitConfig"].(map[string]any)
	gitConfigProperties := gitConfigSchema["properties"].(map[string]any)
	for _, name := range []string{"readEngine", "writeEngine"} {
		property := gitConfigProperties[name].(map[string]any)
		values := property["enum"].([]any)
		if len(values) != 3 {
			t.Fatalf("GitConfig %s enum = %#v, want auto/builtin/system", name, values)
		}
	}
}

func TestGitOperationErrorResponseIncludesStableCode(t *testing.T) {
	app := fiber.New()
	_, group := h.NewAPI(app, &utils.AppConfig{})
	huma.Get(group, "/git-error-contract", func(
		context.Context,
		*struct{},
	) (*h.MessageResponse, error) {
		return nil, mapGitOperationError(&gitutil.OperationError{
			Code:      gitutil.ErrorCodeNonFastForward,
			Operation: gitutil.OperationFastForwardMerge,
			Detail:    "fast-forward required",
		})
	})

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/git-error-contract", nil))
	if err != nil {
		t.Fatalf("request Git error route: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusConflict)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode response %q: %v", body, err)
	}
	if payload["code"] != gitutil.ErrorCodeNonFastForward {
		t.Fatalf("error code = %#v, want %q (body=%s)", payload["code"], gitutil.ErrorCodeNonFastForward, body)
	}
}
