package api

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"code-kanban/api/h"
	"code-kanban/utils"

	"github.com/Masterminds/semver/v3"
	"github.com/danielgtaylor/huma/v2"
)

type settingsBackupImportApplier interface {
	UpdateAIAssistantStatusConfig(utils.AIAssistantStatusConfig)
	UpdateScrollbackEnabled(bool)
	UpdateTerminalStateSnapshotEnabled(bool)
	UpdateRenameTitleEachCommand(bool)
	UpdateShellConfig(utils.TerminalShellConfig)
}

type settingsBackupImportWebSessionManager interface {
	RefreshDeveloperConfig()
}

type settingsBackupExportResponse struct {
	Body struct {
		Item utils.SettingsBackupFile `json:"item"`
	} `json:"body"`
}

type settingsBackupPreviewResponse struct {
	Body struct {
		Item utils.SettingsBackupPreviewResult `json:"item"`
	} `json:"body"`
}

func registerSystemSettingsBackupRoutes(
	group *huma.Group,
	cfg *utils.AppConfig,
	terminalManager settingsBackupImportApplier,
	webSessionManager settingsBackupImportWebSessionManager,
) {
	huma.Get(group, "/system/settings-backup/export", func(
		ctx context.Context,
		_ *struct{},
	) (*h.ItemResponse[utils.SettingsBackupFile], error) {
		createdAt := time.Now().UTC()
		backup := utils.SettingsBackupFile{
			BackupSchemaVersion: utils.SettingsBackupSchemaVersion,
			BackupKind:          utils.SettingsBackupKind,
			CreatedAt:           &createdAt,
			SourceApp:           currentSettingsBackupSourceApp(),
			Payload: utils.SettingsBackupPayload{
				Server: loPtr(utils.BuildSettingsBackupServerPayload(cfg)),
			},
		}

		resp := h.NewItemResponse(backup)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-settings-backup-export"
		op.Summary = "导出服务端设置备份"
		op.Description = "导出服务端可迁移设置，供前端组装为完整 JSON 备份文件"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/settings-backup/preview", func(
		ctx context.Context,
		input *struct {
			Body utils.SettingsBackupFile `json:"body"`
		},
	) (*h.ItemResponse[utils.SettingsBackupPreviewResult], error) {
		result, _, err := previewSettingsBackup(cfg, input.Body)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		resp := h.NewItemResponse(result)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-settings-backup-preview"
		op.Summary = "预览设置备份导入"
		op.Description = "校验备份 JSON，并返回阻断错误与风险警告"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/settings-backup/import", func(
		ctx context.Context,
		input *struct {
			Body utils.SettingsBackupFile `json:"body"`
		},
	) (*h.ItemResponse[utils.SettingsBackupPreviewResult], error) {
		result, migrated, err := previewSettingsBackup(cfg, input.Body)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		if !result.CanImport {
			return nil, huma.Error400BadRequest("backup import is blocked by validation errors")
		}
		if err := applySettingsBackup(cfg, input.Body, terminalManager, webSessionManager); err != nil {
			return nil, huma.Error500InternalServerError("failed to import settings backup", err)
		}
		result.Migrated = migrated
		resp := h.NewItemResponse(result)
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-settings-backup-import"
		op.Summary = "导入服务端设置备份"
		op.Description = "应用已通过校验的服务端设置备份，并即时刷新相关运行时配置"
		op.Tags = []string{systemTag}
	})
}

func previewSettingsBackup(
	cfg *utils.AppConfig,
	input utils.SettingsBackupFile,
) (utils.SettingsBackupPreviewResult, bool, error) {
	if err := utils.ValidateSettingsBackupFileShape(input); err != nil {
		return utils.SettingsBackupPreviewResult{}, false, err
	}

	migratedBackup, migrated, err := utils.MigrateSettingsBackupFile(input)
	if err != nil {
		return utils.SettingsBackupPreviewResult{}, false, err
	}

	result := utils.SettingsBackupPreviewResult{
		BackupSchemaVersion: migratedBackup.BackupSchemaVersion,
		BackupKind:          migratedBackup.BackupKind,
		SourceApp:           migratedBackup.SourceApp,
		CurrentApp:          currentSettingsBackupSourceApp(),
		CanImport:           true,
		Migrated:            migrated,
	}

	addVersionWarnings(&result)
	addPayloadSections(&result, migratedBackup)

	if migratedBackup.Payload.Server.HasContent() {
		validateServerPayload(&result, migratedBackup.Payload.Server)
	}

	if len(result.Errors) > 0 {
		result.CanImport = false
	}

	return result, migrated, nil
}

