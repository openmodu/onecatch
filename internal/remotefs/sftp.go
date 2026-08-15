package remotefs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
)

type SFTPConfig struct {
	Host                string
	Root                string
	SSHBinary           string
	SSHOptions          []string
	ConnectTimeout      time.Duration
	ServerAliveInterval time.Duration
	ServerAliveCountMax int
	Stderr              io.Writer
}

// SFTPBackend talks to the OpenSSH SFTP subsystem over one persistent SSH
// process. Using OpenSSH preserves the user's aliases, ProxyJump settings,
// ssh-agent, host key policy, and other ~/.ssh/config behavior.
type SFTPBackend struct {
	client *sftp.Client
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	root   string

	closeOnce sync.Once
	wait      chan error
}

func NewSFTPBackend(ctx context.Context, config SFTPConfig) (*SFTPBackend, error) {
	if strings.TrimSpace(config.Host) == "" {
		return nil, errors.New("SSH host is required")
	}
	if strings.TrimSpace(config.Root) == "" {
		return nil, errors.New("remote root is required")
	}
	binary := config.SSHBinary
	if binary == "" {
		binary = "ssh"
	}
	args := sshArguments(config)
	cmd := exec.CommandContext(ctx, binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open SSH stdout: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open SSH stdin: %w", err)
	}
	if config.Stderr != nil {
		cmd.Stderr = config.Stderr
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start SSH SFTP subsystem: %w", err)
	}
	wait := make(chan error, 1)
	go func() {
		wait <- cmd.Wait()
		close(wait)
	}()

	client, err := sftp.NewClientPipe(
		stdout,
		stdin,
		sftp.UseConcurrentReads(true),
		sftp.UseConcurrentWrites(true),
		sftp.MaxConcurrentRequestsPerFile(32),
	)
	if err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		<-wait
		return nil, fmt.Errorf("start SFTP client: %w", err)
	}
	backend, err := newSFTPBackend(client, config.Root)
	if err != nil {
		_ = client.Close()
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		<-wait
		return nil, err
	}
	backend.cmd = cmd
	backend.stdin = stdin
	backend.wait = wait
	return backend, nil
}

func newSFTPBackend(client *sftp.Client, configuredRoot string) (*SFTPBackend, error) {
	root, err := client.RealPath(configuredRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve remote root %q: %w", configuredRoot, err)
	}
	info, err := client.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat remote root %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("remote root %q is not a directory", root)
	}
	root = path.Clean(root)
	return &SFTPBackend{
		client: client,
		root:   root,
	}, nil
}

func sshArguments(config SFTPConfig) []string {
	connectTimeout := config.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = 10 * time.Second
	}
	aliveInterval := config.ServerAliveInterval
	if aliveInterval <= 0 {
		aliveInterval = 15 * time.Second
	}
	aliveCount := config.ServerAliveCountMax
	if aliveCount <= 0 {
		aliveCount = 3
	}
	seconds := func(value time.Duration) int {
		result := int(value.Round(time.Second) / time.Second)
		if result < 1 {
			return 1
		}
		return result
	}
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ClearAllForwardings=yes",
		"-o", fmt.Sprintf("ConnectTimeout=%d", seconds(connectTimeout)),
		"-o", fmt.Sprintf("ServerAliveInterval=%d", seconds(aliveInterval)),
		"-o", fmt.Sprintf("ServerAliveCountMax=%d", aliveCount),
	}
	for _, option := range config.SSHOptions {
		if strings.TrimSpace(option) != "" {
			args = append(args, "-o", option)
		}
	}
	return append(args, "-s", config.Host, "sftp")
}

func (b *SFTPBackend) Lstat(name string) (os.FileInfo, error) {
	remote, err := b.resolveLeaf(name)
	if err != nil {
		return nil, err
	}
	return b.client.Lstat(remote)
}

