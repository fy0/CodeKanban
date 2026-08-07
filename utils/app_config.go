package utils

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/samber/lo"
)

type AttachmentConfig struct {
	UseS3     bool   `json:"useS3" yaml:"useS3"`
	Endpoint  string `json:"endpoint" yaml:"endpoint"`
	Bucket    string `json:"bucket" yaml:"bucket"`
	AccessKey string `json:"accessKey" yaml:"accessKey"`
	SecretKey string `json:"secretKey" yaml:"secretKey"`
	Token     string `json:"token" yaml:"token"`
}

type AuthConfig struct {
	FrontendSalt   string                `json:"frontendSalt" yaml:"frontendSalt"`
	PasswordHash   string                `json:"passwordHash" yaml:"passwordHash"`
	TokenSecret    string                `json:"tokenSecret" yaml:"tokenSecret"`
	SessionTTL     string                `json:"sessionTTL" yaml:"sessionTTL"`
	AccessRules    AuthAccessRulesConfig `json:"accessRules" yaml:"accessRules"`
	ProxyHeader    string                `json:"proxyHeader" yaml:"proxyHeader"`
	TrustedProxies []string              `json:"trustedProxies" yaml:"trustedProxies"`

	sessionDuration time.Duration
}

// SessionDuration parses the configured auth session TTL and falls back to 30 days on errors.
func (c *AuthConfig) SessionDuration() time.Duration {
	if c == nil {
		return 0
	}
	if c.sessionDuration != 0 {
		return c.sessionDuration
	}
	if c.SessionTTL == "" {
		c.sessionDuration = 30 * 24 * time.Hour
		return c.sessionDuration
	}
	dur, err := time.ParseDuration(c.SessionTTL)
	if err != nil {
		c.sessionDuration = 30 * 24 * time.Hour
		return c.sessionDuration
	}
	c.sessionDuration = dur
	return c.sessionDuration
}

type TerminalShellConfig struct {
	Windows string `json:"windows" yaml:"windows"`
	Linux   string `json:"linux" yaml:"linux"`
	Darwin  string `json:"darwin" yaml:"darwin"`
}

type DeveloperConfig struct {
	EnableTerminalScrollback              bool                              `json:"enableTerminalScrollback" yaml:"enableTerminalScrollback"`
	EnableTerminalStateSnapshot           bool                              `json:"enableTerminalStateSnapshot" yaml:"enableTerminalStateSnapshot"`
	WebSessionCodexDefaultModel           string                            `json:"webSessionCodexDefaultModel" yaml:"webSessionCodexDefaultModel"`
	WebSessionCodexDefaultReasoningEffort string                            `json:"webSessionCodexDefaultReasoningEffort" yaml:"webSessionCodexDefaultReasoningEffort"`
	WebSessionCodexDefaultPermissionLevel string                            `json:"webSessionCodexDefaultPermissionLevel" yaml:"webSessionCodexDefaultPermissionLevel"`
	WebSessionCodexDefaultSyncMode        string                            `json:"webSessionCodexDefaultSyncMode" yaml:"webSessionCodexDefaultSyncMode"`
	WebSessionActiveCallTimeout           WebSessionActiveCallTimeoutConfig `json:"webSessionActiveCallTimeout" yaml:"webSessionActiveCallTimeout"`
}

type SettingMode string

const (
	SettingModeDefault SettingMode = "default"
	SettingModeOn      SettingMode = "on"
	SettingModeOff     SettingMode = "off"
)

type WebSessionActiveCallTimeoutMode string

const (
	WebSessionActiveCallTimeoutModeDefault WebSessionActiveCallTimeoutMode = "default"
	WebSessionActiveCallTimeoutModeCustom  WebSessionActiveCallTimeoutMode = "custom"
)

type WebSessionActiveCallTimeoutKindsConfig struct {
	UseDefault bool `json:"useDefault" yaml:"useDefault"`
	MCP        bool `json:"mcp" yaml:"mcp"`
	Command    bool `json:"command" yaml:"command"`
	Tool       bool `json:"tool" yaml:"tool"`
}

