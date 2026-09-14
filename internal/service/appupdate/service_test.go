package appupdate

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/buildinfo"
)

func TestUpdaterCopiesSelfToUniqueExecutable(t *testing.T) {
	t.Setenv("APPIMAGE", "")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	digest := func(path string) [32]byte {
		t.Helper()
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			t.Fatal(err)
		}
		return [32]byte(hash.Sum(nil))
	}
	first, err := copyUpdaterHelper()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(first)
	second, err := copyUpdaterHelper()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(second)
	if first == self || first == second {
		t.Fatal("updater must own a fresh executable copy")
	}
	if digest(first) != digest(self) {
		t.Fatal("updater is not a complete copy of the unified executable")
	}
}

func TestUpdateHTTPClientAllowsSlowInstallerDownloads(t *testing.T) {
	if timeout := updateHTTPClient().Timeout; timeout != 10*time.Minute {
		t.Fatalf("update HTTP timeout = %s, want 10m", timeout)
	}
}

func TestSignalReadyFromEnvironmentWritesOnlyOwnedTempMarker(t *testing.T) {
	marker := filepath.Join(os.TempDir(), fmt.Sprintf("onecatch-update-ready-test-%d", os.Getpid()))
	t.Cleanup(func() {
		_ = os.Remove(marker)
		_ = os.Unsetenv(readyEnvironment)
	})
	if err := os.Setenv(readyEnvironment, marker); err != nil {
		t.Fatal(err)
	}
	if err := SignalReadyFromEnvironment(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(content)) != buildinfo.Version {
		t.Fatalf("readiness marker = %q, want version %q", content, buildinfo.Version)
	}
	if value := os.Getenv(readyEnvironment); value != "" {
		t.Fatalf("readiness environment was not cleared: %q", value)
	}
}

func TestSignalReadyFromEnvironmentRejectsArbitraryPath(t *testing.T) {
	unsafe := filepath.Join(t.TempDir(), "onecatch-update-ready-owned-by-someone-else")
	t.Cleanup(func() { _ = os.Unsetenv(readyEnvironment) })
	if err := os.Setenv(readyEnvironment, unsafe); err != nil {
		t.Fatal(err)
	}
	if err := SignalReadyFromEnvironment(); err == nil {
		t.Fatal("expected unsafe readiness path to be rejected")
	}
	if _, err := os.Stat(unsafe); !os.IsNotExist(err) {
		t.Fatalf("unsafe readiness path was written: %v", err)
	}
}
