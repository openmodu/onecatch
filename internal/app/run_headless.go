//go:build onecatch_headless && !ios && !android

package app

import (
	"fmt"
	"os"

	"github.com/openmodu/onecatch/internal/app/command"
)

func Run() {
	if command.Run() {
		return
	}
	fmt.Fprintln(os.Stderr, "This build has no desktop. Use onecatch worker --help.")
	os.Exit(2)
}
