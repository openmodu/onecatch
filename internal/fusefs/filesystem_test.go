package fusefs

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/openmodu/oneshot/internal/remotefs"
	"github.com/pkg/sftp"
)

func TestNodeCreateReadWriteRenameAndSymlink(t *testing.T) {
	t.Parallel()
	backing := t.TempDir()
	backend := &localBackend{root: backing}
	root := New(remotefs.NewCachedBackend(backend, time.Minute), time.Second)
	raw := fs.NewNodeFS(root, nil)
	defer raw.OnUnmount()
	ctx := context.Background()

	var created fuse.EntryOut
	createdInode, handleValue, _, errno := root.Create(ctx, "main.go", uint32(os.O_RDWR), 0o640, &created)
	if errno != 0 {
		t.Fatalf("Create: %v", errno)
	}
	if !root.AddChild("main.go", createdInode, false) {
		t.Fatal("attach created inode")
	}
	handle := handleValue.(*fileHandle)
	if _, errno := handle.Write(ctx, []byte("hello remote"), 0); errno != 0 {
		t.Fatalf("Write: %v", errno)
	}
	buffer := make([]byte, 12)
	result, errno := handle.Read(ctx, buffer, 0)
	if errno != 0 {
		t.Fatalf("Read: %v", errno)
	}
	data, status := result.Bytes(nil)
	result.Done()
	if status != fuse.OK {
		t.Fatalf("Read result status: %v", status)
	}
	if string(data) != "hello remote" {
		t.Fatalf("read = %q", data)
	}
	if errno := handle.Flush(ctx); errno != 0 {
		t.Fatalf("Flush: %v", errno)
	}
	if errno := handle.Release(ctx); errno != 0 {
		t.Fatalf("Release: %v", errno)
	}

	if errno := root.Rename(ctx, "main.go", root, "renamed.go", 0); errno != 0 {
		t.Fatalf("Rename: %v", errno)
	}
	if !root.MvChild("main.go", &root.Inode, "renamed.go", true) {
		t.Fatal("move renamed inode")
	}
	if got, err := os.ReadFile(filepath.Join(backing, "renamed.go")); err != nil || string(got) != "hello remote" {
		t.Fatalf("backing file = %q, %v", got, err)
	}
	renamed := createdInode.Operations().(*Node)
	renamedHandleValue, _, errno := renamed.Open(ctx, uint32(os.O_RDONLY))
	if errno != 0 {
		t.Fatalf("open renamed inode: %v", errno)
	}
	renamedHandle := renamedHandleValue.(*fileHandle)
	buffer = make([]byte, 12)
	result, errno = renamedHandle.Read(ctx, buffer, 0)
	if errno != 0 {
		t.Fatalf("read renamed inode: %v", errno)
	}
	data, status = result.Bytes(nil)
	result.Done()
	if status != fuse.OK || string(data) != "hello remote" {
		t.Fatalf("renamed inode read = %q, status=%v", data, status)
	}
	if errno := renamedHandle.Release(ctx); errno != 0 {
		t.Fatalf("release renamed inode: %v", errno)
	}

	var symlinkOut fuse.EntryOut
	linkNode, errno := root.Symlink(ctx, "renamed.go", "current.go", &symlinkOut)
	if errno != 0 {
		t.Fatalf("Symlink: %v", errno)
	}
	if !root.AddChild("current.go", linkNode, false) {
		t.Fatal("attach symlink inode")
	}
	target, errno := linkNode.Operations().(*Node).Readlink(ctx)
	if errno != 0 || string(target) != "renamed.go" {
		t.Fatalf("Readlink = %q, %v", target, errno)
	}
	if errno := root.Unlink(ctx, "current.go"); errno != 0 {
		t.Fatalf("Unlink symlink: %v", errno)
	}
	if errno := root.Unlink(ctx, "renamed.go"); errno != 0 {
		t.Fatalf("Unlink file: %v", errno)
	}
}

func TestNodeRejectsInvalidChildNames(t *testing.T) {
	t.Parallel()
	root := New(&localBackend{root: t.TempDir()}, time.Second)
	raw := fs.NewNodeFS(root, nil)
	defer raw.OnUnmount()
	for _, name := range []string{"", ".", "..", "dir/file"} {
		var out fuse.EntryOut
		if _, errno := root.Lookup(context.Background(), name, &out); errno != syscall.EINVAL {
			t.Fatalf("Lookup(%q) errno = %v, want EINVAL", name, errno)
		}
	}
}

func TestStatfsFallbackForUnsupportedServer(t *testing.T) {
	t.Parallel()
	root := New(&unsupportedStatBackend{localBackend: localBackend{root: t.TempDir()}}, time.Second)
	var out fuse.StatfsOut
	if errno := root.Statfs(context.Background(), &out); errno != 0 {
		t.Fatalf("Statfs: %v", errno)
	}
	if out.Bsize != 4096 || out.NameLen != 255 {
		t.Fatalf("unexpected fallback statfs: %#v", out)
	}
}

type localBackend struct {
	root string
}

func (b *localBackend) local(name string) string {
	return filepath.Join(b.root, filepath.FromSlash(name))
}
func (b *localBackend) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(b.local(name))
}
func (b *localBackend) ReadDir(name string) ([]os.FileInfo, error) {
	entries, err := os.ReadDir(b.local(name))
	if err != nil {
		return nil, err
	}
	result := make([]os.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, nil
}
func (b *localBackend) OpenFile(name string, flags int, mode os.FileMode) (remotefs.File, error) {
	return os.OpenFile(b.local(name), flags, mode)
}
func (b *localBackend) Readlink(name string) (string, error) {
	return os.Readlink(b.local(name))
}
func (b *localBackend) Symlink(target, name string) error {
	return os.Symlink(target, b.local(name))
}
func (b *localBackend) Mkdir(name string, mode os.FileMode) error {
	return os.Mkdir(b.local(name), mode)
}
func (b *localBackend) Remove(name string) error {
	return os.Remove(b.local(name))
}
func (b *localBackend) RemoveDirectory(name string) error {
	return os.Remove(b.local(name))
}
func (b *localBackend) Rename(oldName, newName string) error {
	return os.Rename(b.local(oldName), b.local(newName))
}
func (b *localBackend) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(b.local(name), mode)
}
func (b *localBackend) Chown(name string, uid, gid int) error {
	return os.Chown(b.local(name), uid, gid)
}
func (b *localBackend) Chtimes(name string, atime, mtime time.Time) error {
	return os.Chtimes(b.local(name), atime, mtime)
}
func (b *localBackend) Truncate(name string, size int64) error {
	return os.Truncate(b.local(name), size)
}
func (b *localBackend) StatFS(string) (remotefs.StatFS, error) {
	return remotefs.StatFS{
		BlockSize:       4096,
		Blocks:          1024,
		BlocksFree:      512,
		BlocksAvailable: 500,
		Files:           100,
		FilesFree:       50,
		NameMax:         255,
	}, nil
}
func (b *localBackend) Close() error { return nil }

type unsupportedStatBackend struct {
	localBackend
}

func (b *unsupportedStatBackend) StatFS(string) (remotefs.StatFS, error) {
	return remotefs.StatFS{}, &sftp.StatusError{Code: uint32(sftp.ErrSSHFxOpUnsupported)}
}
