//go:build ios || android

package app

import "github.com/openmodu/oneshot/internal/app/mobile"

// Run starts the remote-worker workbench on iOS and Android.
func Run() {
	mobile.Run()
}
