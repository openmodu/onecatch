//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareMountPoint(t *testing.T) {
	empty := t.TempDir()
	got, err := prepareMountPoint(empty)
	if err != nil {
		t.Fatalf("prepare empty mount point: %v", err)
	}
	want, err := filepath.Abs(empty)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("mount point = %q, want %q", got, want)
	}

	nonEmpty := t.TempDir()
	if err := os.WriteFile(filepath.Join(nonEmpty, "keep"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareMountPoint(nonEmpty); err == nil {
		t.Fatal("non-empty mount point unexpectedly accepted")
	}

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareMountPoint(file); err == nil {
		t.Fatal("regular file unexpectedly accepted as mount point")
	}
}

func TestSanitizeVolumeName(t *testing.T) {
	tests := map[string]string{
		" Linux, project ": "Linux- project",
		"   ":              "OneCatch Remote",
		"project":          "project",
	}
	for input, want := range tests {
		if got := sanitizeVolumeName(input); got != want {
			t.Errorf("sanitizeVolumeName(%q) = %q, want %q", input, got, want)
		}
	}
}
