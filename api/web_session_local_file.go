package api

import (
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"code-kanban/utils/system"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

const webSessionLocalFileRoute = "/api/v1/projects/:projectId/web-sessions/:sessionId/local-files"

var (
	errWebSessionLocalFilePathRequired = errors.New("path is required")
	errWebSessionLocalFileRootInvalid  = errors.New("session file root is invalid")
	errWebSessionLocalFilePathInvalid  = errors.New("path is invalid for this host")
	errWebSessionLocalFileOutsideRoots = errors.New("file is outside the session and temporary directories")
	errWebSessionLocalFileNotRegular   = errors.New("path is not a regular file")
)

type webSessionLocalFile struct {
	Path string
	Info os.FileInfo
}

type webSessionLocalFileRoot struct {
	absolutePath string
	realPath     string
}

func (c *webSessionController) registerLocalFileRoutes(app *fiber.App) {
	app.Head(webSessionLocalFileRoute+"/content", c.probeLocalFileContent)
	app.Get(webSessionLocalFileRoute+"/content", c.serveLocalFileContent)
	app.Post(webSessionLocalFileRoute+"/open-location", c.openLocalFileLocation)
}

func (c *webSessionController) probeLocalFileContent(ctx *fiber.Ctx) error {
	item, err := c.resolveLocalFileRequest(ctx, ctx.Query("path"))
	if err != nil {
		return err
	}
	setWebSessionLocalFileHeaders(ctx, item)
	ctx.Status(http.StatusOK)
	return nil
}

func (c *webSessionController) serveLocalFileContent(ctx *fiber.Ctx) error {
	item, err := c.resolveLocalFileRequest(ctx, ctx.Query("path"))
	if err != nil {
		return err
	}
	setWebSessionLocalFileHeaders(ctx, item)
	if err := sendWebSessionFileStream(ctx, item.Path, item.Info.Size()); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fiber.NewError(http.StatusNotFound, "file not found")
		}
		if errors.Is(err, fs.ErrPermission) {
			return fiber.NewError(http.StatusForbidden, err.Error())
		}
		return fiber.NewError(http.StatusInternalServerError, "failed to read local file")
	}
	return nil
}

func (c *webSessionController) openLocalFileLocation(ctx *fiber.Ctx) error {
	var input struct {
		Path string `json:"path"`
	}
	if err := ctx.BodyParser(&input); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid request body")
	}

	item, err := c.resolveLocalFileRequest(ctx, input.Path)
	if err != nil {
		return err
	}

	openExplorer := c.openExplorer
	if openExplorer == nil {
		openExplorer = system.OpenExplorer
	}
	if err := openExplorer(filepath.Dir(item.Path)); err != nil {
		switch {
		case errors.Is(err, system.ErrUnsupportedOS):
			return fiber.NewError(http.StatusNotImplemented, err.Error())
		case errors.Is(err, system.ErrNoFileManager):
			return fiber.NewError(http.StatusServiceUnavailable, err.Error())
		default:
			return fiber.NewError(http.StatusInternalServerError, "failed to open file location")
		}
	}

	return ctx.Status(http.StatusOK).JSON(fiber.Map{"message": "file location opened"})
}

func (c *webSessionController) resolveLocalFileRequest(
	ctx *fiber.Ctx,
	rawPath string,
) (webSessionLocalFile, error) {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	sessionID := strings.TrimSpace(ctx.Params("sessionId"))
	if projectID == "" || sessionID == "" {
		return webSessionLocalFile{}, fiber.NewError(
			http.StatusBadRequest,
			"projectId and sessionId are required",
		)
	}
	if c.manager == nil {
		return webSessionLocalFile{}, fiber.NewError(
			http.StatusInternalServerError,
			"web session manager is unavailable",
		)
	}

	session, err := c.manager.GetSession(ctx.UserContext(), sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return webSessionLocalFile{}, fiber.NewError(http.StatusNotFound, "web session not found")
		}
		return webSessionLocalFile{}, fiber.NewError(
			http.StatusInternalServerError,
			"failed to load web session",
		)
	}
	if session.ProjectID != projectID {
		return webSessionLocalFile{}, fiber.NewError(http.StatusNotFound, "web session not found")
	}

	item, err := resolveWebSessionLocalFile(rawPath, session.Cwd, os.TempDir())
	if err == nil {
		return item, nil
	}
	switch {
	case errors.Is(err, errWebSessionLocalFilePathRequired),
		errors.Is(err, errWebSessionLocalFileRootInvalid),
		errors.Is(err, errWebSessionLocalFilePathInvalid),
		errors.Is(err, errWebSessionLocalFileNotRegular):
		return webSessionLocalFile{}, fiber.NewError(http.StatusBadRequest, err.Error())
	case errors.Is(err, errWebSessionLocalFileOutsideRoots), errors.Is(err, fs.ErrPermission):
		return webSessionLocalFile{}, fiber.NewError(http.StatusForbidden, err.Error())
	case errors.Is(err, fs.ErrNotExist):
		return webSessionLocalFile{}, fiber.NewError(http.StatusNotFound, "file not found")
	default:
		return webSessionLocalFile{}, fiber.NewError(
			http.StatusInternalServerError,
			"failed to read local file",
		)
	}
}

