// OneCatch Keel renders its desktop interface entirely from Go.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/openmodu/onecatch/internal/app/command"
	keeldesktop "github.com/openmodu/onecatch/internal/app/keel"
	"github.com/openmodu/onecatch/internal/processmode"
)

func main() {
	// The patched Keel development builder needs GOFLAGS for nested builds.
	// Restore the user's flags before any project tools or process roles run.
	if original, ok := os.LookupEnv("ONECATCH_KEEL_GOFLAGS_BEFORE"); ok {
		_ = os.Setenv("GOFLAGS", original)
		_ = os.Unsetenv("ONECATCH_KEEL_GOFLAGS_BEFORE")
	}
	if os.Getenv(processmode.Env) != "" || len(os.Args) > 1 && os.Args[1] == "worker" {
		command.Run()
		return
	}
	processmode.Register()
	root := flag.String("data-dir", "", "OneCatch data directory (default: ~/.onecatch)")
	flag.Parse()
	if err := keeldesktop.Run(keeldesktop.Options{DataRoot: *root}); err != nil {
		log.Fatal(err)
	}
}