func addVersionWarnings(result *utils.SettingsBackupPreviewResult) {
	sourceVersion := strings.TrimSpace(result.SourceApp.Version)
	currentVersion := strings.TrimSpace(result.CurrentApp.Version)
	if sourceVersion == "" || currentVersion == "" || sourceVersion == currentVersion {
		return
	}

	result.Warnings = append(result.Warnings, utils.SettingsBackupPreviewIssue{
		Code:    "source_app_version_differs",
		Level:   "warning",
		Message: fmt.Sprintf("Backup was created by app version %s, current app version is %s.", sourceVersion, currentVersion),
	})

	sourceSemver, sourceErr := semver.NewVersion(sourceVersion)
	currentSemver, currentErr := semver.NewVersion(currentVersion)
	if sourceErr == nil && currentErr == nil && versionsBreakCompatibility(sourceSemver, currentSemver) {
		result.Warnings = append(result.Warnings, utils.SettingsBackupPreviewIssue{
			Code:    "source_app_breaking_version_differs",
			Level:   "warning",
			Message: fmt.Sprintf("Backup app version %s may be incompatible with current version %s.", sourceVersion, currentVersion),
		})
	}

	sourceChannel := strings.TrimSpace(result.SourceApp.Channel)
	currentChannel := strings.TrimSpace(result.CurrentApp.Channel)
	if sourceChannel != "" && currentChannel != "" && sourceChannel != currentChannel {
		result.Warnings = append(result.Warnings, utils.SettingsBackupPreviewIssue{
			Code:    "source_app_channel_differs",
			Level:   "warning",
			Message: fmt.Sprintf("Backup was created from channel %s, current app channel is %s.", sourceChannel, currentChannel),
		})
	}
}

func addPayloadSections(result *utils.SettingsBackupPreviewResult, backup utils.SettingsBackupFile) {
	if backup.Payload.Server.HasContent() {
		server := backup.Payload.Server
		if server.AIAssistantStatus != nil {
			result.Sections = append(result.Sections, utils.SettingsBackupPreviewSection{
				Key: "server.aiAssistantStatus", Label: "AI assistant status", Action: "replace", Target: "server",
				ChangedKeys: []string{"claudeCode", "codex", "qwenCode", "gemini", "cursor", "copilot"},
			})
		}
		if server.Developer != nil {
			result.Sections = append(result.Sections, utils.SettingsBackupPreviewSection{
				Key: "server.developer", Label: "Developer config", Action: "replace", Target: "server",
				ChangedKeys: []string{"enableTerminalScrollback", "renameSessionTitleEachCommand", "enableTerminalStateSnapshot", "webSessionCodexDefaultSyncMode", "webSessionActiveCallTimeout"},
			})
		}
		if server.PageTitle != nil {
			result.Sections = append(result.Sections, utils.SettingsBackupPreviewSection{
				Key: "server.pageTitle", Label: "Page title", Action: "replace", Target: "server", ChangedKeys: []string{"pageTitle"},
			})
		}
		if server.DailyTip != nil {
			result.Sections = append(result.Sections, utils.SettingsBackupPreviewSection{
				Key: "server.dailyTip", Label: "Daily tip", Action: "replace", Target: "server", ChangedKeys: []string{"enabled"},
			})
		}
		if server.WebSessionQuickInput != nil {
			if server.WebSessionQuickInput.Pinned != nil {
				result.Sections = append(result.Sections, utils.SettingsBackupPreviewSection{
					Key: "server.webSessionQuickInput.pinned", Label: "Web session quick input pinned", Action: "replace", Target: "server", ChangedKeys: []string{"pinned"},
				})
			}
			if server.WebSessionQuickInput.Recent != nil {
				result.Sections = append(result.Sections, utils.SettingsBackupPreviewSection{
					Key: "server.webSessionQuickInput.recent", Label: "Web session quick input recent", Action: "replace", Target: "server", ChangedKeys: []string{"recent"},
				})
			}
		}
		if server.Worktree != nil {
			result.Sections = append(result.Sections, utils.SettingsBackupPreviewSection{
				Key: "server.worktree", Label: "Worktree settings", Action: "replace", Target: "server", ChangedKeys: []string{"globalBaseDir", "globalDirNamePattern"},
			})
		}
		if server.TerminalShell != nil {
			result.Sections = append(result.Sections, utils.SettingsBackupPreviewSection{
				Key: "server.terminalShell", Label: "Terminal shell", Action: "replace", Target: "server", ChangedKeys: []string{"platform", "shell"},
			})
		}
		if server.AuthAccess != nil {
			result.Sections = append(result.Sections, utils.SettingsBackupPreviewSection{
				Key: "server.authAccess", Label: "Security access rules", Action: "replace", Target: "server", ChangedKeys: []string{"accessRules", "proxyHeader", "trustedProxies"},
			})
		}
	}
	if backup.Payload.Client.HasContent() {
		if backup.Payload.Client.Locale != nil {
			result.Sections = append(result.Sections, utils.SettingsBackupPreviewSection{
				Key: "client.locale", Label: "Locale", Action: "replace", Target: "client", ChangedKeys: []string{"app-locale"},
			})
		}
		if len(strings.TrimSpace(string(backup.Payload.Client.Settings))) > 0 {
			result.Sections = append(result.Sections, utils.SettingsBackupPreviewSection{
				Key: "client.settings", Label: "Local settings", Action: "replace", Target: "client", ChangedKeys: []string{"general_settings"},
			})
		}
	}
}

