//go:build !darwin || !cgo

package main

import "unsafe"

func setNativeWindowCornerRadius(_ unsafe.Pointer, _ float64) {}
