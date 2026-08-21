package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
)

func TestModuRuntimeStatusCacheIncludesIntegration(t *testing.T) {
	sdk := runtimeSpecs(RuntimeConfig{ModuIntegration: "sdk"})[2]
	cli := runtimeSpecs(RuntimeConfig{ModuIntegration: "cli"})[2]
	if sdk.cacheKey == cli.cacheKey {
		t.Fatal("Modu SDK and CLI share a runtime status cache key")
	}
}

func TestRuntimeRegistryResolvesIsolatedModuPaths(t *testing.T) {
	root := t.TempDir()
	registry, err := NewRuntimeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	configPath, agentDir := registry.moduSDKPaths(domainsettings.RuntimeSettings{Integration: "sdk", ConfigSource: "onecatch"})
	wantDir := filepath.Join(root, "harnesses", "modu")
	if configPath != filepath.Join(wantDir, "config.toml") || agentDir != wantDir {
		t.Fatalf("isolated paths = (%q, %q), want directory %q", configPath, agentDir, wantDir)
	}
	configPath, agentDir = registry.moduSDKPaths(domainsettings.RuntimeSettings{Integration: "sdk", ConfigSource: "shared"})
	if configPath != "" || agentDir != "" {
		t.Fatalf("shared paths = (%q, %q), want Modu defaults", configPath, agentDir)
	}
}

func TestRuntimeRegistryCopiesSharedModuConfigOnFirstIsolation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	shared := filepath.Join(home, ".modu", "config.toml")
	if err := os.MkdirAll(filepath.Dir(shared), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shared, []byte("version = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	registry, err := NewRuntimeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	settings := domainsettings.Defaults().Runtimes
	modu := settings["modu"]
	modu.ConfigSource = "onecatch"
	settings["modu"] = modu
	registry.ApplySettings(settings, 10)
	target := filepath.Join(root, "harnesses", "modu", "config.toml")
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "version = 2\n" {
		t.Fatalf("isolated config = %q, %v", data, err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("isolated config permissions = %v, %v", info, err)
	}
}

func TestRuntimeRegistryCachesVersionChecks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is Unix-only")
	}
	root := t.TempDir()
	counter := filepath.Join(root, "checks")
	binary := filepath.Join(root, "runtime")
	script := "#!/bin/sh\nprintf 'checked\\n' >> \"$ONECATCH_RUNTIME_TEST_COUNTER\"\nprintf 'runtime 1.0\\n'\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ONECATCH_RUNTIME_TEST_COUNTER", counter)
	registry, err := NewRuntimeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	settings := domainsettings.Defaults().Runtimes
	settings["codex"] = domainsettings.RuntimeSettings{Binary: binary}
	settings["claude"] = domainsettings.RuntimeSettings{Binary: binary}
	registry.ApplySettings(settings, 10)

	first := registry.List()
	second := registry.List()
	if len(first) != 3 || len(second) != 3 || first[0].Version != "runtime 1.0" || first[1].Version != "runtime 1.0" {
		t.Fatalf("unexpected runtime results: first=%+v second=%+v", first, second)
	}
	data, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	if checks := strings.Count(string(data), "checked"); checks != 2 {
		t.Fatalf("version checks = %d, want 2", checks)
	}
}

// Probing costs a process spawn per runtime, which is the single most expensive
// thing on the startup path. Keeping the result on disk is what stops every
// cold launch from paying it again before the runtime list can be shown.
func TestRuntimeRegistryServesStatusFromDiskAfterRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper is Unix-only")
	}
	root := t.TempDir()
	counter := filepath.Join(root, "checks")
	binary := filepath.Join(root, "runtime")
	script := "#!/bin/sh\nprintf 'checked\\n' >> \"$ONECATCH_RUNTIME_TEST_COUNTER\"\nprintf 'runtime 1.0\\n'\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ONECATCH_RUNTIME_TEST_COUNTER", counter)
	settings := domainsettings.Defaults().Runtimes
	settings["codex"] = domainsettings.RuntimeSettings{Binary: binary}
	settings["claude"] = domainsettings.RuntimeSettings{Binary: binary}

	first, err := NewRuntimeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	first.ApplySettings(settings, 10)
	before := first.List()

	restarted, err := NewRuntimeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	restarted.ApplySettings(settings, 10)
	after := restarted.List()

	if len(after) != len(before) {
		t.Fatalf("runtime count = %d, want %d", len(after), len(before))
	}
	for index := range before {
		if after[index].ID != before[index].ID || after[index].Available != before[index].Available || after[index].Version != before[index].Version {
			t.Fatalf("runtime %d = %+v, want %+v", index, after[index], before[index])
		}
	}
	data, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	if checks := strings.Count(string(data), "checked"); checks != 2 {
		t.Fatalf("version checks after restart = %d, want 2 (the restart must not re-probe)", checks)
	}
}
