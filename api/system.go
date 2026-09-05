package api

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"code-kanban/service/websession"

	"github.com/danielgtaylor/huma/v2"

	"code-kanban/api/h"
	"code-kanban/utils"
	gitutil "code-kanban/utils/git"
	"code-kanban/utils/system"
)

const systemTag = "system-系统工具"

type systemTerminalManager interface {
	UpdateScrollbackEnabled(bool)
	UpdateTerminalStateSnapshotEnabled(bool)
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
		CustomCommand string `json:"customCommand,omitempty" doc:"自定义命令，使用 ${path} 作为路径占位符"`
	} `json:"body"`
}

type dailyTipSettings struct {
	Enabled bool `json:"enabled" doc:"是否启用每日小技巧"`
}

type pageTitleSettings struct {
	Title string `json:"title" doc:"浏览器标签页使用的应用标题"`
}

type gitSettingsResult struct {
	ReadEngine  string                `json:"readEngine" enum:"auto,builtin,system"`
	WriteEngine string                `json:"writeEngine" enum:"auto,builtin,system"`
	Executable  string                `json:"executable,omitempty"`
	SystemGit   gitutil.SystemGitInfo `json:"systemGit"`
}

func applyGitRuntimeConfig(config utils.GitConfig) {
	normalized := utils.NormalizeGitConfig(config)
	gitutil.ConfigureEngines(gitutil.EngineSettings{
		Read:       gitutil.EnginePreference(normalized.ReadEngine),
		Write:      gitutil.EnginePreference(normalized.WriteEngine),
		Executable: normalized.Executable,
	})
}

