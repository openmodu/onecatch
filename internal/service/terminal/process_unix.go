//go:build !windows

package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

type unixProcess struct {
	pty       *os.File
	command   *exec.Cmd
	closeOnce sync.Once
}

func startProcess(input CreateInput) (terminalProcess, string, error) {
	if input.RemoteFS != nil {
		binary, arguments, environment, label, err := remoteSSHInvocation(input, terminalEnvironment(os.Environ()))
		if err != nil {
			return nil, "", err
		}
		command := exec.Command(binary, arguments...)
		command.Env = environment
		ptmx, err := pty.StartWithSize(command, &pty.Winsize{Rows: input.Rows, Cols: input.Cols})
		if err != nil {
			return nil, "", fmt.Errorf("start remote SSH terminal: %w", err)
		}
		return &unixProcess{pty: ptmx, command: command}, label, nil
	}
	shell := strings.TrimSpace(input.Shell)
	if shell == "" {
		shell = strings.TrimSpace(os.Getenv("SHELL"))
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	if !filepath.IsAbs(shell) {
		resolved, err := exec.LookPath(shell)
		if err != nil {
			return nil, "", fmt.Errorf("terminal shell is not executable: %s", shell)
		}
		shell = resolved
	}
	if info, err := os.Stat(shell); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return nil, "", fmt.Errorf("terminal shell is not executable: %s", shell)
	}
	command := exec.Command(shell, input.Arguments...)
	command.Dir = input.Workspace
	command.Env = terminalEnvironment(os.Environ())
	ptmx, err := pty.StartWithSize(command, &pty.Winsize{Rows: input.Rows, Cols: input.Cols})
	if err != nil {
		return nil, "", fmt.Errorf("start terminal shell: %w", err)
	}
	return &unixProcess{pty: ptmx, command: command}, filepath.Base(shell), nil
}

func terminalEnvironment(environment []string) []string {
	filtered := environment[:0]
	for _, value := range environment {
		if strings.HasPrefix(value, "TERM=") || strings.HasPrefix(value, "COLORTERM=") || strings.HasPrefix(value, "TERM_PROGRAM=") {
			continue
		}
		filtered = append(filtered, value)
	}
	// Bubble Tea v2 queries synchronized-output mode for most TERM_PROGRAM
	// values. WKWebView's xterm bridge can acknowledge that query but fail to
	// flush the first full-screen frame, so use its conservative Apple Terminal
	// path while retaining truthful xterm capabilities through TERM.
	return append(filtered, "TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=Apple_Terminal")
}

func (p *unixProcess) Read(data []byte) (int, error) {
	n, err := p.pty.Read(data)
	// Linux PTYs report EIO when the slave side closes; it is the normal EOF
	// signal for an interactive shell, not a terminal failure.
	if errors.Is(err, syscall.EIO) {
		err = io.EOF
	}
	return n, err
}
func (p *unixProcess) Write(data []byte) (int, error) { return p.pty.Write(data) }
func (p *unixProcess) PID() int                       { return p.command.Process.Pid }
func (p *unixProcess) Resize(rows, cols uint16) error {
	return pty.Setsize(p.pty, &pty.Winsize{Rows: rows, Cols: cols})
}
func (p *unixProcess) Wait(context.Context) (int, error) {
	err := p.command.Wait()
	if err == nil {
		return 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode(), err
	}
	return -1, err
}
func (p *unixProcess) Close() error {
	var result error
	p.closeOnce.Do(func() {
		result = p.pty.Close()
		if p.command.Process != nil {
			_ = syscall.Kill(-p.command.Process.Pid, syscall.SIGKILL)
		}
	})
	return result
}
