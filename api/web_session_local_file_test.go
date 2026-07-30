package api

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/service/websession"

	"github.com/gofiber/fiber/v2"
)

func TestResolveWebSessionLocalFileAllowsSessionAndTemporaryFiles(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "session")
	temporaryRoot := filepath.Join(root, "temporary")
	for _, directory := range []string{cwd, temporaryRoot} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", directory, err)
		}
	}

	sessionFile := filepath.Join(cwd, "reports", "session.csv")
	if err := os.MkdirAll(filepath.Dir(sessionFile), 0o755); err != nil {
		t.Fatalf("mkdir session report directory: %v", err)
	}
	if err := os.WriteFile(sessionFile, []byte("session"), 0o644); err != nil {
		t.Fatalf("write session file: %v", err)
	}
	temporaryFile := filepath.Join(temporaryRoot, "temporary.csv")
	if err := os.WriteFile(temporaryFile, []byte("temporary"), 0o644); err != nil {
		t.Fatalf("write temporary file: %v", err)
	}

	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "absolute session path", path: sessionFile, want: sessionFile},
		{name: "relative session path", path: filepath.Join("reports", "session.csv"), want: sessionFile},
		{name: "temporary path", path: temporaryFile, want: temporaryFile},
	} {
		t.Run(test.name, func(t *testing.T) {
			item, err := resolveWebSessionLocalFile(test.path, cwd, temporaryRoot)
			if err != nil {
				t.Fatalf("resolveWebSessionLocalFile returned error: %v", err)
			}
			want, err := filepath.EvalSymlinks(test.want)
			if err != nil {
				t.Fatalf("EvalSymlinks(%s): %v", test.want, err)
			}
			if !pathsReferToSameLocation(item.Path, want) {
				t.Fatalf("path = %q, want %q", item.Path, want)
			}
			if !item.Info.Mode().IsRegular() {
				t.Fatalf("resolved item is not a regular file: %s", item.Info.Mode())
			}
		})
	}
}

func TestResolveWebSessionLocalFileRejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "session")
	temporaryRoot := filepath.Join(root, "temporary")
	outsideRoot := filepath.Join(root, "outside")
	for _, directory := range []string{cwd, temporaryRoot, outsideRoot} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", directory, err)
		}
	}
	outsideFile := filepath.Join(outsideRoot, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	for _, test := range []struct {
		name    string
		path    string
		wantErr error
	}{
		{name: "empty", path: "", wantErr: errWebSessionLocalFilePathRequired},
		{name: "outside absolute", path: outsideFile, wantErr: errWebSessionLocalFileOutsideRoots},
		{
			name:    "relative traversal",
			path:    filepath.Join("..", "outside", "secret.txt"),
			wantErr: errWebSessionLocalFileOutsideRoots,
		},
		{name: "directory", path: cwd, wantErr: errWebSessionLocalFileNotRegular},
		{name: "missing", path: filepath.Join(cwd, "missing.txt"), wantErr: fs.ErrNotExist},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolveWebSessionLocalFile(test.path, cwd, temporaryRoot)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want errors.Is(_, %v)", err, test.wantErr)
			}
		})
	}
}

func TestResolveWebSessionLocalFileRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "session")
	temporaryRoot := filepath.Join(root, "temporary")
	outsideRoot := filepath.Join(root, "outside")
	for _, directory := range []string{cwd, temporaryRoot, outsideRoot} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", directory, err)
		}
	}
	outsideFile := filepath.Join(outsideRoot, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	linkPath := filepath.Join(cwd, "linked-secret.txt")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}

	_, err := resolveWebSessionLocalFile(linkPath, cwd, temporaryRoot)
	if !errors.Is(err, errWebSessionLocalFileOutsideRoots) {
		t.Fatalf("error = %v, want outside-roots error", err)
	}
}

