package tray

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"runtime"
	"syscall"
	"time"

	"code-kanban/utils"

	systray "github.com/gauss1190/systray"
)

// StartTray 启动系统托盘，仅提供「打开页面」与「退出」两个菜单项，并支持双击图标打开页面。
// 该实现基于跨平台的 systray 库，确保在 Windows/macOS/Linux 行为一致。
func StartTray(cfg *utils.AppConfig) {
	runtime.LockOSThread()
	systray.Run(func() { onReady(cfg) }, onExit)
}

// StopTray 退出托盘。
func StopTray() {
	systray.Quit()
}

func onReady(cfg *utils.AppConfig) {
	// 标题与提示信息
	title := cfg.APITitle
	if title == "" {
		title = "CodeKanban"
	}
	systray.SetTitle(title)
	systray.SetTooltip("点击菜单或双击图标打开页面")

	// 尝试设置图标（可选）。当无法提供图标时，托盘仍可工作。
	if icon := defaultIconBytes(); len(icon) > 0 {
		systray.SetIcon(icon)
	}

	// 菜单：打开页面、退出
	mOpen := systray.AddMenuItem("打开页面", "打开代码看板")
	mQuit := systray.AddMenuItem("退出", "退出应用程序")

	// 双击图标直接打开页面
	systray.SetOnDClick(func() { openProjectPage(cfg) })

	mOpen.Click(func() { openProjectPage(cfg) })
	mQuit.Click(func() { systray.Quit() })
}

func onExit() {
	// 不知道为啥咋程序关闭托盘自己不退出呢？
	syscall.Exit(0)
}

func openProjectPage(cfg *utils.AppConfig) {
	target := utils.BuildLaunchURL(cfg)
	if target == "" {
		fmt.Println("无法解析页面地址：配置为空或无效")
		return
	}
	// 稍作延时，避免部分平台上托盘事件与浏览器启动竞争导致失败
	time.Sleep(200 * time.Millisecond)
	if err := utils.OpenBrowser(target); err != nil {
		fmt.Printf("打开浏览器失败：%v\n", err)
	}
}

// defaultIconBytes 返回一个最小的 .ico 图标字节（Windows 需要 .ico，其他平台支持 .ico/.png）。
// 若返回空切片，托盘仍会工作，只是使用系统默认或不显示图标。
// defaultIconBytes 返回一个 "CB" 文字的 16x16 图标字节
func defaultIconBytes() []byte {
	// 创建 16x16 的 RGBA 图像
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))

	// 填充蓝色背景
	bgColor := color.RGBA{G: 120, B: 212, A: 255} // Windows蓝色
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, bgColor)
		}
	}

	// 用简单点阵绘制 "CB"
	// 在 16x16 中绘制简单的 "C"
	cPoints := []struct{ x, y int }{
		{3, 5}, {3, 6}, {3, 7}, {3, 8}, {3, 9}, {3, 10},
		{4, 4}, {5, 4}, {6, 4},
		{4, 11}, {5, 11}, {6, 11},
	}

	// 绘制 "B"
	bPoints := []struct{ x, y int }{
		{9, 4}, {9, 5}, {9, 6}, {9, 7}, {9, 8}, {9, 9}, {9, 10}, {9, 11},
		{10, 4}, {11, 5}, {12, 6}, {12, 7},
		{10, 7}, {11, 7}, {12, 8}, {12, 9}, {11, 10}, {10, 11},
	}

	// 绘制白色文字
	textColor := color.White
	for _, p := range cPoints {
		if p.x < 16 && p.y < 16 {
			img.Set(p.x, p.y, textColor)
		}
	}
	for _, p := range bPoints {
		if p.x < 16 && p.y < 16 {
			img.Set(p.x, p.y, textColor)
		}
	}

	// 将图片写入缓冲区
	var buf bytes.Buffer

	// 创建简单的 ICO 文件
	// ICO 文件头
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0)) // 保留字
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // 图标类型
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // 图标数量

	// 图标目录项
	_ = binary.Write(&buf, binary.LittleEndian, byte(16))           // 宽度
	_ = binary.Write(&buf, binary.LittleEndian, byte(16))           // 高度
	_ = binary.Write(&buf, binary.LittleEndian, byte(0))            // 颜色数
	_ = binary.Write(&buf, binary.LittleEndian, byte(0))            // 保留
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))          // 颜色平面
	_ = binary.Write(&buf, binary.LittleEndian, uint16(32))         // 每像素位数
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16*16*4+40)) // 图像数据大小
	_ = binary.Write(&buf, binary.LittleEndian, uint32(22))         // 图像数据偏移

	// BITMAPINFOHEADER
	_ = binary.Write(&buf, binary.LittleEndian, uint32(40))      // 头大小
	_ = binary.Write(&buf, binary.LittleEndian, int32(16))       // 宽度
	_ = binary.Write(&buf, binary.LittleEndian, int32(32))       // 高度*2（包含掩码）
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))       // 平面数
	_ = binary.Write(&buf, binary.LittleEndian, uint16(32))      // 每像素位数
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))       // 压缩方式
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16*16*4)) // 图像数据大小
	_ = binary.Write(&buf, binary.LittleEndian, int32(0))        // 水平分辨率
	_ = binary.Write(&buf, binary.LittleEndian, int32(0))        // 垂直分辨率
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))       // 使用颜色数
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))       // 重要颜色数

	// 写入像素数据（BGR A）
	for y := 15; y >= 0; y-- {
		for x := 0; x < 16; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			buf.WriteByte(byte(b >> 8)) // 蓝
			buf.WriteByte(byte(g >> 8)) // 绿
			buf.WriteByte(byte(r >> 8)) // 红
			buf.WriteByte(byte(a >> 8)) // 透明度
		}
	}

	// 掩码数据（1位/像素，全0表示不透明）
	for i := 0; i < 16*16/8; i++ {
		buf.WriteByte(0)
	}

	return buf.Bytes()
}
