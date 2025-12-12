package main

import (
	"code-kanban/utils/tray"
	"context"
	"embed"
	"fmt"
	"os"
	"time"

	"github.com/jessevdk/go-flags"
	"go.uber.org/zap"

	"code-kanban/api"
	"code-kanban/model"
	"code-kanban/utils"
)

//go:embed all:static
var embedStatic embed.FS

var runningAsService bool

//go:generate go run ./model/sqlc_gen/

func main() {
	var opts struct {
		Version      bool   `short:"v" long:"version" description:"Show version information"`
		Install      bool   `short:"i" long:"install" description:"Install as system service"`
		Uninstall    bool   `long:"uninstall" description:"Uninstall system service"`
		ForceMigrate bool   `short:"m" long:"migrate" description:"Force database migration"`
		UseHomeData  bool   `short:"H" long:"home-data" description:"Use home directory for data storage (~/.codekanban)"`
		Bind         string `short:"b" long:"bind" description:"Bind(host) address (default: 127.0.0.1)"`
		Port         int    `short:"p" long:"port" description:"Server port (default: 3007)"`
	}

	parser := flags.NewParser(&opts, flags.Default)
	parser.Name = PACKAGE_NAME
	parser.Usage = "[OPTIONS]"

	if _, err := parser.ParseArgs(os.Args); err != nil {
		return
	}

	if opts.Version {
		fmt.Printf("%s v%s\n", APPNAME, VERSION.String())
		fmt.Printf("Channel: %s\n", APP_CHANNEL)
		return
	}

	if opts.Install {
		serviceInstall(true)
		return
	}

	if opts.Uninstall {
		serviceInstall(false)
		return
	}

	if opts.UseHomeData {
		utils.SetUseHomeData(true)
	}

	run(opts.ForceMigrate, opts.Bind, opts.Port)
}

func run(forceMigrate bool, bind string, port int) {
	// 异步检查版本更新（不阻塞启动）
	checker := utils.NewVersionChecker(VERSION.String(), PACKAGE_NAME)
	checker.CheckAsync()

	cfg := utils.ReadConfig()
	if forceMigrate {
		cfg.AutoMigrate = true
	}

	// Override config with command line flags if provided
	if bind != "" || port != 0 {
		if bind == "" {
			bind = "127.0.0.1"
		}
		if port == 0 {
			port = 3007
		}
		cfg.ServeAt = fmt.Sprintf("%s:%d", bind, port)
		cfg.Domain = cfg.ServeAt
	}

	logger, cleanup, err := utils.InitLogger(cfg)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	if err := model.InitWithDSN(cfg.DSN, cfg.DBLogLevel, cfg.AutoMigrate); err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}
	defer model.DBClose()

	logger.Info("Starting server", zap.String("listen", cfg.ServeAt))

	if !runningAsService && !cfg.DisableAutoOpenBrowser {
		if url := utils.BuildLaunchURL(cfg); url != "" {
			go func(target string) {
				time.Sleep(800 * time.Millisecond)
				if err := utils.OpenBrowser(target); err != nil {
					logger.Warn("Failed to open browser automatically", zap.String("url", target), zap.Error(err))
				}
			}(url)
		}
	}
	tray.StartTray(cfg)
	ctx := utils.ContextWithLogger(context.Background(), logger)
	if err := api.Init(ctx, cfg, embedStatic, &api.AppInfo{
		Name:        APPNAME,
		Version:     VERSION.String(),
		Channel:     APP_CHANNEL,
		PackageName: PACKAGE_NAME,
	}); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}
