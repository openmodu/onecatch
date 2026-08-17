//go:build !ios && !android

package app

import "github.com/openmodu/onecatch/internal/app/desktop"

// Run starts the desktop workbench on desktop operating systems.
func Run() {
	desktop.Run()
}
