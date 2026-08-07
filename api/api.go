package api

import (
	"context"
	"embed"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"path"
	"reflect"
	"strconv"
	"strings"
	"syscall"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"

	"code-kanban/api/h"
	"code-kanban/service/terminal"
	"code-kanban/service/websession"
	"code-kanban/utils"
)

const (
	staticAssetMaxAgeSeconds  = 30 * 24 * 60 * 60
	faviconAssetMaxAgeSeconds = 5 * 60
)

// AppInfo 应用信息
type AppInfo struct {
	Name        string
	Version     string
	Channel     string
	PackageName string // NPM 包名
}

var appInfo *AppInfo

// Init 初始化 Fiber + Huma 的初始化，启动 HTTP 服务
func Init(ctx context.Context, cfg *utils.AppConfig, assets embed.FS, info *AppInfo) error {
	appInfo = info
	theLogger := utils.LoggerFromContext(ctx)

	bodyLimit := int(cfg.AttachmentSizeLimit * 1024)
	if bodyLimit < 1*1024*1024 {
		bodyLimit = 1 * 1024 * 1024
	}
	authAccessConfig := utils.SanitizeAuthAccessConfig(utils.AuthAccessConfigFromAuthConfig(cfg.Auth))
	utils.ApplyAuthAccessConfigToAuthConfig(&cfg.Auth, authAccessConfig)

	app := fiber.New(fiber.Config{
		BodyLimit:               bodyLimit,
		DisableStartupMessage:   true,
		Immutable:               true, // 防止字符串内存被重用覆盖
		EnableIPValidation:      true,
		EnableTrustedProxyCheck: true,
		ProxyHeader:             utils.DefaultAuthProxyHeader,
		TrustedProxies:          authAccessConfig.TrustedProxies,
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CorsAllowOrigins,
		AllowMethods:     "GET,POST,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowCredentials: cfg.CorsAllowOrigins != "*",
	}))
	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
	app.Use(logger.New())
	app.Use(compress.New())
	registerAuthMiddleware(app, cfg)

	humaAPI, v1 := h.NewAPI(app, cfg)
	humaAPI.UseMiddleware(h.HumaTraceMiddleware)
	h.HumaValidatePatch()
	humaTypesRegister()

	terminalManager := terminal.NewManager(terminal.Config{
		Shell:                 cfg.Terminal.Shell,
		IdleTimeout:           cfg.Terminal.IdleDuration(),
		MaxSessionsPerProject: cfg.Terminal.MaxSessionsPerProject,
		Encoding:              cfg.Terminal.Encoding,
		ScrollbackBytes:       cfg.Terminal.ScrollbackBytes,
		ScrollbackEnabled:     cfg.Developer.EnableTerminalScrollback,
		TerminalStateSnapshot: cfg.Developer.EnableTerminalStateSnapshot,
	}, theLogger)
	terminalManager.StartBackground(ctx)

	webSessionManager, err := websession.NewManager(websession.Config{
		DataDir:             utils.GetDataDir(),
		AttachmentSizeLimit: cfg.AttachmentSizeLimit * 1024,
		DefaultCodexModel: func() string {
			return cfg.Developer.WebSessionCodexDefaultModel
		},
		DefaultCodexReasoningEffort: func() websession.ReasoningEffort {
			return websession.ReasoningEffort(cfg.Developer.WebSessionCodexDefaultReasoningEffort)
		},
		DefaultCodexPermissionLevel: func() string {
			return cfg.Developer.WebSessionCodexDefaultPermissionLevel
		},
		DefaultCodexSyncMode: func() websession.SyncMode {
			return websession.SyncMode(cfg.Developer.WebSessionCodexDefaultSyncMode)
		},
		ActiveCallTimeoutConfig: func() utils.WebSessionActiveCallTimeoutConfig {
			return cfg.Developer.WebSessionActiveCallTimeout
		},
	}, theLogger)
	if err != nil {
		theLogger.Error("failed to initialize web session manager", zap.Error(err))
		return err
	}

	registerAuthRoutes(app, cfg)
	registerHealthRoutes(app, humaAPI)
	registerProjectRoutes(v1)
	registerWorktreeRoutes(v1, cfg)
	registerBranchRoutes(v1)
	registerTaskRoutes(v1)
	registerNotePadRoutes(v1)
	registerAISessionRoutes(app, v1)
	registerWebSessionRoutes(app, v1, webSessionManager, theLogger)
	registerFSRoutes(v1)
	registerSystemRoutes(v1, cfg, terminalManager, webSessionManager)
	registerUploadRoutes(app, v1, cfg, theLogger)
	if err := registerFileManagerRoutes(app, cfg, theLogger, ctx); err != nil {
		return err
	}
	registerTerminalRoutes(app, v1, cfg, terminalManager, theLogger)
	registerCaptureDebugRoute(app, terminalManager, theLogger)
	mountStatic(app, cfg, assets, theLogger)
	exposeOpenAPI(app, humaAPI, cfg, theLogger)

	// 设置信号处理，使 Ctrl+C 可以优雅关闭服务器
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		theLogger.Info("Shutting down server...")
		_ = app.Shutdown()
	}()

	return app.Listen(cfg.ServeAt)
}

