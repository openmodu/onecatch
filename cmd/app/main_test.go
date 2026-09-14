//go:build !ios && !android

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/processmode"
)

func TestMain(m *testing.M) {
	if os.Getenv("ONECATCH_TEST_ENTRY") == "1" {
		if os.Getenv("ONECATCH_TEST_RELAUNCH") == "1" && os.Getenv(processmode.Env) == "" {
			if err := os.WriteFile(os.Getenv("ONECATCH_UPDATE_READY_FILE"), []byte("ready"), 0o600); err != nil {
				os.Exit(1)
			}
			os.Exit(0)
		}
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestUpdaterRoleReplacesFileAndRelaunchesWithoutRole(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	parent := exec.CommandContext(ctx, self, "--help")
	parent.Env = append(os.Environ(), "ONECATCH_TEST_ENTRY=1", processmode.Env+"=", "ONECATCH_TEST_RELAUNCH=")
	if out, err := parent.CombinedOutput(); err != nil {
		t.Fatalf("parent: %v, %s", err, out)
	}
	root := t.TempDir()
	payload := filepath.Join(root, "payload")
	target := filepath.Join(root, "installed")
	if runtime.GOOS == "windows" {
		target += ".exe"
	}
	if err := os.WriteFile(target, []byte("old executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(self)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	file, err := os.OpenFile(payload, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(file, source)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy: %v, close: %v", copyErr, closeErr)
	}
	command := exec.CommandContext(ctx, self, "--mode", "replace-file", "--parent-pid", fmt.Sprint(parent.Process.Pid), "--payload", payload, "--target", target, "--ready-file", filepath.Join(root, "ready"))
	command.Env = append(os.Environ(), "ONECATCH_TEST_ENTRY=1", "ONECATCH_TEST_RELAUNCH=1", processmode.Env+"=updater")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("updater: %v, %s", err, out)
	}
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Fatalf("backup retained after readiness: %v", err)
	}
	expected, _ := source.Stat()
	actual, err := os.Stat(target)
	if err != nil || actual.Size() != expected.Size() {
		t.Fatalf("replacement missing or truncated: %v", err)
	}
}

// Re-exec the app entry point including its linked package initializers.
// Helpers must finish without opening a window or falling back to local execution.
func TestProcessRoles(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		args       []string
		code       int
		output     string
	}{
		{"help", "", []string{"--help"}, 0, "Usage: onecatch"},
		{"worker", "", []string{"worker", "--help"}, 0, "-listen"},
		{"shell", "shell", []string{"echo must-not-run"}, 125, "no target"},
		{"askpass", "askpass", []string{"Enter passphrase:"}, 1, ""},
		{"unknown", "invalid", nil, 2, "unknown internal mode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(ctx, self, tc.args...)
			for _, env := range os.Environ() {
				if strings.HasPrefix(env, "ONECATCH_") || strings.HasPrefix(env, "WAILS_UPDATER_") {
					continue
				}
				cmd.Env = append(cmd.Env, env)
			}
			cmd.Env = append(cmd.Env, "ONECATCH_TEST_ENTRY=1", processmode.Env+"="+tc.mode)
			out, err := cmd.CombinedOutput()
			code := 0
			if err != nil {
				if e, ok := err.(*exec.ExitError); ok {
					code = e.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if code != tc.code || !strings.Contains(string(out), tc.output) {
				t.Fatalf("code=%d, output=%q; want code=%d containing %q", code, out, tc.code, tc.output)
			}
			if tc.name == "askpass" && len(out) != 0 {
				t.Fatalf("askpass polluted output: %q", out)
			}
		})
	}
}
