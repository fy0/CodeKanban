package utils

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"
)

const (
	SettingsBackupSchemaVersion = 1
	SettingsBackupKind          = "settings"
)

type SettingsBackupSourceApp struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Channel string `json:"channel"`
}

type SettingsBackupServerPayload struct {
	AIAssistantStatus  AIAssistantStatusConfig `json:"aiAssistantStatus"`
	Developer          DeveloperConfig         `json:"developer"`
	DailyTip           BackupDailyTipSettings  `json:"dailyTip"`
	WebSessionQuickInput WebSessionQuickInputConfig `json:"webSessionQuickInput"`
	Worktree           WorktreeConfig          `json:"worktree"`
	TerminalShell      SettingsBackupShellConfig `json:"terminalShell"`
	AuthAccess         AuthAccessConfig        `json:"authAccess"`
}

type SettingsBackupShellConfig struct {
	Platform string `json:"platform"`
	Shell    string `json:"shell"`
}

type SettingsBackupClientPayload struct {
	Locale   string          `json:"locale"`
	Settings json.RawMessage `json:"settings"`
}

type SettingsBackupPayload struct {
	Server *SettingsBackupServerPayload `json:"server,omitempty"`
	Client *SettingsBackupClientPayload `json:"client,omitempty"`
}

type SettingsBackupFile struct {
	BackupSchemaVersion int                     `json:"backupSchemaVersion"`
	BackupKind          string                  `json:"backupKind"`
	CreatedAt           time.Time               `json:"createdAt"`
	SourceApp           SettingsBackupSourceApp `json:"sourceApp"`
	Payload             SettingsBackupPayload   `json:"payload"`
}

type SettingsBackupPreviewIssue struct {
	Code    string `json:"code"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

type SettingsBackupPreviewSection struct {
	Key          string   `json:"key"`
	Label        string   `json:"label"`
	Action       string   `json:"action"`
	Target       string   `json:"target"`
	ChangedKeys  []string `json:"changedKeys,omitempty"`
	WarningCodes []string `json:"warningCodes,omitempty"`
}

type SettingsBackupPreviewResult struct {
	BackupSchemaVersion int                            `json:"backupSchemaVersion"`
	BackupKind          string                         `json:"backupKind"`
	SourceApp           SettingsBackupSourceApp        `json:"sourceApp"`
	CurrentApp          SettingsBackupSourceApp        `json:"currentApp"`
	CanImport           bool                           `json:"canImport"`
	Errors              []SettingsBackupPreviewIssue   `json:"errors"`
	Warnings            []SettingsBackupPreviewIssue   `json:"warnings"`
	Sections            []SettingsBackupPreviewSection `json:"sections"`
	Migrated            bool                           `json:"migrated"`
}

func BuildSettingsBackupServerPayload(cfg *AppConfig) SettingsBackupServerPayload {
	currentShell := strings.TrimSpace(CurrentPlatformShell(cfg.Terminal.Shell))
	return SettingsBackupServerPayload{
		AIAssistantStatus:    cfg.Terminal.AIAssistantStatus,
		Developer:            NormalizeDeveloperConfig(cfg.Developer),
		DailyTip:             BackupDailyTipSettings{Enabled: cfg.UI.DailyTipEnabled},
		WebSessionQuickInput: NormalizeWebSessionQuickInputConfig(cfg.UI.WebSessionQuickInput),
		Worktree:             cfg.Worktree,
		TerminalShell: SettingsBackupShellConfig{
			Platform: runtime.GOOS,
			Shell:    currentShell,
		},
		AuthAccess: SanitizeAuthAccessConfig(AuthAccessConfigFromAuthConfig(cfg.Auth)),
	}
}

type BackupDailyTipSettings struct {
	Enabled bool `json:"enabled"`
}

func CurrentPlatformShell(cfg TerminalShellConfig) string {
	switch runtime.GOOS {
	case "windows":
		return cfg.Windows
	case "darwin":
		return cfg.Darwin
	default:
		return cfg.Linux
	}
}

func ApplyCurrentPlatformShell(cfg *TerminalShellConfig, shell string) {
	if cfg == nil {
		return
	}
	switch runtime.GOOS {
	case "windows":
		cfg.Windows = shell
	case "darwin":
		cfg.Darwin = shell
	default:
		cfg.Linux = shell
	}
}

func MigrateSettingsBackupFile(input SettingsBackupFile) (SettingsBackupFile, bool, error) {
	if input.BackupSchemaVersion == SettingsBackupSchemaVersion {
		return input, false, nil
	}
	if input.BackupSchemaVersion <= 0 {
		return SettingsBackupFile{}, false, fmt.Errorf("backup schema version is required")
	}
	return SettingsBackupFile{}, false, fmt.Errorf("unsupported backup schema version: %d", input.BackupSchemaVersion)
}

func ValidateSettingsBackupFileShape(input SettingsBackupFile) error {
	if input.BackupSchemaVersion <= 0 {
		return fmt.Errorf("backupSchemaVersion is required")
	}
	if strings.TrimSpace(input.BackupKind) == "" {
		return fmt.Errorf("backupKind is required")
	}
	if input.BackupKind != SettingsBackupKind {
		return fmt.Errorf("unsupported backup kind: %s", input.BackupKind)
	}
	if input.Payload.Server == nil && input.Payload.Client == nil {
		return fmt.Errorf("backup payload is empty")
	}
	return nil
}