// registerHealthRoutes 注册健康探测接口，用于服务监控
func registerHealthRoutes(app *fiber.App, api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "health-check",
		Method:      http.MethodGet,
		Path:        "/api/v1/health",
		Summary:     "健康探测",
		Tags:        []string{"health-健康检查"},
	}, func(ctx context.Context, _ *struct{}) (*h.MessageResponse, error) {
		resp := h.NewMessageResponse("ok")
		resp.Status = http.StatusOK
		return resp, nil
	})
}

// mountStatic 将内置静态资源或自定义目录挂载到 Fiber 上
func mountStatic(app *fiber.App, cfg *utils.AppConfig, assets embed.FS, logger *zap.Logger) {
	var fs http.FileSystem

	if cfg.UIOverwrite != "" {
		if _, err := os.Stat(cfg.UIOverwrite); err != nil {
			logger.Warn("自定义前端目录不存在，回退到内置资源", zap.String("path", cfg.UIOverwrite), zap.Error(err))
		} else {
			fs = http.Dir(cfg.UIOverwrite)
		}
	}

	if fs == nil {
		fs = http.FS(assets)
	}

	mountPath := normalizeStaticMountPath(cfg.WebUrl)

	app.Use(mountPath, func(c *fiber.Ctx) error {
		err := c.Next()
		if err != nil {
			return err
		}
		if c.Response().StatusCode() != fiber.StatusOK {
			return nil
		}
		if isStaticIndexRequest(c.Path(), mountPath) {
			setNoCacheHeaders(c)
			return nil
		}
		if isShortCacheStaticRequest(c.Path(), mountPath) {
			setPublicCacheHeader(c, faviconAssetMaxAgeSeconds)
		}
		return nil
	})

	app.Use(mountPath, filesystem.New(filesystem.Config{
		Root:       fs,
		PathPrefix: "static",
		Index:      "index.html",
		MaxAge:     staticAssetMaxAgeSeconds,
		Browse:     false,
	}))
}

