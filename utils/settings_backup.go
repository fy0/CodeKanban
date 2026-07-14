package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"
)

const (
	SettingsBackupSchemaVersion = 2
	SettingsBackupKind          = "settings"
)

type SettingsBackupSourceApp struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Channel string `json:"channel"`
}

type SettingsBackupQuickInputSection struct {
	Pinned *[]string `json:"pinned,omitempty"`
	Recent *[]string `json:"recent,omitempty"`
}

func (s *SettingsBackupQuickInputSection) HasContent() bool {
	return s != nil && (s.Pinned != nil || s.Recent != nil)
}

type SettingsBackupServerPayload struct {
	AIAssistantStatus    *AIAssistantStatusConfig         `json:"aiAssistantStatus,omitempty"`
	Developer            *DeveloperConfig                 `json:"developer,omitempty"`
	PageTitle            *string                          `json:"pageTitle,omitempty"`
	DailyTip             *BackupDailyTipSettings          `json:"dailyTip,omitempty"`
	WebSessionQuickInput *SettingsBackupQuickInputSection `json:"webSessionQuickInput,omitempty"`
	Worktree             *WorktreeConfig                  `json:"worktree,omitempty"`
	TerminalShell        *SettingsBackupShellConfig       `json:"terminalShell,omitempty"`
	AuthAccess           *AuthAccessConfig                `json:"authAccess,omitempty"`
}

func (p *SettingsBackupServerPayload) HasContent() bool {
	return p != nil && (p.AIAssistantStatus != nil ||
		p.Developer != nil ||
		p.PageTitle != nil ||
		p.DailyTip != nil ||
		p.WebSessionQuickInput.HasContent() ||
		p.Worktree != nil ||
		p.TerminalShell != nil ||
		p.AuthAccess != nil)
}

type SettingsBackupShellConfig struct {
	Platform string `json:"platform"`
	Shell    string `json:"shell"`
}

type SettingsBackupClientPayload struct {
	Locale   *string         `json:"locale,omitempty"`
	Settings json.RawMessage `json:"settings,omitempty"`
}

func (p *SettingsBackupClientPayload) HasContent() bool {
	return p != nil && (p.Locale != nil || len(bytes.TrimSpace(p.Settings)) > 0)
}

type SettingsBackupPayload struct {
	Server *SettingsBackupServerPayload `json:"server,omitempty"`
	Client *SettingsBackupClientPayload `json:"client,omitempty"`
}

func (p SettingsBackupPayload) HasContent() bool {
	return p.Server.HasContent() || p.Client.HasContent()
}

type SettingsBackupExportOptions struct {
	IncludeServer           bool   `json:"includeServer,omitempty"`
	IncludeClient           bool   `json:"includeClient,omitempty"`
	IncludeSecurityAccess   bool   `json:"includeSecurityAccess,omitempty"`
	IncludeQuickInputRecent bool   `json:"includeQuickInputRecent,omitempty"`
	FileNameRule            string `json:"fileNameRule,omitempty"`
	IncludeMetadata         bool   `json:"includeMetadata,omitempty"`
}

type SettingsBackupMeta struct {
	Description   string                       `json:"description,omitempty"`
	ExportOptions *SettingsBackupExportOptions `json:"exportOptions,omitempty"`
}

type SettingsBackupFile struct {
	BackupSchemaVersion int                     `json:"backupSchemaVersion"`
	BackupKind          string                  `json:"backupKind"`
	CreatedAt           *time.Time              `json:"createdAt,omitempty"`
	SourceApp           SettingsBackupSourceApp `json:"sourceApp"`
	Meta                *SettingsBackupMeta     `json:"meta,omitempty"`
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
	quickInput := NormalizeWebSessionQuickInputConfig(cfg.UI.WebSessionQuickInput)
	aiStatus := cfg.Terminal.AIAssistantStatus
	developer := NormalizeDeveloperConfig(cfg.Developer)
	dailyTip := BackupDailyTipSettings{Enabled: cfg.UI.DailyTipEnabled}
	worktree := cfg.Worktree
	terminalShell := SettingsBackupShellConfig{
		Platform: runtime.GOOS,
		Shell:    currentShell,
	}
	authAccess := SanitizeAuthAccessConfig(AuthAccessConfigFromAuthConfig(cfg.Auth))

	return SettingsBackupServerPayload{
		AIAssistantStatus: ptrValue(aiStatus),
		Developer:         ptrValue(developer),
		PageTitle:         ptrValue(cfg.UI.PageTitle),
		DailyTip:          ptrValue(dailyTip),
		WebSessionQuickInput: &SettingsBackupQuickInputSection{
			Pinned: ptrStringSlice(quickInput.Pinned),
			Recent: ptrStringSlice(quickInput.Recent),
		},
		Worktree:      ptrValue(worktree),
		TerminalShell: ptrValue(terminalShell),
		AuthAccess:    ptrValue(authAccess),
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
	switch input.BackupSchemaVersion {
	case SettingsBackupSchemaVersion:
		return input, false, nil
	case 0:
		return SettingsBackupFile{}, false, fmt.Errorf("backup schema version is required")
	default:
		return SettingsBackupFile{}, false, fmt.Errorf("unsupported backup schema version: %d", input.BackupSchemaVersion)
	}
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
	if !input.Payload.HasContent() {
		return fmt.Errorf("backup payload is empty")
	}
	return nil
}

func NormalizeSettingsBackupQuickInputSection(
	section *SettingsBackupQuickInputSection,
) *SettingsBackupQuickInputSection {
	if section == nil {
		return nil
	}
	normalized := &SettingsBackupQuickInputSection{}
	if section.Pinned != nil {
		pinned := NormalizeWebSessionQuickInputConfig(WebSessionQuickInputConfig{
			Pinned: append([]string(nil), (*section.Pinned)...),
		}).Pinned
		normalized.Pinned = ptrStringSlice(pinned)
	}
	if section.Recent != nil {
		recent := NormalizeWebSessionQuickInputConfig(WebSessionQuickInputConfig{
			Recent: append([]string(nil), (*section.Recent)...),
		}).Recent
		normalized.Recent = ptrStringSlice(recent)
	}
	if !normalized.HasContent() {
		return nil
	}
	return normalized
}

func ptrValue[T any](value T) *T {
	return &value
}

func ptrStringSlice(values []string) *[]string {
	items := append([]string(nil), values...)
	return &items
}
