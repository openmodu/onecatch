//go:build windows

package agentrun

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

const createNoWindow = 0x08000000

// resolveNativeCodexBinary skips the npm-generated codex.cmd launcher. That
// launcher necessarily creates cmd.exe and node.exe before reaching the native
// Codex binary; on some Windows builds this also allocates a visible conhost
// despite CREATE_NO_WINDOW. Prefer the native executable shipped by the same
// npm package, then a native codex.exe elsewhere on PATH.
func resolveNativeCodexBinary(binary string) string {
	if !strings.EqualFold(filepath.Ext(binary), "") {
		if strings.EqualFold(filepath.Ext(binary), ".cmd") || strings.EqualFold(filepath.Ext(binary), ".bat") {
			return nativeCodexBesideLauncher(binary)
		}
		return binary
	}
	launcher, launcherErr := exec.LookPath(binary)
	if launcherErr == nil && (strings.EqualFold(filepath.Ext(launcher), ".cmd") || strings.EqualFold(filepath.Ext(launcher), ".bat")) {
		if native := nativeCodexBesideLauncher(launcher); native != launcher {
			return native
		}
	}
	if executable, err := exec.LookPath(binary + ".exe"); err == nil {
		return executable
	}
	return binary
}

func nativeCodexBesideLauncher(launcher string) string {
	architecture, triple := "x64", "x86_64-pc-windows-msvc"
	if runtime.GOARCH == "arm64" {
		architecture, triple = "arm64", "aarch64-pc-windows-msvc"
	}
	root := filepath.Dir(launcher)
	candidates := []string{
		filepath.Join(root, "node_modules", "@openai", "codex", "node_modules", "@openai", "codex-win32-"+architecture, "vendor", triple, "bin", "codex.exe"),
		filepath.Join(root, "node_modules", "@openai", "codex", "vendor", triple, "bin", "codex.exe"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return launcher
}

// configureProcessWindow prevents CLI shims such as codex.cmd from allocating
// conhost.exe when they are launched by the desktop GUI. Standard streams stay
// available for the JSON-RPC protocol; only the visible console is suppressed.
func configureProcessWindow(command *exec.Cmd) {
	if command == nil {
		return
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.HideWindow = true
	command.SysProcAttr.CreationFlags |= createNoWindow
}
