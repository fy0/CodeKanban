package utils

import (
	"fmt"
	"os"
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
	EnableTerminalScrollback bool `json:"enableTerminalScrollback" yaml:"enableTerminalScrollback"`
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
}

var configStore = koanf.New(".")

// activeConfigPath stores the path of the config file that was actually loaded
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
			EnableTerminalScrollback: false,
		},
	}

	lo.Must0(configStore.Load(structs.Provider(&defaults, "yaml"), nil))

	// Store the active config path for later use by WriteConfig
	activeConfigPath = configPath

	provider := file.Provider(configPath)
	if err := configStore.Load(provider, yaml.Parser()); err != nil {
		fmt.Printf("Failed to read config: %v\n", err)
		if os.IsNotExist(err) {
			WriteConfigToPath(&defaults, configPath)
		} else {
			os.Exit(1)
		}
	}

	config := defaults
	if err := configStore.Unmarshal("", &config); err != nil {
		fmt.Printf("Failed to parse config: %v\n", err)
		os.Exit(1)
	}

	// Normalize derived values to avoid redundant calculations.
	_ = config.Terminal.IdleDuration()

	if config.PrintConfig {
		configStore.Print()
	}

	return &config
}

// WriteConfig 会将当前配置写回磁盘，写入的是启动时实际加载的配置文件路径。
func WriteConfig(config *AppConfig) {
	// Use the config path that was actually loaded during ReadConfig
	// This ensures we write back to the same file we read from
	if activeConfigPath == "" {
		// Fallback to data directory if ReadConfig hasn't been called yet
		dataDir := GetDataDir()
		activeConfigPath = fmt.Sprintf("%s/config.yaml", dataDir)
	}
	WriteConfigToPath(config, activeConfigPath)
}

// WriteConfigToPath writes configuration to specified path
func WriteConfigToPath(config *AppConfig, path string) {
	if config != nil {
		lo.Must0(configStore.Load(structs.Provider(config, "yaml"), nil))
	}

	content, err := yaml.Parser().Marshal(configStore.Raw())
	if err != nil {
		fmt.Println("Failed to write config: serialization error")
		return
	}

	if err := os.WriteFile(path, content, 0o644); err != nil {
		fmt.Printf("Failed to write config: cannot write file %s\n", path)
	}
}
