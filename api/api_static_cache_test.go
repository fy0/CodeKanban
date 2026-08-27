package api

import (
	"embed"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"code-kanban/utils"
)

func TestMountStaticDisablesCacheForIndexOnly(t *testing.T) {
	t.Parallel()

	app := newMountedStaticTestApp(t, "/")

	assertIndexNoCache(t, mustTestRequest(t, app, http.MethodGet, "/"), "/")
	assertIndexNoCache(t, mustTestRequest(t, app, http.MethodGet, "/index.html"), "/index.html")
	assertAssetKeepsDefaultCache(t, mustTestRequest(t, app, http.MethodGet, "/app.js"), "/app.js")
	assertAssetKeepsShortCache(t, mustTestRequest(t, app, http.MethodGet, "/favicon.ico"), "/favicon.ico")
	assertAssetKeepsShortCache(t, mustTestRequest(t, app, http.MethodGet, "/favicon.svg"), "/favicon.svg")
}

func TestMountStaticDisablesCacheForIndexOnCustomWebURL(t *testing.T) {
	t.Parallel()

	app := newMountedStaticTestApp(t, "/kanban/")

	assertIndexNoCache(t, mustTestRequest(t, app, http.MethodGet, "/kanban"), "/kanban")
	assertIndexNoCache(t, mustTestRequest(t, app, http.MethodGet, "/kanban/index.html"), "/kanban/index.html")
	assertAssetKeepsDefaultCache(t, mustTestRequest(t, app, http.MethodGet, "/kanban/app.js"), "/kanban/app.js")
	assertAssetKeepsShortCache(t, mustTestRequest(t, app, http.MethodGet, "/kanban/favicon.ico"), "/kanban/favicon.ico")
	assertAssetKeepsShortCache(t, mustTestRequest(t, app, http.MethodGet, "/kanban/favicon.svg"), "/kanban/favicon.svg")
}

func newMountedStaticTestApp(t *testing.T, webURL string) *fiber.App {
	t.Helper()

	root := t.TempDir()
	staticDir := filepath.Join(root, "static")
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		t.Fatalf("create static dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<html>fresh</html>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "app.js"), []byte("console.log('cached')"), 0o644); err != nil {
		t.Fatalf("write app.js: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "favicon.ico"), []byte("ico"), 0o644); err != nil {
		t.Fatalf("write favicon.ico: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "favicon.svg"), []byte("<svg></svg>"), 0o644); err != nil {
		t.Fatalf("write favicon.svg: %v", err)
	}

	cfg := &utils.AppConfig{
		WebUrl:      webURL,
		UIOverwrite: root,
	}

	app := fiber.New()
	mountStatic(app, cfg, embed.FS{}, zap.NewNop())
	t.Cleanup(func() {
		if err := app.Shutdown(); err != nil {
			t.Errorf("shutdown test app: %v", err)
		}
	})
	return app
}

func assertIndexNoCache(t *testing.T, resp *http.Response, target string) {
	t.Helper()

	if got := resp.StatusCode; got != http.StatusOK {
		t.Fatalf("%s status = %d, want %d", target, got, http.StatusOK)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store, no-cache, must-revalidate" {
		t.Fatalf("%s Cache-Control = %q", target, got)
	}
	if got := resp.Header.Get("Pragma"); got != "no-cache" {
		t.Fatalf("%s Pragma = %q", target, got)
	}
	if got := resp.Header.Get("Expires"); got != "0" {
		t.Fatalf("%s Expires = %q", target, got)
	}
}

func assertAssetKeepsDefaultCache(t *testing.T, resp *http.Response, target string) {
	t.Helper()

	if got := resp.StatusCode; got != http.StatusOK {
		t.Fatalf("%s status = %d, want %d", target, got, http.StatusOK)
	}
	if got := resp.Header.Get("Cache-Control"); got != "public, max-age=2592000" {
		t.Fatalf("%s Cache-Control = %q", target, got)
	}
	if got := resp.Header.Get("Pragma"); got != "" {
		t.Fatalf("%s Pragma = %q", target, got)
	}
	if got := resp.Header.Get("Expires"); got != "" {
		t.Fatalf("%s Expires = %q", target, got)
	}
}

func assertAssetKeepsShortCache(t *testing.T, resp *http.Response, target string) {
	t.Helper()

	if got := resp.StatusCode; got != http.StatusOK {
		t.Fatalf("%s status = %d, want %d", target, got, http.StatusOK)
	}
	if got := resp.Header.Get("Cache-Control"); got != "public, max-age=300" {
		t.Fatalf("%s Cache-Control = %q", target, got)
	}
	if got := resp.Header.Get("Pragma"); got != "" {
		t.Fatalf("%s Pragma = %q", target, got)
	}
	if got := resp.Header.Get("Expires"); got != "" {
		t.Fatalf("%s Expires = %q", target, got)
	}
}

func mustTestRequest(t *testing.T, app *fiber.App, method, target string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, target, err)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test %s %s: %v", method, target, err)
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		_ = resp.Body.Close()
		t.Fatalf("drain response %s %s: %v", method, target, err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close response %s %s: %v", method, target, err)
	}
	return resp
}
