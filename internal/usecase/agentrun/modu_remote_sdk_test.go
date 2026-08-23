//go:build !onecatch_worker

package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	codingtools "github.com/openmodu/modu/pkg/coding_agent/tools"
	"github.com/openmodu/modu/pkg/types"
	"github.com/openmodu/onecatch/internal/remotefs"
	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

type localModuFiles struct{ root string }

func (f localModuFiles) path(name string) string {
	return filepath.Join(f.root, filepath.FromSlash(name))
}

func (f localModuFiles) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(f.path(name))
}

func (f localModuFiles) ReadDir(name string) ([]os.FileInfo, error) {
	entries, err := os.ReadDir(f.path(name))
	if err != nil {
		return nil, err
	}
	result := make([]os.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, infoErr
		}
		result = append(result, info)
	}
	return result, nil
}

func (f localModuFiles) OpenFile(name string, flags int, mode os.FileMode) (remotefs.File, error) {
	return os.OpenFile(f.path(name), flags, mode)
}

func (f localModuFiles) Mkdir(name string, mode os.FileMode) error {
	return os.Mkdir(f.path(name), mode)
}

func (localModuFiles) Close() error { return nil }

func TestRemoteModuProviderRoutesWorkspaceTools(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	provider := &remoteModuToolProvider{
		base: codingtools.NewProvider(codingtools.ToolSetCoding), files: localModuFiles{root: root},
		executor: seam.NewExecutor(seam.Target{}), root: root, reads: make(map[string]string),
	}
	tools := provider.Tools(types.ToolContext{})
	write := moduToolNamed(t, tools, "write")
	read := moduToolNamed(t, tools, "read")
	edit := moduToolNamed(t, tools, "edit")
	bash := moduToolNamed(t, tools, "bash")
	for _, forbidden := range []string{"bash_output", "kill_bash", "enter_worktree", "exit_worktree"} {
		if toolNamed(tools, forbidden) != nil {
			t.Fatalf("remote provider exposed unsupported tool %q", forbidden)
		}
	}

	result, err := write.Execute(context.Background(), "write-1", map[string]any{
		"path": "nested/hello.txt", "content": "hello remote\n",
	}, nil)
	if err != nil || result.IsError {
		t.Fatalf("remote write = %+v, %v", result, err)
	}
	data, err := os.ReadFile(filepath.Join(root, "nested", "hello.txt"))
	if err != nil || string(data) != "hello remote\n" {
		t.Fatalf("written data = %q, %v", data, err)
	}

	result, err = read.Execute(context.Background(), "read-1", map[string]any{"path": "nested/hello.txt"}, nil)
	if err != nil || result.IsError || !strings.Contains(moduToolResultText(result), "1\thello remote") {
		t.Fatalf("remote read = %+v, %v", result, err)
	}
	result, err = edit.Execute(context.Background(), "edit-1", map[string]any{
		"path": "nested/hello.txt", "old_text": "hello", "new_text": "hi",
	}, nil)
	if err != nil || result.IsError {
		t.Fatalf("remote edit = %+v, %v", result, err)
	}
	data, _ = os.ReadFile(filepath.Join(root, "nested", "hello.txt"))
	if string(data) != "hi remote\n" {
		t.Fatalf("edited data = %q", data)
	}

	result, err = bash.Execute(context.Background(), "bash-1", map[string]any{"command": "pwd"}, nil)
	if err != nil || result.IsError || !strings.Contains(moduToolResultText(result), root) {
		t.Fatalf("remote bash = %+v, %v", result, err)
	}
}

func TestRemoteModuProviderReadOnlyToolBoundary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	provider := &remoteModuToolProvider{
		base: codingtools.NewProvider(codingtools.ToolSetReadOnly), files: localModuFiles{root: root},
		executor: seam.NewExecutor(seam.Target{}), root: root, readOnly: true, reads: make(map[string]string),
	}
	tools := provider.Tools(types.ToolContext{})
	for _, expected := range []string{"read", "grep", "find", "ls"} {
		moduToolNamed(t, tools, expected)
	}
	for _, forbidden := range []string{"write", "edit", "bash"} {
		if toolNamed(tools, forbidden) != nil {
			t.Fatalf("read-only remote provider exposed %q", forbidden)
		}
	}
}

func TestRemoteModuPathRejectsWorkspaceEscape(t *testing.T) {
	t.Parallel()
	if got, err := remoteModuPath("/srv/project", "/srv/project/internal/main.go"); err != nil || got != "internal/main.go" {
		t.Fatalf("remoteModuPath inside root = %q, %v", got, err)
	}
	for _, input := range []string{"../secret", "/etc/passwd", "/srv/project-old/secret"} {
		if _, err := remoteModuPath("/srv/project", input); err == nil {
			t.Errorf("remoteModuPath(%q) accepted a workspace escape", input)
		}
	}
}

func TestModuCLIRemoteRunUsesConfiguredSDKAdapter(t *testing.T) {
	t.Parallel()
	fallback := &recordingModuRemoteRunner{}
	runner := NewModuRunner("/bin/true")
	runner.remoteRunner = fallback
	result, err := runner.Run(context.Background(), Request{
		Workspace: t.TempDir(), Remote: &seam.Target{Host: "devbox", Root: "/srv/project"},
	}, nil)
	if err != nil || !fallback.called || result.SessionID != "remote-sdk" {
		t.Fatalf("remote CLI delegation = %+v, called=%v, err=%v", result, fallback.called, err)
	}
}

type recordingModuRemoteRunner struct{ called bool }

func (*recordingModuRemoteRunner) Runtime() Runtime { return RuntimeModu }
func (*recordingModuRemoteRunner) Available() bool  { return true }
func (r *recordingModuRemoteRunner) Run(context.Context, Request, Sink) (Result, error) {
	r.called = true
	return Result{SessionID: "remote-sdk", Succeeded: true}, nil
}

func moduToolNamed(t *testing.T, tools []types.Tool, name string) types.Tool {
	t.Helper()
	tool := toolNamed(tools, name)
	if tool == nil {
		t.Fatalf("tool %q not found", name)
	}
	return tool
}

func toolNamed(tools []types.Tool, name string) types.Tool {
	for _, tool := range tools {
		if tool.Name() == name {
			return tool
		}
	}
	return nil
}

func moduToolResultText(result types.ToolResult) string {
	var text strings.Builder
	for _, block := range result.Content {
		if content, ok := block.(*types.TextContent); ok {
			text.WriteString(content.Text)
		}
	}
	return text.String()
}