type WebSessionActiveCallTimeoutConfig struct {
	EnabledMode          SettingMode                            `json:"enabledMode" yaml:"enabledMode"`
	TimeoutMode          WebSessionActiveCallTimeoutMode        `json:"timeoutMode" yaml:"timeoutMode"`
	CustomTimeoutSeconds int                                    `json:"customTimeoutSeconds" yaml:"customTimeoutSeconds"`
	PromptTemplate       string                                 `json:"promptTemplate" yaml:"promptTemplate"`
	CallKinds            WebSessionActiveCallTimeoutKindsConfig `json:"callKinds" yaml:"callKinds"`
}

type WebSessionQuickInputConfig struct {
	Pinned []string `json:"pinned" yaml:"pinned"`
	Recent []string `json:"recent" yaml:"recent"`
}

type UIConfig struct {
	PageTitle            string                     `json:"pageTitle" yaml:"pageTitle"`
	DailyTipEnabled      bool                       `json:"dailyTipEnabled" yaml:"dailyTipEnabled"`
	WebSessionQuickInput WebSessionQuickInputConfig `json:"webSessionQuickInput" yaml:"webSessionQuickInput"`
}

const (
	DefaultPageTitle                      = "Code Kanban"
	MaxPageTitleRunes                     = 64
	WebSessionQuickInputRecentLimit       = 6
	DefaultWebSessionCodexModel           = "gpt-5.6-sol"
	DefaultWebSessionCodexReasoningEffort = "xhigh"
	DefaultWebSessionCodexPermissionLevel = "elevated"
	DefaultWebSessionCodexSyncMode        = "fast"
	WebSessionCodexDefaultSetting         = "default"
	WebSessionCodexModelDefaultEffort     = "model_default"
	WebSessionCodexStandardPermission     = "standard"
)

var defaultWebSessionQuickInputConfig = WebSessionQuickInputConfig{
	Pinned: []string{"continue"},
	Recent: []string{},
}

const (
	DefaultWebSessionActiveCallTimeoutSeconds = 120
	minWebSessionActiveCallTimeoutSeconds     = 10
	DefaultWebSessionActiveCallTimeoutPrompt  = "The current ${call} call has been running for ${duration} and may be stuck. It was interrupted automatically. Continue."
)

var defaultWebSessionActiveCallTimeoutCallKindsConfig = WebSessionActiveCallTimeoutKindsConfig{
	UseDefault: true,
	MCP:        true,
	Command:    false,
	Tool:       true,
}

var defaultWebSessionActiveCallTimeoutConfig = WebSessionActiveCallTimeoutConfig{
	EnabledMode:          SettingModeDefault,
	TimeoutMode:          WebSessionActiveCallTimeoutModeDefault,
	CustomTimeoutSeconds: DefaultWebSessionActiveCallTimeoutSeconds,
	PromptTemplate:       DefaultWebSessionActiveCallTimeoutPrompt,
	CallKinds:            defaultWebSessionActiveCallTimeoutCallKindsConfig,
}

// WorktreeConfig Worktree 全局配置。
type WorktreeConfig struct {
	GlobalBaseDir        string `json:"globalBaseDir" yaml:"globalBaseDir"`               // 全局 Worktree 基础目录
	GlobalDirNamePattern string `json:"globalDirNamePattern" yaml:"globalDirNamePattern"` // 全局目录命名模式（支持 {projectName}、{branch}）
}

const (
	GitEngineAuto    = "auto"
	GitEngineBuiltin = "builtin"
	GitEngineSystem  = "system"
)

type GitConfig struct {
	ReadEngine  string `json:"readEngine" yaml:"readEngine" enum:"auto,builtin,system"`
	WriteEngine string `json:"writeEngine" yaml:"writeEngine" enum:"auto,builtin,system"`
	Executable  string `json:"executable" yaml:"executable"`
}

func NormalizeGitConfig(config GitConfig) GitConfig {
	normalizeEngine := func(value string) string {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case GitEngineBuiltin:
			return GitEngineBuiltin
		case GitEngineSystem:
			return GitEngineSystem
		default:
			return GitEngineAuto
		}
	}
	return GitConfig{
		ReadEngine:  normalizeEngine(config.ReadEngine),
		WriteEngine: normalizeEngine(config.WriteEngine),
		Executable:  strings.TrimSpace(config.Executable),
	}
}

