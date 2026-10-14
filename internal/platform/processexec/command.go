// Package processexec starts background CLI processes without a visible console.
// Interactive terminals must use their PTY implementation instead.
package processexec

import (
	"context"
	"os/exec"
)

func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	configure(cmd)
	return cmd
}

func Command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	configure(cmd)
	return cmd
}
