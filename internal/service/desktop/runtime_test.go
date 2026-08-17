package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
)

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