func normalizeStaticMountPath(mountPath string) string {
	mountPath = strings.TrimSpace(mountPath)
	if mountPath == "" {
		return "/"
	}
	if !strings.HasPrefix(mountPath, "/") {
		mountPath = "/" + mountPath
	}
	cleaned := path.Clean(mountPath)
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func isStaticIndexRequest(requestPath, mountPath string) bool {
	cleanedPath := normalizeStaticMountPath(requestPath)
	cleanedMountPath := normalizeStaticMountPath(mountPath)
	if cleanedPath == cleanedMountPath {
		return true
	}
	return cleanedPath == path.Join(cleanedMountPath, "index.html")
}

func isShortCacheStaticRequest(requestPath, mountPath string) bool {
	cleanedPath := normalizeStaticMountPath(requestPath)
	cleanedMountPath := normalizeStaticMountPath(mountPath)
	return cleanedPath == path.Join(cleanedMountPath, "favicon.ico") ||
		cleanedPath == path.Join(cleanedMountPath, "favicon.svg")
}

func setNoCacheHeaders(c *fiber.Ctx) {
	c.Set(fiber.HeaderCacheControl, "no-store, no-cache, must-revalidate")
	c.Set(fiber.HeaderPragma, "no-cache")
	c.Set(fiber.HeaderExpires, "0")
}

func setPublicCacheHeader(c *fiber.Ctx, maxAgeSeconds int) {
	c.Set(fiber.HeaderCacheControl, "public, max-age="+strconv.Itoa(maxAgeSeconds))
}

// exposeOpenAPI 在需要时暴露 openapi 文档，提供调试访问
func exposeOpenAPI(app *fiber.App, api huma.API, cfg *utils.AppConfig, logger *zap.Logger) {
	if !cfg.OpenAPIEnabled {
		return
	}

	app.Get("/openapi.json", func(c *fiber.Ctx) error {
		spec := api.OpenAPI()
		// Huma's OpenAPI output uses JSON Schema. We add a `visible` hint to each
		// schema property so template/form generators can distinguish between:
		// - required: object-level `required` array
		// - readOnly: property-level `readOnly`
		// - visible: property-level `visible` (custom hint, defaults to true)
		//
		// `$schema` is a JSON Schema meta field and should not be shown in UIs.
		augmented, err := addSchemaVisibilityToOpenAPI(spec)
		if err != nil {
			logger.Warn("增强 OpenAPI 文档失败，回退原始输出", zap.Error(err))
			augmented = spec
		}

		body, err := json.MarshalIndent(augmented, "", "  ")
		if err != nil {
			logger.Warn("生成 OpenAPI 文档失败", zap.Error(err))
			return fiber.NewError(http.StatusInternalServerError, "OpenAPI 文档生成失败")
		}

		c.Type("json", "utf-8")
		return c.Send(body)
	})
}

func humaTypesRegister() {
	// 注册 any 接口类型的 Schema，使其在文档中表现为任意对象
	huma.RegisterTypeSchema(reflect.TypeOf((*any)(nil)).Elem(), func(huma.Registry) *huma.Schema {
		return &huma.Schema{
			Type:                 "object",
			AdditionalProperties: map[string]*huma.Schema{},
		}
	})

	// 处理 []any
	huma.RegisterTypeSchema(reflect.TypeOf([]any{}), func(huma.Registry) *huma.Schema {
		return &huma.Schema{
			Type: "array",
			Items: &huma.Schema{
				Type:                 "object",
				AdditionalProperties: map[string]*huma.Schema{},
			},
		}
	})
}

func addSchemaVisibilityToOpenAPI(spec any) (any, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}

	components, ok := doc["components"].(map[string]any)
	if !ok {
		return doc, nil
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		return doc, nil
	}

	for _, schema := range schemas {
		addSchemaVisibilityHint(schema)
	}

	return doc, nil
}

func addSchemaVisibilityHint(schema any) {
	obj, ok := schema.(map[string]any)
	if !ok {
		return
	}

	if properties, ok := obj["properties"].(map[string]any); ok {
		for name, propSchema := range properties {
			if propObj, ok := propSchema.(map[string]any); ok {
				if _, exists := propObj["visible"]; !exists {
					propObj["visible"] = name != "$schema"
				}
			}
			addSchemaVisibilityHint(propSchema)
		}
	}

	if items, ok := obj["items"]; ok {
		addSchemaVisibilityHint(items)
	}

	for _, key := range []string{"oneOf", "anyOf", "allOf"} {
		if variants, ok := obj[key].([]any); ok {
			for _, variant := range variants {
				addSchemaVisibilityHint(variant)
			}
		}
	}

	if additional, ok := obj["additionalProperties"]; ok {
		addSchemaVisibilityHint(additional)
	}

	for _, key := range []string{"not", "if", "then", "else", "contains", "propertyNames"} {
		if nested, ok := obj[key]; ok {
			addSchemaVisibilityHint(nested)
		}
	}
}
