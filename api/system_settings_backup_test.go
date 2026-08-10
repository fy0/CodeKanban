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
	scrollbackEnabled            bool
	terminalStateSnapshotEnabled bool
	shellConfig                  utils.TerminalShellConfig
}

type settingsBackupResponseEnvelope[T any] struct {
	Item T `json:"item"`
}

func (s *settingsBackupTerminalManagerStub) UpdateScrollbackEnabled(value bool) {
	s.scrollbackEnabled = value
}

func (s *settingsBackupTerminalManagerStub) UpdateTerminalStateSnapshotEnabled(value bool) {
	s.terminalStateSnapshotEnabled = value
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
  pageTitle: Staging Board
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
	if item.CreatedAt == nil || item.CreatedAt.IsZero() {
		t.Fatal("expected createdAt to be set")
	}
	if item.Payload.Server.DailyTip == nil || item.Payload.Server.DailyTip.Enabled {
		t.Fatal("expected exported daily tip to be false")
	}
	if item.Payload.Server.PageTitle == nil || *item.Payload.Server.PageTitle != "Staging Board" {
		t.Fatalf("expected exported page title, got %#v", item.Payload.Server.PageTitle)
	}
	if item.Payload.Server.TerminalShell == nil {
		t.Fatal("expected terminal shell payload")
	}
	if got := item.Payload.Server.TerminalShell.Shell; strings.TrimSpace(got) != "/bin/sh" {
		t.Fatalf("terminal shell = %q, want /bin/sh", got)
	}
	if item.Payload.Server.AuthAccess == nil {
		t.Fatal("expected auth access payload")
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
	foundBreakingWarning := false
	for _, issue := range payload.Item.Warnings {
		if issue.Code == "source_app_breaking_version_differs" {
			foundBreakingWarning = true
			break
		}
	}
	if !foundBreakingWarning {
		t.Fatalf("expected breaking version warning, got %#v", payload.Item.Warnings)
	}
	foundPermissionSetting := false
	foundAutoRetryDefaults := false
	for _, section := range payload.Item.Sections {
		if section.Key != "server.developer" {
			continue
		}
		for _, key := range section.ChangedKeys {
			if key == "webSessionCodexDefaultPermissionLevel" {
				foundPermissionSetting = true
			}
			if key == "webSessionAutoRetryDefaults" {
				foundAutoRetryDefaults = true
			}
		}
	}
	if !foundPermissionSetting {
		t.Fatalf("expected developer backup preview to include the Codex permission setting, got %#v", payload.Item.Sections)
	}
	if !foundAutoRetryDefaults {
		t.Fatalf("expected developer backup preview to include the web session auto-retry defaults, got %#v", payload.Item.Sections)
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
				Developer: loPtr(cfg.Developer),
				DailyTip:  loPtr(utils.BackupDailyTipSettings{Enabled: true}),
				WebSessionQuickInput: &utils.SettingsBackupQuickInputSection{
					Pinned: loPtr(append([]string(nil), cfg.UI.WebSessionQuickInput.Pinned...)),
					Recent: loPtr(append([]string(nil), cfg.UI.WebSessionQuickInput.Recent...)),
				},
				Worktree:      loPtr(cfg.Worktree),
				TerminalShell: loPtr(utils.SettingsBackupShellConfig{Platform: "linux", Shell: "/definitely/missing/shell"}),
				AuthAccess:    loPtr(utils.DefaultAuthAccessConfig()),
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

func TestSystemSettingsBackupPreviewRejectsInvalidPageTitle(t *testing.T) {
	cfg, _ := loadSystemSettingsBackupTestConfig(t, "")
	app := newSystemSettingsBackupTestApp(t, cfg, nil, nil)
	backup := utils.SettingsBackupFile{
		BackupSchemaVersion: utils.SettingsBackupSchemaVersion,
		BackupKind:          utils.SettingsBackupKind,
		SourceApp:           utils.SettingsBackupSourceApp{Name: "Code Kanban", Version: "1.0.0", Channel: "stable"},
		Payload: utils.SettingsBackupPayload{
			Server: &utils.SettingsBackupServerPayload{
				PageTitle: loPtr(strings.Repeat("界", utils.MaxPageTitleRunes+1)),
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

	var payload settingsBackupResponseEnvelope[utils.SettingsBackupPreviewResult]
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload.Item.CanImport {
		t.Fatal("expected invalid page title to block import")
	}
	if len(payload.Item.Errors) != 1 || payload.Item.Errors[0].Code != "invalid_page_title" {
		t.Fatalf("expected invalid_page_title error, got %#v", payload.Item.Errors)
	}
}

func TestSystemSettingsBackupImportAppliesConfigAndHotReloads(t *testing.T) {
	cfg, configPath := loadSystemSettingsBackupTestConfig(t, `
ui:
  pageTitle: Original
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
				PageTitle: loPtr("Imported Board"),
				Developer: loPtr(utils.DeveloperConfig{
					EnableTerminalScrollback:              true,
					EnableTerminalStateSnapshot:           true,
					WebSessionCodexDefaultModel:           "custom-codex-model",
					WebSessionCodexDefaultReasoningEffort: "high",
					WebSessionCodexDefaultPermissionLevel: "yolo",
					WebSessionCodexDefaultSyncMode:        "deep",
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
				}),
				DailyTip: loPtr(utils.BackupDailyTipSettings{Enabled: false}),
				WebSessionQuickInput: &utils.SettingsBackupQuickInputSection{
					Pinned: loPtr([]string{"Plan", "Ship"}),
					Recent: loPtr([]string{"Deploy"}),
				},
				Worktree: loPtr(utils.WorktreeConfig{
					GlobalBaseDir:        t.TempDir(),
					GlobalDirNamePattern: "{projectName}-custom",
				}),
				TerminalShell: loPtr(utils.SettingsBackupShellConfig{
					Platform: "linux",
					Shell:    "/bin/sh",
				}),
				AuthAccess: loPtr(utils.AuthAccessConfig{
					AccessRules: utils.AuthAccessRulesConfig{
						BypassIPs: []string{"127.0.0.1"},
					},
					ProxyHeader:    utils.DefaultAuthProxyHeader,
					TrustedProxies: []string{"10.0.0.0/24"},
				}),
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
	if got := cfg.UI.PageTitle; got != "Imported Board" {
		t.Fatalf("page title = %q, want Imported Board", got)
	}
	if !cfg.Developer.EnableTerminalScrollback {
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
	if !strings.Contains(content, "pageTitle: Imported Board") &&
		!strings.Contains(content, "pageTitle: 'Imported Board'") &&
		!strings.Contains(content, `pageTitle: "Imported Board"`) {
		t.Fatalf("expected imported page title in config, got:\n%s", content)
	}
	if !strings.Contains(content, "globalDirNamePattern: '{projectName}-custom'") &&
		!strings.Contains(content, "globalDirNamePattern: \"{projectName}-custom\"") &&
		!strings.Contains(content, "globalDirNamePattern: {projectName}-custom") {
		t.Fatalf("expected worktree pattern rewrite, got:\n%s", content)
	}
}

func TestSystemSettingsBackupPreviewRejectsLegacySchemaV1(t *testing.T) {
	cfg, _ := loadSystemSettingsBackupTestConfig(t, "")
	app := newSystemSettingsBackupTestApp(t, cfg, nil, nil)
	backup := utils.SettingsBackupFile{
		BackupSchemaVersion: 1,
		BackupKind:          utils.SettingsBackupKind,
		SourceApp:           utils.SettingsBackupSourceApp{Name: "Code Kanban", Version: "0.1.0-alpha", Channel: "dev"},
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
	if resp.StatusCode != http.StatusBadRequest {
		rawBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body=%s", resp.StatusCode, http.StatusBadRequest, string(rawBody))
	}
}

func TestSystemSettingsBackupImportAppliesOnlySelectedServerSections(t *testing.T) {
	cfg, _ := loadSystemSettingsBackupTestConfig(t, `
ui:
  pageTitle: Original
  dailyTipEnabled: true
  webSessionQuickInput:
    pinned: ["continue"]
    recent: ["keep"]
developer:
  enableTerminalScrollback: false
worktree:
  globalBaseDir: /tmp/original
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
		SourceApp:           utils.SettingsBackupSourceApp{Name: "Code Kanban", Version: "0.1.0-alpha", Channel: "dev"},
		Payload: utils.SettingsBackupPayload{
			Server: &utils.SettingsBackupServerPayload{
				DailyTip: loPtr(utils.BackupDailyTipSettings{Enabled: false}),
				WebSessionQuickInput: &utils.SettingsBackupQuickInputSection{
					Pinned: loPtr([]string{"Plan"}),
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
		t.Fatal("expected daily tip to be updated")
	}
	if got := cfg.UI.PageTitle; got != "Original" {
		t.Fatalf("page title should remain unchanged, got %q", got)
	}
	if got := cfg.UI.WebSessionQuickInput.Pinned; len(got) != 1 || got[0] != "Plan" {
		t.Fatalf("unexpected pinned quick input: %#v", got)
	}
	if got := cfg.UI.WebSessionQuickInput.Recent; len(got) != 1 || got[0] != "keep" {
		t.Fatalf("expected recent quick input to be preserved, got %#v", got)
	}
	if cfg.Developer.EnableTerminalScrollback {
		t.Fatalf("developer config should remain unchanged: %#v", cfg.Developer)
	}
	if terminalStub.scrollbackEnabled {
		t.Fatal("did not expect developer hot reload when developer section is omitted")
	}
	if webSessionStub.refreshCount != 0 {
		t.Fatalf("unexpected web session refresh count = %d", webSessionStub.refreshCount)
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
