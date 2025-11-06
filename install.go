package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kardianos/service"
)

type program struct{}

// Start 会由服务管理器回调，实际启动逻辑放到单独协程中避免阻塞。
func (p *program) Start(s service.Service) error {
	go func() {
		run(false)
	}()
	return nil
}

// Stop 目前无需额外清理，直接返回即可。
func (p *program) Stop(service.Service) error {
	return nil
}

// serviceInstall 根据用户指令安装或卸载 Windows 服务。
func serviceInstall(isInstall bool) {
	cwd, _ := os.Getwd()
	wd, _ := filepath.Abs(cwd)

	svcConfig := &service.Config{
		Name:             "go-template",
		DisplayName:      "Go Template Service",
		Description:      "Go 项目模板自动以 Windows 服务运行",
		WorkingDirectory: wd,
	}

	srv, err := service.New(&program{}, svcConfig)
	if err != nil {
		fmt.Printf("创建服务失败: %v\n", err)
		return
	}

	if isInstall {
		fmt.Println("正在安装系统服务...")
		if err := srv.Install(); err != nil {
			fmt.Printf("安装失败: %v\n", err)
			return
		}
		fmt.Println("安装完成，请通过服务管理器启动或停止。")
		return
	}

	fmt.Println("正在卸载系统服务...")
	if err := srv.Stop(); err != nil {
		fmt.Printf("停止服务失败: %v\n", err)
	}
	if err := srv.Uninstall(); err != nil {
		fmt.Printf("卸载失败: %v\n", err)
		return
	}
	fmt.Println("卸载完成。")
}
