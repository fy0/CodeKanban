package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/knadh/koanf/v2"
	"gopkg.in/yaml.v3"
)

func TestConfigDatabaseMigratesLegacyYAMLAndPersistsAcrossRestart(t *testing.T) {
	legacy := []byte(strings.TrimSpace(`
serveAt: ":3999"
auth:
  sessionTTL: 24h
  frontendSalt: legacy-frontend-salt
  passwordHash: legacy-password-hash
  tokenSecret: legacy-token-secret
  proxyHeader: X-Real-IP
  trustedProxies: [10.0.0.0/24]
developer:
  enableTerminalScrollback: true
git:
  readEngine: builtin
  writeEngine: system
worktree:
  globalBaseDir: /tmp/worktrees
  globalDirNamePattern: "{projectName}-custom"
terminal:
  shell:
    linux: /bin/sh
ui:
  pageTitle: Legacy Board
  dailyTipEnabled: true
  webSessionQuickInput:
    pinned: [Plan]
    recent: [global prompt]
    recentByProject:
      project-1: [project prompt]
`) + "\n")

	configPath := prepareConfigDatabaseTest(t, legacy)
	config := ReadConfig()
	store, err := InitConfigDatabase(config)
	if err != nil {
		t.Fatalf("InitConfigDatabase() error = %v", err)
	}

	wantDatabasePath, err := filepath.Abs(filepath.Join("data", configDatabaseFileName))
	if err != nil {
		t.Fatalf("resolve expected database path: %v", err)
	}
	if store.path != wantDatabasePath {
		t.Fatalf("database path = %q, want %q", store.path, wantDatabasePath)
	}
	if config.ConfigStoreVersion != CurrentConfigStoreVersion {
		t.Fatalf("config store version = %d, want %d", config.ConfigStoreVersion, CurrentConfigStoreVersion)
	}
	if config.UI.PageTitle != "Legacy Board" || !config.UI.DailyTipEnabled {
		t.Fatalf("legacy UI settings were not imported: %#v", config.UI)
	}
	if config.Auth.FrontendSalt != "legacy-frontend-salt" || config.Auth.TokenSecret != "legacy-token-secret" {
		t.Fatalf("legacy auth credentials were not imported: %#v", config.Auth)
	}

	view, err := store.QuickInputView("project-1")
	if err != nil {
		t.Fatalf("QuickInputView() error = %v", err)
	}
	if !reflect.DeepEqual(view.GlobalRecent, []string{"global prompt"}) {
		t.Fatalf("global history = %#v, want only legacy global history", view.GlobalRecent)
	}
	if !reflect.DeepEqual(view.ProjectRecent, []string{"project prompt"}) {
		t.Fatalf("project history = %#v", view.ProjectRecent)
	}
	health := store.Health(t.Context())
	if health.Mode != "read_write" || !health.ReadProbe.OK || !health.WriteProbe.OK {
		t.Fatalf("unexpected writable config database health: %#v", health)
	}

	backupPath := configPath + ".pre-config-db-v1.bak"
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read legacy backup: %v", err)
	}
	if !reflect.DeepEqual(backup, legacy) {
		t.Fatalf("legacy backup changed:\n%s", backup)
	}
	assertBootstrapOnlyConfig(t, configPath)

	if err := UpdateConfig(config, func(next *AppConfig) {
		next.UI.PageTitle = "Database Board"
		next.UI.DailyTipEnabled = false
		next.Developer.EnableTerminalScrollback = false
	}); err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}
	if _, err := store.RecordQuickInput("project prompt two", "project-1"); err != nil {
		t.Fatalf("RecordQuickInput() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close first config database: %v", err)
	}

	restartedConfig := ReadConfig()
	restartedStore, err := InitConfigDatabase(restartedConfig)
	if err != nil {
		t.Fatalf("restart InitConfigDatabase() error = %v", err)
	}
	if restartedConfig.UI.PageTitle != "Database Board" || restartedConfig.UI.DailyTipEnabled {
		t.Fatalf("runtime settings did not survive restart: %#v", restartedConfig.UI)
	}
	if restartedConfig.Developer.EnableTerminalScrollback {
		t.Fatalf("developer settings did not survive restart: %#v", restartedConfig.Developer)
	}
	restartedView, err := restartedStore.QuickInputView("project-1")
	if err != nil {
		t.Fatalf("restarted QuickInputView() error = %v", err)
	}
	if !reflect.DeepEqual(restartedView.GlobalRecent, []string{"project prompt two", "global prompt"}) {
		t.Fatalf("restarted global history = %#v", restartedView.GlobalRecent)
	}
	if !reflect.DeepEqual(restartedView.ProjectRecent, []string{"project prompt two", "project prompt"}) {
		t.Fatalf("restarted project history = %#v", restartedView.ProjectRecent)
	}
	restartedBackup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup after restart: %v", err)
	}
	if !reflect.DeepEqual(restartedBackup, legacy) {
		t.Fatal("one-time legacy backup was overwritten on restart")
	}
	assertBootstrapOnlyConfig(t, configPath)
}

