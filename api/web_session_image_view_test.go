package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func newWebSessionImageViewTestApp() *fiber.App {
	app := fiber.New()
	ctrl := &webSessionController{logger: zap.NewNop()}
	app.Get("/api/v1/web-sessions/image-view", ctrl.serveImageViewPreview)
	return app
}

func TestWebSessionImageViewPreviewServesAbsolutePath(t *testing.T) {
	app := newWebSessionImageViewTestApp()
	filePath := filepath.Join(t.TempDir(), "preview.png")
	wantBody := "fake-png"
	if err := os.WriteFile(filePath, []byte(wantBody), 0o644); err != nil {
		t.Fatalf("write test image failed: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/web-sessions/image-view?path="+url.QueryEscape(filePath),
		nil,
	)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "image/png") {
		t.Fatalf("content-type = %q, want image/png", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control = %q, want no-store", got)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if string(body) != wantBody {
		t.Fatalf("body = %q, want %q", body, wantBody)
	}
}

func TestWebSessionImageViewPreviewResolvesRelativePathWithCwd(t *testing.T) {
	app := newWebSessionImageViewTestApp()
	cwd := t.TempDir()
	relativePath := filepath.Join("captures", "preview.png")
	absolutePath := filepath.Join(cwd, relativePath)
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(absolutePath, []byte("relative-image"), 0o644); err != nil {
		t.Fatalf("write test image failed: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/web-sessions/image-view?path="+url.QueryEscape(relativePath)+"&cwd="+url.QueryEscape(cwd),
		nil,
	)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestWebSessionImageViewPreviewRejectsNonImageFiles(t *testing.T) {
	app := newWebSessionImageViewTestApp()
	filePath := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(filePath, []byte("not-an-image"), 0o644); err != nil {
		t.Fatalf("write test file failed: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/web-sessions/image-view?path="+url.QueryEscape(filePath),
		nil,
	)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestWebSessionImageViewPreviewReturnsNotFoundForMissingFiles(t *testing.T) {
	app := newWebSessionImageViewTestApp()
	filePath := filepath.Join(t.TempDir(), "missing.png")

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/web-sessions/image-view?path="+url.QueryEscape(filePath),
		nil,
	)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestWebSessionImageViewPreviewRejectsEmptyPath(t *testing.T) {
	app := newWebSessionImageViewTestApp()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/web-sessions/image-view", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}
