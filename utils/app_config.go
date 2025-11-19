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
	ServeAt             string           `json:"serveAt" yaml:"serveAt"`
	Domain              string           `json:"domain" yaml:"domain"`
	RegisterOpen        bool             `json:"registerOpen" yaml:"registerOpen"`
	WebUrl              string           `json:"webUrl" yaml:"webUrl"`
	AttachmentSizeLimit int64            `json:"attachmentSizeLimit" yaml:"attachmentSizeLimit"`
	ImageCompress       bool             `json:"imageCompress" yaml:"imageCompress"`
	LogFile             string           `json:"logFile" yaml:"logFile"`
	LogLevel            string           `json:"logLevel" yaml:"logLevel"`
	DBLogLevel          int              `json:"dbLogLevel" yaml:"dbLogLevel"`
	CorsAllowOrigins    string           `json:"corsAllowOrigins" yaml:"corsAllowOrigins"`
	UIOverwrite         string           `json:"uiOverwrite" yaml:"uiOverwrite"`
	AutoMigrate         bool             `json:"autoMigrate" yaml:"autoMigrate"`
	OpenAPIEnabled      bool             `json:"openapiEnabled" yaml:"openapiEnabled"`
	DocsPath            string           `json:"docsPath" yaml:"docsPath"`
	APITitle            string           `json:"apiTitle" yaml:"apiTitle"`
	APIVersion          string           `json:"apiVersion" yaml:"apiVersion"`
	AttachmentConfig    AttachmentConfig `json:"attachmentConfig" yaml:"attachmentConfig"`
	DSN                 string           `json:"dbUrl" yaml:"dbUrl"`
	PrintConfig         bool             `json:"printConfig" yaml:"printConfig"`
	Terminal            TerminalConfig   `json:"terminal" yaml:"terminal"`
}

var configStore = koanf.New(".")

// ReadConfig 会加载 config.yaml，若不存在则写入默认配置。
func ReadConfig() *AppConfig {
	// 获取数据目录（npm 全局安装时使用 ~/.codekanban，否则使用 ./data）
	dataDir := GetDataDir()

	// 打印工作目录信息
	if cwd, err := os.Getwd(); err == nil {
		fmt.Printf("Working directory: %s\n", cwd)
	}
	fmt.Printf("Data directory: %s\n", dataDir)
	fmt.Println()

	// 确保数据目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		fmt.Printf("创建数据目录失败: %v\n", err)
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
		APITitle:            "Go Template API",
		APIVersion:          "1.0.0",
		AttachmentConfig: AttachmentConfig{
			UseS3: false,
		},
		DSN:         fmt.Sprintf("%s/data.db", dataDir),
		PrintConfig: true,
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
	}

	lo.Must0(configStore.Load(structs.Provider(&defaults, "yaml"), nil))

	// 配置文件路径：优先使用工作目录的 config.yaml，如果不存在则使用数据目录的
	workDirConfig := "config.yaml"
	dataDirConfig := fmt.Sprintf("%s/config.yaml", dataDir)

	var configPath string
	if _, err := os.Stat(workDirConfig); err == nil {
		configPath = workDirConfig
	} else {
		configPath = dataDirConfig
	}

	provider := file.Provider(configPath)
	if err := configStore.Load(provider, yaml.Parser()); err != nil {
		fmt.Printf("读取配置失败: %v\n", err)
		if os.IsNotExist(err) {
			WriteConfigToPath(&defaults, configPath)
		} else {
			os.Exit(1)
		}
	}

	config := defaults
	if err := configStore.Unmarshal("", &config); err != nil {
		fmt.Printf("解析配置失败: %v\n", err)
		os.Exit(1)
	}

	// Normalize derived values to avoid重复计算.
	_ = config.Terminal.IdleDuration()

	if config.PrintConfig {
		configStore.Print()
	}

	return &config
}

// WriteConfig 会将当前配置写回磁盘，常用于初始化默认配置。
func WriteConfig(config *AppConfig) {
	dataDir := GetDataDir()
	configPath := fmt.Sprintf("%s/config.yaml", dataDir)
	WriteConfigToPath(config, configPath)
}

// WriteConfigToPath 将配置写入指定路径
func WriteConfigToPath(config *AppConfig, path string) {
	if config != nil {
		lo.Must0(configStore.Load(structs.Provider(config, "yaml"), nil))
	}

	content, err := yaml.Parser().Marshal(configStore.Raw())
	if err != nil {
		fmt.Println("写入配置失败: 序列化错误")
		return
	}

	if err := os.WriteFile(path, content, 0o644); err != nil {
		fmt.Printf("写入配置失败: 无法写入文件 %s\n", path)
	}
}