func TestUpdateConfigRequiresInitializedDatabase(t *testing.T) {
	prepareConfigDatabaseTest(t, []byte("ui:\n  pageTitle: Original\n"))
	config := ReadConfig()
	err := UpdateConfig(config, func(next *AppConfig) {
		next.UI.PageTitle = "Changed"
	})
	if !errors.Is(err, ErrConfigStoreNotInitialized) {
		t.Fatalf("UpdateConfig() error = %v, want %v", err, ErrConfigStoreNotInitialized)
	}
	if config.UI.PageTitle != "Original" {
		t.Fatalf("failed update changed in-memory config to %q", config.UI.PageTitle)
	}
}

func TestConfigDatabaseQuickInputRecordsGlobalAndProjectHistory(t *testing.T) {
	prepareConfigDatabaseTest(t, []byte("ui:\n  webSessionQuickInput:\n    pinned: [continue]\n"))
	config := ReadConfig()
	store, err := InitConfigDatabase(config)
	if err != nil {
		t.Fatalf("InitConfigDatabase() error = %v", err)
	}

	for index := 1; index <= WebSessionQuickInputRecentLimit+1; index++ {
		if _, err := store.RecordQuickInput(fmt.Sprintf("prompt %02d", index), "project-1"); err != nil {
			t.Fatalf("RecordQuickInput(%d) error = %v", index, err)
		}
	}
	if _, err := store.RecordQuickInput("prompt 10", "project-1"); err != nil {
		t.Fatalf("record duplicate prompt: %v", err)
	}

	view, err := store.QuickInputView("project-1")
	if err != nil {
		t.Fatalf("QuickInputView() error = %v", err)
	}
	if len(view.GlobalRecent) != WebSessionQuickInputRecentLimit || len(view.ProjectRecent) != WebSessionQuickInputRecentLimit {
		t.Fatalf("history lengths = global %d, project %d", len(view.GlobalRecent), len(view.ProjectRecent))
	}
	if view.GlobalRecent[0] != "prompt 10" || view.ProjectRecent[0] != "prompt 10" {
		t.Fatalf("duplicate prompt was not moved to the front: %#v / %#v", view.GlobalRecent, view.ProjectRecent)
	}
	if containsString(view.GlobalRecent, "prompt 01") || containsString(view.ProjectRecent, "prompt 01") {
		t.Fatal("history was not capped at the newest thirty unique prompts")
	}

	projectBefore := append([]string(nil), view.ProjectRecent...)
	globalOnly, err := store.RecordQuickInput("global only", "")
	if err != nil {
		t.Fatalf("record global prompt: %v", err)
	}
	if globalOnly.GlobalRecent[0] != "global only" {
		t.Fatalf("global-only prompt was not recorded: %#v", globalOnly.GlobalRecent)
	}
	projectAfter, err := store.QuickInputView("project-1")
	if err != nil {
		t.Fatalf("load project history after global prompt: %v", err)
	}
	if !reflect.DeepEqual(projectAfter.ProjectRecent, projectBefore) {
		t.Fatalf("global-only prompt changed project history: %#v", projectAfter.ProjectRecent)
	}
}

