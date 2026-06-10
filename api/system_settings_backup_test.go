package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"code-kanban/api/h"
	"code-kanban/utils"
)

type settingsBackupTerminalManagerStub struct {
	aiStatus                     utils.AIAssistantStatusConfig
	scrollbackEnabled            bool
	terminalStateSnapshotEnabled bool
	renameTitleEachCommand       bool
	shellConfig                  utils.TerminalShellConfig
}

type settingsBackupResponseEnvelope[T any] struct {
	Item T `json:"item"`
}

func (s *settingsBackupTerminalManagerStub) UpdateAIAssistantStatusConfig(config utils.AIAssistantStatusConfig) {
	s.aiStatus = config
}

func (s *settingsBackupTerminalManagerStub) UpdateScrollbackEnabled(value bool) {
	s.scrollbackEnabled = value
}

func (s *settingsBackupTerminalManagerStub) UpdateTerminalStateSnapshotEnabled(value bool) {
	s.terminalStateSnapshotEnabled = value
}

func (s *settingsBackupTerminalManagerStub) UpdateRenameTitleEachCommand(value bool) {
	s.renameTitleEachCommand = value
}

func (s *settingsBackupTerminalManagerStub) UpdateShellConfig(config utils.TerminalShellConfig) {
	s.shellConfig = config
}

type settingsBackupWebSessionManagerStub struct {
	refreshCount int
}

func (s *settingsBackupWebSessionManagerStub) RefreshDeveloperConfig() {
	s.refreshCount++
}

func TestSystemSettingsBackupExportReturnsServerPayload(t *testing.T) {
	cfg, _ := loadSystemSettingsBackupTestConfig(t, `
ui:
  dailyTipEnabled: false
  webSessionQuickInput:
    pinned: ["Ship"]
    recent: ["Draft"]
developer:
  enableTerminalScrollback: true
worktree:
  globalBaseDir: /tmp/worktrees
  globalDirNamePattern: "{projectName}-{branch}"
terminal:
  shell:
    linux: /bin/sh
`)

	app := newSystemSettingsBackupTestApp(t, cfg, nil, nil)
	resp := mustSystemSettingsBackupRequest(t, app, http.MethodGet, "/api/v1/system/settings-backup/export", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response failed: %v", err)
	}
	var payload settingsBackupResponseEnvelope[utils.SettingsBackupFile]
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		t.Fatalf("decode response failed: %v; body=%s", err, string(rawBody))
	}
	item := payload.Item
	if item.BackupSchemaVersion != utils.SettingsBackupSchemaVersion {
		t.Fatalf("backup schema version = %d, want %d; body=%s", item.BackupSchemaVersion, utils.SettingsBackupSchemaVersion, string(rawBody))
	}
	if item.Payload.Server == nil {
		t.Fatal("expected server payload")
	}
	if item.Payload.Server.DailyTip.Enabled {
		t.Fatal("expected exported daily tip to be false")
	}
	if got := item.Payload.Server.TerminalShell.Shell; strings.TrimSpace(got) != "/bin/sh" {
		t.Fatalf("terminal shell = %q, want /bin/sh", got)
	}
	if item.Payload.Server.AuthAccess.ProxyHeader != utils.DefaultAuthProxyHeader {
		t.Fatalf("proxyHeader = %q, want %q", item.Payload.Server.AuthAccess.ProxyHeader, utils.DefaultAuthProxyHeader)
	}
}

func TestSystemSettingsBackupPreviewWarnsOnVersionDifference(t *testing.T) {
	cfg, _ := loadSystemSettingsBackupTestConfig(t, "")
	oldAppInfo := appInfo
	appInfo = &AppInfo{Name: "Code Kanban", Version: "2.5.0", Channel: "stable"}
	t.Cleanup(func() {
		appInfo = oldAppInfo
	})

	app := newSystemSettingsBackupTestApp(t, cfg, nil, nil)
	backup := utils.SettingsBackupFile{
		BackupSchemaVersion: utils.SettingsBackupSchemaVersion,
		BackupKind:          utils.SettingsBackupKind,
		SourceApp: utils.SettingsBackupSourceApp{
			Name:    "Code Kanban",
			Version: "1.4.3",
			Channel: "stable",
		},
		Payload: utils.SettingsBackupPayload{
			Server: loPtr(utils.BuildSettingsBackupServerPayload(cfg)),
		},
	}

	body, err := json.Marshal(backup)
	if err != nil {
		t.Fatalf("marshal backup failed: %v", err)
	}
	resp := mustSystemSettingsBackupRequest(
		t,
		app,
		http.MethodPost,
		"/api/v1/system/settings-backup/preview",
		bytes.NewBuffer(body),
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response failed: %v", err)
	}
	var payload settingsBackupResponseEnvelope[utils.SettingsBackupPreviewResult]
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		t.Fatalf("decode response failed: %v; body=%s", err, string(rawBody))
	}
	if !payload.Item.CanImport {
		t.Fatalf("expected preview to remain importable; body=%s", string(rawBody))
	}
	if len(payload.Item.Warnings) == 0 {
		t.Fatal("expected version difference warning")
	}
}

