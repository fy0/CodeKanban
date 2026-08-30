package utils

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const (
	CurrentConfigStoreVersion = 1
	configDatabaseFileName    = "config.db"
	configMetaSchemaVersion   = "schema_version"
	configMetaInitialized     = "initialized"
	configMetaWriteProbe      = "__write_probe__"

	runtimeSettingAuthCredentials = "auth.credentials"
	runtimeSettingAuthAccess      = "auth.access"
	runtimeSettingDeveloper       = "developer"
	runtimeSettingGit             = "git"
	runtimeSettingWorktree        = "worktree"
	runtimeSettingTerminalShell   = "terminal.shell"
	runtimeSettingUI              = "ui"
	runtimeSettingQuickPinned     = "quickInput.pinned"

	quickInputScopeGlobal  = "global"
	quickInputScopeProject = "project"
)

var (
	ErrConfigStoreReadOnly       = errors.New("config_store_read_only")
	ErrConfigStoreNotInitialized = errors.New("config store is not initialized")
	runtimeConfigDB              *ConfigDatabase
)

type configMetaRow struct {
	Key   string `gorm:"primaryKey;type:text"`
	Value string `gorm:"type:text;not null"`
}

func (configMetaRow) TableName() string { return "config_meta" }

type runtimeSettingRow struct {
	Key       string    `gorm:"primaryKey;type:text"`
	ValueJSON string    `gorm:"column:value_json;type:text;not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (runtimeSettingRow) TableName() string { return "runtime_settings" }

type quickInputRecentRow struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	Scope     string    `gorm:"type:text;not null;uniqueIndex:idx_quick_input_scope_prompt,priority:1;index:idx_quick_input_scope_order,priority:1"`
	ProjectID string    `gorm:"column:project_id;type:text;not null;default:'';uniqueIndex:idx_quick_input_scope_prompt,priority:2;index:idx_quick_input_scope_order,priority:2"`
	Prompt    string    `gorm:"type:text;not null;uniqueIndex:idx_quick_input_scope_prompt,priority:3"`
	CreatedAt time.Time `gorm:"not null;index:idx_quick_input_scope_order,priority:3,sort:desc"`
}

func (quickInputRecentRow) TableName() string { return "quick_input_recents" }

type runtimeAuthCredentials struct {
	FrontendSalt string `json:"frontendSalt"`
	PasswordHash string `json:"passwordHash"`
	TokenSecret  string `json:"tokenSecret"`
}

type runtimeUISettings struct {
	PageTitle       string `json:"pageTitle"`
	DailyTipEnabled bool   `json:"dailyTipEnabled"`
}

type WebSessionQuickInputView struct {
	Pinned        []string `json:"pinned"`
	GlobalRecent  []string `json:"globalRecent"`
	ProjectID     string   `json:"projectId,omitempty"`
	ProjectRecent []string `json:"projectRecent"`
}

type ConfigDatabase struct {
	db            *gorm.DB
	sqlDB         *sql.DB
	path          string
	readOnly      bool
	schemaVersion int
	config        *AppConfig
}

type ConfigDatabaseProbe struct {
	OK         bool   `json:"ok"`
	DurationMs int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

type ConfigDatabaseHealth struct {
	Path          string              `json:"path"`
	Mode          string              `json:"mode"`
	SchemaVersion int                 `json:"schemaVersion"`
	DatabaseBytes int64               `json:"databaseBytes"`
	WALBytes      int64               `json:"walBytes"`
	ReadProbe     ConfigDatabaseProbe `json:"readProbe"`
	WriteProbe    ConfigDatabaseProbe `json:"writeProbe"`
}

// InitConfigDatabase opens the instance-local config.db, imports legacy YAML
// settings on first use, and makes the database the runtime persistence target.
func InitConfigDatabase(config *AppConfig) (*ConfigDatabase, error) {
	if config == nil {
		return nil, fmt.Errorf("application config is required")
	}
	path, err := filepath.Abs(filepath.Join(GetDataDir(), configDatabaseFileName))
	if err != nil {
		return nil, fmt.Errorf("resolve config database path: %w", err)
	}
	if config.ConfigStoreVersion >= CurrentConfigStoreVersion {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("config database required by configStoreVersion is unavailable: %w", err)
		}
	}

	store, writableErr := openConfigDatabase(path, config.DBLogLevel, false)
	if writableErr == nil {
		if err := store.initializeWritable(config); err == nil {
			probe := probeConfigDatabase(context.Background(), store.sqlDB, true)
			if !probe.OK {
				writableErr = fmt.Errorf("config database write probe failed: %s", probe.Error)
				store.Close()
			} else if err := store.activate(config); err != nil {
				store.Close()
				return nil, err
			} else {
				return store, nil
			}
		} else {
			writableErr = err
			store.Close()
		}
	}

	readOnlyStore, readOnlyErr := openConfigDatabase(path, config.DBLogLevel, true)
	if readOnlyErr != nil {
		return nil, fmt.Errorf("open config database: writable=%v; read-only=%w", writableErr, readOnlyErr)
	}
	if err := readOnlyStore.initializeReadOnly(config); err != nil {
		readOnlyStore.Close()
		return nil, fmt.Errorf("open config database read-only after writable failure %v: %w", writableErr, err)
	}
	if err := readOnlyStore.activate(config); err != nil {
		readOnlyStore.Close()
		return nil, err
	}
	return readOnlyStore, nil
}

func openConfigDatabase(path string, logLevel int, readOnly bool) (*ConfigDatabase, error) {
	if !readOnly {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			file, createErr := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
			if createErr != nil && !os.IsExist(createErr) {
				return nil, createErr
			}
			if createErr == nil {
				if closeErr := file.Close(); closeErr != nil {
					return nil, closeErr
				}
			}
		} else if err != nil {
			return nil, err
		}
	}
	dsn := path
	if readOnly {
		dsn = "file:" + filepath.ToSlash(path) + "?mode=ro"
	}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		TranslateError: true,
		Logger:         logger.Default.LogMode(logger.LogLevel(logLevel)),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if result := db.Exec("PRAGMA busy_timeout = 5000"); result.Error != nil {
		_ = sqlDB.Close()
		return nil, result.Error
	}
	if readOnly {
		if result := db.Exec("PRAGMA query_only = ON"); result.Error != nil {
			_ = sqlDB.Close()
			return nil, result.Error
		}
	} else if result := db.Exec("PRAGMA journal_mode = WAL"); result.Error != nil {
		_ = sqlDB.Close()
		return nil, result.Error
	}
	return &ConfigDatabase{db: db, sqlDB: sqlDB, path: path, readOnly: readOnly}, nil
}

func (store *ConfigDatabase) initializeWritable(config *AppConfig) error {
	hasMetadata := store.db.Migrator().HasTable(&configMetaRow{})
	if config.ConfigStoreVersion >= CurrentConfigStoreVersion && !hasMetadata {
		return fmt.Errorf("config database metadata is missing but YAML migration marker is present")
	}

	initialized := false
	version := 0
	if hasMetadata {
		var err error
		initialized, err = store.metaBool(configMetaInitialized)
		if err != nil {
			return err
		}
		version, err = store.metaInt(configMetaSchemaVersion)
		if err != nil {
			return err
		}
	}
	if version > CurrentConfigStoreVersion {
		return fmt.Errorf("unsupported config database schema version %d", version)
	}
	if config.ConfigStoreVersion >= CurrentConfigStoreVersion && !initialized {
		return fmt.Errorf("config database is not initialized but YAML migration marker is present")
	}

	if initialized {
		if version != CurrentConfigStoreVersion {
			return fmt.Errorf("unsupported config database schema version %d", version)
		}
		if err := store.validateCurrentSchema(); err != nil {
			return err
		}
	} else {
		if err := store.db.AutoMigrate(&configMetaRow{}, &runtimeSettingRow{}, &quickInputRecentRow{}); err != nil {
			return err
		}
		if err := backupLegacyConfigOnce(); err != nil {
			return err
		}
		if err := ensureRuntimeAuthSecrets(config); err != nil {
			return err
		}
		if err := normalizeRuntimeConfig(config); err != nil {
			return err
		}
		if err := store.db.Transaction(func(tx *gorm.DB) error {
			if err := persistRuntimeSettingsTx(tx, config); err != nil {
				return err
			}
			if err := replaceQuickInputHistoryTx(tx, config.UI.WebSessionQuickInput); err != nil {
				return err
			}
			if err := upsertConfigMeta(tx, configMetaSchemaVersion, strconv.Itoa(CurrentConfigStoreVersion)); err != nil {
				return err
			}
			return upsertConfigMeta(tx, configMetaInitialized, "true")
		}); err != nil {
			return err
		}
		if err := store.validateCurrentSchema(); err != nil {
			return err
		}
	}
	store.schemaVersion = CurrentConfigStoreVersion
	return store.loadRuntimeConfig(config)
}

func (store *ConfigDatabase) initializeReadOnly(config *AppConfig) error {
	if err := store.validateCurrentSchema(); err != nil {
		return err
	}
	initialized, err := store.metaBool(configMetaInitialized)
	if err != nil {
		return err
	}
	version, err := store.metaInt(configMetaSchemaVersion)
	if err != nil {
		return err
	}
	if !initialized || version != CurrentConfigStoreVersion {
		return fmt.Errorf("read-only config database must be initialized with schema version %d", CurrentConfigStoreVersion)
	}
	store.schemaVersion = version
	return store.loadRuntimeConfig(config)
}

func (store *ConfigDatabase) validateCurrentSchema() error {
	tables := []struct {
		model   any
		name    string
		columns []string
	}{
		{model: &configMetaRow{}, name: "config_meta", columns: []string{"key", "value"}},
		{model: &runtimeSettingRow{}, name: "runtime_settings", columns: []string{"key", "value_json", "updated_at"}},
		{model: &quickInputRecentRow{}, name: "quick_input_recents", columns: []string{"id", "scope", "project_id", "prompt", "created_at"}},
	}
	for _, table := range tables {
		if !store.db.Migrator().HasTable(table.model) {
			return fmt.Errorf("required config database table %s is missing", table.name)
		}
		for _, column := range table.columns {
			if !store.db.Migrator().HasColumn(table.model, column) {
				return fmt.Errorf("required config database column %s.%s is missing", table.name, column)
			}
		}
	}
	for _, index := range []string{"idx_quick_input_scope_prompt", "idx_quick_input_scope_order"} {
		if !store.db.Migrator().HasIndex(&quickInputRecentRow{}, index) {
			return fmt.Errorf("required config database index %s is missing", index)
		}
	}

	rows, err := store.sqlDB.Query("PRAGMA quick_check")
	if err != nil {
		return fmt.Errorf("check config database integrity: %w", err)
	}
	defer rows.Close()
	checked := false
	for rows.Next() {
		checked = true
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("read config database integrity result: %w", err)
		}
		if result != "ok" {
			return fmt.Errorf("config database integrity check failed: %s", result)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("check config database integrity: %w", err)
	}
	if !checked {
		return fmt.Errorf("config database integrity check returned no result")
	}
	return nil
}

func (store *ConfigDatabase) activate(config *AppConfig) error {
	store.config = config
	needsBootstrapRewrite := config.ConfigStoreVersion < CurrentConfigStoreVersion
	config.ConfigStoreVersion = CurrentConfigStoreVersion
	if needsBootstrapRewrite {
		if err := backupLegacyConfigOnce(); err != nil {
			return err
		}
		if err := WriteBootstrapConfigToPath(config, activeConfigPath); err != nil {
			return err
		}
	}
	for _, path := range []string{store.path, store.path + "-wal", store.path + "-shm"} {
		_ = os.Chmod(path, 0o600)
	}
	runtimeConfigDB = store
	return nil
}

func (store *ConfigDatabase) Close() error {
	if store == nil || store.sqlDB == nil {
		return nil
	}
	if runtimeConfigDB == store {
		runtimeConfigDB = nil
	}
	return store.sqlDB.Close()
}

func CurrentConfigDatabase() *ConfigDatabase { return runtimeConfigDB }

func (store *ConfigDatabase) IsReadOnly() bool { return store == nil || store.readOnly }

func (store *ConfigDatabase) persistRuntimeSettings(config *AppConfig) error {
	if store == nil {
		return ErrConfigStoreNotInitialized
	}
	if store.readOnly {
		return ErrConfigStoreReadOnly
	}
	if err := ensureRuntimeAuthSecrets(config); err != nil {
		return err
	}
	if err := normalizeRuntimeConfig(config); err != nil {
		return err
	}
	return configStoreMutationError(store.db.Transaction(func(tx *gorm.DB) error {
		return persistRuntimeSettingsTx(tx, config)
	}))
}

func (store *ConfigDatabase) persistRuntimeSettingsAndQuickInput(config *AppConfig) error {
	if store == nil {
		return ErrConfigStoreNotInitialized
	}
	if store.readOnly {
		return ErrConfigStoreReadOnly
	}
	if err := ensureRuntimeAuthSecrets(config); err != nil {
		return err
	}
	if err := normalizeRuntimeConfig(config); err != nil {
		return err
	}
	config.UI.WebSessionQuickInput = NormalizeWebSessionQuickInputConfig(config.UI.WebSessionQuickInput)
	return configStoreMutationError(store.db.Transaction(func(tx *gorm.DB) error {
		if err := persistRuntimeSettingsTx(tx, config); err != nil {
			return err
		}
		return replaceQuickInputHistoryTx(tx, config.UI.WebSessionQuickInput)
	}))
}

func persistRuntimeSettingsTx(tx *gorm.DB, config *AppConfig) error {
	values := map[string]any{
		runtimeSettingAuthCredentials: runtimeAuthCredentials{
			FrontendSalt: config.Auth.FrontendSalt,
			PasswordHash: config.Auth.PasswordHash,
			TokenSecret:  config.Auth.TokenSecret,
		},
		runtimeSettingAuthAccess:    AuthAccessConfigFromAuthConfig(config.Auth),
		runtimeSettingDeveloper:     config.Developer,
		runtimeSettingGit:           config.Git,
		runtimeSettingWorktree:      config.Worktree,
		runtimeSettingTerminalShell: config.Terminal.Shell,
		runtimeSettingUI: runtimeUISettings{
			PageTitle:       config.UI.PageTitle,
			DailyTipEnabled: config.UI.DailyTipEnabled,
		},
		runtimeSettingQuickPinned: config.UI.WebSessionQuickInput.Pinned,
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	now := time.Now().UTC()
	for _, key := range keys {
		content, err := json.Marshal(values[key])
		if err != nil {
			return fmt.Errorf("encode runtime setting %s: %w", key, err)
		}
		row := runtimeSettingRow{Key: key, ValueJSON: string(content), UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value_json", "updated_at"}),
		}).Create(&row).Error; err != nil {
			return fmt.Errorf("persist runtime setting %s: %w", key, err)
		}
	}
	return nil
}

func (store *ConfigDatabase) loadRuntimeConfig(config *AppConfig) error {
	var rows []runtimeSettingRow
	if err := store.db.Find(&rows).Error; err != nil {
		return err
	}
	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.ValueJSON
	}
	required := []string{
		runtimeSettingAuthCredentials,
		runtimeSettingAuthAccess,
		runtimeSettingDeveloper,
		runtimeSettingGit,
		runtimeSettingWorktree,
		runtimeSettingTerminalShell,
		runtimeSettingUI,
		runtimeSettingQuickPinned,
	}
	for _, key := range required {
		if _, ok := values[key]; !ok {
			return fmt.Errorf("required runtime setting %s is missing", key)
		}
	}

	var credentials runtimeAuthCredentials
	var access AuthAccessConfig
	var developer DeveloperConfig
	var gitConfig GitConfig
	var worktree WorktreeConfig
	var shell TerminalShellConfig
	var ui runtimeUISettings
	var pinned []string
	for key, target := range map[string]any{
		runtimeSettingAuthCredentials: &credentials,
		runtimeSettingAuthAccess:      &access,
		runtimeSettingDeveloper:       &developer,
		runtimeSettingGit:             &gitConfig,
		runtimeSettingWorktree:        &worktree,
		runtimeSettingTerminalShell:   &shell,
		runtimeSettingUI:              &ui,
		runtimeSettingQuickPinned:     &pinned,
	} {
		if err := json.Unmarshal([]byte(values[key]), target); err != nil {
			return fmt.Errorf("decode runtime setting %s: %w", key, err)
		}
	}
	if strings.TrimSpace(credentials.FrontendSalt) == "" || strings.TrimSpace(credentials.TokenSecret) == "" {
		return fmt.Errorf("runtime authentication credentials are incomplete")
	}
	normalizedAccess, err := NormalizeAuthAccessConfig(access)
	if err != nil {
		return fmt.Errorf("invalid runtime auth access config: %w", err)
	}
	pageTitle, err := NormalizePageTitle(ui.PageTitle)
	if err != nil {
		return fmt.Errorf("invalid runtime page title: %w", err)
	}

	config.Auth.FrontendSalt = credentials.FrontendSalt
	config.Auth.PasswordHash = credentials.PasswordHash
	config.Auth.TokenSecret = credentials.TokenSecret
	ApplyAuthAccessConfigToAuthConfig(&config.Auth, normalizedAccess)
	config.Developer = NormalizeDeveloperConfig(developer)
	config.Git = NormalizeGitConfig(gitConfig)
	config.Worktree = WorktreeConfig{
		GlobalBaseDir:        strings.TrimSpace(worktree.GlobalBaseDir),
		GlobalDirNamePattern: strings.TrimSpace(worktree.GlobalDirNamePattern),
	}
	config.Terminal.Shell = shell
	config.UI.PageTitle = pageTitle
	config.UI.DailyTipEnabled = ui.DailyTipEnabled
	config.UI.WebSessionQuickInput.Pinned = NormalizeWebSessionQuickInputConfig(WebSessionQuickInputConfig{Pinned: pinned}).Pinned
	return store.refreshQuickInputCache(config)
}

func normalizeRuntimeConfig(config *AppConfig) error {
	config.Auth = SanitizeAuthConfig(config.Auth)
	access, err := NormalizeAuthAccessConfig(AuthAccessConfigFromAuthConfig(config.Auth))
	if err != nil {
		return err
	}
	ApplyAuthAccessConfigToAuthConfig(&config.Auth, access)
	config.Developer = NormalizeDeveloperConfig(config.Developer)
	config.Git = NormalizeGitConfig(config.Git)
	config.Worktree.GlobalBaseDir = strings.TrimSpace(config.Worktree.GlobalBaseDir)
	config.Worktree.GlobalDirNamePattern = strings.TrimSpace(config.Worktree.GlobalDirNamePattern)
	pageTitle, err := NormalizePageTitle(config.UI.PageTitle)
	if err != nil {
		return err
	}
	config.UI.PageTitle = pageTitle
	config.UI.WebSessionQuickInput.Pinned = NormalizeWebSessionQuickInputConfig(WebSessionQuickInputConfig{
		Pinned: config.UI.WebSessionQuickInput.Pinned,
	}).Pinned
	return nil
}

func ensureRuntimeAuthSecrets(config *AppConfig) error {
	if strings.TrimSpace(config.Auth.FrontendSalt) == "" {
		value, err := NewAuthFrontendSalt()
		if err != nil {
			return err
		}
		config.Auth.FrontendSalt = value
	}
	if strings.TrimSpace(config.Auth.TokenSecret) == "" {
		value, err := NewAuthTokenSecret()
		if err != nil {
			return err
		}
		config.Auth.TokenSecret = value
	}
	return nil
}

func (store *ConfigDatabase) metaBool(key string) (bool, error) {
	var row configMetaRow
	err := store.db.First(&row, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	value, err := strconv.ParseBool(row.Value)
	if err != nil {
		return false, fmt.Errorf("invalid config metadata %s: %w", key, err)
	}
	return value, nil
}

func (store *ConfigDatabase) metaInt(key string) (int, error) {
	var row configMetaRow
	err := store.db.First(&row, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(row.Value)
	if err != nil {
		return 0, fmt.Errorf("invalid config metadata %s: %w", key, err)
	}
	return value, nil
}

func upsertConfigMeta(db *gorm.DB, key, value string) error {
	row := configMetaRow{Key: key, Value: value}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&row).Error
}

func backupLegacyConfigOnce() error {
	if !activeConfigExisted || activeConfigPath == "" {
		return nil
	}
	backupPath := activeConfigPath + ".pre-config-db-v1.bak"
	if _, err := os.Stat(backupPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	content, err := os.ReadFile(activeConfigPath)
	if err != nil {
		return fmt.Errorf("read legacy config backup source: %w", err)
	}
	file, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return fmt.Errorf("create legacy config backup: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(backupPath)
		}
	}()
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}

func replaceQuickInputHistoryTx(tx *gorm.DB, config WebSessionQuickInputConfig) error {
	normalized := NormalizeWebSessionQuickInputConfig(config)
	if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&quickInputRecentRow{}).Error; err != nil {
		return err
	}
	insert := func(scope, projectID string, values []string) error {
		for index := len(values) - 1; index >= 0; index-- {
			row := quickInputRecentRow{
				Scope:     scope,
				ProjectID: projectID,
				Prompt:    values[index],
				CreatedAt: time.Now().UTC(),
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	}
	if err := insert(quickInputScopeGlobal, "", normalized.Recent); err != nil {
		return err
	}
	projectIDs := make([]string, 0, len(normalized.RecentByProject))
	for projectID := range normalized.RecentByProject {
		projectIDs = append(projectIDs, projectID)
	}
	sort.Strings(projectIDs)
	for _, projectID := range projectIDs {
		if err := insert(quickInputScopeProject, projectID, normalized.RecentByProject[projectID]); err != nil {
			return err
		}
	}
	return nil
}

func (store *ConfigDatabase) refreshQuickInputCache(config *AppConfig) error {
	if config == nil {
		return nil
	}
	var rows []quickInputRecentRow
	if err := store.db.Order("id DESC").Find(&rows).Error; err != nil {
		return err
	}
	global := make([]string, 0, WebSessionQuickInputRecentLimit)
	byProject := map[string][]string{}
	for _, row := range rows {
		switch row.Scope {
		case quickInputScopeGlobal:
			global = append(global, row.Prompt)
		case quickInputScopeProject:
			byProject[row.ProjectID] = append(byProject[row.ProjectID], row.Prompt)
		}
	}
	configMu.Lock()
	config.UI.WebSessionQuickInput.Recent = global
	config.UI.WebSessionQuickInput.RecentByProject = byProject
	configMu.Unlock()
	return nil
}

func (store *ConfigDatabase) quickInputRecent(scope, projectID string) ([]string, error) {
	var rows []quickInputRecentRow
	if err := store.db.Where("scope = ? AND project_id = ?", scope, projectID).
		Order("id DESC").Limit(WebSessionQuickInputRecentLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		values = append(values, row.Prompt)
	}
	return values, nil
}

func (store *ConfigDatabase) QuickInputView(projectID string) (WebSessionQuickInputView, error) {
	if store == nil {
		return WebSessionQuickInputView{}, ErrConfigStoreNotInitialized
	}
	projectID = strings.TrimSpace(projectID)
	global, err := store.quickInputRecent(quickInputScopeGlobal, "")
	if err != nil {
		return WebSessionQuickInputView{}, err
	}
	projectRecent := []string{}
	if projectID != "" {
		projectRecent, err = store.quickInputRecent(quickInputScopeProject, projectID)
		if err != nil {
			return WebSessionQuickInputView{}, err
		}
	}
	pinned := []string{}
	if store.config != nil {
		configMu.RLock()
		pinned = append(pinned, store.config.UI.WebSessionQuickInput.Pinned...)
		configMu.RUnlock()
	}
	return WebSessionQuickInputView{
		Pinned:        pinned,
		GlobalRecent:  global,
		ProjectID:     projectID,
		ProjectRecent: projectRecent,
	}, nil
}

func (store *ConfigDatabase) RecordQuickInput(text, projectID string) (WebSessionQuickInputView, error) {
	if store == nil {
		return WebSessionQuickInputView{}, ErrConfigStoreNotInitialized
	}
	if store.readOnly {
		return WebSessionQuickInputView{}, ErrConfigStoreReadOnly
	}
	normalized := NormalizeWebSessionQuickInputConfig(WebSessionQuickInputConfig{Recent: []string{text}}).Recent
	if len(normalized) == 0 {
		return store.QuickInputView(projectID)
	}
	projectID = strings.TrimSpace(projectID)
	if err := configStoreMutationError(store.db.Transaction(func(tx *gorm.DB) error {
		if err := touchQuickInputRecentTx(tx, quickInputScopeGlobal, "", normalized[0]); err != nil {
			return err
		}
		if projectID != "" {
			return touchQuickInputRecentTx(tx, quickInputScopeProject, projectID, normalized[0])
		}
		return nil
	})); err != nil {
		return WebSessionQuickInputView{}, err
	}
	view, err := store.QuickInputView(projectID)
	if err != nil {
		return WebSessionQuickInputView{}, err
	}
	store.applyQuickInputViewToCache(view)
	return view, nil
}

func (store *ConfigDatabase) applyQuickInputViewToCache(view WebSessionQuickInputView) {
	if store.config == nil {
		return
	}
	configMu.Lock()
	defer configMu.Unlock()
	store.config.UI.WebSessionQuickInput.Recent = append([]string(nil), view.GlobalRecent...)
	if view.ProjectID == "" {
		return
	}
	if len(view.ProjectRecent) == 0 {
		delete(store.config.UI.WebSessionQuickInput.RecentByProject, view.ProjectID)
		return
	}
	if store.config.UI.WebSessionQuickInput.RecentByProject == nil {
		store.config.UI.WebSessionQuickInput.RecentByProject = make(map[string][]string)
	}
	store.config.UI.WebSessionQuickInput.RecentByProject[view.ProjectID] = append([]string(nil), view.ProjectRecent...)
}

func touchQuickInputRecentTx(tx *gorm.DB, scope, projectID, prompt string) error {
	if err := tx.Where("scope = ? AND project_id = ? AND prompt = ?", scope, projectID, prompt).
		Delete(&quickInputRecentRow{}).Error; err != nil {
		return err
	}
	row := quickInputRecentRow{Scope: scope, ProjectID: projectID, Prompt: prompt, CreatedAt: time.Now().UTC()}
	if err := tx.Create(&row).Error; err != nil {
		return err
	}
	var staleIDs []uint64
	if err := tx.Model(&quickInputRecentRow{}).
		Where("scope = ? AND project_id = ?", scope, projectID).
		Order("id DESC").Offset(WebSessionQuickInputRecentLimit).Pluck("id", &staleIDs).Error; err != nil {
		return err
	}
	if len(staleIDs) > 0 {
		return tx.Where("id IN ?", staleIDs).Delete(&quickInputRecentRow{}).Error
	}
	return nil
}

func (store *ConfigDatabase) UpdateQuickInputPinned(items []string) (WebSessionQuickInputView, error) {
	if store == nil || store.config == nil {
		return WebSessionQuickInputView{}, ErrConfigStoreNotInitialized
	}
	if err := UpdateConfig(store.config, func(config *AppConfig) {
		config.UI.WebSessionQuickInput.Pinned = append([]string(nil), items...)
	}); err != nil {
		return WebSessionQuickInputView{}, err
	}
	return store.QuickInputView("")
}

func (store *ConfigDatabase) ReplaceQuickInput(config WebSessionQuickInputConfig) error {
	if store == nil || store.config == nil {
		return ErrConfigStoreNotInitialized
	}
	if store.readOnly {
		return ErrConfigStoreReadOnly
	}
	normalized := NormalizeWebSessionQuickInputConfig(config)
	if err := configStoreMutationError(store.db.Transaction(func(tx *gorm.DB) error {
		content, err := json.Marshal(normalized.Pinned)
		if err != nil {
			return err
		}
		row := runtimeSettingRow{Key: runtimeSettingQuickPinned, ValueJSON: string(content), UpdatedAt: time.Now().UTC()}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value_json", "updated_at"}),
		}).Create(&row).Error; err != nil {
			return err
		}
		return replaceQuickInputHistoryTx(tx, normalized)
	})); err != nil {
		return err
	}
	configMu.Lock()
	store.config.UI.WebSessionQuickInput = normalized
	configMu.Unlock()
	return nil
}

func (store *ConfigDatabase) DeleteProjectQuickInput(projectID string) error {
	if store == nil {
		return ErrConfigStoreNotInitialized
	}
	if store.readOnly {
		return ErrConfigStoreReadOnly
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	if err := configStoreMutationError(store.db.Where("scope = ? AND project_id = ?", quickInputScopeProject, projectID).
		Delete(&quickInputRecentRow{}).Error); err != nil {
		return err
	}
	if store.config != nil {
		configMu.Lock()
		delete(store.config.UI.WebSessionQuickInput.RecentByProject, projectID)
		configMu.Unlock()
	}
	return nil
}

func (store *ConfigDatabase) ReconcileProjectQuickInput(projectIDs []string) error {
	if store == nil {
		return ErrConfigStoreNotInitialized
	}
	if store.readOnly {
		return ErrConfigStoreReadOnly
	}
	valid := make([]string, 0, len(projectIDs))
	for _, projectID := range projectIDs {
		if value := strings.TrimSpace(projectID); value != "" {
			valid = append(valid, value)
		}
	}
	query := store.db.Where("scope = ?", quickInputScopeProject)
	if len(valid) > 0 {
		query = query.Where("project_id NOT IN ?", valid)
	}
	if err := configStoreMutationError(query.Delete(&quickInputRecentRow{}).Error); err != nil {
		return err
	}
	if store.config != nil {
		return store.refreshQuickInputCache(store.config)
	}
	return nil
}

func configStoreMutationError(err error) error {
	if err == nil || errors.Is(err, ErrConfigStoreReadOnly) {
		return err
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "readonly") || strings.Contains(message, "read-only") || strings.Contains(message, "read only") {
		return fmt.Errorf("%w: %v", ErrConfigStoreReadOnly, err)
	}
	return err
}

func (store *ConfigDatabase) Health(ctx context.Context) ConfigDatabaseHealth {
	health := ConfigDatabaseHealth{Path: store.path, SchemaVersion: store.schemaVersion, Mode: "read_write"}
	if store.readOnly {
		health.Mode = "read_only"
	}
	if info, err := os.Stat(store.path); err == nil {
		health.DatabaseBytes = info.Size()
	}
	if info, err := os.Stat(store.path + "-wal"); err == nil {
		health.WALBytes = info.Size()
	}
	health.ReadProbe = probeConfigDatabase(ctx, store.sqlDB, false)
	if store.readOnly {
		health.WriteProbe = ConfigDatabaseProbe{Error: ErrConfigStoreReadOnly.Error()}
	} else {
		health.WriteProbe = probeConfigDatabase(ctx, store.sqlDB, true)
	}
	return health
}

func probeConfigDatabase(ctx context.Context, db *sql.DB, write bool) ConfigDatabaseProbe {
	started := time.Now()
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	probe := ConfigDatabaseProbe{}
	if !write {
		var value int
		err := db.QueryRowContext(probeCtx, "SELECT 1").Scan(&value)
		probe.OK = err == nil && value == 1
		if err != nil {
			probe.Error = err.Error()
		}
		probe.DurationMs = time.Since(started).Milliseconds()
		return probe
	}
	tx, err := db.BeginTx(probeCtx, nil)
	if err == nil {
		_, err = tx.ExecContext(
			probeCtx,
			"INSERT INTO config_meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
			configMetaWriteProbe,
			strconv.FormatInt(time.Now().UnixNano(), 10),
		)
		rollbackErr := tx.Rollback()
		if err == nil && rollbackErr != nil {
			err = rollbackErr
		}
	}
	probe.OK = err == nil
	if err != nil {
		probe.Error = err.Error()
	}
	probe.DurationMs = time.Since(started).Milliseconds()
	return probe
}
