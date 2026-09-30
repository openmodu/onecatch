//go:build (!darwin && !windows) || (darwin && !cgo)

package desktop

import "unsafe"

func installNativeWindowChrome(_ unsafe.Pointer) {}

func setNativeWindowZoomButtonHidden(_ unsafe.Pointer, _ bool) {}

func setNativeWindowDialogButtonsHidden(_ unsafe.Pointer) {}

func setNativeApplicationIcon(_ []byte) {}

func setNativeWindowAppearance(_ unsafe.Pointer, _ unsafe.Pointer) {}