func TestConfigDatabaseReadOnlyRejectsMutations(t *testing.T) {
	prepareConfigDatabaseTest(t, []byte("ui:\n  pageTitle: Original\n"))
	config := ReadConfig()
	store, err := InitConfigDatabase(config)
	if err != nil {
		t.Fatalf("InitConfigDatabase() error = %v", err)
	}
	path := store.path
	if err := store.Close(); err != nil {
		t.Fatalf("close writable store: %v", err)
	}

	readOnlyStore, err := openConfigDatabase(path, config.DBLogLevel, true)
	if err != nil {
		t.Fatalf("open read-only store: %v", err)
	}
	if err := readOnlyStore.initializeReadOnly(config); err != nil {
		_ = readOnlyStore.Close()
		t.Fatalf("initialize read-only store: %v", err)
	}
	if err := readOnlyStore.activate(config); err != nil {
		_ = readOnlyStore.Close()
		t.Fatalf("activate read-only store: %v", err)
	}

	if err := UpdateConfig(config, func(next *AppConfig) {
		next.UI.PageTitle = "Changed"
	}); !errors.Is(err, ErrConfigStoreReadOnly) {
		t.Fatalf("UpdateConfig() error = %v, want %v", err, ErrConfigStoreReadOnly)
	}
	if config.UI.PageTitle != "Original" {
		t.Fatalf("failed read-only update changed in-memory config to %q", config.UI.PageTitle)
	}
	for name, err := range map[string]error{
		"record":    func() error { _, err := readOnlyStore.RecordQuickInput("prompt", "project-1"); return err }(),
		"pinned":    func() error { _, err := readOnlyStore.UpdateQuickInputPinned([]string{"Plan"}); return err }(),
		"replace":   readOnlyStore.ReplaceQuickInput(WebSessionQuickInputConfig{Recent: []string{"prompt"}}),
		"delete":    readOnlyStore.DeleteProjectQuickInput("project-1"),
		"reconcile": readOnlyStore.ReconcileProjectQuickInput([]string{"project-1"}),
	} {
		if !errors.Is(err, ErrConfigStoreReadOnly) {
			t.Errorf("%s error = %v, want %v", name, err, ErrConfigStoreReadOnly)
		}
	}
	if _, err := readOnlyStore.QuickInputView(""); err != nil {
		t.Fatalf("read-only QuickInputView() error = %v", err)
	}
	health := readOnlyStore.Health(t.Context())
	if health.Mode != "read_only" || health.WriteProbe.OK || health.WriteProbe.Error != ErrConfigStoreReadOnly.Error() {
		t.Fatalf("unexpected read-only health: %#v", health)
	}
}

func TestConfigDatabaseDeletesOrphanedProjectHistory(t *testing.T) {
	prepareConfigDatabaseTest(t, []byte("ui: {}\n"))
	config := ReadConfig()
	store, err := InitConfigDatabase(config)
	if err != nil {
		t.Fatalf("InitConfigDatabase() error = %v", err)
	}
	if _, err := store.RecordQuickInput("project one", "project-1"); err != nil {
		t.Fatalf("record project-1 prompt: %v", err)
	}
	if _, err := store.RecordQuickInput("project two", "project-2"); err != nil {
		t.Fatalf("record project-2 prompt: %v", err)
	}

	if err := store.DeleteProjectQuickInput("project-1"); err != nil {
		t.Fatalf("DeleteProjectQuickInput() error = %v", err)
	}
	deletedView, err := store.QuickInputView("project-1")
	if err != nil {
		t.Fatalf("load deleted project history: %v", err)
	}
	if len(deletedView.ProjectRecent) != 0 {
		t.Fatalf("deleted project history = %#v, want empty", deletedView.ProjectRecent)
	}
	if !containsString(deletedView.GlobalRecent, "project one") {
		t.Fatalf("project deletion removed global history: %#v", deletedView.GlobalRecent)
	}

	if err := store.ReconcileProjectQuickInput([]string{"project-1"}); err != nil {
		t.Fatalf("ReconcileProjectQuickInput() error = %v", err)
	}
	orphanView, err := store.QuickInputView("project-2")
	if err != nil {
		t.Fatalf("load reconciled project history: %v", err)
	}
	if len(orphanView.ProjectRecent) != 0 {
		t.Fatalf("orphaned project history = %#v, want empty", orphanView.ProjectRecent)
	}
}

