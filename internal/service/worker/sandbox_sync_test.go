package worker

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"github.com/openmodu/onecatch/pkg/localfile"
)

type helperPipe struct {
	io.Reader
	io.WriteCloser
	cmd *exec.Cmd
}

func (p *helperPipe) Close() error {
	_ = p.WriteCloser.Close()
	_ = p.cmd.Process.Kill()
	return p.cmd.Wait()
}
func localSandboxFiles(t *testing.T, dir string) *sandboxFiles {
	t.Helper()
	dir, _ = filepath.EvalSymlinks(dir)
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3 is required for sandbox helper tests")
	}
	cmd := exec.Command(python, "-u", "-c", sandboxFileHelper)
	input, _ := cmd.StdinPipe()
	output, _ := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pipe := &helperPipe{output, input, cmd}
	t.Cleanup(func() { _ = pipe.Close() })
	f := newSandboxFiles(pipe)
	if _, err = f.call(map[string]any{"op": "init", "root": dir, "blocked": syncBlocked}); err != nil {
		t.Fatal(err)
	}
	return f
}
func writeSyncFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
func TestSyncLocalSelectionAndSafety(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{".gitignore": "ignored/\n", "main.go": "local dirty source", "nested/a.txt": "ok", "ignored/large": "ignored", ".env": "secret", ".aws/config": "secret", "node_modules/dependency": "large", "a.pem": "private", ".git/config": "private"} {
		writeSyncFixture(t, dir, name, content)
	}
	_ = os.Symlink("/etc/passwd", filepath.Join(dir, "outside"))
	root, _ := os.OpenRoot(dir)
	defer root.Close()
	manifest, err := scanSyncLocal(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 3 || manifest["main.go"].Hash != digestSync([]byte("local dirty source")) {
		t.Fatalf("bad selection: %v", manifest)
	}
	for _, name := range []string{"../escape", "/absolute", "a/../../escape", "a\\b", ".git/config", "nested/.env.local", "nested/private.key"} {
		if syncAllowed(name) {
			t.Errorf("unsafe path accepted: %s", name)
		}
	}
	if _, _, err := syncFileAt(root, "outside"); err == nil {
		t.Fatal("followed outside symlink")
	}
}
func TestSandboxFileRPCIntegrityAndIncrementalTransfer(t *testing.T) {
	dir := t.TempDir()
	f := localSandboxFiles(t, dir)
	data := []byte(strings.Repeat("binary\x00世界\n", 100000))
	file := syncFile{digestSync(data), int64(len(data)), true}
	if err := f.upload("src/data.bin", data, file, nil); err != nil {
		t.Fatal(err)
	}
	manifest, err := f.manifest(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 1 || manifest["src/data.bin"] != file {
		t.Fatalf("manifest mismatch: %v", manifest)
	}
	got, err := f.download("src/data.bin", file)
	if err != nil || string(got) != string(data) {
		t.Fatalf("binary roundtrip failed: %v", err)
	}
	// CAS protects changes made after the last manifest.
	if _, err := f.call(map[string]any{"op": "delete", "path": "src/data.bin", "before": nil}); err == nil {
		t.Fatal("deleted changed file")
	}
	if _, err := f.call(map[string]any{"op": "delete", "path": "src/data.bin", "before": file}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "src/data.bin")); !os.IsNotExist(err) {
		t.Fatal("delete not applied")
	}
	for _, name := range []string{"../escape", "/tmp/escape", ".env", "a/private.pem"} {
		if _, err := f.call(map[string]any{"op": "get", "path": name, "offset": 0}); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
	outside := t.TempDir()
	_ = os.Symlink(outside, filepath.Join(dir, "link"))
	if err := f.upload("link/escape", nil, syncFile{digestSync(nil), 0, false}, nil); err == nil {
		t.Fatal("uploaded through symlink")
	}
}
func TestSandboxPullPreservesConcurrentLocalEdits(t *testing.T) {
	localDir, remoteDir, stateDir := t.TempDir(), t.TempDir(), t.TempDir()
	for _, dir := range []string{localDir, remoteDir} {
		for _, name := range []string{"same.txt", "remote.txt", "delete.txt", "local.txt"} {
			writeSyncFixture(t, dir, name, "base")
		}
	}
	root, _ := os.OpenRoot(localDir)
	defer root.Close()
	base, err := scanSyncLocal(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := syncState{Files: base}
	statePath := filepath.Join(stateDir, "state.json")
	writeSyncFixture(t, localDir, "same.txt", "local concurrent edit")
	writeSyncFixture(t, localDir, "local.txt", "local only")
	writeSyncFixture(t, remoteDir, "same.txt", "remote concurrent edit")
	writeSyncFixture(t, remoteDir, "remote.txt", "remote only")
	writeSyncFixture(t, remoteDir, "new.txt", "new remote")
	_ = os.Remove(filepath.Join(remoteDir, "delete.txt"))
	f := localSandboxFiles(t, remoteDir)
	remote, err := f.manifest(base)
	if err != nil {
		t.Fatal(err)
	}
	err = pullSandboxFiles(root, f, &state, remote, stateDir, statePath)
	if err == nil || !strings.Contains(err.Error(), "同步冲突") {
		t.Fatalf("expected conflict: %v", err)
	}
	for name, want := range map[string]string{"same.txt": "local concurrent edit", "local.txt": "local only", "remote.txt": "remote only", "new.txt": "new remote"} {
		got, _ := os.ReadFile(filepath.Join(localDir, name))
		if string(got) != want {
			t.Errorf("%s: %q", name, got)
		}
	}
	if _, err := os.Stat(filepath.Join(localDir, "delete.txt")); !os.IsNotExist(err) {
		t.Fatal("remote deletion missing")
	}
	backups, _ := filepath.Glob(filepath.Join(stateDir, "conflicts", "*", "same.txt.remote"))
	if len(backups) != 1 {
		t.Fatal("remote conflict not preserved")
	}
	got, _ := os.ReadFile(backups[0])
	if string(got) != "remote concurrent edit" {
		t.Fatal("wrong recovery content")
	}
	var persisted syncState
	err = localfile.ReadJSON(statePath, &persisted)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Files["same.txt"].Hash != remote["same.txt"].Hash {
		t.Fatal("recovery base not persisted")
	}
	// Retrying after reviewing the recovery copy keeps local changes for upload.
	if err = pullSandboxFiles(root, f, &persisted, remote, stateDir, statePath); err != nil {
		t.Fatal(err)
	}
}
func TestSyncApplyRejectsLocalChange(t *testing.T) {
	dir := t.TempDir()
	writeSyncFixture(t, dir, "file", "local")
	root, _ := os.OpenRoot(dir)
	defer root.Close()
	base := syncFile{digestSync([]byte("old")), 3, false}
	next := syncFile{digestSync([]byte("remote")), 6, false}
	if err := applySyncFile(root, "file", &base, &next, []byte("remote")); err == nil {
		t.Fatal("overwrote concurrent edit")
	}
	if err := applySyncFile(root, "../outside", nil, &next, []byte("remote")); err == nil {
		t.Fatal("accepted traversal")
	}
}
func TestSandboxSyncLock(t *testing.T) {
	unlock, err := takeSandboxSyncLock(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = takeSandboxSyncLock(t.Name()); err == nil {
		t.Fatal("concurrent run allowed")
	}
	unlock()
	unlock, err = takeSandboxSyncLock(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}
func TestSandboxLiveFileSync(t *testing.T) {
	endpoint := os.Getenv("ONECATCH_TEST_SANDBOX_URL")
	if endpoint == "" {
		t.Skip("live file sync opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	input := Input{Provider: ProviderVolcengineSandbox, BaseURL: endpoint, RemotePath: "/home/gem"}
	if err := normalizeSandboxInput(&input); err != nil {
		t.Fatal(err)
	}
	config := Config{BaseURL: input.BaseURL, Token: input.Token, RemotePath: input.RemotePath}
	client := NewClient()
	process := "python3 -u -c " + sandboxQuote("import base64;exec(base64.b64decode("+fmt.Sprintf("%q", base64.StdEncoding.EncodeToString([]byte(sandboxFileHelper)))+"))")
	stream, err := client.openSandboxProcess(ctx, config, process)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	f := newSandboxFiles(stream)
	dir := fmt.Sprintf("/home/gem/.onecatch-sync-test-%d", time.Now().UnixNano())
	if _, err = f.call(map[string]any{"op": "init", "root": dir, "blocked": syncBlocked}); err != nil {
		t.Fatal(err)
	}
	// Only synthetic fixture data crosses the network.
	data := []byte(strings.Repeat("onecatch file sync smoke test\n", 20000))
	item := syncFile{digestSync(data), int64(len(data)), false}
	if err = f.upload("fixture.txt", data, item, nil); err != nil {
		t.Fatal(err)
	}
	manifest, err := f.manifest(nil)
	if err != nil || manifest["fixture.txt"] != item {
		t.Fatalf("manifest failed: %v", err)
	}
	got, err := f.download("fixture.txt", item)
	if err != nil || string(got) != string(data) {
		t.Fatalf("download failed: %v", err)
	}
	if _, err = f.call(map[string]any{"op": "delete", "path": "fixture.txt", "before": item}); err != nil {
		t.Fatal(err)
	}
	t.Log("live upload, manifest, download and delete passed")
}

func TestSandboxLiveSyncedTurn(t *testing.T) {
	endpoint := os.Getenv("ONECATCH_TEST_SANDBOX_URL")
	if endpoint == "" || os.Getenv("ONECATCH_TEST_SANDBOX_TURN") != "1" {
		t.Skip("live synced model turn opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	input := Input{Provider: ProviderVolcengineSandbox, BaseURL: endpoint, RemotePath: "/home/gem"}
	if err := normalizeSandboxInput(&input); err != nil {
		t.Fatal(err)
	}
	config := Config{ID: "sync-smoke", BaseURL: input.BaseURL, Token: input.Token, RemotePath: input.RemotePath, Provider: input.Provider, SyncLocal: true}
	local, states := t.TempDir(), t.TempDir()
	writeSyncFixture(t, local, "hello.txt", "local-first\n")
	writeSyncFixture(t, local, ".env", "synthetic-secret-do-not-transfer\n")
	client := NewClient()
	var syncEvents []string
	sink := func(event agentrun.Event) {
		if event.Kind == agentrun.KindToolResult && strings.Contains(event.Text, "已同步") {
			syncEvents = append(syncEvents, event.Text)
		}
	}
	first, err := client.RunSandboxSynced(ctx, config, agentrun.Request{Runtime: agentrun.RuntimeCodex, Workspace: local, Sandbox: agentrun.SandboxWorkspaceWrite, Prompt: "This is a synthetic file sync smoke test. Read hello.txt in the current directory; it must contain local-first. Verify that .env is absent. Replace hello.txt with exactly remote-first followed by a newline. Create new.txt containing exactly created followed by a newline. Do not touch any other directories. Reply DONE."}, states, sink)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Succeeded {
		t.Fatal("first turn failed")
	}
	data, _ := os.ReadFile(filepath.Join(local, "hello.txt"))
	if string(data) != "remote-first\n" {
		t.Fatalf("remote edit did not return: %q", data)
	}
	data, _ = os.ReadFile(filepath.Join(local, "new.txt"))
	if string(data) != "created\n" {
		t.Fatalf("new remote file did not return: %q", data)
	}
	writeSyncFixture(t, local, "hello.txt", "local-second\n")
	second, err := client.RunSandboxSynced(ctx, config, agentrun.Request{Runtime: agentrun.RuntimeCodex, Workspace: local, Sandbox: agentrun.SandboxWorkspaceWrite, ResumeSessionID: first.SessionID, Prompt: "Read hello.txt again; it was edited locally and must now contain local-second. Replace it with exactly remote-second followed by a newline. Delete new.txt. Do not touch other directories. Reply DONE."}, states, sink)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Succeeded || second.SessionID != first.SessionID {
		t.Fatal("continuation failed")
	}
	data, _ = os.ReadFile(filepath.Join(local, "hello.txt"))
	if string(data) != "remote-second\n" {
		t.Fatalf("second edit did not return: %q", data)
	}
	if _, err := os.Stat(filepath.Join(local, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("remote deletion did not return")
	}
	if len(syncEvents) != 2 || !strings.Contains(syncEvents[1], "1 个文件") {
		t.Fatalf("expected one-file incremental upload: %v", syncEvents)
	}
	t.Logf("two synced turns passed: %v", syncEvents)
}

func TestSandboxReplicaGenerationAndSymlinkReplacement(t *testing.T) {
	dir := t.TempDir()
	f := localSandboxFiles(t, dir)
	marker, err := os.ReadFile(filepath.Join(dir, ".onecatch-sync-generation"))
	if err != nil || len(marker) != 32 {
		t.Fatalf("generation missing: %v", err)
	}
	data := []byte("source")
	item := syncFile{digestSync(data), int64(len(data)), false}
	if err = f.upload("file", data, item, nil); err != nil {
		t.Fatal(err)
	}
	base := syncManifest{"file": item}
	_ = os.Remove(filepath.Join(dir, "file"))
	_ = os.Symlink("/etc/passwd", filepath.Join(dir, "file"))
	if _, err = f.manifest(base); err == nil {
		t.Fatal("symlink replacement interpreted as deletion")
	}
	// A reconstructed replica always receives a different generation.
	other := t.TempDir()
	_ = localSandboxFiles(t, other)
	next, _ := os.ReadFile(filepath.Join(other, ".onecatch-sync-generation"))
	if string(next) == string(marker) {
		t.Fatal("replica identity reused")
	}
}
func TestSandboxSyncPreferencePersists(t *testing.T) {
	registry := NewRegistry(filepath.Join(t.TempDir(), "workers.json"))
	input := Input{ID: "sandbox-sync", Name: "sync", Provider: ProviderVolcengineSandbox, BaseURL: "https://test.volceapi.com/?faasInstanceName=test", Token: "fixture", Enabled: true, SyncLocal: true}
	info, err := registry.Save(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !info.SyncLocal {
		t.Fatal("public worker lost sync preference")
	}
	config, err := registry.Get(context.Background(), info.ID)
	if err != nil || !config.SyncLocal {
		t.Fatal("sync preference not persisted")
	}
	info, err = registry.Update(context.Background(), UpdateInput{ID: info.ID, Name: info.Name, BaseURL: info.BaseURL, Provider: info.Provider, Enabled: true, SyncLocal: false})
	if err != nil || info.SyncLocal {
		t.Fatalf("disable sync failed: %v", err)
	}
}

func TestSandboxBatchUploadAndStaleWrite(t *testing.T) {
	dir := t.TempDir()
	f := localSandboxFiles(t, dir)
	var batch []map[string]any
	for _, name := range []string{"one.txt", "nested/two.txt", "empty"} {
		data := []byte(name)
		if name == "empty" {
			data = []byte{}
		}
		batch = append(batch, map[string]any{"path": name, "data": data, "file": syncFile{digestSync(data), int64(len(data)), false}, "before": nil})
	}
	if _, err := f.call(map[string]any{"op": "batch", "uploads": batch}); err != nil {
		t.Fatal(err)
	}
	manifest, err := f.manifest(nil)
	if err != nil || len(manifest) != 3 {
		t.Fatalf("batch not applied: %v", err)
	}
	// Replaying a stale write is rejected, preserving remote changes.
	writeSyncFixture(t, dir, "one.txt", "edited remotely")
	if _, err := f.call(map[string]any{"op": "batch", "uploads": batch}); err == nil {
		t.Fatal("stale batch overwrote remote edit")
	}
	got, _ := os.ReadFile(filepath.Join(dir, "one.txt"))
	if string(got) != "edited remotely" {
		t.Fatal("remote edit lost")
	}
}
