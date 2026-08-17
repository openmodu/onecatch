// Package fusefs exposes a remotefs.Backend through the FUSE protocol.
package fusefs

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/openmodu/onecatch/internal/remotefs"
	"github.com/pkg/sftp"
)

type FileSystem struct {
	backend remotefs.Backend
	ttl     time.Duration
	uid     uint32
	gid     uint32
}

type Node struct {
	fs.Inode
	fileSystem *FileSystem
	isRoot     bool
}

func New(backend remotefs.Backend, ttl time.Duration) *Node {
	return &Node{
		isRoot: true,
		fileSystem: &FileSystem{
			backend: backend,
			ttl:     ttl,
			uid:     uint32(os.Getuid()),
			gid:     uint32(os.Getgid()),
		},
	}
}

func Mount(mountPoint string, root *Node, options *fs.Options) (*fuse.Server, error) {
	if options == nil {
		ttl := root.fileSystem.ttl
		options = &fs.Options{
			EntryTimeout:    &ttl,
			AttrTimeout:     &ttl,
			NegativeTimeout: &ttl,
			NullPermissions: true,
			UID:             root.fileSystem.uid,
			GID:             root.fileSystem.gid,
			RootStableAttr:  &fs.StableAttr{Mode: fuse.S_IFDIR},
		}
	}
	return fs.Mount(mountPoint, root, options)
}

func (n *Node) childPath(name string) (string, syscall.Errno) {
	if name == "" || name == "." || name == ".." || path.Base(name) != name {
		return "", syscall.EINVAL
	}
	parent := n.remotePath()
	if parent == "" {
		return name, 0
	}
	return path.Join(parent, name), 0
}

func (n *Node) remotePath() string {
	if n.isRoot {
		return ""
	}
	return n.Path(n.Root())
}

func (n *Node) newChild(ctx context.Context, info os.FileInfo) *fs.Inode {
	child := &Node{fileSystem: n.fileSystem}
	return n.NewInode(ctx, child, fs.StableAttr{Mode: fileType(info.Mode())})
}

func (n *Node) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	childPath, errno := n.childPath(name)
	if errno != 0 {
		return nil, errno
	}
	info, err := n.fileSystem.backend.Lstat(childPath)
	if err != nil {
		return nil, toErrno(err)
	}
	fillEntry(out, info, n.fileSystem)
	return n.newChild(ctx, info), 0
}

func (n *Node) Getattr(_ context.Context, handle fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	var (
		info os.FileInfo
		err  error
	)
	if file, ok := handle.(*fileHandle); ok {
		info, err = file.remote.Stat()
	} else {
		info, err = n.fileSystem.backend.Lstat(n.remotePath())
	}
	if err != nil {
		return toErrno(err)
	}
	fillAttr(&out.Attr, info, n.fileSystem)
	out.SetTimeout(n.fileSystem.ttl)
	return 0
}

func (n *Node) Readdir(_ context.Context) (fs.DirStream, syscall.Errno) {
	infos, err := n.fileSystem.backend.ReadDir(n.remotePath())
	if err != nil {
		return nil, toErrno(err)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name() < infos[j].Name() })
	entries := make([]fuse.DirEntry, 0, len(infos))
	for _, info := range infos {
		entries = append(entries, fuse.DirEntry{Name: info.Name(), Mode: fileType(info.Mode())})
	}
	return fs.NewListDirStream(entries), 0
}

func (n *Node) Open(_ context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	file, err := n.fileSystem.backend.OpenFile(n.remotePath(), sanitizeOpenFlags(flags), 0)
	if err != nil {
		return nil, 0, toErrno(err)
	}
	return &fileHandle{remote: file}, 0, 0
}

func (n *Node) Create(
	ctx context.Context,
	name string,
	flags uint32,
	mode uint32,
	out *fuse.EntryOut,
) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	childPath, errno := n.childPath(name)
	if errno != 0 {
		return nil, nil, 0, errno
	}
	file, err := n.fileSystem.backend.OpenFile(
		childPath,
		sanitizeOpenFlags(flags)|os.O_CREATE,
		os.FileMode(mode),
	)
	if err != nil {
		return nil, nil, 0, toErrno(err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, 0, toErrno(err)
	}
	fillEntry(out, info, n.fileSystem)
	return n.newChild(ctx, info), &fileHandle{remote: file}, 0, 0
}