func TestSystemSettingsBackupPreviewRejectsInvalidShell(t *testing.T) {
	cfg, _ := loadSystemSettingsBackupTestConfig(t, "")
	app := newSystemSettingsBackupTestApp(t, cfg, nil, nil)
	backup := utils.SettingsBackupFile{
		BackupSchemaVersion: utils.SettingsBackupSchemaVersion,
		BackupKind:          utils.SettingsBackupKind,
		SourceApp:           utils.SettingsBackupSourceApp{Name: "Code Kanban", Version: "1.0.0", Channel: "stable"},
		Payload: utils.SettingsBackupPayload{
			Server: &utils.SettingsBackupServerPayload{
				AIAssistantStatus:    cfg.Terminal.AIAssistantStatus,
				Developer:            cfg.Developer,
				DailyTip:             utils.BackupDailyTipSettings{Enabled: true},
				WebSessionQuickInput: cfg.UI.WebSessionQuickInput,
				Worktree:             cfg.Worktree,
				TerminalShell:        utils.SettingsBackupShellConfig{Platform: "linux", Shell: "/definitely/missing/shell"},
				AuthAccess:           utils.DefaultAuthAccessConfig(),
			},
		},
	}

	body, err := json.Marshal(backup)
	if err != nil {
		t.Fatalf("marshal backup failed: %v", err)
	}
	resp := mustSystemSettingsBackupRequest(
		t,
		app,
		http.MethodPost,
		"/api/v1/system/settings-backup/preview",
		bytes.NewBuffer(body),
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response failed: %v", err)
	}
	var payload settingsBackupResponseEnvelope[utils.SettingsBackupPreviewResult]
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		t.Fatalf("decode response failed: %v; body=%s", err, string(rawBody))
	}
	if payload.Item.CanImport {
		t.Fatalf("expected preview to block invalid shell; body=%s", string(rawBody))
	}
	if len(payload.Item.Errors) == 0 {
		t.Fatalf("expected preview errors; body=%s", string(rawBody))
	}
}

