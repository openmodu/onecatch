//go:build !darwin || !cgo

package desktop

import "unsafe"

func setNativeWindowCornerRadius(_ unsafe.Pointer, _ float64) {}
