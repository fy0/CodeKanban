package main

import (
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
		Install      bool `short:"i" long:"install" description:"安装为系统服务"`
		Uninstall    bool `long:"uninstall" description:"卸载系统服务"`
		ForceMigrate bool `short:"m" long:"migrate" description:"强制执行数据库迁移"`
	}

	if _, err := flags.ParseArgs(&opts, os.Args); err != nil {
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

	run(opts.ForceMigrate)
}

func run(forceMigrate bool) {
	cfg := utils.ReadConfig()
	if forceMigrate {
		cfg.AutoMigrate = true
	}

	logger, cleanup, err := utils.InitLogger(cfg)
	if err != nil {
		fmt.Printf("初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	if err := model.InitWithDSN(cfg.DSN, cfg.DBLogLevel, cfg.AutoMigrate); err != nil {
		logger.Fatal("初始化数据层失败", zap.Error(err))
	}
	defer model.DBClose()

	logger.Info("服务启动中", zap.String("listen", cfg.ServeAt))

	if !runningAsService {
		if url := utils.BuildLaunchURL(cfg); url != "" {
			go func(target string) {
				time.Sleep(800 * time.Millisecond)
				if err := utils.OpenBrowser(target); err != nil {
					logger.Warn("自动打开浏览器失败", zap.String("url", target), zap.Error(err))
				}
			}(url)
		}
	}

	ctx := utils.ContextWithLogger(context.Background(), logger)
	if err := api.Init(ctx, cfg, embedStatic, &api.AppInfo{
		Name:    APPNAME,
		Version: VERSION.String(),
		Channel: APP_CHANNEL,
	}); err != nil {
		logger.Fatal("服务启动失败", zap.Error(err))
	}
}