type TerminalConfig struct {
	Shell                 TerminalShellConfig `json:"shell" yaml:"shell"`
	IdleTimeout           string              `json:"idleTimeout" yaml:"idleTimeout"`
	MaxSessionsPerProject int                 `json:"maxSessionsPerProject" yaml:"maxSessionsPerProject"`
	AllowedRoots          []string            `json:"allowedRoots" yaml:"allowedRoots"`
	Encoding              string              `json:"encoding" yaml:"encoding"`
	ScrollbackBytes       int                 `json:"scrollbackBytes" yaml:"scrollbackBytes"`

	idleDuration time.Duration
}

// IdleDuration parses the configured timeout string and falls back to 10 minutes on errors.
func (c *TerminalConfig) IdleDuration() time.Duration {
	if c == nil {
		return 0
	}
	if c.idleDuration != 0 {
		return c.idleDuration
	}
	if c.IdleTimeout == "" {
		c.idleDuration = 10 * time.Minute
		return c.idleDuration
	}
	dur, err := time.ParseDuration(c.IdleTimeout)
	if err != nil {
		c.idleDuration = 10 * time.Minute
		return c.idleDuration
	}
	c.idleDuration = dur
	return c.idleDuration
}

type AppConfig struct {
	ServeAt                string           `json:"serveAt" yaml:"serveAt"`
	Domain                 string           `json:"domain" yaml:"domain"`
	RegisterOpen           bool             `json:"registerOpen" yaml:"registerOpen"`
	WebUrl                 string           `json:"webUrl" yaml:"webUrl"`
	AttachmentSizeLimit    int64            `json:"attachmentSizeLimit" yaml:"attachmentSizeLimit"`
	ImageCompress          bool             `json:"imageCompress" yaml:"imageCompress"`
	LogFile                string           `json:"logFile" yaml:"logFile"`
	LogLevel               string           `json:"logLevel" yaml:"logLevel"`
	DBLogLevel             int              `json:"dbLogLevel" yaml:"dbLogLevel"`
	CorsAllowOrigins       string           `json:"corsAllowOrigins" yaml:"corsAllowOrigins"`
	UIOverwrite            string           `json:"uiOverwrite" yaml:"uiOverwrite"`
	AutoMigrate            bool             `json:"autoMigrate" yaml:"autoMigrate"`
	OpenAPIEnabled         bool             `json:"openapiEnabled" yaml:"openapiEnabled"`
	DocsPath               string           `json:"docsPath" yaml:"docsPath"`
	APITitle               string           `json:"apiTitle" yaml:"apiTitle"`
	APIVersion             string           `json:"apiVersion" yaml:"apiVersion"`
	AttachmentConfig       AttachmentConfig `json:"attachmentConfig" yaml:"attachmentConfig"`
	DSN                    string           `json:"dbUrl" yaml:"dbUrl"`
	PrintConfig            bool             `json:"printConfig" yaml:"printConfig"`
	DisableAutoOpenBrowser bool             `json:"disableAutoOpenBrowser" yaml:"disableAutoOpenBrowser"`
	Auth                   AuthConfig       `json:"auth" yaml:"auth"`
	Terminal               TerminalConfig   `json:"terminal" yaml:"terminal"`
	Developer              DeveloperConfig  `json:"developer" yaml:"developer"`
	UI                     UIConfig         `json:"ui" yaml:"ui"`
	Worktree               WorktreeConfig   `json:"worktree" yaml:"worktree"`
	Git                    GitConfig        `json:"git" yaml:"git"`
}

var configStore = koanf.New(".")

// configMu 保护对 configStore 和 activeConfigPath 的并发访问
var configMu sync.RWMutex

// activeConfigPath 存储实际加载的配置文件路径
var activeConfigPath string

