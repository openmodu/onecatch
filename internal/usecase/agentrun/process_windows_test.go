//go:build windows

package agentrun

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestConfigureProcessWindowSuppressesConsole(t *testing.T) {
	command := exec.Command("cmd.exe", "/c", "exit", "0")
	configureProcessWindow(command)
	if command.SysProcAttr == nil || !command.SysProcAttr.HideWindow {
		t.Fatal("CLI process does not hide its console window")
	}
	if command.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatal("CLI process does not use CREATE_NO_WINDOW")
	}
}

func TestResolveNativeCodexBinarySkipsNPMLauncher(t *testing.T) {
	root := t.TempDir()
	launcher := filepath.Join(root, "codex.cmd")
	if err := os.WriteFile(launcher, []byte("@echo off\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	architecture, triple := "x64", "x86_64-pc-windows-msvc"
	if runtime.GOARCH == "arm64" {
		architecture, triple = "arm64", "aarch64-pc-windows-msvc"
	}
	native := filepath.Join(root, "node_modules", "@openai", "codex", "node_modules", "@openai", "codex-win32-"+architecture, "vendor", triple, "bin", "codex.exe")
	if err := os.MkdirAll(filepath.Dir(native), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(native, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if resolved := resolveNativeCodexBinary(launcher); resolved != native {
		t.Fatalf("resolved Codex binary = %q, want %q", resolved, native)
	}
}