func validateServerPayload(result *utils.SettingsBackupPreviewResult, payload *utils.SettingsBackupServerPayload) {
	if payload.Developer != nil {
		normalized := utils.NormalizeDeveloperConfig(*payload.Developer)
		payload.Developer = &normalized
	}
	payload.WebSessionQuickInput = utils.NormalizeSettingsBackupQuickInputSection(payload.WebSessionQuickInput)
	if payload.AuthAccess != nil {
		sanitized := utils.SanitizeAuthAccessConfig(*payload.AuthAccess)
		payload.AuthAccess = &sanitized
	}
	if payload.PageTitle != nil {
		pageTitle, err := utils.NormalizePageTitle(*payload.PageTitle)
		if err != nil {
			result.Errors = append(result.Errors, utils.SettingsBackupPreviewIssue{
				Code: "invalid_page_title", Level: "error", Message: err.Error(),
			})
		} else {
			payload.PageTitle = &pageTitle
		}
	}

	if payload.TerminalShell != nil &&
		payload.TerminalShell.Platform != "" &&
		payload.TerminalShell.Platform != runtimePlatform() {
		result.Warnings = append(result.Warnings, utils.SettingsBackupPreviewIssue{
			Code:    "shell_platform_differs",
			Level:   "warning",
			Message: fmt.Sprintf("Backup shell was exported from platform %s, current platform is %s. Import will apply only the current platform shell field.", payload.TerminalShell.Platform, runtimePlatform()),
		})
	}

	if payload.TerminalShell != nil && strings.TrimSpace(payload.TerminalShell.Shell) != "" {
		if err := utils.ValidateShellCommand(payload.TerminalShell.Shell); err != nil {
			result.Errors = append(result.Errors, utils.SettingsBackupPreviewIssue{
				Code:    "invalid_terminal_shell",
				Level:   "error",
				Message: "Terminal shell is invalid: " + err.Error(),
			})
		}
	}

	if payload.AuthAccess != nil {
		if _, err := utils.NormalizeAuthAccessConfig(*payload.AuthAccess); err != nil {
			result.Errors = append(result.Errors, utils.SettingsBackupPreviewIssue{
				Code:    "invalid_auth_access",
				Level:   "error",
				Message: err.Error(),
			})
		}
	}

	if payload.Worktree == nil {
		return
	}

	globalBaseDir := strings.TrimSpace(payload.Worktree.GlobalBaseDir)
	if globalBaseDir != "" {
		if !isAbsPath(globalBaseDir) {
			result.Errors = append(result.Errors, utils.SettingsBackupPreviewIssue{
				Code:    "invalid_worktree_base_dir",
				Level:   "error",
				Message: "worktree globalBaseDir must be an absolute path",
			})
		} else if utils.IsSensitiveSystemDir(globalBaseDir) {
			result.Errors = append(result.Errors, utils.SettingsBackupPreviewIssue{
				Code:    "sensitive_worktree_base_dir",
				Level:   "error",
				Message: "worktree globalBaseDir cannot be a system directory",
			})
		}
	}
	if strings.TrimSpace(payload.Worktree.GlobalDirNamePattern) == "" {
		result.Errors = append(result.Errors, utils.SettingsBackupPreviewIssue{
			Code:    "missing_worktree_dir_pattern",
			Level:   "error",
			Message: "worktree globalDirNamePattern is required",
		})
	}
}

