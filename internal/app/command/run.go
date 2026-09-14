// Package command dispatches process roles before desktop startup.
package command

import (
	"fmt"
	"os"

	"github.com/openmodu/onecatch/internal/app/askpass"
	"github.com/openmodu/onecatch/internal/app/shell"
	"github.com/openmodu/onecatch/internal/app/updatehelper"
	"github.com/openmodu/onecatch/internal/app/worker"
	"github.com/openmodu/onecatch/internal/processmode"
)

// Run returns false only for a normal desktop launch.
func Run() bool {
	processmode.Register()
	mode := os.Getenv(processmode.Env)
	_ = os.Unsetenv(processmode.Env)
	if mode != "" {
		switch mode {
		case "shell":
			shell.Run()
		case "askpass":
			askpass.Run()
		case "updater":
			updatehelper.Run()
		default:
			fmt.Fprintln(os.Stderr, "onecatch: unknown internal mode:", mode)
			os.Exit(2)
		}
		return true
	}
	if len(os.Args) < 2 {
		return false
	}
	switch os.Args[1] {
	case "worker":
		attachConsole()
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
		worker.Run()
	case "--help", "-h":
		attachConsole()
		fmt.Fprintln(os.Stdout, "Usage: onecatch [worker [options]]\n\nNo command opens the desktop. Use onecatch worker --help for server options.")
	default:
		return false
	}
	return true
}
