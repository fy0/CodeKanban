//go:build !windows && !darwin
// +build !windows,!darwin

package tray

import (
	"code-kanban/utils"
)

func StartTray(_ *utils.AppConfig) {
	// 什么也不做，Linux大部分时候也不需要托盘
}

// StopTray 退出托盘。
func StopTray() {
}