func applySettingsBackup(
	cfg *utils.AppConfig,
	backup utils.SettingsBackupFile,
	terminalManager settingsBackupImportApplier,
	webSessionManager settingsBackupImportWebSessionManager,
) error {
	if !backup.Payload.Server.HasContent() {
		return nil
	}

	server := *backup.Payload.Server
	if server.Developer != nil {
		developer := utils.NormalizeDeveloperConfig(*server.Developer)
		server.Developer = &developer
	}
	if server.PageTitle != nil {
		pageTitle, err := utils.NormalizePageTitle(*server.PageTitle)
		if err != nil {
			return err
		}
		server.PageTitle = &pageTitle
	}
	server.WebSessionQuickInput = utils.NormalizeSettingsBackupQuickInputSection(server.WebSessionQuickInput)
	if server.AuthAccess != nil {
		authAccess, err := utils.NormalizeAuthAccessConfig(*server.AuthAccess)
		if err != nil {
			return err
		}
		server.AuthAccess = &authAccess
	}

	if err := utils.UpdateConfig(cfg, func(c *utils.AppConfig) {
		if server.AIAssistantStatus != nil {
			c.Terminal.AIAssistantStatus = *server.AIAssistantStatus
		}
		if server.Developer != nil {
			c.Developer = *server.Developer
		}
		if server.PageTitle != nil {
			c.UI.PageTitle = *server.PageTitle
		}
		if server.DailyTip != nil {
			c.UI.DailyTipEnabled = server.DailyTip.Enabled
		}
		if server.WebSessionQuickInput != nil {
			next := c.UI.WebSessionQuickInput
			if server.WebSessionQuickInput.Pinned != nil {
				next.Pinned = append([]string(nil), (*server.WebSessionQuickInput.Pinned)...)
			}
			if server.WebSessionQuickInput.Recent != nil {
				next.Recent = append([]string(nil), (*server.WebSessionQuickInput.Recent)...)
			}
			c.UI.WebSessionQuickInput = utils.NormalizeWebSessionQuickInputConfig(next)
		}
		if server.Worktree != nil {
			c.Worktree.GlobalBaseDir = strings.TrimSpace(server.Worktree.GlobalBaseDir)
			c.Worktree.GlobalDirNamePattern = strings.TrimSpace(server.Worktree.GlobalDirNamePattern)
		}
		if server.TerminalShell != nil {
			utils.ApplyCurrentPlatformShell(&c.Terminal.Shell, strings.TrimSpace(server.TerminalShell.Shell))
		}
		if server.AuthAccess != nil {
			utils.ApplyAuthAccessConfigToAuthConfig(&c.Auth, *server.AuthAccess)
		}
	}); err != nil {
		return err
	}

	if terminalManager != nil {
		if server.AIAssistantStatus != nil {
			terminalManager.UpdateAIAssistantStatusConfig(*server.AIAssistantStatus)
		}
		if server.Developer != nil {
			terminalManager.UpdateScrollbackEnabled(server.Developer.EnableTerminalScrollback)
			terminalManager.UpdateTerminalStateSnapshotEnabled(server.Developer.EnableTerminalStateSnapshot)
			terminalManager.UpdateRenameTitleEachCommand(server.Developer.RenameSessionTitleEachCommand)
		}
		if server.TerminalShell != nil {
			terminalManager.UpdateShellConfig(cfg.Terminal.Shell)
		}
	}
	if webSessionManager != nil && server.Developer != nil {
		webSessionManager.RefreshDeveloperConfig()
	}
	return nil
}

func runtimePlatform() string {
	return strings.TrimSpace(strings.ToLower(runtime.GOOS))
}

func versionsBreakCompatibility(sourceVersion, currentVersion *semver.Version) bool {
	if sourceVersion == nil || currentVersion == nil {
		return false
	}
	if sourceVersion.Major() != currentVersion.Major() {
		return true
	}
	return sourceVersion.Major() == 0 && currentVersion.Major() == 0 &&
		sourceVersion.Minor() != currentVersion.Minor()
}

func isAbsPath(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "/") {
		return true
	}
	return len(trimmed) >= 3 &&
		(trimmed[1] == ':' && (trimmed[2] == '\\' || trimmed[2] == '/'))
}

func loPtr[T any](value T) *T {
	return &value
}

func currentSettingsBackupSourceApp() utils.SettingsBackupSourceApp {
	if appInfo == nil {
		return utils.SettingsBackupSourceApp{
			Name:    "Code Kanban",
			Version: "0.0.0",
			Channel: "unknown",
		}
	}
	return utils.SettingsBackupSourceApp{
		Name:    appInfo.Name,
		Version: appInfo.Version,
		Channel: appInfo.Channel,
	}
}
