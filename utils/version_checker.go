package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver/v3"
)

const (
	checkInterval  = 24 * time.Hour
	requestTimeout = 5 * time.Second
)

type VersionChecker struct {
	currentVersion string
	packageName    string
	cacheFile      string
}

type versionCache struct {
	LastCheck  time.Time `json:"last_check"`
	LatestVer  string    `json:"latest_version"`
	CurrentVer string    `json:"current_version"`
}

type npmRegistry struct {
	DistTags struct {
		Latest string `json:"latest"`
	} `json:"dist-tags"`
}

// NewVersionChecker 创建版本检查器
func NewVersionChecker(currentVersion, packageName string) *VersionChecker {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		// 如果无法获取用户配置目录，使用临时目录
		userConfigDir = os.TempDir()
	}

	configDir := filepath.Join(userConfigDir, "codekanban")
	os.MkdirAll(configDir, 0755)

	return &VersionChecker{
		currentVersion: currentVersion,
		packageName:    packageName,
		cacheFile:      filepath.Join(configDir, "version-cache.json"),
	}
}

// CheckAsync 异步检查版本（不阻塞主程序）
func (vc *VersionChecker) CheckAsync() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// 防止版本检查崩溃影响主程序
			}
		}()
		vc.Check()
	}()
}

// CheckUpdate 同步检查更新（供 API 调用）
// 返回：最新版本号、是否有更新、错误
func (vc *VersionChecker) CheckUpdate() (string, bool, error) {
	// 从 NPM 获取最新版本
	latestVersion, err := vc.fetchLatestVersion()
	if err != nil {
		return "", false, err
	}

	// 比较版本
	current, err1 := semver.NewVersion(vc.currentVersion)
	latest, err2 := semver.NewVersion(latestVersion)

	if err1 != nil || err2 != nil {
		return latestVersion, false, fmt.Errorf("版本号解析失败")
	}

	hasUpdate := latest.GreaterThan(current)
	return latestVersion, hasUpdate, nil
}

// Check 同步检查版本
func (vc *VersionChecker) Check() {
	cache := vc.loadCache()

	// 检查是否需要更新
	if !vc.shouldCheck(cache) {
		// 显示已缓存的更新提示
		if cache != nil && cache.LatestVer != "" {
			vc.showNotification(cache.LatestVer)
		}
		return
	}

	// 从 NPM 获取最新版本
	latestVersion, err := vc.fetchLatestVersion()
	if err != nil {
		// 网络错误，使用缓存
		if cache != nil && cache.LatestVer != "" {
			vc.showNotification(cache.LatestVer)
		}
		return
	}

	// 保存缓存
	vc.saveCache(&versionCache{
		LastCheck:  time.Now(),
		LatestVer:  latestVersion,
		CurrentVer: vc.currentVersion,
	})

	// 显示通知
	vc.showNotification(latestVersion)
}

// loadCache 加载缓存
func (vc *VersionChecker) loadCache() *versionCache {
	data, err := os.ReadFile(vc.cacheFile)
	if err != nil {
		return nil
	}

	var cache versionCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}

	return &cache
}

// saveCache 保存缓存
func (vc *VersionChecker) saveCache(cache *versionCache) {
	data, _ := json.MarshalIndent(cache, "", "  ")
	os.WriteFile(vc.cacheFile, data, 0644)
}

// shouldCheck 是否需要检查
func (vc *VersionChecker) shouldCheck(cache *versionCache) bool {
	if cache == nil {
		return true // 首次运行
	}

	// 版本号变了，重新检查
	if cache.CurrentVer != vc.currentVersion {
		return true
	}

	// 超过检查间隔
	if time.Since(cache.LastCheck) > checkInterval {
		return true
	}

	return false
}

// fetchLatestVersion 从 NPM 获取最新版本
func (vc *VersionChecker) fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: requestTimeout}
	url := fmt.Sprintf("https://registry.npmjs.org/%s", vc.packageName)

	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var registry npmRegistry
	if err := json.NewDecoder(resp.Body).Decode(&registry); err != nil {
		return "", err
	}

	return registry.DistTags.Latest, nil
}

// showNotification 显示更新通知
func (vc *VersionChecker) showNotification(latestVersion string) {
	if latestVersion == "" || latestVersion == vc.currentVersion {
		return
	}

	current, err1 := semver.NewVersion(vc.currentVersion)
	latest, err2 := semver.NewVersion(latestVersion)

	if err1 != nil || err2 != nil {
		return
	}

	if latest.GreaterThan(current) {
		// 计算各行内容
		updateCmd := fmt.Sprintf("npm install -g %s@latest", vc.packageName)
		viewLink := fmt.Sprintf("npmjs.com/package/%s", vc.packageName)

		// 不使用边框，简洁显示
		fmt.Printf("\n")
		fmt.Printf("🎉 New version available\n")
		fmt.Printf("\n")
		fmt.Printf("Current: %s    Latest: %s\n", vc.currentVersion, latestVersion)
		fmt.Printf("\n")
		fmt.Printf("Update command:\n")
		fmt.Printf("  %s\n", updateCmd)
		fmt.Printf("\n")
		fmt.Printf("View updates:\n")
		fmt.Printf("  %s\n", viewLink)
		fmt.Printf("\n")
	}
}
