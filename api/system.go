package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"code-kanban/api/h"
	"code-kanban/utils"
	"code-kanban/utils/system"
)

const systemTag = "system-系统工具"

type versionResponse struct {
	Body struct {
		Name    string `json:"name" doc:"应用名称"`
		Version string `json:"version" doc:"版本号"`
		Channel string `json:"channel" doc:"更新频道"`
	} `json:"body"`
}

type checkUpdateResponse struct {
	Body struct {
		CurrentVersion string `json:"currentVersion" doc:"当前版本"`
		LatestVersion  string `json:"latestVersion" doc:"最新版本"`
		HasUpdate      bool   `json:"hasUpdate" doc:"是否有更新"`
		UpdateURL      string `json:"updateUrl,omitempty" doc:"更新地址"`
		Message        string `json:"message,omitempty" doc:"提示信息"`
	} `json:"body"`
}

type openPathInput struct {
	Body struct {
		Path string `json:"path" doc:"目标路径" required:"true"`
	} `json:"body"`
}

type openEditorInput struct {
	Body struct {
		Path          string `json:"path" doc:"目标路径" required:"true"`
		Editor        string `json:"editor" doc:"目标编辑器(vscode/cursor/trae/zed/custom)" required:"true"`
		CustomCommand string `json:"customCommand,omitempty" doc:"自定义命令，使用 {{path}} 作为路径占位符"`
	} `json:"body"`
}

func registerSystemRoutes(group *huma.Group, cfg *utils.AppConfig) {
	huma.Get(group, "/system/version", func(ctx context.Context, input *struct{}) (*versionResponse, error) {
		resp := &versionResponse{}
		resp.Body.Name = appInfo.Name
		resp.Body.Version = appInfo.Version
		resp.Body.Channel = appInfo.Channel
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-version"
		op.Summary = "获取应用版本信息"
		op.Tags = []string{systemTag}
	})

	huma.Get(group, "/system/check-update", func(ctx context.Context, input *struct{}) (*checkUpdateResponse, error) {
		resp := &checkUpdateResponse{}
		resp.Body.CurrentVersion = appInfo.Version

		// 创建版本检查器
		checker := utils.NewVersionChecker(appInfo.Version, appInfo.PackageName)

		// 获取最新版本（同步调用）
		latestVersion, hasUpdate, err := checker.CheckUpdate()
		if err != nil {
			// 网络错误或其他错误，返回当前信息但不报错
			resp.Body.LatestVersion = appInfo.Version
			resp.Body.HasUpdate = false
			resp.Body.Message = "无法检查更新: " + err.Error()
			return resp, nil
		}

		resp.Body.LatestVersion = latestVersion
		resp.Body.HasUpdate = hasUpdate

		if hasUpdate {
			resp.Body.UpdateURL = "https://www.npmjs.com/package/" + appInfo.PackageName
			resp.Body.Message = "发现新版本！请使用 npm install -g " + appInfo.PackageName + "@latest 更新"
		} else {
			resp.Body.Message = "当前已是最新版本"
		}

		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-check-update"
		op.Summary = "检查版本更新"
		op.Description = "检查 npm 上是否有新版本可用"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/open-explorer", func(ctx context.Context, input *openPathInput) (*h.MessageResponse, error) {
		if err := system.OpenExplorer(input.Body.Path); err != nil {
			return nil, mapSystemError(err)
		}

		resp := h.NewMessageResponse("explorer opened")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-open-explorer"
		op.Summary = "打开文件管理器"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/open-terminal", func(ctx context.Context, input *openPathInput) (*h.MessageResponse, error) {
		if err := system.OpenTerminal(input.Body.Path); err != nil {
			return nil, mapSystemError(err)
		}

		resp := h.NewMessageResponse("terminal opened")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-open-terminal"
		op.Summary = "打开终端"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/open-editor", func(ctx context.Context, input *openEditorInput) (*h.MessageResponse, error) {
		if err := system.OpenEditor(input.Body.Path, input.Body.Editor, input.Body.CustomCommand); err != nil {
			return nil, mapSystemError(err)
		}

		resp := h.NewMessageResponse("editor opened")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-open-editor"
		op.Summary = "使用指定编辑器打开目录"
		op.Tags = []string{systemTag}
	})

	// AI 助手状态监测配置
	huma.Get(group, "/system/ai-assistant-status", func(ctx context.Context, input *struct{}) (*h.ItemResponse[utils.AIAssistantStatusConfig], error) {
		resp := h.NewItemResponse(cfg.Terminal.AIAssistantStatus)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-ai-assistant-status-get"
		op.Summary = "获取 AI 助手状态监测配置"
		op.Description = "获取当前 AI 助手状态监测的启用/禁用配置"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/ai-assistant-status/update", func(ctx context.Context, input *struct {
		Body utils.AIAssistantStatusConfig `json:"body"`
	}) (*h.MessageResponse, error) {
		// 更新内存中的配置
		cfg.Terminal.AIAssistantStatus = input.Body

		// 写回配置文件
		utils.WriteConfig(cfg)

		resp := h.NewMessageResponse("AI assistant status config updated. Restart required for existing terminals.")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-ai-assistant-status-update"
		op.Summary = "更新 AI 助手状态监测配置"
		op.Description = "更新 AI 助手状态监测的启用/禁用配置，需要重启才能对现有终端生效"
		op.Tags = []string{systemTag}
	})
}

func mapSystemError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, system.ErrUnsupportedOS):
		return huma.Error501NotImplemented(err.Error())
	case errors.Is(err, system.ErrNoFileManager),
		errors.Is(err, system.ErrNoTerminal):
		return huma.Error503ServiceUnavailable(err.Error())
	case errors.Is(err, system.ErrEditorCommandMissing):
		return huma.Error503ServiceUnavailable(err.Error())
	case errors.Is(err, system.ErrUnsupportedEditor),
		errors.Is(err, system.ErrCustomEditorCommand):
		return huma.Error400BadRequest(err.Error())
	default:
		return huma.Error500InternalServerError(err.Error())
	}
}
