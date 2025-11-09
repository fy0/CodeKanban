package api

import (
	"context"
	"embed"
	"encoding/json"
	"net/http"
	"os"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"go.uber.org/zap"

	"go-template/api/h"
	"go-template/api/ptytest"
	"go-template/api/terminal"
	"go-template/utils"
)

// Init 初始化 Fiber + Huma 的初始化，启动 HTTP 服务
func Init(ctx context.Context, cfg *utils.AppConfig, assets embed.FS) error {
	theLogger := utils.LoggerFromContext(ctx)

	bodyLimit := int(cfg.AttachmentSizeLimit * 1024)
	if bodyLimit < 1*1024*1024 {
		bodyLimit = 1 * 1024 * 1024
	}

	app := fiber.New(fiber.Config{
		BodyLimit:             bodyLimit,
		DisableStartupMessage: true,
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CorsAllowOrigins,
		AllowMethods:     "GET,POST,PUT,DELETE,PATCH",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowCredentials: cfg.CorsAllowOrigins != "*",
	}))
	app.Use(recover.New(recover.Config{EnableStackTrace: true}))
	app.Use(logger.New())
	app.Use(compress.New())

	humaAPI, v1 := h.NewAPI(app, cfg)
	humaTypesRegister()

	terminalManager := terminal.NewManager(terminal.Config{
		Shell:                 cfg.Terminal.Shell,
		IdleTimeout:           cfg.Terminal.IdleDuration(),
		MaxSessionsPerProject: cfg.Terminal.MaxSessionsPerProject,
		Encoding:              cfg.Terminal.Encoding,
	}, theLogger)
	terminalManager.StartBackground(ctx)
	ptyTestManager := ptytest.NewManager(ptytest.Config{
		Shell: cfg.Terminal.Shell,
	}, theLogger)

	registerHealthRoutes(app, humaAPI)
	registerProjectRoutes(v1)
	registerWorktreeRoutes(v1)
	registerTaskRoutes(v1)
	registerSystemRoutes(v1)
	registerTerminalRoutes(app, v1, cfg, terminalManager, theLogger)
	registerPtyTestRoutes(app, v1, cfg, ptyTestManager, theLogger)
	registerMetricsRoute(app)
	mountStatic(app, cfg, assets, theLogger)
	exposeOpenAPI(app, humaAPI, cfg, theLogger)

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

	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
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

	mountPath := cfg.WebUrl
	if mountPath == "" {
		mountPath = "/"
	}

	app.Use(mountPath, filesystem.New(filesystem.Config{
		Root:       fs,
		PathPrefix: "",
		MaxAge:     300,
	}))
}

// exposeOpenAPI 在需要时暴露 openapi 文档，提供调试访问
func exposeOpenAPI(app *fiber.App, api huma.API, cfg *utils.AppConfig, logger *zap.Logger) {
	if !cfg.OpenAPIEnabled {
		return
	}

	app.Get("/openapi.json", func(c *fiber.Ctx) error {
		spec := api.OpenAPI()
		body, err := json.MarshalIndent(spec, "", "  ")
		if err != nil {
			logger.Warn("生成 OpenAPI 文档失败", zap.Error(err))
			return fiber.NewError(http.StatusInternalServerError, "OpenAPI 文档生成失败")
		}

		c.Type("json", "utf-8")
		return c.Send(body)
	})
}

func registerMetricsRoute(app *fiber.App) {
	handler := fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler())
	app.Get("/metrics", func(c *fiber.Ctx) error {
		handler(c.Context())
		return nil
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
