package utils

import (
	"path/filepath"
	"runtime"
	"strings"
)

// SensitiveSystemDirs 包含不应用作 worktree 基础目录的敏感系统目录。
// 这些是 Unix 和 Windows 系统上的关键路径。
var SensitiveSystemDirs = []string{
	// Unix 系统目录
	"/etc", "/bin", "/sbin", "/usr", "/var", "/boot", "/root", "/lib", "/lib64",
	"/proc", "/sys", "/dev", "/run", "/snap",
	// Windows 系统目录
	"C:\\Windows", "C:\\Program Files", "C:\\Program Files (x86)",
	"C:\\ProgramData", "C:\\System Volume Information",
}

// IsSensitiveSystemDir 检查给定路径是否为敏感系统目录或位于敏感系统目录下。
// 出于安全原因，返回 true 表示应拒绝该路径。
func IsSensitiveSystemDir(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}

	cleanPath := filepath.Clean(path)

	// 检查根目录
	if cleanPath == "/" || cleanPath == "\\" {
		return true
	}

	// 在 Windows 上检查驱动器根目录（如 "C:\"）
	if runtime.GOOS == "windows" && len(cleanPath) == 3 && cleanPath[1] == ':' && (cleanPath[2] == '\\' || cleanPath[2] == '/') {
		return true
	}

	// 检查敏感目录列表
	for _, sensitive := range SensitiveSystemDirs {
		// 精确匹配（Windows 上不区分大小写）
		if pathEquals(cleanPath, sensitive) {
			return true
		}

		// 检查路径是否在敏感目录下
		if isSubPath(cleanPath, sensitive) {
			return true
		}
	}

	return false
}

// pathEquals 比较两个路径是否相等，Windows 上不区分大小写。
func pathEquals(path1, path2 string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(path1, path2)
	}
	return path1 == path2
}

// isSubPath 检查 path 是否在 basePath 目录下。
func isSubPath(path, basePath string) bool {
	sep := string(filepath.Separator)

	if runtime.GOOS == "windows" {
		pathLower := strings.ToLower(path)
		baseLower := strings.ToLower(basePath)
		return strings.HasPrefix(pathLower, baseLower+sep)
	}

	return strings.HasPrefix(path, basePath+sep)
}
