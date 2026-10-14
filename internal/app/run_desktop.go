//go:build !ios && !android && !onecatch_headless

package app

import (
	"flag"
	"github.com/openmodu/onecatch/internal/app/command"
	keeldesktop "github.com/openmodu/onecatch/internal/app/keel"
	"log"
)

// Run dispatches process roles before opening the native Keel desktop.
func Run() {
	if command.Run() {
		return
	}
	root := flag.String("data-dir", "", "OneCatch data directory (default: ~/.onecatch)")
	flag.Parse()
	if err := keeldesktop.Run(keeldesktop.Options{DataRoot: *root}); err != nil {
		log.Fatal(err)
	}
}
