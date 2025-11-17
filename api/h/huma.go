package h

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	"github.com/gofiber/fiber/v2"

	"code-kanban/utils"
)

// NewAPI 会基于 Fiber 实例构建 Huma API，并按约定创建 /api/v1 分组。
func NewAPI(app *fiber.App, cfg *utils.AppConfig) (huma.API, *huma.Group) {
	title := cfg.APITitle
	if title == "" {
		title = "Go Template API"
	}

	version := cfg.APIVersion
	if version == "" {
		version = "1.0.0"
	}

	apiConfig := huma.DefaultConfig(title, version)

	if !cfg.OpenAPIEnabled {
		apiConfig.OpenAPIPath = ""
		apiConfig.DocsPath = ""
	}

	if cfg.OpenAPIEnabled && cfg.DocsPath != "" {
		apiConfig.DocsPath = cfg.DocsPath
	}

	api := humafiber.New(app, apiConfig)
	group := huma.NewGroup(api, "/api/v1")

	return api, group
}