// ReadConfig 会加载 config.yaml，若不存在则写入默认配置。
func ReadConfig() *AppConfig {
	// 获取数据目录（npm 全局安装时使用 ~/.codekanban，否则使用 ./data）
	dataDir := GetDataDir()

	workDirConfig := "config.yaml"
	dataDirConfig := fmt.Sprintf("%s/config.yaml", dataDir)

	configPath := dataDirConfig
	if _, err := os.Stat(workDirConfig); err == nil {
		configPath = workDirConfig
	}

	// 打印工作目录信息
	if cwd, err := os.Getwd(); err == nil {
		fmt.Printf("Working directory: %s\n", cwd)
	}
	fmt.Printf("Data directory: %s\n", dataDir)
	fmt.Printf("Config file: %s\n", configPath)
	fmt.Println()

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		fmt.Printf("Failed to create data directory: %v\n", err)
	}

	defaults := AppConfig{
		ServeAt:             ":3007",
		Domain:              "127.0.0.1:3007",
		RegisterOpen:        true,
		WebUrl:              "/",
		AttachmentSizeLimit: 8192,
		ImageCompress:       true,
		LogFile:             fmt.Sprintf("%s/service.log", dataDir),
		LogLevel:            string(LogLevelInfo),
		CorsAllowOrigins:    "*",
		AutoMigrate:         true,
		OpenAPIEnabled:      true,
		DocsPath:            "/docs",
		APITitle:            "Code Kanban",
		APIVersion:          "1.0.0",
		AttachmentConfig: AttachmentConfig{
			UseS3: false,
		},
		DSN:                    fmt.Sprintf("%s/data.db", dataDir),
		PrintConfig:            false,
		DisableAutoOpenBrowser: false,
		Auth: AuthConfig{
			SessionTTL:     "720h",
			AccessRules:    DefaultAuthAccessConfig().AccessRules,
			ProxyHeader:    DefaultAuthProxyHeader,
			TrustedProxies: []string{},
		},
		Terminal: TerminalConfig{
			Shell: TerminalShellConfig{
				Windows: "pwsh.exe -NoLogo",
				Linux:   "/bin/bash",
				Darwin:  "/bin/zsh",
			},
			IdleTimeout:           "0s",
			MaxSessionsPerProject: 12,
			AllowedRoots:          []string{},
			Encoding:              "utf-8",
			ScrollbackBytes:       262144,
		},
		Developer: DeveloperConfig{
			EnableTerminalScrollback:              false,
			EnableTerminalStateSnapshot:           runtime.GOOS != "windows",
			WebSessionCodexDefaultModel:           WebSessionCodexDefaultSetting,
			WebSessionCodexDefaultReasoningEffort: WebSessionCodexDefaultSetting,
			WebSessionCodexDefaultPermissionLevel: WebSessionCodexDefaultSetting,
			WebSessionCodexDefaultSyncMode:        WebSessionCodexDefaultSetting,
			WebSessionActiveCallTimeout:           NormalizeWebSessionActiveCallTimeoutConfig(defaultWebSessionActiveCallTimeoutConfig),
		},
		UI: UIConfig{
			PageTitle:            DefaultPageTitle,
			DailyTipEnabled:      false,
			WebSessionQuickInput: NormalizeWebSessionQuickInputConfig(defaultWebSessionQuickInputConfig),
		},
		Worktree: WorktreeConfig{
			GlobalBaseDir:        "",
			GlobalDirNamePattern: "{projectName}-{branch}",
		},
		Git: NormalizeGitConfig(GitConfig{}),
	}

	lo.Must0(configStore.Load(structs.Provider(&defaults, "yaml"), nil))

	// 存储活动配置路径以供后续 WriteConfig 使用
	activeConfigPath = configPath

	fileConfigStore := koanf.New(".")
	provider := file.Provider(configPath)
	if err := configStore.Load(provider, yaml.Parser()); err != nil {
		fmt.Printf("Failed to read config: %v\n", err)
		if os.IsNotExist(err) {
			if writeErr := WriteConfigToPath(&defaults, configPath); writeErr != nil {
				fmt.Printf("Failed to write default config: %v\n", writeErr)
			}
		} else {
			os.Exit(1)
		}
	} else {
		lo.Must0(fileConfigStore.Load(provider, yaml.Parser()))
	}

	config := defaults
	if err := configStore.Unmarshal("", &config); err != nil {
		fmt.Printf("Failed to parse config: %v\n", err)
		os.Exit(1)
	}

	// 规范化派生值，避免重复计算
	config.Auth = SanitizeAuthConfig(config.Auth)
	_ = config.Auth.SessionDuration()
	_ = config.Terminal.IdleDuration()
	pageTitle, err := NormalizePageTitle(config.UI.PageTitle)
	if err != nil {
		fmt.Printf("Invalid ui.pageTitle, falling back to %q: %v\n", DefaultPageTitle, err)
		pageTitle = DefaultPageTitle
	}
	config.UI.PageTitle = pageTitle
	config.UI.WebSessionQuickInput = NormalizeWebSessionQuickInputConfig(config.UI.WebSessionQuickInput)
	config.Developer = NormalizeDeveloperConfig(config.Developer)
	config.Git = NormalizeGitConfig(config.Git)
	if webSessionActiveCallTimeoutConfigNeedsRewrite(fileConfigStore) {
		if writeErr := WriteConfigToPath(&config, configPath); writeErr != nil {
			fmt.Printf("Failed to rewrite migrated config: %v\n", writeErr)
		}
	}

	if config.PrintConfig {
		configStore.Print()
	}

	return &config
}