func (b *SFTPBackend) ReadDir(name string) ([]os.FileInfo, error) {
	remote, err := b.resolveExisting(name)
	if err != nil {
		return nil, err
	}
	return b.client.ReadDir(remote)
}

func (b *SFTPBackend) OpenFile(name string, flags int, mode os.FileMode) (File, error) {
	remote, existed, err := b.resolveOpen(name)
	if err != nil {
		return nil, err
	}
	file, err := b.client.OpenFile(remote, flags)
	if err != nil {
		return nil, err
	}
	if flags&os.O_CREATE != 0 && !existed {
		if err := file.Chmod(mode.Perm()); err != nil {
			_ = file.Close()
			_ = b.client.Remove(remote)
			return nil, err
		}
	}
	return &sftpFile{File: file}, nil
}

func (b *SFTPBackend) Readlink(name string) (string, error) {
	remote, err := b.resolveLeaf(name)
	if err != nil {
		return "", err
	}
	target, err := b.client.ReadLink(remote)
	if err != nil {
		return "", err
	}
	if path.IsAbs(target) {
		if !withinRoot(b.root, path.Clean(target)) {
			return "", ErrPathEscape
		}
		if b.root == "/" {
			return path.Clean(target), nil
		}
		relative := strings.TrimPrefix(path.Clean(target), b.root)
		if relative == "" {
			return "/", nil
		}
		return relative, nil
	}
	if _, err := cleanRelative(path.Join(parentPath(name), target)); err != nil {
		return "", err
	}
	return target, nil
}

func (b *SFTPBackend) Symlink(target, name string) error {
	remote, err := b.resolveLeaf(name)
	if err != nil {
		return err
	}
	remoteTarget := target
	if path.IsAbs(target) {
		relative, err := cleanRelative(strings.TrimPrefix(target, "/"))
		if err != nil {
			return err
		}
		remoteTarget = path.Join(b.root, relative)
	} else if _, err := cleanRelative(path.Join(parentPath(name), target)); err != nil {
		return err
	}
	if err := b.client.Symlink(remoteTarget, remote); err != nil {
		return err
	}
	return nil
}

func (b *SFTPBackend) Mkdir(name string, mode os.FileMode) error {
	remote, err := b.resolveLeaf(name)
	if err != nil {
		return err
	}
	if err := b.client.Mkdir(remote); err != nil {
		return err
	}
	if err := b.client.Chmod(remote, mode.Perm()); err != nil {
		_ = b.client.RemoveDirectory(remote)
		return err
	}
	return nil
}

func (b *SFTPBackend) Remove(name string) error {
	remote, err := b.resolveLeaf(name)
	if err != nil {
		return err
	}
	if err := b.client.Remove(remote); err != nil {
		return err
	}
	return nil
}

func (b *SFTPBackend) RemoveDirectory(name string) error {
	remote, err := b.resolveLeaf(name)
	if err != nil {
		return err
	}
	if err := b.client.RemoveDirectory(remote); err != nil {
		return err
	}
	return nil
}

func (b *SFTPBackend) Rename(oldName, newName string) error {
	oldRemote, err := b.resolveLeaf(oldName)
	if err != nil {
		return err
	}
	newRemote, err := b.resolveLeaf(newName)
	if err != nil {
		return err
	}
	if data, ok := b.client.HasExtension("posix-rename@openssh.com"); ok && data == "1" {
		err = b.client.PosixRename(oldRemote, newRemote)
	} else {
		err = b.client.Rename(oldRemote, newRemote)
	}
	return err
}

func (b *SFTPBackend) Chmod(name string, mode os.FileMode) error {
	remote, err := b.resolveExisting(name)
	if err != nil {
		return err
	}
	return b.client.Chmod(remote, mode.Perm())
}

func (b *SFTPBackend) Chown(name string, uid, gid int) error {
	remote, err := b.resolveExisting(name)
	if err != nil {
		return err
	}
	return b.client.Chown(remote, uid, gid)
}

