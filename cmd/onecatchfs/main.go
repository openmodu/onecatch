//go:build darwin

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/openmodu/onecatch/internal/fusefs"
	"github.com/openmodu/onecatch/internal/remotefs"
)

type stringList []string

func (values *stringList) String() string {
	return strings.Join(*values, ",")
}

func (values *stringList) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("SSH option cannot be empty")
	}
	*values = append(*values, value)
	return nil
}

func main() {
	os.Exit(run())
}

func run() int {
	var sshOptions stringList
	host := flag.String("host", "", "OpenSSH host or ~/.ssh/config alias")
	remoteRoot := flag.String("root", "", "absolute project directory on the SSH host")
	mountPoint := flag.String("mount", "", "empty local directory used as the mount point")
	cacheTTL := flag.Duration("cache-ttl", time.Second, "metadata and directory cache lifetime")
	connectTimeout := flag.Duration("connect-timeout", 10*time.Second, "SSH connection timeout")
	volumeName := flag.String("volume-name", "OneCatch Remote", "name shown by macOS")
	allowOther := flag.Bool("allow-other", false, "allow other local users to access the mount")
	debug := flag.Bool("debug", false, "enable FUSE protocol logging")
	flag.Var(&sshOptions, "ssh-option", "additional OpenSSH -o option; may be repeated")
	flag.Parse()

	if *host == "" || *remoteRoot == "" || *mountPoint == "" {
		flag.Usage()
		return 2
	}
	if !strings.HasPrefix(*remoteRoot, "/") {
		log.Printf("remote root must be absolute: %q", *remoteRoot)
		return 2
	}
	localMount, err := prepareMountPoint(*mountPoint)
	if err != nil {
		log.Printf("prepare mount point: %v", err)
		return 1
	}
	if err := requireMacFUSE(); err != nil {
		log.Print(err)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	remote, err := remotefs.NewSFTPBackend(ctx, remotefs.SFTPConfig{
		Host:           *host,
		Root:           *remoteRoot,
		SSHOptions:     sshOptions,
		ConnectTimeout: *connectTimeout,
		Stderr:         os.Stderr,
	})
	if err != nil {
		log.Printf("connect remote workspace: %v", err)
		return 1
	}
	defer func() {
		if err := remote.Close(); err != nil {
			log.Printf("close remote workspace: %v", err)
		}
	}()

	backend := remotefs.NewCachedBackend(remote, *cacheTTL)
	root := fusefs.New(backend, *cacheTTL)
	ttl := *cacheTTL
	options := &fs.Options{
		EntryTimeout:    &ttl,
		AttrTimeout:     &ttl,
		NegativeTimeout: &ttl,
		NullPermissions: true,
		UID:             uint32(os.Getuid()),
		GID:             uint32(os.Getgid()),
		RootStableAttr:  &fs.StableAttr{Mode: fuse.S_IFDIR},
		MountOptions: fuse.MountOptions{
			AllowOther:    *allowOther,
			Debug:         *debug,
			FsName:        fmt.Sprintf("onecatchfs#%s:%s", *host, *remoteRoot),
			Name:          "onecatchfs",
			MaxBackground: 64,
			MaxWrite:      1024 * 1024,
			MaxReadAhead:  1024 * 1024,
			Options: []string{
				"jail_symlinks",
				"noappledouble",
				"noapplexattr",
				"volname=" + sanitizeVolumeName(*volumeName),
			},
		},
	}
	server, err := fusefs.Mount(localMount, root, options)
	if err != nil {
		log.Printf("mount remote workspace: %v", err)
		return 1
	}
	log.Printf("mounted %s:%s at %s", *host, *remoteRoot, localMount)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		<-signals
		if err := server.Unmount(); err != nil {
			log.Printf("unmount %s: %v", localMount, err)
		}
	}()

	server.Wait()
	log.Printf("unmounted %s", localMount)
	return 0
}

func prepareMountPoint(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", absolute)
	}
	entries, err := os.ReadDir(absolute)
	if err != nil {
		return "", err
	}
	if len(entries) != 0 {
		return "", fmt.Errorf("%s is not empty", absolute)
	}
	return absolute, nil
}

func requireMacFUSE() error {
	locations := []string{
		"/Library/Filesystems/macfuse.fs/Contents/Resources/mount_macfuse",
		"/Library/Filesystems/osxfuse.fs/Contents/Resources/mount_osxfuse",
	}
	for _, location := range locations {
		if _, err := os.Stat(location); err == nil {
			return nil
		}
	}
	return errors.New("macFUSE is not installed; install it from https://macfuse.github.io/")
}

func sanitizeVolumeName(value string) string {
	value = strings.ReplaceAll(value, ",", "-")
	value = strings.TrimSpace(value)
	if value == "" {
		return "OneCatch Remote"
	}
	return value
}
