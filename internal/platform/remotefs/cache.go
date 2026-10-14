package remotefs

import (
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

type cacheEntry[T any] struct {
	value     T
	expiresAt time.Time
}

// CachedBackend caches metadata and directory listings. File contents remain
// remote and are never copied into a persistent local mirror.
type CachedBackend struct {
	Backend
	ttl time.Duration
	now func() time.Time

	mu    sync.RWMutex
	stats map[string]cacheEntry[os.FileInfo]
	dirs  map[string]cacheEntry[[]os.FileInfo]
}

func NewCachedBackend(backend Backend, ttl time.Duration) *CachedBackend {
	return newCachedBackend(backend, ttl, time.Now)
}

func newCachedBackend(backend Backend, ttl time.Duration, now func() time.Time) *CachedBackend {
	return &CachedBackend{
		Backend: backend,
		ttl:     ttl,
		now:     now,
		stats:   make(map[string]cacheEntry[os.FileInfo]),
		dirs:    make(map[string]cacheEntry[[]os.FileInfo]),
	}
}

func (b *CachedBackend) Lstat(name string) (os.FileInfo, error) {
	if b.ttl > 0 {
		b.mu.RLock()
		entry, ok := b.stats[name]
		b.mu.RUnlock()
		if ok && b.now().Before(entry.expiresAt) {
			return entry.value, nil
		}
	}
	info, err := b.Backend.Lstat(name)
	if err != nil || b.ttl <= 0 {
		return info, err
	}
	b.mu.Lock()
	b.stats[name] = cacheEntry[os.FileInfo]{value: info, expiresAt: b.now().Add(b.ttl)}
	b.mu.Unlock()
	return info, nil
}

func (b *CachedBackend) ReadDir(name string) ([]os.FileInfo, error) {
	if b.ttl > 0 {
		b.mu.RLock()
		entry, ok := b.dirs[name]
		b.mu.RUnlock()
		if ok && b.now().Before(entry.expiresAt) {
			return append([]os.FileInfo(nil), entry.value...), nil
		}
	}
	entries, err := b.Backend.ReadDir(name)
	if err != nil || b.ttl <= 0 {
		return entries, err
	}
	b.mu.Lock()
	b.dirs[name] = cacheEntry[[]os.FileInfo]{
		value:     append([]os.FileInfo(nil), entries...),
		expiresAt: b.now().Add(b.ttl),
	}
	b.mu.Unlock()
	return entries, nil
}

func (b *CachedBackend) OpenFile(name string, flags int, mode os.FileMode) (File, error) {
	file, err := b.Backend.OpenFile(name, flags, mode)
	if err != nil {
		return nil, err
	}
	if flags&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0 {
		b.invalidate(name)
		return &cachedFile{File: file, path: name, backend: b}, nil
	}
	return file, nil
}

func (b *CachedBackend) Symlink(target, name string) error {
	err := b.Backend.Symlink(target, name)
	if err == nil {
		b.invalidate(name)
	}
	return err
}

func (b *CachedBackend) Mkdir(name string, mode os.FileMode) error {
	err := b.Backend.Mkdir(name, mode)
	if err == nil {
		b.invalidate(name)
	}
	return err
}

func (b *CachedBackend) Remove(name string) error {
	err := b.Backend.Remove(name)
	if err == nil {
		b.invalidate(name)
	}
	return err
}

func (b *CachedBackend) RemoveDirectory(name string) error {
	err := b.Backend.RemoveDirectory(name)
	if err == nil {
		b.invalidateTree(name)
	}
	return err
}

func (b *CachedBackend) Rename(oldName, newName string) error {
	err := b.Backend.Rename(oldName, newName)
	if err == nil {
		b.invalidateTree(oldName)
		b.invalidateTree(newName)
	}
	return err
}

func (b *CachedBackend) Chmod(name string, mode os.FileMode) error {
	err := b.Backend.Chmod(name, mode)
	if err == nil {
		b.invalidate(name)
	}
	return err
}

func (b *CachedBackend) Chown(name string, uid, gid int) error {
	err := b.Backend.Chown(name, uid, gid)
	if err == nil {
		b.invalidate(name)
	}
	return err
}

func (b *CachedBackend) Chtimes(name string, atime, mtime time.Time) error {
	err := b.Backend.Chtimes(name, atime, mtime)
	if err == nil {
		b.invalidate(name)
	}
	return err
}

func (b *CachedBackend) Truncate(name string, size int64) error {
	err := b.Backend.Truncate(name, size)
	if err == nil {
		b.invalidate(name)
	}
	return err
}

func (b *CachedBackend) invalidate(name string) {
	b.mu.Lock()
	delete(b.stats, name)
	delete(b.dirs, name)
	delete(b.dirs, parentPath(name))
	b.mu.Unlock()
}

func (b *CachedBackend) invalidateTree(name string) {
	b.mu.Lock()
	prefix := strings.TrimSuffix(name, "/") + "/"
	for key := range b.stats {
		if key == name || strings.HasPrefix(key, prefix) {
			delete(b.stats, key)
		}
	}
	for key := range b.dirs {
		if key == name || strings.HasPrefix(key, prefix) {
			delete(b.dirs, key)
		}
	}
	delete(b.dirs, parentPath(name))
	b.mu.Unlock()
}

func parentPath(name string) string {
	parent := path.Dir(name)
	if parent == "." {
		return ""
	}
	return parent
}

type cachedFile struct {
	File
	path    string
	backend *CachedBackend
}

func (f *cachedFile) WriteAt(data []byte, offset int64) (int, error) {
	n, err := f.File.WriteAt(data, offset)
	if n > 0 {
		f.backend.invalidate(f.path)
	}
	return n, err
}

func (f *cachedFile) Truncate(size int64) error {
	err := f.File.Truncate(size)
	if err == nil {
		f.backend.invalidate(f.path)
	}
	return err
}

func (f *cachedFile) Chmod(mode os.FileMode) error {
	err := f.File.Chmod(mode)
	if err == nil {
		f.backend.invalidate(f.path)
	}
	return err
}

func (f *cachedFile) Chown(uid, gid int) error {
	err := f.File.Chown(uid, gid)
	if err == nil {
		f.backend.invalidate(f.path)
	}
	return err
}