func TestConfigDatabaseMarkerRequiresCompleteDatabase(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		prepareConfigDatabaseTest(t, []byte("configStoreVersion: 1\n"))
		config := ReadConfig()
		if _, err := InitConfigDatabase(config); err == nil || !strings.Contains(err.Error(), "required by configStoreVersion") {
			t.Fatalf("InitConfigDatabase() error = %v, want missing database error", err)
		}
	})

	t.Run("incomplete", func(t *testing.T) {
		prepareConfigDatabaseTest(t, []byte("configStoreVersion: 1\n"))
		if err := os.MkdirAll(filepath.Join("data"), 0o755); err != nil {
			t.Fatalf("create data directory: %v", err)
		}
		path, err := filepath.Abs(filepath.Join("data", configDatabaseFileName))
		if err != nil {
			t.Fatalf("resolve database path: %v", err)
		}
		partial, err := openConfigDatabase(path, 0, false)
		if err != nil {
			t.Fatalf("open partial database: %v", err)
		}
		if err := partial.db.AutoMigrate(&configMetaRow{}); err != nil {
			t.Fatalf("create partial schema: %v", err)
		}
		if err := upsertConfigMeta(partial.db, configMetaSchemaVersion, "1"); err != nil {
			t.Fatalf("write schema version: %v", err)
		}
		if err := upsertConfigMeta(partial.db, configMetaInitialized, "true"); err != nil {
			t.Fatalf("write initialized marker: %v", err)
		}
		if err := partial.Close(); err != nil {
			t.Fatalf("close partial database: %v", err)
		}

		config := ReadConfig()
		if _, err := InitConfigDatabase(config); err == nil || !strings.Contains(err.Error(), "required config database table") {
			t.Fatalf("InitConfigDatabase() error = %v, want incomplete schema error", err)
		}
	})

	t.Run("corrupt", func(t *testing.T) {
		prepareConfigDatabaseTest(t, []byte("configStoreVersion: 1\n"))
		if err := os.MkdirAll(filepath.Join("data"), 0o755); err != nil {
			t.Fatalf("create data directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join("data", configDatabaseFileName), []byte("not a sqlite database"), 0o600); err != nil {
			t.Fatalf("write corrupt database: %v", err)
		}
		config := ReadConfig()
		if _, err := InitConfigDatabase(config); err == nil {
			t.Fatal("InitConfigDatabase() succeeded with a corrupt database")
		}
	})
}

func prepareConfigDatabaseTest(t *testing.T, content []byte) string {
	t.Helper()
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd(): %v", err)
	}
	oldConfigStore := configStore
	oldActiveConfigPath := activeConfigPath
	oldActiveConfigExisted := activeConfigExisted
	oldUseHomeData := useHomeData
	oldRuntimeConfigDB := runtimeConfigDB
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir(%q): %v", tempDir, err)
	}
	configStore = koanf.New(".")
	activeConfigPath = ""
	activeConfigExisted = false
	useHomeData = false
	runtimeConfigDB = nil

	t.Cleanup(func() {
		if runtimeConfigDB != nil && runtimeConfigDB != oldRuntimeConfigDB {
			_ = runtimeConfigDB.Close()
		}
		runtimeConfigDB = oldRuntimeConfigDB
		configStore = oldConfigStore
		activeConfigPath = oldActiveConfigPath
		activeConfigExisted = oldActiveConfigExisted
		useHomeData = oldUseHomeData
		_ = os.Chdir(oldWD)
	})
	return configPath
}

func assertBootstrapOnlyConfig(t *testing.T, configPath string) {
	t.Helper()
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read bootstrap config: %v", err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("parse bootstrap config: %v", err)
	}
	if parsed["configStoreVersion"] != CurrentConfigStoreVersion {
		t.Fatalf("configStoreVersion = %#v, want %d", parsed["configStoreVersion"], CurrentConfigStoreVersion)
	}
	for _, key := range []string{"developer", "git", "ui", "worktree"} {
		if _, ok := parsed[key]; ok {
			t.Fatalf("dynamic section %q remained in bootstrap config:\n%s", key, content)
		}
	}
	auth, _ := parsed["auth"].(map[string]any)
	for _, key := range []string{"frontendSalt", "passwordHash", "tokenSecret", "accessRules", "proxyHeader", "trustedProxies"} {
		if _, ok := auth[key]; ok {
			t.Fatalf("dynamic auth setting %q remained in bootstrap config:\n%s", key, content)
		}
	}
	terminal, _ := parsed["terminal"].(map[string]any)
	if _, ok := terminal["shell"]; ok {
		t.Fatalf("dynamic terminal shell remained in bootstrap config:\n%s", content)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
