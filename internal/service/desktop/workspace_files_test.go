package desktop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceFilesListReadAndWrite(t *testing.T) {
	app, _ := newStorageTestApp(t)
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: root})
	if err != nil {
		t.Fatal(err)
	}

	entries, err := app.ListWorkspaceFiles(ctx, workspace.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Path != "docs" || !entries[0].Directory || entries[1].Path != "README.md" {
		t.Fatalf("entries = %+v", entries)
	}

	document, err := app.ReadWorkspaceFile(ctx, workspace.ID, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if document.Content != "first\n" || document.Hash == "" {
		t.Fatalf("document = %+v", document)
	}
	written, err := app.WriteWorkspaceFile(ctx, WriteWorkspaceFileInput{
		WorkspaceID:  workspace.ID,
		Path:         document.Path,
		Content:      "second\n",
		ExpectedHash: document.Hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if written.Content != "second\n" || written.Hash == document.Hash {
		t.Fatalf("written = %+v", written)
	}
	info, err := os.Stat(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestWorkspaceFileWriteRejectsStaleContent(t *testing.T) {
	app, _ := newStorageTestApp(t)
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	document, err := app.ReadWorkspaceFile(ctx, workspace.ID, "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("outside edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = app.WriteWorkspaceFile(ctx, WriteWorkspaceFileInput{
		WorkspaceID:  workspace.ID,
		Path:         document.Path,
		Content:      "editor edit",
		ExpectedHash: document.Hash,
	})
	if errorCode(err) != "workspace_file_conflict" {
		t.Fatalf("error = %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil || string(data) != "outside edit" {
		t.Fatalf("content = %q, error = %v", data, readErr)
	}
}

func TestWorkspaceFilesStayInsideWorkspace(t *testing.T) {
	app, _ := newStorageTestApp(t)
	ctx := context.Background()
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked.txt")); err != nil {
		t.Fatal(err)
	}
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: root})
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"../outside.txt", ".git/config", ".onecatch/attachments/file"} {
		if _, err := app.ReadWorkspaceFile(ctx, workspace.ID, path); errorCode(err) != "workspace_file_invalid_path" {
			t.Fatalf("ReadWorkspaceFile(%q) error = %v", path, err)
		}
	}
	if _, err := app.ReadWorkspaceFile(ctx, workspace.ID, "linked.txt"); errorCode(err) != "workspace_file_invalid_path" {
		t.Fatalf("symlink error = %v", err)
	}
	entries, err := app.ListWorkspaceFiles(ctx, workspace.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("symlink was listed: %+v", entries)
	}
}

func TestWorkspaceFileEditorAcceptsOnlyBoundedUTF8Text(t *testing.T) {
	app, _ := newStorageTestApp(t)
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "binary.dat"), []byte{'a', 0, 'b'}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(strings.Repeat("x", maxWorkspaceFileBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.ReadWorkspaceFile(ctx, workspace.ID, "binary.dat"); errorCode(err) != "workspace_file_not_text" {
		t.Fatalf("binary error = %v", err)
	}
	if _, err := app.ReadWorkspaceFile(ctx, workspace.ID, "large.txt"); errorCode(err) != "workspace_file_too_large" {
		t.Fatalf("large error = %v", err)
	}
	if _, err := app.ReadWorkspaceFile(ctx, workspace.ID, "missing.txt"); errorCode(err) != "workspace_file_not_found" {
		t.Fatalf("missing error = %v", err)
	}
	if _, err := app.ListWorkspaceFiles(context.Background(), workspace.ID, "missing"); errorCode(err) != "workspace_file_not_found" {
		t.Fatalf("directory error = %v", err)
	}
}
