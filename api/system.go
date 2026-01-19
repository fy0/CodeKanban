package api

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"code-kanban/api/h"
	"code-kanban/utils"
	"code-kanban/utils/system"
)

const systemTag = "system-系统工具"

type systemTerminalManager interface {
	UpdateAIAssistantStatusConfig(utils.AIAssistantStatusConfig)
	UpdateScrollbackEnabled(bool)
	UpdateRenameTitleEachCommand(bool)
	UpdateAutoCreateTaskOnStartWork(bool)
	UpdateShellConfig(utils.TerminalShellConfig)
}

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

func registerSystemRoutes(group *huma.Group, cfg *utils.AppConfig, terminalManager systemTerminalManager) {
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
		// 原子更新：在锁内完成修改+写盘
		if err := utils.UpdateConfig(cfg, func(c *utils.AppConfig) {
			c.Terminal.AIAssistantStatus = input.Body
		}); err != nil {
			return nil, huma.Error500InternalServerError("failed to save configuration")
		}

		// 热重载：更新所有现有终端的配置
		if terminalManager != nil {
			terminalManager.UpdateAIAssistantStatusConfig(input.Body)
		}

		resp := h.NewMessageResponse("AI assistant status config updated and applied to all active terminals.")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-ai-assistant-status-update"
		op.Summary = "更新 AI 助手状态监测配置"
		op.Description = "更新 AI 助手状态监测的启用/禁用配置，立即对所有终端生效"
		op.Tags = []string{systemTag}
	})

	huma.Get(group, "/system/developer-config", func(ctx context.Context, input *struct{}) (*h.ItemResponse[utils.DeveloperConfig], error) {
		resp := h.NewItemResponse(cfg.Developer)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-developer-config-get"
		op.Summary = "获取开发者调试配置"
		op.Description = "返回开发者相关的实时调试配置，例如是否启用终端 scrollback"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/developer-config/update", func(ctx context.Context, input *struct {
		Body utils.DeveloperConfig `json:"body"`
	}) (*h.MessageResponse, error) {
		// 原子更新：在锁内完成修改+写盘
		if err := utils.UpdateConfig(cfg, func(c *utils.AppConfig) {
			c.Developer = input.Body
		}); err != nil {
			return nil, huma.Error500InternalServerError("failed to save configuration")
		}

		if terminalManager != nil {
			terminalManager.UpdateScrollbackEnabled(input.Body.EnableTerminalScrollback)
			terminalManager.UpdateRenameTitleEachCommand(input.Body.RenameSessionTitleEachCommand)
			terminalManager.UpdateAutoCreateTaskOnStartWork(input.Body.AutoCreateTaskOnStartWork)
		}

		resp := h.NewMessageResponse("Developer config updated.")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-developer-config-update"
		op.Summary = "更新开发者调试配置"
		op.Description = "更新开发者相关设置，例如终端 scrollback 是否启用，并实时应用到活动终端"
		op.Tags = []string{systemTag}
	})

	// Terminal Shell Settings
	huma.Get(group, "/system/terminal-shells", func(ctx context.Context, input *struct{}) (*h.ItemResponse[utils.AvailableShellsResponse], error) {
		shells := utils.GetAvailableShells(cfg.Terminal.Shell)
		resp := h.NewItemResponse(shells)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-terminal-shells-get"
		op.Summary = "获取可用终端Shell列表"
		op.Description = "返回当前平台可用的终端Shell选项，包括检测状态"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/terminal-shells/update", func(ctx context.Context, input *struct {
		Body struct {
			Shell string `json:"shell" doc:"Shell命令，空值表示使用自动选择"`
		} `json:"body"`
	}) (*h.MessageResponse, error) {
		// 验证 Shell 命令有效性
		if err := utils.ValidateShellCommand(input.Body.Shell); err != nil {
			return nil, huma.Error400BadRequest("Invalid shell command: " + err.Error())
		}

		// 获取当前平台以便更新对应配置
		platform := utils.GetAvailableShells(cfg.Terminal.Shell).Platform

		// 原子更新：在锁内完成修改+写盘
		if err := utils.UpdateConfig(cfg, func(c *utils.AppConfig) {
			switch platform {
			case "windows":
				c.Terminal.Shell.Windows = input.Body.Shell
			case "darwin":
				c.Terminal.Shell.Darwin = input.Body.Shell
			default:
				c.Terminal.Shell.Linux = input.Body.Shell
			}
		}); err != nil {
			return nil, huma.Error500InternalServerError("failed to save configuration")
		}

		// 热重载：更新终端管理器的 Shell 配置，新会话生效
		if terminalManager != nil {
			terminalManager.UpdateShellConfig(cfg.Terminal.Shell)
		}

		resp := h.NewMessageResponse("Terminal shell updated. New terminals will use the selected shell.")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-terminal-shells-update"
		op.Summary = "更新终端Shell设置"
		op.Description = "更新当前平台的默认终端Shell，新建终端时生效"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/terminal-shells/validate", func(ctx context.Context, input *struct {
		Body struct {
			Shell string `json:"shell" doc:"要验证的Shell命令"`
		} `json:"body"`
	}) (*struct {
		Body struct {
			Valid   bool   `json:"valid" doc:"命令是否有效"`
			Message string `json:"message,omitempty" doc:"错误信息"`
		} `json:"body"`
	}, error) {
		resp := &struct {
			Body struct {
				Valid   bool   `json:"valid" doc:"命令是否有效"`
				Message string `json:"message,omitempty" doc:"错误信息"`
			} `json:"body"`
		}{}

		if err := utils.ValidateShellCommand(input.Body.Shell); err != nil {
			resp.Body.Valid = false
			resp.Body.Message = err.Error()
		} else {
			resp.Body.Valid = true
		}

		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-terminal-shells-validate"
		op.Summary = "验证Shell命令"
		op.Description = "检查指定的Shell命令是否有效可用"
		op.Tags = []string{systemTag}
	})

	huma.Get(group, "/system/worktree-settings", func(ctx context.Context, input *struct{}) (*h.ItemResponse[utils.WorktreeConfig], error) {
		resp := h.NewItemResponse(cfg.Worktree)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-worktree-settings-get"
		op.Summary = "获取 Worktree 全局设置"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/worktree-settings/update", func(ctx context.Context, input *struct {
		Body utils.WorktreeConfig `json:"body"`
	}) (*h.ItemResponse[utils.WorktreeConfig], error) {
		globalBaseDir := strings.TrimSpace(input.Body.GlobalBaseDir)
		pattern := strings.TrimSpace(input.Body.GlobalDirNamePattern)
		if globalBaseDir != "" && !filepath.IsAbs(globalBaseDir) {
			return nil, huma.Error400BadRequest("globalBaseDir must be an absolute path")
		}
		if pattern == "" {
			return nil, huma.Error400BadRequest("globalDirNamePattern is required")
		}

		// 安全检查：全局基础目录不能是敏感系统目录
		if globalBaseDir != "" && utils.IsSensitiveSystemDir(globalBaseDir) {
			return nil, huma.Error400BadRequest("globalBaseDir cannot be a system directory")
		}

		// 原子更新：在锁内完成修改+写盘
		if err := utils.UpdateConfig(cfg, func(c *utils.AppConfig) {
			c.Worktree.GlobalBaseDir = globalBaseDir
			c.Worktree.GlobalDirNamePattern = pattern
		}); err != nil {
			return nil, huma.Error500InternalServerError("failed to save configuration")
		}

		resp := h.NewItemResponse(cfg.Worktree)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-worktree-settings-update"
		op.Summary = "更新 Worktree 全局设置"
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