func NormalizePageTitle(value string) (string, error) {
	for _, char := range value {
		if unicode.IsControl(char) {
			return "", fmt.Errorf("page title cannot contain control characters")
		}
	}
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return DefaultPageTitle, nil
	}
	if utf8.RuneCountInString(normalized) > MaxPageTitleRunes {
		return "", fmt.Errorf("page title cannot exceed %d characters", MaxPageTitleRunes)
	}
	return normalized, nil
}

func NormalizeWebSessionQuickInputConfig(config WebSessionQuickInputConfig) WebSessionQuickInputConfig {
	return WebSessionQuickInputConfig{
		Pinned: normalizeWebSessionQuickInputItems(config.Pinned, 0),
		Recent: normalizeWebSessionQuickInputItems(config.Recent, WebSessionQuickInputRecentLimit),
	}
}

func NormalizeDeveloperConfig(config DeveloperConfig) DeveloperConfig {
	config.WebSessionCodexDefaultModel = strings.TrimSpace(config.WebSessionCodexDefaultModel)
	if config.WebSessionCodexDefaultModel == "" ||
		strings.EqualFold(config.WebSessionCodexDefaultModel, WebSessionCodexDefaultSetting) {
		config.WebSessionCodexDefaultModel = WebSessionCodexDefaultSetting
	}
	config.WebSessionCodexDefaultReasoningEffort = normalizeWebSessionCodexReasoningEffort(
		config.WebSessionCodexDefaultReasoningEffort,
	)
	config.WebSessionCodexDefaultPermissionLevel = normalizeWebSessionCodexPermissionLevel(
		config.WebSessionCodexDefaultPermissionLevel,
	)
	switch strings.ToLower(strings.TrimSpace(config.WebSessionCodexDefaultSyncMode)) {
	case WebSessionCodexDefaultSetting:
		config.WebSessionCodexDefaultSyncMode = WebSessionCodexDefaultSetting
	case "deep":
		config.WebSessionCodexDefaultSyncMode = "deep"
	case "fast":
		config.WebSessionCodexDefaultSyncMode = "fast"
	default:
		config.WebSessionCodexDefaultSyncMode = WebSessionCodexDefaultSetting
	}
	config.WebSessionActiveCallTimeout = NormalizeWebSessionActiveCallTimeoutConfig(config.WebSessionActiveCallTimeout)
	return config
}

