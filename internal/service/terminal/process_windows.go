//go:build windows

package terminal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/UserExistsError/conpty"
)

type windowsProcess struct{ pty *conpty.ConPty }

func startProcess(input CreateInput) (terminalProcess, string, error) {
	shell := strings.TrimSpace(input.Shell)
	if shell == "" {
		shell = windowsShell()
	} else if !filepath.IsAbs(shell) {
		resolved, err := exec.LookPath(shell)
		if err != nil {
			return nil, "", fmt.Errorf("terminal shell is not executable: %s", shell)
		}
		shell = resolved
	}
	command := syscall.EscapeArg(shell)
	for _, argument := range input.Arguments {
		command += " " + syscall.EscapeArg(argument)
	}
	process, err := conpty.Start(command,
		conpty.ConPtyDimensions(int(input.Cols), int(input.Rows)),
		conpty.ConPtyWorkDir(input.Workspace),
		conpty.ConPtyEnv(append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")),
	)
	if err != nil {
		return nil, "", fmt.Errorf("start terminal shell: %w", err)
	}
	return &windowsProcess{pty: process}, filepath.Base(shell), nil
}

func windowsShell() string {
	for _, name := range []string{"pwsh.exe", "powershell.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	if shell := strings.TrimSpace(os.Getenv("COMSPEC")); shell != "" {
		return shell
	}
	return "cmd.exe"
}

func (p *windowsProcess) Read(data []byte) (int, error)  { return p.pty.Read(data) }
func (p *windowsProcess) Write(data []byte) (int, error) { return p.pty.Write(data) }
func (p *windowsProcess) PID() int                       { return p.pty.Pid() }
func (p *windowsProcess) Resize(rows, cols uint16) error { return p.pty.Resize(int(cols), int(rows)) }
func (p *windowsProcess) Wait(ctx context.Context) (int, error) {
	code, err := p.pty.Wait(ctx)
	return int(code), err
}
func (p *windowsProcess) Close() error { return p.pty.Close() }
