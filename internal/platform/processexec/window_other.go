//go:build !windows

package processexec

import "os/exec"

func configure(cmd *exec.Cmd) {}
