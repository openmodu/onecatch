package worker

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openmodu/oneshot/internal/gitinspect"
)

func TestPrepareWorkspaceClonesUpdatesAndPersistsMapping(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	remote := filepath.Join(root, "state", "projects", "project-1")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "init")
	runGit(t, source, "config", "user.email", "worker-test@example.com")
	runGit(t, source, "config", "user.name", "Worker Test")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "add", "README.md")
	runGit(t, source, "commit", "-m", "initial")
	firstRevision := strings.TrimSpace(runGit(t, source, "rev-parse", "HEAD"))

	registryPath := filepath.Join(root, "state", "workspaces.json")
	service := NewServer("remote-1", "Remote", "secret", nil, fakeEngine{}, 1)
	if err := service.SetWorkspaceRegistry(context.Background(), NewWorkspaceRegistry(registryPath)); err != nil {
		t.Fatal(err)
	}
	service.SetGitInspector(gitinspect.New(""))
	server := httptest.NewServer(service.Handler())
	client := NewClient()
	config := Config{ID: "remote-1", BaseURL: server.URL, Token: "secret", Enabled: true}

	prepared, err := client.PrepareWorkspace(context.Background(), config, "project-1", WorkspacePrepareRequest{
		RemoteURL: source, Revision: firstRevision,
	})
	if err != nil || prepared.Mapping.Path != remote || prepared.Git.Head != firstRevision {
		t.Fatalf("first prepare = %+v, %v", prepared, err)
	}
	if _, err := client.PrepareWorkspace(context.Background(), config, "project-1", WorkspacePrepareRequest{
		RemoteURL: filepath.Join(root, "different-origin"), Revision: firstRevision,
	}); err == nil || !strings.Contains(err.Error(), "worker_workspace_remote_mismatch") {
		t.Fatalf("remote mismatch error = %v", err)
	}
	server.Close()

	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "add", "README.md")
	runGit(t, source, "commit", "-m", "second")
	secondRevision := strings.TrimSpace(runGit(t, source, "rev-parse", "HEAD"))

	restarted := NewServer("remote-1", "Remote", "secret", nil, fakeEngine{}, 1)
	if err := restarted.SetWorkspaceRegistry(context.Background(), NewWorkspaceRegistry(registryPath)); err != nil {
		t.Fatal(err)
	}
	restarted.SetGitInspector(gitinspect.New(""))
	restartedHTTP := httptest.NewServer(restarted.Handler())
	defer restartedHTTP.Close()
	config.BaseURL = restartedHTTP.URL

	snapshot, err := client.GitStatus(context.Background(), config, "project-1")
	if err != nil || snapshot.Head != firstRevision {
		t.Fatalf("persisted mapping status = %+v, %v", snapshot, err)
	}
	prepared, err = client.PrepareWorkspace(context.Background(), config, "project-1", WorkspacePrepareRequest{
		RemoteURL: source, Revision: secondRevision,
	})
	if err != nil || prepared.Git.Head != secondRevision {
		t.Fatalf("updated prepare = %+v, %v", prepared, err)
	}
}

func TestPrepareWorkspaceFailedCloneLeavesNoPartialTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "project")
	remoteErr := prepareGitWorkspace(context.Background(), target, filepath.Join(root, "missing-origin"), strings.Repeat("0", 40))
	if remoteErr == nil || remoteErr.Code != "worker_workspace_clone_failed" {
		t.Fatalf("clone error = %+v", remoteErr)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("partial target remained after failed clone: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary clone entries remained: %+v", entries)
	}
}