func MergeDeveloperConfig(current DeveloperConfig, incoming DeveloperConfig) DeveloperConfig {
	if strings.TrimSpace(incoming.WebSessionCodexDefaultModel) == "" {
		incoming.WebSessionCodexDefaultModel = current.WebSessionCodexDefaultModel
	}
	if strings.TrimSpace(incoming.WebSessionCodexDefaultReasoningEffort) == "" {
		incoming.WebSessionCodexDefaultReasoningEffort = current.WebSessionCodexDefaultReasoningEffort
	}
	if strings.TrimSpace(incoming.WebSessionCodexDefaultPermissionLevel) == "" {
		incoming.WebSessionCodexDefaultPermissionLevel = current.WebSessionCodexDefaultPermissionLevel
	}
	if strings.TrimSpace(incoming.WebSessionCodexDefaultSyncMode) == "" {
		incoming.WebSessionCodexDefaultSyncMode = current.WebSessionCodexDefaultSyncMode
	}
	if incoming.WebSessionActiveCallTimeout == (WebSessionActiveCallTimeoutConfig{}) {
		incoming.WebSessionActiveCallTimeout = current.WebSessionActiveCallTimeout
	}
	return NormalizeDeveloperConfig(incoming)
}

func normalizeWebSessionCodexReasoningEffort(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case WebSessionCodexDefaultSetting,
		WebSessionCodexModelDefaultEffort,
		"none",
		"minimal",
		"low",
		"medium",
		"high",
		"xhigh",
		"max",
		"ultra":
		return normalized
	default:
		return WebSessionCodexDefaultSetting
	}
}

func normalizeWebSessionCodexPermissionLevel(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case WebSessionCodexDefaultSetting, WebSessionCodexStandardPermission, "elevated", "yolo":
		return normalized
	default:
		return WebSessionCodexDefaultSetting
	}
}

func NormalizeWebSessionActiveCallTimeoutConfig(config WebSessionActiveCallTimeoutConfig) WebSessionActiveCallTimeoutConfig {
	normalized := defaultWebSessionActiveCallTimeoutConfig
	normalized.EnabledMode = normalizeSettingMode(config.EnabledMode)
	normalized.TimeoutMode = normalizeWebSessionActiveCallTimeoutMode(config.TimeoutMode)
	if config.CustomTimeoutSeconds != 0 {
		normalized.CustomTimeoutSeconds = clampWebSessionActiveCallTimeoutSeconds(config.CustomTimeoutSeconds)
	}
	if strings.TrimSpace(config.PromptTemplate) != "" {
		normalized.PromptTemplate = strings.TrimSpace(config.PromptTemplate)
	}
	if config.CallKinds != (WebSessionActiveCallTimeoutKindsConfig{}) {
		normalized.CallKinds = normalizeWebSessionActiveCallTimeoutKindsConfig(config.CallKinds)
	}
	return normalized
}

func normalizeWebSessionActiveCallTimeoutKindsConfig(
	config WebSessionActiveCallTimeoutKindsConfig,
) WebSessionActiveCallTimeoutKindsConfig {
	if config.UseDefault {
		return defaultWebSessionActiveCallTimeoutCallKindsConfig
	}
	return WebSessionActiveCallTimeoutKindsConfig{
		UseDefault: false,
		MCP:        config.MCP,
		Command:    config.Command,
		Tool:       config.Tool,
	}
}

func normalizeWebSessionActiveCallTimeoutMode(
	value WebSessionActiveCallTimeoutMode,
) WebSessionActiveCallTimeoutMode {
	switch strings.ToLower(strings.TrimSpace(string(value))) {
	case string(WebSessionActiveCallTimeoutModeCustom):
		return WebSessionActiveCallTimeoutModeCustom
	default:
		return WebSessionActiveCallTimeoutModeDefault
	}
}

func normalizeSettingMode(value SettingMode) SettingMode {
	switch strings.ToLower(strings.TrimSpace(string(value))) {
	case string(SettingModeOn):
		return SettingModeOn
	case string(SettingModeOff):
		return SettingModeOff
	default:
		return SettingModeDefault
	}
}

func clampWebSessionActiveCallTimeoutSeconds(value int) int {
	if value <= 0 {
		return DefaultWebSessionActiveCallTimeoutSeconds
	}
	if value < minWebSessionActiveCallTimeoutSeconds {
		return minWebSessionActiveCallTimeoutSeconds
	}
	return value
}

func effectiveWebSessionActiveCallTimeoutSeconds(config WebSessionActiveCallTimeoutConfig) int {
	normalized := NormalizeWebSessionActiveCallTimeoutConfig(config)
	if normalized.TimeoutMode == WebSessionActiveCallTimeoutModeCustom {
		return normalized.CustomTimeoutSeconds
	}
	return DefaultWebSessionActiveCallTimeoutSeconds
}

