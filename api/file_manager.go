package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"code-kanban/api/h"
	"code-kanban/service/filemanager"
	"code-kanban/utils"
)

type fileManagerController struct {
	service *filemanager.Service
	logger  *zap.Logger
}

func registerFileManagerRoutes(app *fiber.App, cfg *utils.AppConfig, logger *zap.Logger, bgCtx context.Context) error {
	service, err := filemanager.NewService(filemanager.Config{
		DataDir: utils.GetDataDir(),
	}, logger)
	if err != nil {
		return err
	}
	service.StartBackground(bgCtx)

	ctrl := &fileManagerController{
		service: service,
		logger:  logger.Named("file-manager-controller"),
	}

	base := "/api/v1/projects/:projectId/files"
	app.Get(base+"/scopes", ctrl.handleListScopes)
	app.Get(base+"/changes", ctrl.handleListChanges)
	app.Get(base+"/changes-summary", ctrl.handleChangesSummary)
	app.Get(base+"/list", ctrl.handleList)
	app.Get(base+"/search", ctrl.handleSearch)
	app.Get(base+"/preview", ctrl.handlePreview)
	app.Get(base+"/diff", ctrl.handleDiff)
	app.Get(base+"/content", ctrl.handleContent)
	app.Post(base+"/directories", ctrl.handleCreateDirectory)
	app.Post(base+"/rename", ctrl.handleRename)
	app.Post(base+"/copy", ctrl.handleCopy)
	app.Post(base+"/move", ctrl.handleMove)
	app.Post(base+"/delete", ctrl.handleDelete)
	app.Post(base+"/archives", ctrl.handleCreateArchive)
	app.Get(base+"/archives/:archiveId", ctrl.handleDownloadArchive)
	app.Post(base+"/upload-sessions", ctrl.handleCreateUploadSession)
	app.Get(base+"/upload-sessions/:uploadId", ctrl.handleGetUploadSession)
	app.Patch(base+"/upload-sessions/:uploadId", ctrl.handleUploadChunk)
	app.Post(base+"/upload-sessions/:uploadId/complete", ctrl.handleCompleteUpload)
	app.Delete(base+"/upload-sessions/:uploadId", ctrl.handleCancelUpload)

	return nil
}

func (c *fileManagerController) handleListScopes(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	scopes, err := c.service.ListScopes(ctx.UserContext(), projectID)
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemsResponse(scopes)
	resp.Status = http.StatusOK
	return ctx.Status(http.StatusOK).JSON(resp)
}

func (c *fileManagerController) handleListChanges(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	scopeID := ctx.Query("scopeId")
	includeUntracked, err := parseBoolQueryWithDefault(ctx.Query("includeUntracked"), true)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid includeUntracked")
	}
	withStats, err := parseBoolQueryWithDefault(ctx.Query("withStats"), true)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid withStats")
	}
	fresh, err := parseBoolQuery(ctx.Query("fresh"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid fresh")
	}

	var timeout time.Duration
	if raw := strings.TrimSpace(ctx.Query("timeoutMs")); raw != "" {
		timeoutMs, err := strconv.Atoi(raw)
		if err != nil || timeoutMs < 0 {
			return fiber.NewError(http.StatusBadRequest, "invalid timeoutMs")
		}
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}

	maxEntries := 0
	if raw := strings.TrimSpace(ctx.Query("maxEntries")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return fiber.NewError(http.StatusBadRequest, "invalid maxEntries")
		}
		maxEntries = parsed
	}

	item, err := c.service.ListChanges(ctx.UserContext(), projectID, scopeID, filemanager.ListChangesOptions{
		IncludeUntracked: &includeUntracked,
		WithStats:        &withStats,
		Fresh:            fresh,
		Timeout:          timeout,
		MaxEntries:       maxEntries,
	})
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(item)
	resp.Status = http.StatusOK
	return ctx.Status(http.StatusOK).JSON(resp)
}

