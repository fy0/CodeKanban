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
	defaults := AppConfig{
		ServeAt:             ":9005",
		Domain:              "127.0.0.1:9005",
		RegisterOpen:        true,
		WebUrl:              "/",
		AttachmentSizeLimit: 8192,
		ImageCompress:       true,
		LogFile:             "./data/service.log",
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
		DSN:         "./data/data.db",
		PrintConfig: true,
		Terminal: TerminalConfig{
			Shell: TerminalShellConfig{
				Windows: "pwsh.exe -NoLogo",
				Linux:   "/bin/bash",
				Darwin:  "/bin/zsh",
			},
			IdleTimeout:           "10m",
			MaxSessionsPerProject: 6,
			AllowedRoots:          []string{},
			Encoding:              "utf-8",
		},
	}

	lo.Must0(configStore.Load(structs.Provider(&defaults, "yaml"), nil))

	provider := file.Provider("config.yaml")
	if err := configStore.Load(provider, yaml.Parser()); err != nil {
		fmt.Printf("读取配置失败: %v\n", err)
		if os.IsNotExist(err) {
			WriteConfig(&defaults)
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
	if config != nil {
		lo.Must0(configStore.Load(structs.Provider(config, "yaml"), nil))
	}

	content, err := yaml.Parser().Marshal(configStore.Raw())
	if err != nil {
		fmt.Println("写入配置失败: 序列化错误")
		return
	}

	if err := os.WriteFile("./config.yaml", content, 0o644); err != nil {
		fmt.Println("写入配置失败: 无法写入文件")
	}
}
