package remotefs

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"
)

func TestCachedBackendCachesAndExpiresMetadata(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0)
	base := &stubBackend{info: stubInfo{name: "main.go", size: 3, mode: 0o644}}
	cache := newCachedBackend(base, time.Second, func() time.Time { return now })

	if _, err := cache.Lstat("main.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Lstat("main.go"); err != nil {
		t.Fatal(err)
	}
	if base.lstatCalls != 1 {
		t.Fatalf("Lstat calls = %d, want 1", base.lstatCalls)
	}
	now = now.Add(2 * time.Second)
	if _, err := cache.Lstat("main.go"); err != nil {
		t.Fatal(err)
	}
	if base.lstatCalls != 2 {
		t.Fatalf("Lstat calls after expiry = %d, want 2", base.lstatCalls)
	}
}

func TestCachedBackendInvalidatesAfterWrite(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0)
	base := &stubBackend{info: stubInfo{name: "main.go", size: 3, mode: 0o644}}
	cache := newCachedBackend(base, time.Minute, func() time.Time { return now })
	if _, err := cache.Lstat("main.go"); err != nil {
		t.Fatal(err)
	}
	file, err := cache.OpenFile("main.go", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte("new"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Lstat("main.go"); err != nil {
		t.Fatal(err)
	}
	if base.lstatCalls != 2 {
		t.Fatalf("Lstat calls after write = %d, want 2", base.lstatCalls)
	}
}

type stubBackend struct {
	info       os.FileInfo
	lstatCalls int
}

func (b *stubBackend) Lstat(string) (os.FileInfo, error) {
	b.lstatCalls++
	return b.info, nil
}
func (b *stubBackend) ReadDir(string) ([]os.FileInfo, error) { return nil, nil }
func (b *stubBackend) OpenFile(string, int, os.FileMode) (File, error) {
	return &stubFile{info: b.info}, nil
}
func (b *stubBackend) Readlink(string) (string, error)            { return "", nil }
func (b *stubBackend) Symlink(string, string) error               { return nil }
func (b *stubBackend) Mkdir(string, os.FileMode) error            { return nil }
func (b *stubBackend) Remove(string) error                        { return nil }
func (b *stubBackend) RemoveDirectory(string) error               { return nil }
func (b *stubBackend) Rename(string, string) error                { return nil }
func (b *stubBackend) Chmod(string, os.FileMode) error            { return nil }
func (b *stubBackend) Chown(string, int, int) error               { return nil }
func (b *stubBackend) Chtimes(string, time.Time, time.Time) error { return nil }
func (b *stubBackend) Truncate(string, int64) error               { return nil }
func (b *stubBackend) StatFS(string) (StatFS, error)              { return StatFS{}, nil }
func (b *stubBackend) Close() error                               { return nil }

type stubFile struct {
	data bytes.Buffer
	info os.FileInfo
}

func (f *stubFile) ReadAt(buffer []byte, offset int64) (int, error) {
	data := f.data.Bytes()
	if offset >= int64(len(data)) {
		return 0, io.EOF
	}
	return copy(buffer, data[offset:]), nil
}
func (f *stubFile) WriteAt(data []byte, offset int64) (int, error) {
	current := f.data.Bytes()
	required := int(offset) + len(data)
	next := make([]byte, required)
	copy(next, current)
	copy(next[offset:], data)
	f.data.Reset()
	_, _ = f.data.Write(next)
	return len(data), nil
}
func (f *stubFile) Stat() (os.FileInfo, error) { return f.info, nil }
func (f *stubFile) Sync() error                { return nil }
func (f *stubFile) Truncate(int64) error       { return nil }
func (f *stubFile) Chmod(os.FileMode) error    { return nil }
func (f *stubFile) Chown(int, int) error       { return nil }
func (f *stubFile) Close() error               { return nil }

type stubInfo struct {
	name string
	size int64
	mode os.FileMode
}

func (i stubInfo) Name() string       { return i.name }
func (i stubInfo) Size() int64        { return i.size }
func (i stubInfo) Mode() os.FileMode  { return i.mode }
func (i stubInfo) ModTime() time.Time { return time.Unix(50, 0) }
func (i stubInfo) IsDir() bool        { return i.mode.IsDir() }
func (i stubInfo) Sys() any           { return nil }