func registerSystemRoutes(
	group *huma.Group,
	cfg *utils.AppConfig,
	terminalManager systemTerminalManager,
	webSessionManager *websession.Manager,
) {
	applyGitRuntimeConfig(cfg.Git)
	registerSystemSettingsBackupRoutes(group, cfg, terminalManager, webSessionManager)
	registerSystemDatabaseRoutes(group, cfg, webSessionManager)
	registerSystemHistoryCleanupRoutes(group, webSessionManager)
	registerSystemWorkTimingBackfillRoutes(group, webSessionManager)

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

	huma.Get(group, "/system/git-settings", func(ctx context.Context, input *struct {
		Refresh bool `query:"refresh"`
	}) (*h.ItemResponse[gitSettingsResult], error) {
		normalized := utils.NormalizeGitConfig(cfg.Git)
		resp := h.NewItemResponse(gitSettingsResult{
			ReadEngine:  normalized.ReadEngine,
			WriteEngine: normalized.WriteEngine,
			Executable:  normalized.Executable,
			SystemGit:   gitutil.ProbeSystemGit(ctx, input.Refresh),
		})
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-git-settings-get"
		op.Summary = "获取 Git 引擎设置"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/git-settings/update", func(ctx context.Context, input *struct {
		Body utils.GitConfig `json:"body"`
	}) (*h.ItemResponse[gitSettingsResult], error) {
		if strings.IndexByte(input.Body.Executable, 0) >= 0 {
			return nil, huma.Error400BadRequest("Git executable path is invalid")
		}
		normalized := utils.NormalizeGitConfig(input.Body)
		if err := utils.UpdateConfig(cfg, func(c *utils.AppConfig) {
			c.Git = normalized
		}); err != nil {
			return nil, configStoreAPIError(err, "failed to save Git settings")
		}
		applyGitRuntimeConfig(normalized)
		resp := h.NewItemResponse(gitSettingsResult{
			ReadEngine:  normalized.ReadEngine,
			WriteEngine: normalized.WriteEngine,
			Executable:  normalized.Executable,
			SystemGit:   gitutil.ProbeSystemGit(ctx, true),
		})
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-git-settings-update"
		op.Summary = "更新 Git 引擎设置"
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

	huma.Get(group, "/system/developer-config", func(ctx context.Context, input *struct{}) (*h.ItemResponse[utils.DeveloperConfig], error) {
		resp := h.NewItemResponse(utils.NormalizeDeveloperConfig(cfg.Developer))
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
		if !utils.ValidCodexContextWindow(input.Body.WebSessionCodexContextWindow) {
			return nil, huma.Error400BadRequest("invalid Codex context window preset")
		}
		normalized := utils.MergeDeveloperConfig(cfg.Developer, input.Body)

		// 原子更新：在锁内完成修改+写盘
		if err := utils.UpdateConfig(cfg, func(c *utils.AppConfig) {
			c.Developer = normalized
		}); err != nil {
			return nil, configStoreAPIError(err, "failed to save configuration")
		}

		if terminalManager != nil {
			terminalManager.UpdateScrollbackEnabled(normalized.EnableTerminalScrollback)
			terminalManager.UpdateTerminalStateSnapshotEnabled(normalized.EnableTerminalStateSnapshot)
		}
		if webSessionManager != nil {
			webSessionManager.RefreshDeveloperConfig()
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

	huma.Get(group, "/system/codex-skills", func(
		ctx context.Context,
		_ *struct{},
	) (*h.ItemsResponse[websession.CodexSkillSummary], error) {
		items := []websession.CodexSkillSummary{}
		if webSessionManager != nil {
			var err error
			items, err = webSessionManager.ListCodexSkills()
			if err != nil {
				return nil, huma.Error500InternalServerError("failed to load codex skills", err)
			}
		}
		resp := h.NewItemsResponse(items)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-codex-skills"
		op.Summary = "获取本机可用 Codex skill 列表"
		op.Description = "扫描已安装和仓库内置的 Codex skill 元数据，返回用于前端展示和插入的摘要信息"
		op.Tags = []string{systemTag}
	})

	huma.Get(group, "/system/daily-tip-settings", func(ctx context.Context, input *struct{}) (*h.ItemResponse[dailyTipSettings], error) {
		resp := h.NewItemResponse(dailyTipSettings{
			Enabled: cfg.UI.DailyTipEnabled,
		})
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-daily-tip-settings-get"
		op.Summary = "获取每日小技巧设置"
		op.Description = "返回每日小技巧的服务端全局启用状态"
		op.Tags = []string{systemTag}
	})

	huma.Get(group, "/system/page-title-settings", func(ctx context.Context, input *struct{}) (*h.ItemResponse[pageTitleSettings], error) {
		resp := h.NewItemResponse(pageTitleSettings{
			Title: cfg.UI.PageTitle,
		})
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-page-title-settings-get"
		op.Summary = "获取网页标题设置"
		op.Description = "返回服务端实例级浏览器标签页标题，未登录时也可读取"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/page-title-settings/update", func(ctx context.Context, input *struct {
		Body pageTitleSettings `json:"body"`
	}) (*h.ItemResponse[pageTitleSettings], error) {
		title, err := utils.NormalizePageTitle(input.Body.Title)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if err := utils.UpdateConfig(cfg, func(c *utils.AppConfig) {
			c.UI.PageTitle = title
		}); err != nil {
			return nil, configStoreAPIError(err, "failed to save configuration")
		}

		resp := h.NewItemResponse(pageTitleSettings{Title: title})
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-page-title-settings-update"
		op.Summary = "更新网页标题设置"
		op.Description = "更新实例级浏览器标签页标题，并持久化到配置数据库"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/daily-tip-settings/update", func(ctx context.Context, input *struct {
		Body dailyTipSettings `json:"body"`
	}) (*h.ItemResponse[dailyTipSettings], error) {
		next := dailyTipSettings{
			Enabled: input.Body.Enabled,
		}
		if err := utils.UpdateConfig(cfg, func(c *utils.AppConfig) {
			c.UI.DailyTipEnabled = next.Enabled
		}); err != nil {
			return nil, configStoreAPIError(err, "failed to save configuration")
		}

		resp := h.NewItemResponse(next)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-daily-tip-settings-update"
		op.Summary = "更新每日小技巧设置"
		op.Description = "更新每日小技巧的服务端全局启用状态，并持久化到配置数据库"
		op.Tags = []string{systemTag}
	})

	huma.Get(group, "/system/web-session-quick-input", func(ctx context.Context, input *struct {
		ProjectID string `query:"projectId"`
	}) (*h.ItemResponse[utils.WebSessionQuickInputView], error) {
		store := utils.CurrentConfigDatabase()
		if store == nil {
			return nil, configStoreAPIError(utils.ErrConfigStoreNotInitialized, "failed to load quick input")
		}
		view, err := store.QuickInputView(input.ProjectID)
		if err != nil {
			return nil, configStoreAPIError(err, "failed to load quick input")
		}
		resp := h.NewItemResponse(view)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-web-session-quick-input-get"
		op.Summary = "获取会话快捷输入配置"
		op.Description = "返回常驻项、全局最近输入以及指定项目的最近输入"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/web-session-quick-input/recent", func(ctx context.Context, input *struct {
		Body struct {
			Text      string `json:"text" required:"true"`
			ProjectID string `json:"projectId,omitempty"`
		} `json:"body"`
	}) (*h.ItemResponse[utils.WebSessionQuickInputView], error) {
		store := utils.CurrentConfigDatabase()
		if store == nil {
			return nil, configStoreAPIError(utils.ErrConfigStoreNotInitialized, "failed to record quick input")
		}
		view, err := store.RecordQuickInput(input.Body.Text, input.Body.ProjectID)
		if err != nil {
			return nil, configStoreAPIError(err, "failed to record quick input")
		}
		resp := h.NewItemResponse(view)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-web-session-quick-input-recent-create"
		op.Summary = "记录会话最近输入"
		op.Description = "将一条成功提交的 Prompt 同时写入全局历史和可选的项目历史"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/web-session-quick-input/pinned/update", func(ctx context.Context, input *struct {
		Body struct {
			Items []string `json:"items"`
		} `json:"body"`
	}) (*h.ItemResponse[utils.WebSessionQuickInputView], error) {
		store := utils.CurrentConfigDatabase()
		if store == nil {
			return nil, configStoreAPIError(utils.ErrConfigStoreNotInitialized, "failed to update pinned quick input")
		}
		view, err := store.UpdateQuickInputPinned(input.Body.Items)
		if err != nil {
			return nil, configStoreAPIError(err, "failed to update pinned quick input")
		}
		resp := h.NewItemResponse(view)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-web-session-quick-input-pinned-update"
		op.Summary = "更新会话快捷输入常驻项"
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
			return nil, configStoreAPIError(err, "failed to save configuration")
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
			return nil, configStoreAPIError(err, "failed to save configuration")
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
