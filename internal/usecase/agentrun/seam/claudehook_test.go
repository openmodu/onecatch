package seam

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runClaudeHookTest(t *testing.T, session *Session, event map[string]any) claudeHookReply {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunClaudeHook(context.Background(), session, bytes.NewReader(data), &output); err != nil {
		t.Fatal(err)
	}
	var reply claudeHookReply
	if err := json.Unmarshal(output.Bytes(), &reply); err != nil {
		t.Fatalf("decode hook reply %q: %v", output.String(), err)
	}
	return reply
}

func newClaudeHookSession(t *testing.T) (*Session, string) {
	t.Helper()
	t.Setenv(DirEnv, filepath.Join(t.TempDir(), "seams"))
	target := t.TempDir()
	session, err := NewSession("claude-hook-test", Target{Root: target})
	if err != nil {
		t.Fatal(err)
	}
	return session, target
}

func TestClaudeHookMirrorsReadAndAtomicWrite(t *testing.T) {
	session, target := newClaudeHookSession(t)
	targetPath := filepath.Join(target, "nested", "main.go")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("old\n"), 0o750); err != nil {
		t.Fatal(err)
	}

	pre := runClaudeHookTest(t, session, map[string]any{
		"hook_event_name": "PreToolUse", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": targetPath, "old_string": "old", "new_string": "new"},
	})
	if pre.HookSpecificOutput == nil || pre.HookSpecificOutput.PermissionDecision != "allow" {
		t.Fatalf("pre hook = %+v, want allow", pre)
	}
	var updated map[string]any
	if err := json.Unmarshal(pre.HookSpecificOutput.UpdatedInput, &updated); err != nil {
		t.Fatal(err)
	}
	local := updated["file_path"].(string)
	if strings.HasPrefix(local, target) {
		t.Fatalf("file path was not redirected into the private mirror: %s", local)
	}
	if data, err := os.ReadFile(local); err != nil || string(data) != "old\n" {
		t.Fatalf("mirrored data = %q, %v", data, err)
	}
	if err := os.WriteFile(local, []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	post := runClaudeHookTest(t, session, map[string]any{
		"hook_event_name": "PostToolUse", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": local},
	})
	if post.HookSpecificOutput != nil && post.HookSpecificOutput.AdditionalContext != "" {
		t.Fatalf("post hook reported failure: %+v", post)
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(targetPath); string(data) != "new\n" || info.Mode().Perm() != 0o750 {
		t.Fatalf("remote result = %q mode=%o", data, info.Mode().Perm())
	}
}

func TestClaudeHookRefusesConflictingWrite(t *testing.T) {
	session, target := newClaudeHookSession(t)
	targetPath := filepath.Join(target, "config.txt")
	if err := os.WriteFile(targetPath, []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pre := runClaudeHookTest(t, session, map[string]any{
		"hook_event_name": "PreToolUse", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": targetPath},
	})
	var updated map[string]any
	_ = json.Unmarshal(pre.HookSpecificOutput.UpdatedInput, &updated)
	local := updated["file_path"].(string)
	if err := os.WriteFile(local, []byte("agent\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("external\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	post := runClaudeHookTest(t, session, map[string]any{
		"hook_event_name": "PostToolUse", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": local},
	})
	if post.HookSpecificOutput == nil || !strings.Contains(post.HookSpecificOutput.AdditionalContext, "NOT SAVED") {
		t.Fatalf("conflicting post hook = %+v", post)
	}
	if data, _ := os.ReadFile(targetPath); string(data) != "external\n" {
		t.Fatalf("conflict overwrote target with %q", data)
	}
}

func TestClaudeHookDeniesSearchAndWorkspaceEscape(t *testing.T) {
	session, target := newClaudeHookSession(t)
	for _, event := range []map[string]any{
		{"hook_event_name": "PreToolUse", "tool_name": "Grep", "tool_input": map[string]any{"path": target}},
		{"hook_event_name": "PreToolUse", "tool_name": "Read", "tool_input": map[string]any{"file_path": "/etc/passwd"}},
	} {
		reply := runClaudeHookTest(t, session, event)
		if reply.HookSpecificOutput == nil || reply.HookSpecificOutput.PermissionDecision != "deny" {
			t.Fatalf("hook = %+v, want deny", reply)
		}
	}
}

func TestClaudeHookRefusesSymlinkEscape(t *testing.T) {
	session, target := newClaudeHookSession(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("private\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(target, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	reply := runClaudeHookTest(t, session, map[string]any{
		"hook_event_name": "PreToolUse", "tool_name": "Read",
		"tool_input": map[string]any{"file_path": link},
	})
	if reply.HookSpecificOutput == nil || reply.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("hook = %+v, want symlink denial", reply)
	}
}