func resolveWebSessionLocalFile(
	rawPath string,
	sessionCwd string,
	temporaryRoot string,
) (webSessionLocalFile, error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return webSessionLocalFile{}, errWebSessionLocalFilePathRequired
	}

	roots := make([]webSessionLocalFileRoot, 0, 2)
	for _, rootPath := range []string{sessionCwd, temporaryRoot} {
		root, err := prepareWebSessionLocalFileRoot(rootPath)
		if err == nil && !containsWebSessionLocalFileRoot(roots, root) {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		return webSessionLocalFile{}, errWebSessionLocalFileRootInvalid
	}

	isAbsolute := filepath.IsAbs(path)
	if looksLikeWindowsAbsolutePath(path) {
		if runtime.GOOS != "windows" {
			return webSessionLocalFile{}, errWebSessionLocalFilePathInvalid
		}
		isAbsolute = true
	}
	if !isAbsolute {
		cwd := strings.TrimSpace(sessionCwd)
		if cwd == "" || !filepath.IsAbs(cwd) {
			return webSessionLocalFile{}, errWebSessionLocalFileRootInvalid
		}
		path = filepath.Join(cwd, path)
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return webSessionLocalFile{}, fmt.Errorf("%w: %v", errWebSessionLocalFilePathInvalid, err)
	}
	absolutePath = filepath.Clean(absolutePath)
	if !isWebSessionLocalFileWithinRoots(absolutePath, roots, false) {
		return webSessionLocalFile{}, errWebSessionLocalFileOutsideRoots
	}

	realPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return webSessionLocalFile{}, err
	}
	realPath, err = filepath.Abs(realPath)
	if err != nil {
		return webSessionLocalFile{}, fmt.Errorf("%w: %v", errWebSessionLocalFilePathInvalid, err)
	}
	realPath = filepath.Clean(realPath)
	if !isWebSessionLocalFileWithinRoots(realPath, roots, true) {
		return webSessionLocalFile{}, errWebSessionLocalFileOutsideRoots
	}

	info, err := os.Stat(realPath)
	if err != nil {
		return webSessionLocalFile{}, err
	}
	if !info.Mode().IsRegular() {
		return webSessionLocalFile{}, errWebSessionLocalFileNotRegular
	}
	return webSessionLocalFile{Path: realPath, Info: info}, nil
}

func prepareWebSessionLocalFileRoot(rawRoot string) (webSessionLocalFileRoot, error) {
	root := strings.TrimSpace(rawRoot)
	if root == "" || !filepath.IsAbs(root) {
		return webSessionLocalFileRoot{}, errWebSessionLocalFileRootInvalid
	}
	absolutePath, err := filepath.Abs(root)
	if err != nil {
		return webSessionLocalFileRoot{}, err
	}
	absolutePath = filepath.Clean(absolutePath)
	realPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return webSessionLocalFileRoot{}, err
	}
	realPath, err = filepath.Abs(realPath)
	if err != nil {
		return webSessionLocalFileRoot{}, err
	}
	return webSessionLocalFileRoot{
		absolutePath: absolutePath,
		realPath:     filepath.Clean(realPath),
	}, nil
}

func containsWebSessionLocalFileRoot(
	roots []webSessionLocalFileRoot,
	candidate webSessionLocalFileRoot,
) bool {
	for _, root := range roots {
		if pathsReferToSameLocation(root.realPath, candidate.realPath) {
			return true
		}
	}
	return false
}

func isWebSessionLocalFileWithinRoots(
	path string,
	roots []webSessionLocalFileRoot,
	useRealPath bool,
) bool {
	for _, root := range roots {
		rootPath := root.absolutePath
		if useRealPath {
			rootPath = root.realPath
		}
		if pathWithinRoot(path, rootPath) {
			return true
		}
		if !useRealPath && pathWithinRoot(path, root.realPath) {
			return true
		}
	}
	return false
}

func pathWithinRoot(path string, root string) bool {
	relativePath, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(relativePath) {
		return false
	}
	return relativePath == "." ||
		(relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator)))
}

func pathsReferToSameLocation(left string, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func setWebSessionLocalFileHeaders(ctx *fiber.Ctx, item webSessionLocalFile) {
	disposition := mime.FormatMediaType("attachment", map[string]string{
		"filename": item.Info.Name(),
	})
	if disposition == "" {
		disposition = "attachment"
	}
	ctx.Set(fiber.HeaderContentType, "application/octet-stream")
	ctx.Set(fiber.HeaderContentDisposition, disposition)
	ctx.Set(fiber.HeaderCacheControl, "no-store")
	ctx.Set("X-Content-Type-Options", "nosniff")
	ctx.Set(fiber.HeaderContentLength, strconv.FormatInt(item.Info.Size(), 10))
}

func sendWebSessionFileStream(ctx *fiber.Ctx, path string, size int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	maxInt := int64(^uint(0) >> 1)
	if size >= 0 && size <= maxInt {
		return ctx.SendStream(file, int(size))
	}
	return ctx.SendStream(file)
}
