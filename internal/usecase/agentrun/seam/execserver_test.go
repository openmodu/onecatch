package seam

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newLocalExecServer(t *testing.T, readOnly bool) (*ExecServer, string, string) {
	t.Helper()
	dir := t.TempDir()
	workspace := filepath.Join(dir, "local-workspace")
	target := filepath.Join(dir, "remote-workspace")
	for _, value := range []string{workspace, target} {
		if err := os.MkdirAll(value, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(DirEnv, filepath.Join(dir, "sessions"))
	session, err := NewSession("exec-test", Target{Root: target})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.SetReadOnly(readOnly); err != nil {
		t.Fatal(err)
	}
	server, err := NewExecServer(context.Background(), session, workspace)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	if _, rpcErr := server.handleInitialize(nil); rpcErr != nil {
		t.Fatal(rpcErr)
	}
	return server, workspace, target
}

func TestExecServerFileCallsOperateOnTarget(t *testing.T) {
	server, workspace, target := newLocalExecServer(t, false)
	localPath := filepath.Join(workspace, "nested", "answer.txt")
	targetPath := filepath.Join(target, "nested", "answer.txt")

	mkdirParams, _ := json.Marshal(map[string]any{"path": pathToURI(filepath.Dir(localPath)), "recursive": true})
	if _, rpcErr := server.handleFSCreateDirectory(context.Background(), mkdirParams); rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	content := []byte{'r', 'e', 'm', 'o', 't', 'e', 0, 0xff}
	writeParams, _ := json.Marshal(map[string]any{
		"path": localPathURI(localPath), "dataBase64": base64.StdEncoding.EncodeToString(content),
	})
	if _, rpcErr := server.handleFSWriteFile(context.Background(), writeParams); rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("native fs write touched the local workspace: %v", err)
	}
	got, err := os.ReadFile(targetPath)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("target content = %q, %v; want %q", got, err, content)
	}

	readParams, _ := json.Marshal(map[string]any{"path": localPathURI(localPath)})
	result, rpcErr := server.handleFSReadFile(context.Background(), readParams)
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	encoded := result.(map[string]any)["dataBase64"].(string)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !bytes.Equal(decoded, content) {
		t.Fatalf("read = %q, %v; want %q", decoded, err, content)
	}
}

func TestExecServerReadOnlyIsEnforcedAtTargetBoundary(t *testing.T) {
	server, workspace, target := newLocalExecServer(t, true)
	params, _ := json.Marshal(map[string]any{
		"path":       pathToURI(filepath.Join(workspace, "blocked.txt")),
		"dataBase64": base64.StdEncoding.EncodeToString([]byte("must not land")),
	})
	if _, rpcErr := server.handle(context.Background(), "fs/writeFile", params); rpcErr == nil || !strings.Contains(rpcErr.Message, "read-only") {
		t.Fatalf("write result = %v, want read-only error", rpcErr)
	}
	if _, err := os.Stat(filepath.Join(target, "blocked.txt")); !os.IsNotExist(err) {
		t.Fatalf("read-only write reached target: %v", err)
	}
	processParams, _ := json.Marshal(map[string]any{
		"processId": "blocked", "argv": []string{"/bin/sh", "-lc", "touch blocked"},
		"cwd": pathToURI(workspace),
	})
	if _, rpcErr := server.handle(context.Background(), "process/start", processParams); rpcErr == nil || !strings.Contains(rpcErr.Message, "read-only") {
		t.Fatalf("process result = %v, want read-only error", rpcErr)
	}
}

func TestExecServerStreamsTargetProcessOutput(t *testing.T) {
	server, workspace, target := newLocalExecServer(t, false)
	if err := os.WriteFile(filepath.Join(target, "TARGET_ONLY"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var protocol bytes.Buffer
	server.out = &protocol
	params, _ := json.Marshal(map[string]any{
		"processId": "p1", "argv": []string{"/bin/sh", "-lc", "pwd; ls"},
		"cwd": pathToURI(workspace),
	})
	if _, rpcErr := server.handleProcessStart(context.Background(), params); rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	server.mu.Lock()
	process := server.processes["p1"]
	server.mu.Unlock()
	select {
	case <-process.done:
	case <-time.After(5 * time.Second):
		t.Fatal("target process did not close")
	}
	frames := protocol.String()
	if !strings.Contains(frames, "process/output") {
		t.Fatalf("protocol did not push process output: %s", frames)
	}
	decodedFrames := decodeProtocolChunks(t, frames)
	if !strings.Contains(decodedFrames, target) || !strings.Contains(decodedFrames, "TARGET_ONLY") {
		t.Fatalf("command did not run in target: %q", decodedFrames)
	}
	if strings.Contains(decodedFrames, workspace) {
		t.Fatalf("command exposed the local workspace: %q", decodedFrames)
	}
}

func TestEnvironmentsTOMLDisablesLocalFallback(t *testing.T) {
	document := EnvironmentsTOML(`/tmp/onecatch "shell"`, "run-1")
	for _, expected := range []string{
		`include_local = false`, `args = ["exec-server"]`,
		`ONECATCH_SEAM_SESSION = "run-1"`, `program = "/tmp/onecatch \"shell\""`,
	} {
		if !strings.Contains(document, expected) {
			t.Errorf("environment document missing %q:\n%s", expected, document)
		}
	}
}

func localPathURI(value string) string { return pathToURI(value) }

func decodeProtocolChunks(t *testing.T, frames string) string {
	t.Helper()
	var result strings.Builder
	for _, line := range strings.Split(frames, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var frame struct {
			Params struct {
				Chunk string `json:"chunk"`
			} `json:"params"`
		}
		if json.Unmarshal([]byte(line), &frame) != nil || frame.Params.Chunk == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(frame.Params.Chunk)
		if err != nil {
			t.Fatal(err)
		}
		result.Write(data)
	}
	return result.String()
}
