package agentrun

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

// The desktop updater maintains a current app-server independently of an npm
// CLI on PATH. Use the newer installation for both discovery and execution.
// An explicit executable setting always wins.
func defaultCodexBinary() string {
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return codexBinaryDefault
		}
		home = filepath.Join(userHome, ".codex")
	}
	name := "codex"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	managed := filepath.Join(home, "packages", "app-server-daemon", "current", "bin", name)
	return newerCodexBinary(codexBinaryDefault, managed)
}

func newerCodexBinary(fallback, managed string) string {
	info, err := os.Stat(managed)
	if err != nil || info.IsDir() {
		return fallback
	}
	managedVersion := codexBinaryVersion(managed)
	if managedVersion == nil {
		return fallback
	}
	fallbackVersion := codexBinaryVersion(resolveNativeCodexBinary(fallback))
	if fallbackVersion == nil || slices.Compare(managedVersion, fallbackVersion) > 0 {
		return managed
	}
	return fallback
}

func codexBinaryVersion(binary string) []int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--version")
	configureProcessWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	version := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(output)), "codex-cli "))
	var major, minor, patch int
	if n, _ := fmt.Sscanf(version, "%d.%d.%d", &major, &minor, &patch); n != 3 {
		return nil
	}
	return []int{major, minor, patch}
}

func (r *CodexRunner) Executable() string { return r.binary }
