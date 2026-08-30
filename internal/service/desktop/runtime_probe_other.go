//go:build !windows

package desktop

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

func probeRuntimeVersion(binary string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	version := strings.TrimSpace(string(output))
	if index := strings.IndexByte(version, '\n'); index >= 0 {
		version = version[:index]
	}
	return version
}
