package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyTreePreservesInstallUnit(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "onecatch.exe"), []byte("app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "bin", "onecatch-worker.exe"), []byte("worker"), 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "backup")
	if err := copyTree(source, destination); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"onecatch.exe": "app",
		filepath.Join("bin", "onecatch-worker.exe"): "worker",
	} {
		content, err := os.ReadFile(filepath.Join(destination, path))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != want {
			t.Fatalf("%s = %q, want %q", path, content, want)
		}
	}
}
