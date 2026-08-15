//go:build darwin

package fusefs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMountedFilesystemSmoke(t *testing.T) {
	helper := "/Library/Filesystems/macfuse.fs/Contents/Resources/mount_macfuse"
	if _, err := os.Stat(helper); err != nil {
		t.Skip("macFUSE is not installed")
	}
	backing := t.TempDir()
	mountPoint := t.TempDir()
	root := New(&localBackend{root: backing}, 50*time.Millisecond)
	server, err := Mount(mountPoint, root, nil)
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	defer func() {
		if err := server.Unmount(); err != nil {
			t.Errorf("Unmount: %v", err)
		}
		server.Wait()
	}()

	mountedFile := filepath.Join(mountPoint, "smoke.txt")
	if err := os.WriteFile(mountedFile, []byte("mounted"), 0o600); err != nil {
		t.Fatalf("write through mount: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(backing, "smoke.txt"))
	if err != nil {
		t.Fatalf("read backing file: %v", err)
	}
	if string(data) != "mounted" {
		t.Fatalf("backing data = %q", data)
	}
}