func (c *fileManagerController) handleChangesSummary(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	scopeID := ctx.Query("scopeId")
	includeUntracked, err := parseBoolQuery(ctx.Query("includeUntracked"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid includeUntracked")
	}
	withStats, err := parseBoolQuery(ctx.Query("withStats"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid withStats")
	}
	fresh, err := parseBoolQuery(ctx.Query("fresh"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid fresh")
	}

	var timeout time.Duration
	if raw := strings.TrimSpace(ctx.Query("timeoutMs")); raw != "" {
		timeoutMs, err := strconv.Atoi(raw)
		if err != nil || timeoutMs < 0 {
			return fiber.NewError(http.StatusBadRequest, "invalid timeoutMs")
		}
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}

	item, err := c.service.ChangesSummary(ctx.UserContext(), projectID, scopeID, filemanager.ChangesSummaryOptions{
		IncludeUntracked: includeUntracked,
		WithStats:        withStats,
		Fresh:            fresh,
		StatsTimeout:     timeout,
	})
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(item)
	resp.Status = http.StatusOK
	return ctx.Status(http.StatusOK).JSON(resp)
}

func (c *fileManagerController) handleList(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	scopeID := ctx.Query("scopeId")
	path := ctx.Query("path")
	item, err := c.service.List(ctx.UserContext(), projectID, scopeID, path)
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(item)
	resp.Status = http.StatusOK
	return ctx.Status(http.StatusOK).JSON(resp)
}

func (c *fileManagerController) handleSearch(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	scopeID := ctx.Query("scopeId")
	path := ctx.Query("path")
	query := ctx.Query("query")
	useRegex, err := parseBoolQuery(ctx.Query("regex"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid regex")
	}
	item, err := c.service.Search(ctx.UserContext(), projectID, scopeID, path, query, useRegex)
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(item)
	resp.Status = http.StatusOK
	return ctx.Status(http.StatusOK).JSON(resp)
}

func parseBoolQuery(value string) (bool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(trimmed)
	if err != nil {
		return false, err
	}
	return parsed, nil
}

func parseBoolQueryWithDefault(value string, defaultValue bool) (bool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return defaultValue, nil
	}
	return strconv.ParseBool(trimmed)
}

func (c *fileManagerController) handlePreview(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	scopeID := ctx.Query("scopeId")
	path := ctx.Query("path")
	item, err := c.service.Preview(ctx.UserContext(), projectID, scopeID, path)
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(struct {
		*filemanager.PreviewResult
		InlineURL   string `json:"inlineUrl"`
		DownloadURL string `json:"downloadUrl"`
	}{
		PreviewResult: item,
		InlineURL:     buildFileContentURL(projectID, scopeID, path, "inline"),
		DownloadURL:   buildFileContentURL(projectID, scopeID, path, "attachment"),
	})
	resp.Status = http.StatusOK
	return ctx.Status(http.StatusOK).JSON(resp)
}

func (c *fileManagerController) handleDiff(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	scopeID := ctx.Query("scopeId")
	path := ctx.Query("path")
	item, err := c.service.Diff(ctx.UserContext(), projectID, scopeID, path)
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(item)
	resp.Status = http.StatusOK
	return ctx.Status(http.StatusOK).JSON(resp)
}

func (c *fileManagerController) handleContent(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	scopeID := ctx.Query("scopeId")
	path := ctx.Query("path")
	disposition := strings.ToLower(strings.TrimSpace(ctx.Query("disposition")))
	if disposition != "attachment" {
		disposition = "inline"
	}

	_, filePath, info, normalizedPath, err := c.service.ResolveFile(ctx.UserContext(), projectID, scopeID, path)
	if err != nil {
		return c.writeError(ctx, err)
	}
	contentType := mimeTypeForDownload(info.Name())
	if contentType != "" {
		ctx.Set(fiber.HeaderContentType, contentType)
	}
	ctx.Set(fiber.HeaderContentDisposition, fmt.Sprintf("%s; filename=%q", disposition, info.Name()))
	ctx.Set("X-File-Path", normalizedPath)
	return ctx.SendFile(filePath, false)
}

func (c *fileManagerController) handleCreateDirectory(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	var body struct {
		ScopeID    string `json:"scopeId"`
		ParentPath string `json:"parentPath"`
		Name       string `json:"name"`
	}
	if err := ctx.BodyParser(&body); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid request body")
	}
	item, err := c.service.CreateDirectory(ctx.UserContext(), projectID, body.ScopeID, body.ParentPath, body.Name)
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(item)
	resp.Status = http.StatusCreated
	return ctx.Status(http.StatusCreated).JSON(resp)
}

func (c *fileManagerController) handleRename(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	var body struct {
		ScopeID string `json:"scopeId"`
		Path    string `json:"path"`
		NewName string `json:"newName"`
	}
	if err := ctx.BodyParser(&body); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid request body")
	}
	item, err := c.service.Rename(ctx.UserContext(), projectID, body.ScopeID, body.Path, body.NewName)
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(item)
	resp.Status = http.StatusOK
	return ctx.Status(http.StatusOK).JSON(resp)
}

func (c *fileManagerController) handleCopy(ctx *fiber.Ctx) error {
	return c.handleBulkTransfer(ctx, false)
}

func (c *fileManagerController) handleMove(ctx *fiber.Ctx) error {
	return c.handleBulkTransfer(ctx, true)
}

func (c *fileManagerController) handleBulkTransfer(ctx *fiber.Ctx, move bool) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	var body struct {
		ScopeID         string   `json:"scopeId"`
		SourcePaths     []string `json:"sourcePaths"`
		DestinationPath string   `json:"destinationPath"`
	}
	if err := ctx.BodyParser(&body); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid request body")
	}
	var (
		item *filemanager.BulkResult
		err  error
	)
	if move {
		item, err = c.service.Move(ctx.UserContext(), projectID, body.ScopeID, body.SourcePaths, body.DestinationPath)
	} else {
		item, err = c.service.Copy(ctx.UserContext(), projectID, body.ScopeID, body.SourcePaths, body.DestinationPath)
	}
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(item)
	resp.Status = http.StatusOK
	return ctx.Status(http.StatusOK).JSON(resp)
}

