// Package remotefs defines the file operations needed by the OneCatch remote
// filesystem. Paths passed to a Backend are always slash-separated and
// relative to the configured workspace root.
package remotefs

import (
	"errors"
	"io"
	"os"
	"time"
)

var ErrPathEscape = errors.New("path escapes the remote workspace root")

type File interface {
	io.ReaderAt
	io.WriterAt
	Stat() (os.FileInfo, error)
	Sync() error
	Truncate(size int64) error
	Chmod(mode os.FileMode) error
	Chown(uid, gid int) error
	Close() error
}

type StatFS struct {
	BlockSize       uint64
	Blocks          uint64
	BlocksFree      uint64
	BlocksAvailable uint64
	Files           uint64
	FilesFree       uint64
	NameMax         uint64
}

type Backend interface {
	Lstat(path string) (os.FileInfo, error)
	ReadDir(path string) ([]os.FileInfo, error)
	OpenFile(path string, flags int, mode os.FileMode) (File, error)
	Readlink(path string) (string, error)
	Symlink(target, path string) error
	Mkdir(path string, mode os.FileMode) error
	Remove(path string) error
	RemoveDirectory(path string) error
	Rename(oldPath, newPath string) error
	Chmod(path string, mode os.FileMode) error
	Chown(path string, uid, gid int) error
	Chtimes(path string, atime, mtime time.Time) error
	Truncate(path string, size int64) error
	StatFS(path string) (StatFS, error)
	Close() error
}