func TestSystemSettingsBackupImportAppliesConfigAndHotReloads(t *testing.T) {
	cfg, configPath := loadSystemSettingsBackupTestConfig(t, `
ui:
  dailyTipEnabled: true
  webSessionQuickInput:
    pinned: ["continue"]
developer:
  enableTerminalScrollback: false
worktree:
  globalBaseDir: ""
  globalDirNamePattern: "{projectName}-{branch}"
terminal:
  shell:
    linux: /bin/bash
`)
	terminalStub := &settingsBackupTerminalManagerStub{}
	webSessionStub := &settingsBackupWebSessionManagerStub{}
	app := newSystemSettingsBackupTestApp(t, cfg, terminalStub, webSessionStub)

	backup := utils.SettingsBackupFile{
		BackupSchemaVersion: utils.SettingsBackupSchemaVersion,
		BackupKind:          utils.SettingsBackupKind,
		SourceApp:           utils.SettingsBackupSourceApp{Name: "Code Kanban", Version: "1.0.0", Channel: "stable"},
		Payload: utils.SettingsBackupPayload{
			Server: &utils.SettingsBackupServerPayload{
				AIAssistantStatus: utils.AIAssistantStatusConfig{
					ClaudeCode: false,
					Codex:      false,
					QwenCode:   true,
					Gemini:     true,
					Cursor:     false,
					Copilot:    true,
				},
				Developer: utils.DeveloperConfig{
					EnableTerminalScrollback:       true,
					RenameSessionTitleEachCommand:  true,
					EnableTerminalStateSnapshot:    true,
					WebSessionCodexDefaultSyncMode: "deep",
					WebSessionActiveCallTimeout: utils.WebSessionActiveCallTimeoutConfig{
						EnabledMode:          utils.SettingModeOn,
						TimeoutMode:          utils.WebSessionActiveCallTimeoutModeCustom,
						CustomTimeoutSeconds: 180,
						PromptTemplate:       "Resume",
						CallKinds: utils.WebSessionActiveCallTimeoutKindsConfig{
							UseDefault: false,
							MCP:        true,
							Command:    true,
							Tool:       false,
						},
					},
				},
				DailyTip: utils.BackupDailyTipSettings{Enabled: false},
				WebSessionQuickInput: utils.WebSessionQuickInputConfig{
					Pinned: []string{"Plan", "Ship"},
					Recent: []string{"Deploy"},
				},
				Worktree: utils.WorktreeConfig{
					GlobalBaseDir:        t.TempDir(),
					GlobalDirNamePattern: "{projectName}-custom",
				},
				TerminalShell: utils.SettingsBackupShellConfig{
					Platform: "linux",
					Shell:    "/bin/sh",
				},
				AuthAccess: utils.AuthAccessConfig{
					AccessRules: utils.AuthAccessRulesConfig{
						BypassIPs: []string{"127.0.0.1"},
					},
					ProxyHeader:    utils.DefaultAuthProxyHeader,
					TrustedProxies: []string{"10.0.0.0/24"},
				},
			},
		},
	}

	body, err := json.Marshal(backup)
	if err != nil {
		t.Fatalf("marshal backup failed: %v", err)
	}
	resp := mustSystemSettingsBackupRequest(
		t,
		app,
		http.MethodPost,
		"/api/v1/system/settings-backup/import",
		bytes.NewBuffer(body),
	)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if cfg.UI.DailyTipEnabled {
		t.Fatal("expected daily tip to be disabled after import")
	}
	if !cfg.Developer.EnableTerminalScrollback || !cfg.Developer.RenameSessionTitleEachCommand {
		t.Fatalf("developer config not applied: %#v", cfg.Developer)
	}
	if got := strings.TrimSpace(utils.CurrentPlatformShell(cfg.Terminal.Shell)); got != "/bin/sh" {
		t.Fatalf("current platform shell = %q, want /bin/sh", got)
	}
	if terminalStub.scrollbackEnabled != true {
		t.Fatal("expected terminal scrollback hot reload")
	}
	if strings.TrimSpace(utils.CurrentPlatformShell(terminalStub.shellConfig)) != "/bin/sh" {
		t.Fatalf("hot reloaded shell = %q, want /bin/sh", utils.CurrentPlatformShell(terminalStub.shellConfig))
	}
	if webSessionStub.refreshCount != 1 {
		t.Fatalf("web session refresh count = %d, want 1", webSessionStub.refreshCount)
	}

	rewritten, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	content := string(rewritten)
	if !strings.Contains(content, "dailyTipEnabled: false") {
		t.Fatalf("expected config file rewrite, got:\n%s", content)
	}
	if !strings.Contains(content, "globalDirNamePattern: '{projectName}-custom'") &&
		!strings.Contains(content, "globalDirNamePattern: \"{projectName}-custom\"") &&
		!strings.Contains(content, "globalDirNamePattern: {projectName}-custom") {
		t.Fatalf("expected worktree pattern rewrite, got:\n%s", content)
	}
}

func loadSystemSettingsBackupTestConfig(t *testing.T, configYAML string) (*utils.AppConfig, string) {
	t.Helper()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	content := strings.TrimSpace(configYAML)
	if content == "" {
		content = `
ui:
  dailyTipEnabled: true
`
	}
	if err := os.WriteFile(configPath, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	return utils.ReadConfig(), configPath
}

func newSystemSettingsBackupTestApp(
	t *testing.T,
	cfg *utils.AppConfig,
	terminalManager systemTerminalManager,
	webSessionManager settingsBackupImportWebSessionManager,
) *fiber.App {
	t.Helper()

	app := fiber.New()
	_, v1 := h.NewAPI(app, cfg)
	registerSystemSettingsBackupRoutes(v1, cfg, terminalManager, webSessionManager)
	return app
}

func mustSystemSettingsBackupRequest(
	t *testing.T,
	app *fiber.App,
	method string,
	target string,
	body *bytes.Buffer,
) *http.Response {
	t.Helper()

	var payload *bytes.Buffer
	if body != nil {
		payload = body
	} else {
		payload = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, target, payload)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	return resp
}