func (c *fileManagerController) handleDelete(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	var body struct {
		ScopeID string   `json:"scopeId"`
		Paths   []string `json:"paths"`
	}
	if err := ctx.BodyParser(&body); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid request body")
	}
	item, err := c.service.Delete(ctx.UserContext(), projectID, body.ScopeID, body.Paths)
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(item)
	resp.Status = http.StatusOK
	return ctx.Status(http.StatusOK).JSON(resp)
}

func (c *fileManagerController) handleCreateArchive(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	var body struct {
		ScopeID  string   `json:"scopeId"`
		Paths    []string `json:"paths"`
		FileName string   `json:"fileName"`
	}
	if err := ctx.BodyParser(&body); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid request body")
	}
	item, err := c.service.CreateArchive(ctx.UserContext(), projectID, body.ScopeID, body.Paths, body.FileName)
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(struct {
		*filemanager.ArchiveJob
		DownloadURL string `json:"downloadUrl"`
	}{
		ArchiveJob:  item,
		DownloadURL: fmt.Sprintf("/api/v1/projects/%s/files/archives/%s", projectID, item.ID),
	})
	resp.Status = http.StatusCreated
	return ctx.Status(http.StatusCreated).JSON(resp)
}

func (c *fileManagerController) handleDownloadArchive(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	archiveID := strings.TrimSpace(ctx.Params("archiveId"))
	job, archivePath, err := c.service.GetArchive(projectID, archiveID)
	if err != nil {
		return c.writeError(ctx, err)
	}
	ctx.Set(fiber.HeaderContentType, "application/zip")
	ctx.Set(fiber.HeaderContentDisposition, fmt.Sprintf("attachment; filename=%q", job.FileName))
	return ctx.SendFile(archivePath, false)
}

func (c *fileManagerController) handleCreateUploadSession(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	var body struct {
		ScopeID       string `json:"scopeId"`
		DirectoryPath string `json:"directoryPath"`
		FileName      string `json:"fileName"`
		Size          int64  `json:"size"`
	}
	if err := ctx.BodyParser(&body); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid request body")
	}
	item, err := c.service.CreateUploadSession(ctx.UserContext(), projectID, body.ScopeID, body.DirectoryPath, body.FileName, body.Size)
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(item)
	resp.Status = http.StatusCreated
	return ctx.Status(http.StatusCreated).JSON(resp)
}

