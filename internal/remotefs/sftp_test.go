package remotefs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
)

func TestCleanRelative(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "", want: "", ok: true},
		{input: ".", want: "", ok: true},
		{input: "internal/../main.go", want: "main.go", ok: true},
		{input: "../secret", ok: false},
		{input: "dir/../../secret", ok: false},
		{input: "/etc/passwd", ok: false},
		{input: "bad\x00name", ok: false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			got, err := cleanRelative(test.input)
			if test.ok && err != nil {
				t.Fatalf("cleanRelative(%q): %v", test.input, err)
			}
			if !test.ok && err == nil {
				t.Fatalf("cleanRelative(%q) unexpectedly succeeded with %q", test.input, got)
			}
			if got != test.want {
				t.Fatalf("cleanRelative(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestWithinRootUsesPathBoundary(t *testing.T) {
	t.Parallel()
	if !withinRoot("/home/dev/project", "/home/dev/project/internal/main.go") {
		t.Fatal("child should be inside root")
	}
	if withinRoot("/home/dev/project", "/home/dev/project-old/secret") {
		t.Fatal("prefix sibling must not be accepted")
	}
	if !withinRoot("/", "/etc/passwd") {
		t.Fatal("filesystem root should contain absolute paths")
	}
}

func TestSSHArguments(t *testing.T) {
	t.Parallel()
	got, err := sshArguments(SFTPConfig{
		Host:                "devbox",
		ConnectTimeout:      4 * time.Second,
		ServerAliveInterval: 12 * time.Second,
		ServerAliveCountMax: 2,
		SSHOptions:          []string{"ProxyJump=bastion", "Compression=yes"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"-o", "BatchMode=yes",
		"-o", "ClearAllForwardings=yes",
		"-o", "SendEnv=-*",
		"-o", "ConnectTimeout=4",
		"-o", "ServerAliveInterval=12",
		"-o", "ServerAliveCountMax=2",
		"-o", "ProxyJump=bastion",
		"-o", "Compression=yes",
		"-s", "devbox", "sftp",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sshArguments() = %#v, want %#v", got, want)
	}
}

func TestSSHArgumentsUseAskPassForPasswordCredential(t *testing.T) {
	got, err := sshArguments(SFTPConfig{
		Host: "devbox", Username: "deploy", CredentialID: "sshcred_0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	for _, expected := range []string{
		"BatchMode=no", "PubkeyAuthentication=no", "PreferredAuthentications=password",
		"NumberOfPasswordPrompts=1", "-l deploy", "-s devbox sftp",
	} {
		if !strings.Contains(joined, expected) {
			t.Errorf("SSH arguments do not contain %q: %v", expected, got)
		}
	}
	if strings.Contains(joined, "0123456789abcdef0123456789abcdef") {
		t.Fatalf("credential identifier leaked into SSH argv: %v", got)
	}
}

func TestSSHArgumentsSplitHostAndPort(t *testing.T) {
	t.Parallel()
	got, err := sshArguments(SFTPConfig{Host: "192.168.5.98:2222"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "-p 2222") || !strings.Contains(joined, "-s 192.168.5.98 sftp") {
		t.Fatalf("SSH arguments did not split host and port: %v", got)
	}
	if strings.Contains(joined, "192.168.5.98:2222") {
		t.Fatalf("host:port was passed as a hostname: %v", got)
	}
}

func TestNewSFTPBackendReportsOpenSSHDiagnostic(t *testing.T) {
	sshStub := filepath.Join(t.TempDir(), "ssh-stub")
	if err := os.WriteFile(sshStub, []byte("#!/bin/sh\nprintf '%s\\n' 'Host key verification failed.' >&2\nexit 255\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := NewSFTPBackend(context.Background(), SFTPConfig{
		Host:      "devbox",
		Root:      "/srv/project",
		SSHBinary: sshStub,
	})
	if err == nil {
		t.Fatal("NewSFTPBackend unexpectedly succeeded")
	}
	if got := err.Error(); !strings.Contains(got, "Host key verification failed.") {
		t.Fatalf("error = %q, want the OpenSSH diagnostic", got)
	}
}

func TestSFTPBackendAgainstOpenSSHServer(t *testing.T) {
	client, shutdown := startLocalSFTPServer(t)
	defer shutdown()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend, err := newSFTPBackend(client, root)
	if err != nil {
		t.Fatalf("newSFTPBackend: %v", err)
	}
	defer func() {
		if err := backend.Close(); err != nil {
			t.Errorf("close backend: %v", err)
		}
	}()

	info, err := backend.Lstat("hello.txt")
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Size() != 11 {
		t.Fatalf("size = %d, want 11", info.Size())
	}
	entries, err := backend.ReadDir("")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "hello.txt" {
		t.Fatalf("readdir = %#v, want hello.txt", entries)
	}

	file, err := backend.OpenFile("hello.txt", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	buffer := make([]byte, 5)
	if count, err := file.ReadAt(buffer, 6); count != 5 || (err != nil && !errors.Is(err, io.EOF)) {
		t.Fatalf("read: count=%d err=%v", count, err)
	}
	if string(buffer) != "world" {
		t.Fatalf("read = %q, want world", buffer)
	}
	if count, err := file.WriteAt([]byte("SFTP"), 6); err != nil || count != 4 {
		t.Fatalf("write: count=%d err=%v", count, err)
	}
	if err := file.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "hello.txt")); err != nil || string(data) != "hello SFTPd" {
		t.Fatalf("remote write result = %q, err=%v", data, err)
	}

	if err := backend.Mkdir("docs", 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "docs")); err != nil || info.Mode().Perm() != 0o750 {
		t.Fatalf("created directory mode = %v, err=%v", infoMode(info), err)
	}
	created, err := backend.OpenFile("docs/new.txt", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o640)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := created.WriteAt([]byte("new data"), 0); err != nil {
		t.Fatalf("write created file: %v", err)
	}
	if err := created.Close(); err != nil {
		t.Fatalf("close created file: %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "docs/new.txt")); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("created file mode = %v, err=%v", infoMode(info), err)
	}
	if err := backend.Rename("docs/new.txt", "docs/renamed.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := backend.Truncate("docs/renamed.txt", 3); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if err := backend.Chmod("docs/renamed.txt", 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	modified := time.Unix(1_700_000_000, 0)
	if err := backend.Chtimes("docs/renamed.txt", modified, modified); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "docs/renamed.txt")); err != nil || info.Size() != 3 || info.Mode().Perm() != 0o600 || !info.ModTime().Equal(modified) {
		t.Fatalf("updated file info = size:%d mode:%v mtime:%v, err=%v", infoSize(info), infoMode(info), infoModTime(info), err)
	}
	if err := backend.Symlink("/docs/renamed.txt", "latest"); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if target, err := backend.Readlink("latest"); err != nil || target != "/docs/renamed.txt" {
		t.Fatalf("readlink = %q, err=%v", target, err)
	}

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.ReadDir("escape"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("read through escaping symlink: got %v, want ErrPathEscape", err)
	}
	if _, err := backend.OpenFile("escape/secret", os.O_RDONLY, 0); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("open through escaping symlink: got %v, want ErrPathEscape", err)
	}

	if err := backend.Remove("latest"); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}
	if err := backend.Remove("docs/renamed.txt"); err != nil {
		t.Fatalf("remove file: %v", err)
	}
	if err := backend.RemoveDirectory("docs"); err != nil {
		t.Fatalf("remove directory: %v", err)
	}
}

func infoMode(info os.FileInfo) os.FileMode {
	if info == nil {
		return 0
	}
	return info.Mode().Perm()
}

func infoSize(info os.FileInfo) int64 {
	if info == nil {
		return -1
	}
	return info.Size()
}

func infoModTime(info os.FileInfo) time.Time {
	if info == nil {
		return time.Time{}
	}
	return info.ModTime()
}

func startLocalSFTPServer(t *testing.T) (*sftp.Client, func()) {
	t.Helper()
	serverBinary := ""
	for _, candidate := range []string{
		"/usr/libexec/sftp-server",
		"/usr/lib/openssh/sftp-server",
		"/usr/lib/ssh/sftp-server",
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			serverBinary = candidate
			break
		}
	}
	if serverBinary == "" {
		if candidate, err := exec.LookPath("sftp-server"); err == nil {
			serverBinary = candidate
		}
	}
	if serverBinary == "" {
		t.Skip("OpenSSH sftp-server is not installed")
	}

	command := exec.Command(serverBinary)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start %s: %v", serverBinary, err)
	}
	wait := make(chan error, 1)
	go func() {
		wait <- command.Wait()
	}()
	client, err := sftp.NewClientPipe(
		stdout,
		stdin,
		sftp.UseConcurrentReads(true),
		sftp.UseConcurrentWrites(true),
	)
	if err != nil {
		_ = stdin.Close()
		_ = command.Process.Kill()
		<-wait
		t.Fatalf("connect to local SFTP server: %v (%s)", err, strings.TrimSpace(stderr.String()))
	}
	shutdown := func() {
		_ = client.Close()
		_ = stdin.Close()
		select {
		case err := <-wait:
			if err != nil {
				t.Errorf("local SFTP server exited: %v (%s)", err, strings.TrimSpace(stderr.String()))
			}
		case <-time.After(2 * time.Second):
			_ = command.Process.Kill()
			<-wait
			t.Error("local SFTP server did not exit after its client closed")
		}
	}
	return client, shutdown
}