func webSessionActiveCallTimeoutConfigNeedsRewrite(store *koanf.Koanf) bool {
	if store == nil {
		return false
	}
	if store.Get("developer.webSessionActiveCallTimeout") == nil {
		return false
	}
	if store.Get("developer.webSessionActiveCallTimeout.timeoutSeconds") != nil {
		return true
	}
	if store.Get("developer.webSessionActiveCallTimeout.timeoutMode") == nil {
		return true
	}
	if !boolValueOrDefault(store.Get("developer.webSessionActiveCallTimeout.callKinds.useDefault"), false) {
		return false
	}
	if boolValueOrDefault(
		store.Get("developer.webSessionActiveCallTimeout.callKinds.mcp"),
		defaultWebSessionActiveCallTimeoutCallKindsConfig.MCP,
	) != defaultWebSessionActiveCallTimeoutCallKindsConfig.MCP {
		return true
	}
	if boolValueOrDefault(
		store.Get("developer.webSessionActiveCallTimeout.callKinds.command"),
		defaultWebSessionActiveCallTimeoutCallKindsConfig.Command,
	) != defaultWebSessionActiveCallTimeoutCallKindsConfig.Command {
		return true
	}
	if boolValueOrDefault(
		store.Get("developer.webSessionActiveCallTimeout.callKinds.tool"),
		defaultWebSessionActiveCallTimeoutCallKindsConfig.Tool,
	) != defaultWebSessionActiveCallTimeoutCallKindsConfig.Tool {
		return true
	}
	return false
}

func boolValueOrDefault(value any, fallback bool) bool {
	typed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return typed
}

func normalizeWebSessionQuickInputItems(items []string, limit int) []string {
	if len(items) == 0 {
		return []string{}
	}

	normalized := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))

	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		normalized = append(normalized, trimmed)
		seen[trimmed] = struct{}{}
		if limit > 0 && len(normalized) >= limit {
			break
		}
	}

	return normalized
}

// WriteConfig 会将当前配置写回磁盘，写入的是启动时实际加载的配置文件路径。
// Deprecated: 推荐使用 UpdateConfig 进行原子更新，避免并发修改问题。
func WriteConfig(config *AppConfig) error {
	configMu.Lock()
	defer configMu.Unlock()

	// 使用 ReadConfig 时实际加载的配置路径
	// 确保写入与读取的是同一个文件
	if activeConfigPath == "" {
		// 如果 ReadConfig 尚未调用，回退到数据目录
		dataDir := GetDataDir()
		activeConfigPath = fmt.Sprintf("%s/config.yaml", dataDir)
	}
	return writeConfigToPathLocked(config, activeConfigPath)
}

// UpdateConfig 提供原子更新配置的能力，在锁内完成"修改+写盘"操作。
// modifier 函数接收当前配置指针，可直接修改其字段。
// 修改完成后自动持久化到磁盘。
func UpdateConfig(config *AppConfig, modifier func(*AppConfig)) error {
	configMu.Lock()
	defer configMu.Unlock()

	// 在锁内应用修改
	modifier(config)

	// 持久化到磁盘
	if activeConfigPath == "" {
		dataDir := GetDataDir()
		activeConfigPath = fmt.Sprintf("%s/config.yaml", dataDir)
	}
	return writeConfigToPathLocked(config, activeConfigPath)
}

// WriteConfigToPath 将配置写入指定路径
func WriteConfigToPath(config *AppConfig, path string) error {
	configMu.Lock()
	defer configMu.Unlock()
	return writeConfigToPathLocked(config, path)
}

// writeConfigToPathLocked 不获取锁直接写入配置（调用者必须持有锁）
func writeConfigToPathLocked(config *AppConfig, path string) error {
	store := configStore
	if config != nil {
		store = koanf.New(".")
		lo.Must0(store.Load(structs.Provider(config, "yaml"), nil))
		configStore = store
	}

	content, err := yaml.Parser().Marshal(store.Raw())
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", path, err)
	}
	return nil
}