func (c *fileManagerController) handleGetUploadSession(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	uploadID := strings.TrimSpace(ctx.Params("uploadId"))
	item, err := c.service.GetUploadSession(projectID, uploadID)
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(item)
	resp.Status = http.StatusOK
	return ctx.Status(http.StatusOK).JSON(resp)
}

func (c *fileManagerController) handleUploadChunk(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	uploadID := strings.TrimSpace(ctx.Params("uploadId"))
	offsetRaw := strings.TrimSpace(ctx.Get("Upload-Offset"))
	expectedOffset, err := strconv.ParseInt(offsetRaw, 10, 64)
	if err != nil || expectedOffset < 0 {
		return fiber.NewError(http.StatusBadRequest, "invalid Upload-Offset header")
	}
	contentLength := int64(ctx.Context().Request.Header.ContentLength())
	reader := ctx.Context().RequestBodyStream()
	if reader == nil {
		reader = bytes.NewReader(ctx.BodyRaw())
		if contentLength <= 0 {
			contentLength = int64(len(ctx.BodyRaw()))
		}
	}
	item, err := c.service.AppendUploadChunk(projectID, uploadID, expectedOffset, contentLength, reader)
	if err != nil {
		if errors.Is(err, filemanager.ErrOffsetMismatch()) {
			return fiber.NewError(http.StatusConflict, err.Error())
		}
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(item)
	resp.Status = http.StatusOK
	return ctx.Status(http.StatusOK).JSON(resp)
}

func (c *fileManagerController) handleCompleteUpload(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	uploadID := strings.TrimSpace(ctx.Params("uploadId"))
	item, err := c.service.CompleteUpload(ctx.UserContext(), projectID, uploadID)
	if err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewItemResponse(item)
	resp.Status = http.StatusOK
	return ctx.Status(http.StatusOK).JSON(resp)
}

func (c *fileManagerController) handleCancelUpload(ctx *fiber.Ctx) error {
	projectID := strings.TrimSpace(ctx.Params("projectId"))
	uploadID := strings.TrimSpace(ctx.Params("uploadId"))
	if err := c.service.CancelUpload(projectID, uploadID); err != nil {
		return c.writeError(ctx, err)
	}
	resp := h.NewMessageResponse("upload canceled")
	resp.Status = http.StatusOK
	return ctx.Status(http.StatusOK).JSON(resp)
}

func (c *fileManagerController) writeError(ctx *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, filemanager.ErrScopeNotFound()),
		errors.Is(err, filemanager.ErrArchiveNotFound()),
		errors.Is(err, filemanager.ErrUploadNotFound()):
		return fiber.NewError(http.StatusNotFound, err.Error())
	case errors.Is(err, filemanager.ErrProtectedPath()),
		errors.Is(err, filemanager.ErrUnsupportedEntry()),
		errors.Is(err, filemanager.ErrTargetExists()),
		errors.Is(err, filemanager.ErrOffsetMismatch()),
		errors.Is(err, filemanager.ErrInvalidSearchPattern()):
		status := http.StatusBadRequest
		if errors.Is(err, filemanager.ErrTargetExists()) {
			status = http.StatusConflict
		}
		if errors.Is(err, filemanager.ErrOffsetMismatch()) {
			status = http.StatusConflict
		}
		return fiber.NewError(status, err.Error())
	default:
		if err != nil {
			c.logger.Warn("file manager request failed", zap.Error(err))
		}
		return fiber.NewError(http.StatusBadRequest, err.Error())
	}
}

func buildFileContentURL(projectID, scopeID, path, disposition string) string {
	query := make(url.Values)
	query.Set("scopeId", scopeID)
	query.Set("path", path)
	query.Set("disposition", disposition)
	return fmt.Sprintf("/api/v1/projects/%s/files/content?%s", projectID, query.Encode())
}

func mimeTypeForDownload(name string) string {
	contentType := filemanager.MimeTypeForName(name)
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}
