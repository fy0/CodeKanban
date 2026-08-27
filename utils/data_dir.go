package utils

import (
	"os"
	"path/filepath"
)

var useHomeData bool

// SetUseHomeData 设置是否使用用户目录存储数据
func SetUseHomeData(use bool) {
	useHomeData = use
}

// GetDataDir 返回数据目录路径
// 使用 ~/.codekanban 的条件：
// 1. 通过 --home-data 参数指定
// 2. 可执行文件目录下存在 .npm-global 标记文件
// 否则使用 ./data
func GetDataDir() string {
	// 检查是否通过参数指定使用 home 目录
	if useHomeData {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(homeDir, ".codekanban")
		}
	}

	// 检查可执行文件目录下是否有 npm 全局安装的标记文件
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		markerFile := filepath.Join(execDir, ".npm-global")

		if _, err := os.Stat(markerFile); err == nil {
			// 存在标记文件，使用 home 目录
			homeDir, err := os.UserHomeDir()
			if err == nil {
				return filepath.Join(homeDir, ".codekanban")
			}
		}
	}

	// 默认使用当前目录下的 data
	return "./data"
}

// isDevMode 检查是否是开发模式
// 通过检查当前工作目录下是否存在 go.mod 文件判断
