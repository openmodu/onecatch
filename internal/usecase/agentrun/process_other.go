//go:build !windows

package agentrun

import "os/exec"

func resolveNativeCodexBinary(binary string) string { return binary }

func configureProcessWindow(*exec.Cmd) {}