func (n *Node) Mkdir(
	ctx context.Context,
	name string,
	mode uint32,
	out *fuse.EntryOut,
) (*fs.Inode, syscall.Errno) {
	childPath, errno := n.childPath(name)
	if errno != 0 {
		return nil, errno
	}
	if err := n.fileSystem.backend.Mkdir(childPath, os.FileMode(mode)); err != nil {
		return nil, toErrno(err)
	}
	info, err := n.fileSystem.backend.Lstat(childPath)
	if err != nil {
		_ = n.fileSystem.backend.RemoveDirectory(childPath)
		return nil, toErrno(err)
	}
	fillEntry(out, info, n.fileSystem)
	return n.newChild(ctx, info), 0
}

func (n *Node) Unlink(_ context.Context, name string) syscall.Errno {
	childPath, errno := n.childPath(name)
	if errno != 0 {
		return errno
	}
	return toErrno(n.fileSystem.backend.Remove(childPath))
}

func (n *Node) Rmdir(_ context.Context, name string) syscall.Errno {
	childPath, errno := n.childPath(name)
	if errno != 0 {
		return errno
	}
	return toErrno(n.fileSystem.backend.RemoveDirectory(childPath))
}

func (n *Node) Rename(
	_ context.Context,
	name string,
	newParent fs.InodeEmbedder,
	newName string,
	flags uint32,
) syscall.Errno {
	if flags != 0 {
		return syscall.ENOTSUP
	}
	parent, ok := newParent.(*Node)
	if !ok || parent.fileSystem != n.fileSystem {
		return syscall.EXDEV
	}
	oldPath, errno := n.childPath(name)
	if errno != 0 {
		return errno
	}
	newPath, errno := parent.childPath(newName)
	if errno != 0 {
		return errno
	}
	return toErrno(n.fileSystem.backend.Rename(oldPath, newPath))
}

func (n *Node) Symlink(
	ctx context.Context,
	target string,
	name string,
	out *fuse.EntryOut,
) (*fs.Inode, syscall.Errno) {
	childPath, errno := n.childPath(name)
	if errno != 0 {
		return nil, errno
	}
	if err := n.fileSystem.backend.Symlink(target, childPath); err != nil {
		return nil, toErrno(err)
	}
	info, err := n.fileSystem.backend.Lstat(childPath)
	if err != nil {
		_ = n.fileSystem.backend.Remove(childPath)
		return nil, toErrno(err)
	}
	fillEntry(out, info, n.fileSystem)
	return n.newChild(ctx, info), 0
}

func (n *Node) Readlink(_ context.Context) ([]byte, syscall.Errno) {
	target, err := n.fileSystem.backend.Readlink(n.remotePath())
	if err != nil {
		return nil, toErrno(err)
	}
	return []byte(target), 0
}

func (n *Node) Setattr(
	ctx context.Context,
	handle fs.FileHandle,
	in *fuse.SetAttrIn,
	out *fuse.AttrOut,
) syscall.Errno {
	file, _ := handle.(*fileHandle)
	if mode, ok := in.GetMode(); ok {
		var err error
		if file != nil {
			err = file.remote.Chmod(os.FileMode(mode))
		} else {
			err = n.fileSystem.backend.Chmod(n.remotePath(), os.FileMode(mode))
		}
		if err != nil {
			return toErrno(err)
		}
	}
	uid, hasUID := in.GetUID()
	gid, hasGID := in.GetGID()
	if hasUID || hasGID {
		ownerUID, ownerGID := -1, -1
		if hasUID {
			ownerUID = int(uid)
		}
		if hasGID {
			ownerGID = int(gid)
		}
		var err error
		if file != nil {
			err = file.remote.Chown(ownerUID, ownerGID)
		} else {
			err = n.fileSystem.backend.Chown(n.remotePath(), ownerUID, ownerGID)
		}
		if err != nil {
			return toErrno(err)
		}
	}
	if size, ok := in.GetSize(); ok {
		var err error
		if file != nil {
			err = file.remote.Truncate(int64(size))
		} else {
			err = n.fileSystem.backend.Truncate(n.remotePath(), int64(size))
		}
		if err != nil {
			return toErrno(err)
		}
	}
	atime, hasATime := in.GetATime()
	mtime, hasMTime := in.GetMTime()
	if hasATime || hasMTime {
		info, err := n.currentInfo(file)
		if err != nil {
			return toErrno(err)
		}
		if !hasATime {
			atime = accessTime(info)
		}
		if !hasMTime {
			mtime = info.ModTime()
		}
		if err := n.fileSystem.backend.Chtimes(n.remotePath(), atime, mtime); err != nil {
			return toErrno(err)
		}
	}
	return n.Getattr(ctx, handle, out)
}

