//go:build !ios && !android && !onecatch_headless

package app

import (
	"github.com/openmodu/onecatch/internal/app/command"
	"github.com/openmodu/onecatch/internal/app/desktop"
)

// Run starts the desktop workbench on desktop operating systems.
func Run() {
	if command.Run() {
		return
	}
	desktop.Run()
}