func TestWebSessionLocalFileRoutesDownloadAndOpenLocation(t *testing.T) {
	cwd := t.TempDir()
	filePath := filepath.Join(cwd, "report.csv")
	wantBody := "name,value\nexample,1\n"
	if err := os.WriteFile(filePath, []byte(wantBody), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	setupWebSessionLocalFileTestDB(t)
	session := &tables.WebSessionTable{
		ProjectID:       "project-1",
		OrderIndex:      1,
		Agent:           "codex",
		Title:           "Local file test",
		WorkflowMode:    "default",
		PermissionLevel: "elevated",
		Cwd:             cwd,
		Status:          "idle",
		ActivityAt:      time.Now(),
	}
	session.Init()
	if err := model.GetDB().Create(session).Error; err != nil {
		t.Fatalf("create web session: %v", err)
	}

	var openedDirectory string
	controller := &webSessionController{
		manager: new(websession.Manager),
		openExplorer: func(path string) error {
			openedDirectory = path
			return nil
		},
	}
	app := fiber.New(fiber.Config{Immutable: true})
	controller.registerLocalFileRoutes(app)

	contentURL := "/api/v1/projects/project-1/web-sessions/" + session.ID +
		"/local-files/content?path=" + url.QueryEscape(filePath)
	headResponse := requestWebSessionLocalFile(t, app, http.MethodHead, contentURL, "")
	if headResponse.StatusCode != http.StatusOK {
		t.Fatalf("HEAD status = %d, want %d", headResponse.StatusCode, http.StatusOK)
	}
	if got := headResponse.Header.Get("Content-Disposition"); !strings.Contains(got, "report.csv") {
		t.Fatalf("Content-Disposition = %q, want report.csv filename", got)
	}
	if got := headResponse.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := headResponse.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	headResponse.Body.Close()

	getResponse := requestWebSessionLocalFile(t, app, http.MethodGet, contentURL, "")
	if getResponse.StatusCode != http.StatusOK {
		getResponse.Body.Close()
		t.Fatalf("GET status = %d, want %d", getResponse.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(getResponse.Body)
	getResponse.Body.Close()
	if err != nil {
		t.Fatalf("read download body: %v", err)
	}
	if string(body) != wantBody {
		t.Fatalf("download body = %q, want %q", string(body), wantBody)
	}

	openURL := "/api/v1/projects/project-1/web-sessions/" + session.ID +
		"/local-files/open-location"
	openResponse := requestWebSessionLocalFile(
		t,
		app,
		http.MethodPost,
		openURL,
		`{"path":`+strconv.Quote(filePath)+`}`,
	)
	openResponse.Body.Close()
	if openResponse.StatusCode != http.StatusOK {
		t.Fatalf("open-location status = %d, want %d", openResponse.StatusCode, http.StatusOK)
	}
	wantDirectory, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatalf("EvalSymlinks(cwd): %v", err)
	}
	if !pathsReferToSameLocation(openedDirectory, wantDirectory) {
		t.Fatalf("opened directory = %q, want %q", openedDirectory, wantDirectory)
	}

	mismatchURL := strings.Replace(contentURL, "/projects/project-1/", "/projects/project-2/", 1)
	mismatchResponse := requestWebSessionLocalFile(t, app, http.MethodHead, mismatchURL, "")
	mismatchResponse.Body.Close()
	if mismatchResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("project mismatch status = %d, want %d", mismatchResponse.StatusCode, http.StatusNotFound)
	}
}

func setupWebSessionLocalFileTestDB(t *testing.T) {
	t.Helper()
	model.DBClose()
	dsn := filepath.Join(t.TempDir(), "web-session-local-file.db")
	if err := model.InitWithDSN(dsn, 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}
	t.Cleanup(model.DBClose)
}

func requestWebSessionLocalFile(
	t *testing.T,
	app *fiber.App,
	method string,
	target string,
	body string,
) *http.Response {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("app.Test returned error: %v", err)
	}
	return response
}