func (b *SFTPBackend) Chtimes(name string, atime, mtime time.Time) error {
	remote, err := b.resolveExisting(name)
	if err != nil {
		return err
	}
	return b.client.Chtimes(remote, atime, mtime)
}

func (b *SFTPBackend) Truncate(name string, size int64) error {
	remote, err := b.resolveExisting(name)
	if err != nil {
		return err
	}
	return b.client.Truncate(remote, size)
}

func (b *SFTPBackend) StatFS(name string) (StatFS, error) {
	remote, err := b.resolveExisting(name)
	if err != nil {
		return StatFS{}, err
	}
	stat, err := b.client.StatVFS(remote)
	if err != nil {
		return StatFS{}, err
	}
	return StatFS{
		BlockSize:       stat.Frsize,
		Blocks:          stat.Blocks,
		BlocksFree:      stat.Bfree,
		BlocksAvailable: stat.Bavail,
		Files:           stat.Files,
		FilesFree:       stat.Ffree,
		NameMax:         stat.Namemax,
	}, nil
}

func (b *SFTPBackend) Close() error {
	var closeErr error
	b.closeOnce.Do(func() {
		closeErr = b.client.Close()
		if b.stdin != nil {
			_ = b.stdin.Close()
		}
		if b.wait == nil {
			return
		}
		select {
		case err := <-b.wait:
			if closeErr == nil && err != nil {
				closeErr = err
			}
		case <-time.After(2 * time.Second):
			if b.cmd != nil && b.cmd.Process != nil {
				_ = b.cmd.Process.Kill()
			}
			<-b.wait
		}
	})
	return closeErr
}

func (b *SFTPBackend) resolveOpen(name string) (remote string, existed bool, err error) {
	relative, err := cleanRelative(name)
	if err != nil {
		return "", false, err
	}
	leaf, err := b.resolveLeaf(relative)
	if err != nil {
		return "", false, err
	}
	if _, err := b.client.Lstat(leaf); err != nil {
		if os.IsNotExist(err) {
			return leaf, false, nil
		}
		return "", false, err
	}
	// Resolve an existing leaf so a symlink cannot lead outside root. A
	// dangling link is deliberately rejected rather than followed by O_CREATE.
	remote, err = b.resolveExisting(relative)
	return remote, true, err
}

func (b *SFTPBackend) resolveLeaf(name string) (string, error) {
	relative, err := cleanRelative(name)
	if err != nil {
		return "", err
	}
	if relative == "" {
		return b.root, nil
	}
	parent, err := b.resolveExisting(parentPath(relative))
	if err != nil {
		return "", err
	}
	candidate := path.Join(parent, path.Base(relative))
	if !withinRoot(b.root, candidate) {
		return "", ErrPathEscape
	}
	return candidate, nil
}

func (b *SFTPBackend) resolveExisting(name string) (string, error) {
	relative, err := cleanRelative(name)
	if err != nil {
		return "", err
	}
	resolved, err := b.client.RealPath(path.Join(b.root, relative))
	if err != nil {
		return "", err
	}
	resolved = path.Clean(resolved)
	if !withinRoot(b.root, resolved) {
		return "", ErrPathEscape
	}
	return resolved, nil
}

func cleanRelative(name string) (string, error) {
	if strings.IndexByte(name, 0) >= 0 || path.IsAbs(name) {
		return "", ErrPathEscape
	}
	cleaned := path.Clean(name)
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrPathEscape
	}
	return cleaned, nil
}

func withinRoot(root, candidate string) bool {
	if root == "/" {
		return path.IsAbs(candidate)
	}
	return candidate == root || strings.HasPrefix(candidate, root+"/")
}

type sftpFile struct {
	*sftp.File
}

func (f *sftpFile) Sync() error {
	err := f.File.Sync()
	if errors.Is(err, sftp.ErrSSHFxOpUnsupported) {
		return nil
	}
	return err
}
