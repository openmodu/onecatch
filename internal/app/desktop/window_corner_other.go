//go:build !darwin || !cgo

package desktop

import "unsafe"

func setNativeWindowCornerRadius(_ unsafe.Pointer, _ float64) {}

func setNativeWindowZoomButtonHidden(_ unsafe.Pointer, _ bool) {}

func setNativeApplicationIcon(_ []byte) {}

func setNativeWindowAppearance(_ unsafe.Pointer, _ unsafe.Pointer) {}
