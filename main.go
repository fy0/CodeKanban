package main

import (
	"context"
	"embed"
	"errors"
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
	dataDir := utils.GetDataDir()
	appLock, err := utils.AcquireAppInstanceLock(dataDir)
	if err != nil {
		var lockedErr *utils.AppInstanceLockedError
		if errors.As(err, &lockedErr) {
			fmt.Fprintf(os.Stderr, "CodeKanban is already running with data directory %s\n", lockedErr.DataDir)
			fmt.Fprintln(os.Stderr, "Close the existing instance or use a different data directory.")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Failed to acquire application lock: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := appLock.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to release application lock: %v\n", closeErr)
		}
	}()

	// 异步检查版本更新（不阻塞启动）
	checker := utils.NewVersionChecker(VERSION.String(), PACKAGE_NAME)
	checker.CheckAsync()

	cfg := utils.ReadConfig()
	configDatabase, err := utils.InitConfigDatabase(cfg)
	if err != nil {
		fmt.Printf("Failed to initialize configuration database: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := configDatabase.Close(); closeErr != nil {
			fmt.Printf("Failed to close configuration database: %v\n", closeErr)
		}
	}()
	if err := utils.EnsureAuthConfig(cfg); err != nil {
		fmt.Printf("Failed to initialize auth config: %v\n", err)
		os.Exit(1)
	}
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

	if projects, listErr := model.NewProjectService().ListProjects(context.Background()); listErr == nil {
		projectIDs := make([]string, 0, len(projects))
		for _, project := range projects {
			projectIDs = append(projectIDs, project.Id)
		}
		if cleanupErr := configDatabase.ReconcileProjectQuickInput(projectIDs); cleanupErr != nil &&
			!errors.Is(cleanupErr, utils.ErrConfigStoreReadOnly) {
			logger.Warn("Failed to reconcile project quick input history", zap.Error(cleanupErr))
		}
	}

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
