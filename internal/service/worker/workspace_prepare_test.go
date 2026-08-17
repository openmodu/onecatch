package worker

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openmodu/onecatch/internal/repo/git"
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
	service.SetGitInspector(gitrepo.New(""))
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
	restarted.SetGitInspector(gitrepo.New(""))
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

func TestBindExistingWorkspaceAndRemoveMapping(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "existing")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, existing, "init")
	runGit(t, existing, "config", "user.email", "worker-test@example.com")
	runGit(t, existing, "config", "user.name", "Worker Test")
	if err := os.WriteFile(filepath.Join(existing, "README.md"), []byte("bound\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, existing, "add", "README.md")
	runGit(t, existing, "commit", "-m", "initial")
	head := strings.TrimSpace(runGit(t, existing, "rev-parse", "HEAD"))

	service := NewServer("remote-1", "Remote", "secret", nil, fakeEngine{}, 1)
	if err := service.SetWorkspaceRegistry(context.Background(), NewWorkspaceRegistry(filepath.Join(root, "state", "workspaces.json"))); err != nil {
		t.Fatal(err)
	}
	service.SetGitInspector(gitrepo.New(""))
	httpServer := httptest.NewServer(service.Handler())
	defer httpServer.Close()
	client := NewClient()
	config := Config{ID: "remote-1", BaseURL: httpServer.URL, Token: "secret", Enabled: true}

	prepared, err := client.PrepareWorkspace(context.Background(), config, "existing-project", WorkspacePrepareRequest{
		Name: "Existing Project", Path: existing,
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Mapping.Name != "Existing Project" || prepared.Mapping.Managed || prepared.Mapping.Revision != head {
		t.Fatalf("prepared mapping = %+v", prepared.Mapping)
	}
	items, err := client.ListWorkspaces(context.Background(), config)
	if err != nil || len(items) != 1 || items[0].Path != existing || items[0].Name != "Existing Project" {
		t.Fatalf("workspace list = %+v, %v", items, err)
	}
	if err := client.RemoveWorkspace(context.Background(), config, "existing-project", true); err == nil || !strings.Contains(err.Error(), "worker_workspace_delete_forbidden") {
		t.Fatalf("delete existing files error = %v", err)
	}
	if err := client.RemoveWorkspace(context.Background(), config, "existing-project", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(existing); err != nil {
		t.Fatalf("unmanaged workspace was removed: %v", err)
	}
	items, err = client.ListWorkspaces(context.Background(), config)
	if err != nil || len(items) != 0 {
		t.Fatalf("workspace list after remove = %+v, %v", items, err)
	}
}

func TestRemoveManagedWorkspaceCanDeleteCleanClone(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "init")
	runGit(t, source, "config", "user.email", "worker-test@example.com")
	runGit(t, source, "config", "user.name", "Worker Test")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("managed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "add", "README.md")
	runGit(t, source, "commit", "-m", "initial")

	registry := NewWorkspaceRegistry(filepath.Join(root, "state", "workspaces.json"))
	service := NewServer("remote-1", "Remote", "secret", nil, fakeEngine{}, 1)
	if err := service.SetWorkspaceRegistry(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(service.Handler())
	defer httpServer.Close()
	client := NewClient()
	config := Config{ID: "remote-1", BaseURL: httpServer.URL, Token: "secret", Enabled: true}

	prepared, err := client.PrepareWorkspace(context.Background(), config, "managed-project", WorkspacePrepareRequest{RemoteURL: source, Revision: "HEAD"})
	if err != nil || !prepared.Mapping.Managed {
		t.Fatalf("prepared mapping = %+v, %v", prepared.Mapping, err)
	}
	if err := client.RemoveWorkspace(context.Background(), config, "managed-project", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(registry.DefaultPath("managed-project")); !os.IsNotExist(err) {
		t.Fatalf("managed clone still exists: %v", err)
	}
}

func TestWorkspacePathsCannotOverlap(t *testing.T) {
	root := t.TempDir()
	if !pathsOverlap(filepath.Join(root, "project"), filepath.Join(root, "project")) {
		t.Fatal("equal workspace paths should overlap")
	}
	if !pathsOverlap(filepath.Join(root, "project"), filepath.Join(root, "project", "nested")) {
		t.Fatal("nested workspace paths should overlap")
	}
	if pathsOverlap(filepath.Join(root, "project-a"), filepath.Join(root, "project-b")) {
		t.Fatal("sibling workspace paths should not overlap")
	}
}
