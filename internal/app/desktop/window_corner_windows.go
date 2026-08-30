//go:build windows

package desktop

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmSetIcon  = 0x0080
	iconSmall  = 0
	iconBig    = 1
	imageIcon  = 1
	lrShared   = 0x00008000
	smCXIcon   = 11
	smCYIcon   = 12
	smCXSmIcon = 49
	smCYSmIcon = 50
	appIconID  = 3
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	loadImage        = user32.NewProc("LoadImageW")
	sendMessage      = user32.NewProc("SendMessageW")
	getSystemMetrics = user32.NewProc("GetSystemMetrics")
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	getModuleHandle  = kernel32.NewProc("GetModuleHandleW")
)

// installNativeWindowChrome explicitly supplies both icon sizes. Wails sets
// WM_SETICON's large icon, but Windows may ask a frameless window for the small
// icon when composing its taskbar button. Without it, the shell can fall back
// to a stale/default square icon instead of the rounded ICO resource.
func installNativeWindowChrome(window unsafe.Pointer) {
	if window == nil {
		return
	}
	module, _, _ := getModuleHandle.Call(0)
	if module == 0 {
		return
	}
	setWindowIcon(uintptr(window), module, iconBig, smCXIcon, smCYIcon)
	setWindowIcon(uintptr(window), module, iconSmall, smCXSmIcon, smCYSmIcon)
}

func setWindowIcon(window, module, kind, widthMetric, heightMetric uintptr) {
	width, _, _ := getSystemMetrics.Call(widthMetric)
	height, _, _ := getSystemMetrics.Call(heightMetric)
	icon, _, _ := loadImage.Call(module, appIconID, imageIcon, width, height, lrShared)
	if icon != 0 {
		_, _, _ = sendMessage.Call(window, wmSetIcon, kind, icon)
	}
}

func setNativeWindowZoomButtonHidden(_ unsafe.Pointer, _ bool) {}

func setNativeApplicationIcon(_ []byte) {}

func setNativeWindowAppearance(_ unsafe.Pointer, _ unsafe.Pointer) {}
