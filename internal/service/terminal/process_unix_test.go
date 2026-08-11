//go:build !windows

package terminal

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestUnixProcessRunsInteractiveShellInWorkspace(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	workspace := t.TempDir()
	process, shell, err := startProcess(CreateInput{Workspace: workspace, Rows: 20, Cols: 90})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	if shell != "sh" {
		t.Fatalf("shell = %q", shell)
	}
	if _, err := process.Write([]byte("pwd; printf 'oneshot-pty-ok\\n'; exit\r")); err != nil {
		t.Fatal(err)
	}
	type readResult struct {
		output string
		err    error
	}
	readDone := make(chan readResult, 1)
	go func() {
		var output bytes.Buffer
		buffer := make([]byte, 4096)
		for {
			n, readErr := process.Read(buffer)
			output.Write(buffer[:n])
			if readErr != nil {
				if readErr == io.EOF {
					readErr = nil
				}
				readDone <- readResult{output: output.String(), err: readErr}
				return
			}
		}
	}()
	var result readResult
	select {
	case result = <-readDone:
	case <-time.After(3 * time.Second):
		_ = process.Close()
		t.Fatal("timed out waiting for shell output")
	}
	if result.err != nil {
		t.Fatalf("read: %v, output: %q", result.err, result.output)
	}
	exitCode, err := process.Wait(context.Background())
	if err != nil || exitCode != 0 {
		t.Fatalf("wait = %d, %v", exitCode, err)
	}
	text := strings.ReplaceAll(result.output, "\r", "")
	if !strings.Contains(text, workspace) || !strings.Contains(text, "oneshot-pty-ok") {
		t.Fatalf("output = %q", text)
	}
}
