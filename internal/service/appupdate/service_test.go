package appupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openmodu/onecatch/internal/buildinfo"
)

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