func (n *Node) currentInfo(file *fileHandle) (os.FileInfo, error) {
	if file != nil {
		return file.remote.Stat()
	}
	return n.fileSystem.backend.Lstat(n.remotePath())
}

func (n *Node) Statfs(_ context.Context, out *fuse.StatfsOut) syscall.Errno {
	stat, err := n.fileSystem.backend.StatFS(n.remotePath())
	if err != nil {
		// StatFS is optional in SFTP servers, but macOS requires a successful
		// response while mounting. Return conservative values when unsupported.
		var status *sftp.StatusError
		if errors.As(err, &status) && status.FxCode() == sftp.ErrSSHFxOpUnsupported {
			out.Bsize = 4096
			out.Frsize = 4096
			out.NameLen = 255
			return 0
		}
		return toErrno(err)
	}
	out.Blocks = stat.Blocks
	out.Bfree = stat.BlocksFree
	out.Bavail = stat.BlocksAvailable
	out.Files = stat.Files
	out.Ffree = stat.FilesFree
	out.Bsize = uint32(stat.BlockSize)
	out.Frsize = uint32(stat.BlockSize)
	out.NameLen = uint32(stat.NameMax)
	return 0
}

func fillEntry(out *fuse.EntryOut, info os.FileInfo, fileSystem *FileSystem) {
	fillAttr(&out.Attr, info, fileSystem)
	out.SetEntryTimeout(fileSystem.ttl)
	out.SetAttrTimeout(fileSystem.ttl)
}

func fillAttr(out *fuse.Attr, info os.FileInfo, fileSystem *FileSystem) {
	modified := info.ModTime()
	out.Size = uint64(max(info.Size(), 0))
	out.Blocks = (out.Size + 511) / 512
	out.Mtime = uint64(modified.Unix())
	out.Mtimensec = uint32(modified.Nanosecond())
	accessed := accessTime(info)
	out.Atime = uint64(accessed.Unix())
	out.Atimensec = uint32(accessed.Nanosecond())
	out.Ctime = out.Mtime
	out.Ctimensec = out.Mtimensec
	out.Mode = fileType(info.Mode()) | uint32(info.Mode().Perm())
	out.Nlink = 1
	if info.IsDir() {
		out.Nlink = 2
	}
	out.Uid = fileSystem.uid
	out.Gid = fileSystem.gid
	out.Blksize = 32 * 1024
}

func accessTime(info os.FileInfo) time.Time {
	if stat := info.Sys(); stat != nil {
		if value, ok := stat.(interface{ AccessTime() time.Time }); ok {
			return value.AccessTime()
		}
	}
	return info.ModTime()
}

func fileType(mode os.FileMode) uint32 {
	switch {
	case mode.IsDir():
		return fuse.S_IFDIR
	case mode&os.ModeSymlink != 0:
		return fuse.S_IFLNK
	case mode&os.ModeNamedPipe != 0:
		return fuse.S_IFIFO
	default:
		return fuse.S_IFREG
	}
}

func sanitizeOpenFlags(flags uint32) int {
	allowed := uint32(os.O_WRONLY | os.O_RDWR | os.O_APPEND | os.O_CREATE | os.O_EXCL | os.O_SYNC | os.O_TRUNC)
	return int(flags & allowed)
}

func toErrno(err error) syscall.Errno {
	if err == nil {
		return 0
	}
	switch {
	case errors.Is(err, remotefs.ErrPathEscape):
		return syscall.EPERM
	case errors.Is(err, os.ErrNotExist):
		return syscall.ENOENT
	case errors.Is(err, os.ErrPermission):
		return syscall.EACCES
	case errors.Is(err, os.ErrExist):
		return syscall.EEXIST
	case errors.Is(err, os.ErrInvalid):
		return syscall.EINVAL
	case errors.Is(err, os.ErrClosed):
		return syscall.EBADF
	case errors.Is(err, io.ErrUnexpectedEOF):
		return syscall.EIO
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno
	}
	var status *sftp.StatusError
	if errors.As(err, &status) {
		switch status.FxCode() {
		case sftp.ErrSSHFxNoSuchFile:
			return syscall.ENOENT
		case sftp.ErrSSHFxPermissionDenied:
			return syscall.EACCES
		case sftp.ErrSSHFxOpUnsupported:
			return syscall.ENOTSUP
		default:
			return syscall.EIO
		}
	}
	return syscall.EIO
}
