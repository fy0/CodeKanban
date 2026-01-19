package utils

import (
	"fmt"
	"os"
	"sync"
	"time"

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

type TerminalShellConfig struct {
	Windows string `json:"windows" yaml:"windows"`
	Linux   string `json:"linux" yaml:"linux"`
	Darwin  string `json:"darwin" yaml:"darwin"`
}

type DeveloperConfig struct {
	EnableTerminalScrollback      bool `json:"enableTerminalScrollback" yaml:"enableTerminalScrollback"`
	RenameSessionTitleEachCommand bool `json:"renameSessionTitleEachCommand" yaml:"renameSessionTitleEachCommand"`
	AutoCreateTaskOnStartWork     bool `json:"autoCreateTaskOnStartWork" yaml:"autoCreateTaskOnStartWork"`
}

// WorktreeConfig Worktree 全局配置。
type WorktreeConfig struct {
	GlobalBaseDir        string `json:"globalBaseDir" yaml:"globalBaseDir"`               // 全局 Worktree 基础目录
	GlobalDirNamePattern string `json:"globalDirNamePattern" yaml:"globalDirNamePattern"` // 全局目录命名模式（支持 {projectName}、{branch}）
}

type AIAssistantStatusConfig struct {
	ClaudeCode bool `json:"claudeCode" yaml:"claudeCode"` // 状态监测准确，默认启用
	Codex      bool `json:"codex" yaml:"codex"`           // 默认启用
	QwenCode   bool `json:"qwenCode" yaml:"qwenCode"`     // 状态监测准确，默认启用
	Gemini     bool `json:"gemini" yaml:"gemini"`         // 未充分测试，默认禁用
	Cursor     bool `json:"cursor" yaml:"cursor"`         // 未充分测试，默认禁用
	Copilot    bool `json:"copilot" yaml:"copilot"`       // 未充分测试，默认禁用
}

type TerminalConfig struct {
	Shell                 TerminalShellConfig     `json:"shell" yaml:"shell"`
	IdleTimeout           string                  `json:"idleTimeout" yaml:"idleTimeout"`
	MaxSessionsPerProject int                     `json:"maxSessionsPerProject" yaml:"maxSessionsPerProject"`
	AllowedRoots          []string                `json:"allowedRoots" yaml:"allowedRoots"`
	Encoding              string                  `json:"encoding" yaml:"encoding"`
	ScrollbackBytes       int                     `json:"scrollbackBytes" yaml:"scrollbackBytes"`
	AIAssistantStatus     AIAssistantStatusConfig `json:"aiAssistantStatus" yaml:"aiAssistantStatus"`

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

// IsEnabled 检查指定 AI 助手类型是否启用了状态监测
func (c *AIAssistantStatusConfig) IsEnabled(assistantType string) bool {
	switch assistantType {
	case "claude-code":
		return c.ClaudeCode
	case "codex":
		return c.Codex
	case "qwen-code":
		return c.QwenCode
	case "gemini":
		return c.Gemini
	case "cursor":
		return c.Cursor
	case "copilot":
		return c.Copilot
	default:
		return false // 未知类型默认禁用
	}
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
	Terminal               TerminalConfig   `json:"terminal" yaml:"terminal"`
	Developer              DeveloperConfig  `json:"developer" yaml:"developer"`
	Worktree               WorktreeConfig   `json:"worktree" yaml:"worktree"`
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
			AIAssistantStatus: AIAssistantStatusConfig{
				ClaudeCode: true,  // 状态监测准确
				Codex:      true,  // 默认启用
				QwenCode:   true,  // 状态监测准确
				Gemini:     false, // 未充分测试
				Cursor:     false, // 未充分测试
				Copilot:    false, // 未充分测试
			},
		},
		Developer: DeveloperConfig{
			EnableTerminalScrollback:      false,
			RenameSessionTitleEachCommand: false,
			AutoCreateTaskOnStartWork:     true,
		},
		Worktree: WorktreeConfig{
			GlobalBaseDir:        "",
			GlobalDirNamePattern: "{projectName}-{branch}",
		},
	}

	lo.Must0(configStore.Load(structs.Provider(&defaults, "yaml"), nil))

	// 存储活动配置路径以供后续 WriteConfig 使用
	activeConfigPath = configPath

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
	}

	config := defaults
	if err := configStore.Unmarshal("", &config); err != nil {
		fmt.Printf("Failed to parse config: %v\n", err)
		os.Exit(1)
	}

	// 规范化派生值，避免重复计算
	_ = config.Terminal.IdleDuration()

	if config.PrintConfig {
		configStore.Print()
	}

	return &config
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
	if config != nil {
		lo.Must0(configStore.Load(structs.Provider(config, "yaml"), nil))
	}

	content, err := yaml.Parser().Marshal(configStore.Raw())
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", path, err)
	}
	return nil
}
