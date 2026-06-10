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
		backup := utils.SettingsBackupFile{
			BackupSchemaVersion: utils.SettingsBackupSchemaVersion,
			BackupKind:          utils.SettingsBackupKind,
			CreatedAt:           time.Now().UTC(),
			SourceApp: currentSettingsBackupSourceApp(),
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
		op.Description = "校验备份 JSON、迁移到当前支持的备份格式，并返回阻断错误与风险警告"
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
		backup := input.Body
		if migrated {
			backup, _, err = utils.MigrateSettingsBackupFile(input.Body)
			if err != nil {
				return nil, huma.Error400BadRequest(err.Error())
			}
		}
		if err := applySettingsBackup(cfg, backup, terminalManager, webSessionManager); err != nil {
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
		CanImport: true,
		Migrated:  migrated,
	}

	addVersionWarnings(&result)
	addPayloadSections(&result, migratedBackup)

	if migratedBackup.Payload.Server != nil {
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

	sourceMajor := majorVersion(sourceVersion)
	currentMajor := majorVersion(currentVersion)
	if sourceMajor != "" && currentMajor != "" && sourceMajor != currentMajor {
		result.Warnings = append(result.Warnings, utils.SettingsBackupPreviewIssue{
			Code:    "source_app_major_version_differs",
			Level:   "warning",
			Message: fmt.Sprintf("Backup app major version %s differs from current major version %s.", sourceMajor, currentMajor),
		})
	}
}

func addPayloadSections(result *utils.SettingsBackupPreviewResult, backup utils.SettingsBackupFile) {
	if backup.Payload.Server != nil {
		result.Sections = append(result.Sections,
			utils.SettingsBackupPreviewSection{Key: "server.aiAssistantStatus", Label: "AI assistant status", Action: "replace", Target: "server", ChangedKeys: []string{"claudeCode", "codex", "qwenCode", "gemini", "cursor", "copilot"}},
			utils.SettingsBackupPreviewSection{Key: "server.developer", Label: "Developer config", Action: "replace", Target: "server", ChangedKeys: []string{"enableTerminalScrollback", "renameSessionTitleEachCommand", "enableTerminalStateSnapshot", "webSessionCodexDefaultSyncMode", "webSessionActiveCallTimeout"}},
			utils.SettingsBackupPreviewSection{Key: "server.dailyTip", Label: "Daily tip", Action: "replace", Target: "server", ChangedKeys: []string{"enabled"}},
			utils.SettingsBackupPreviewSection{Key: "server.webSessionQuickInput", Label: "Web session quick input", Action: "replace", Target: "server", ChangedKeys: []string{"pinned", "recent"}},
			utils.SettingsBackupPreviewSection{Key: "server.worktree", Label: "Worktree settings", Action: "replace", Target: "server", ChangedKeys: []string{"globalBaseDir", "globalDirNamePattern"}},
			utils.SettingsBackupPreviewSection{Key: "server.terminalShell", Label: "Terminal shell", Action: "replace", Target: "server", ChangedKeys: []string{"platform", "shell"}},
			utils.SettingsBackupPreviewSection{Key: "server.authAccess", Label: "Security access rules", Action: "replace", Target: "server", ChangedKeys: []string{"accessRules", "proxyHeader", "trustedProxies"}},
		)
	}
	if backup.Payload.Client != nil {
		result.Sections = append(result.Sections,
			utils.SettingsBackupPreviewSection{Key: "client.locale", Label: "Locale", Action: "replace", Target: "client", ChangedKeys: []string{"app-locale"}},
			utils.SettingsBackupPreviewSection{Key: "client.settings", Label: "Local settings", Action: "replace", Target: "client", ChangedKeys: []string{"general_settings"}},
		)
	}
}

func validateServerPayload(result *utils.SettingsBackupPreviewResult, payload *utils.SettingsBackupServerPayload) {
	payload.Developer = utils.NormalizeDeveloperConfig(payload.Developer)
	payload.WebSessionQuickInput = utils.NormalizeWebSessionQuickInputConfig(payload.WebSessionQuickInput)
	payload.AuthAccess = utils.SanitizeAuthAccessConfig(payload.AuthAccess)

	if payload.TerminalShell.Platform != "" && payload.TerminalShell.Platform != runtimePlatform() {
		result.Warnings = append(result.Warnings, utils.SettingsBackupPreviewIssue{
			Code:    "shell_platform_differs",
			Level:   "warning",
			Message: fmt.Sprintf("Backup shell was exported from platform %s, current platform is %s. Import will apply only the current platform shell field.", payload.TerminalShell.Platform, runtimePlatform()),
		})
	}

	if err := utils.ValidateShellCommand(payload.TerminalShell.Shell); err != nil {
		result.Errors = append(result.Errors, utils.SettingsBackupPreviewIssue{
			Code:    "invalid_terminal_shell",
			Level:   "error",
			Message: "Terminal shell is invalid: " + err.Error(),
		})
	}

	if _, err := utils.NormalizeAuthAccessConfig(payload.AuthAccess); err != nil {
		result.Errors = append(result.Errors, utils.SettingsBackupPreviewIssue{
			Code:    "invalid_auth_access",
			Level:   "error",
			Message: err.Error(),
		})
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
	if backup.Payload.Server == nil {
		return nil
	}

	server := *backup.Payload.Server
	server.Developer = utils.NormalizeDeveloperConfig(server.Developer)
	server.WebSessionQuickInput = utils.NormalizeWebSessionQuickInputConfig(server.WebSessionQuickInput)
	authAccess, err := utils.NormalizeAuthAccessConfig(server.AuthAccess)
	if err != nil {
		return err
	}
	server.AuthAccess = authAccess

	if err := utils.UpdateConfig(cfg, func(c *utils.AppConfig) {
		c.Terminal.AIAssistantStatus = server.AIAssistantStatus
		c.Developer = server.Developer
		c.UI.DailyTipEnabled = server.DailyTip.Enabled
		c.UI.WebSessionQuickInput = server.WebSessionQuickInput
		c.Worktree.GlobalBaseDir = strings.TrimSpace(server.Worktree.GlobalBaseDir)
		c.Worktree.GlobalDirNamePattern = strings.TrimSpace(server.Worktree.GlobalDirNamePattern)
		utils.ApplyCurrentPlatformShell(&c.Terminal.Shell, strings.TrimSpace(server.TerminalShell.Shell))
		utils.ApplyAuthAccessConfigToAuthConfig(&c.Auth, server.AuthAccess)
	}); err != nil {
		return err
	}

	if terminalManager != nil {
		terminalManager.UpdateAIAssistantStatusConfig(server.AIAssistantStatus)
		terminalManager.UpdateScrollbackEnabled(server.Developer.EnableTerminalScrollback)
		terminalManager.UpdateTerminalStateSnapshotEnabled(server.Developer.EnableTerminalStateSnapshot)
		terminalManager.UpdateRenameTitleEachCommand(server.Developer.RenameSessionTitleEachCommand)
		terminalManager.UpdateShellConfig(cfg.Terminal.Shell)
	}
	if webSessionManager != nil {
		webSessionManager.RefreshDeveloperConfig()
	}
	return nil
}

func majorVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	if version == "" {
		return ""
	}
	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func runtimePlatform() string {
	return strings.TrimSpace(strings.ToLower(runtime.GOOS))
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
		((trimmed[1] == ':' && (trimmed[2] == '\\' || trimmed[2] == '/')))
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
