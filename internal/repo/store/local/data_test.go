package localdata_test

import (
	"os"
	"path/filepath"
	"testing"

	localdata "github.com/openmodu/onecatch/internal/repo/store/local"
)

func TestDefaultRootUsesDotOneCatchInHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := localdata.DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root != filepath.Join(home, ".onecatch") {
		t.Fatalf("DefaultRoot() = %q", root)
	}
}

// The rename from Oneshot to OneCatch moved the data root. Everything an
// existing installation owns lives under the old name, so these cover the three
// states the adoption has to tell apart.
func TestOpenAdoptsPreRenameDataRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	legacy := filepath.Join(home, ".oneshot")
	if err := os.MkdirAll(filepath.Join(legacy, "tasks"), 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(legacy, "tasks", "task_1.json")
	if err := os.WriteFile(marker, []byte(`{"id":"task_1"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := localdata.Open("")
	if err != nil {
		t.Fatal(err)
	}

	if data.Paths.Root != filepath.Join(home, ".onecatch") {
		t.Fatalf("Root = %q", data.Paths.Root)
	}
	moved, err := os.ReadFile(filepath.Join(home, ".onecatch", "tasks", "task_1.json"))
	if err != nil {
		t.Fatalf("pre-rename task did not survive: %v", err)
	}
	if string(moved) != `{"id":"task_1"}` {
		t.Fatalf("task content = %q", moved)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("the old root should be gone once adopted, not duplicated")
	}
}

func TestOpenNeverOverwritesAnExistingRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	current := filepath.Join(home, ".onecatch", "tasks")
	if err := os.MkdirAll(current, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(home, ".oneshot", "tasks")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "stale.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := localdata.Open(""); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(current, "stale.json")); !os.IsNotExist(err) {
		t.Fatal("a second launch must not merge the old root back in")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal("the old root must be left untouched, not consumed")
	}
}

func TestOpenLeavesAnExplicitRootAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".oneshot"), 0o700); err != nil {
		t.Fatal(err)
	}
	explicit := filepath.Join(t.TempDir(), "custom")

	data, err := localdata.Open(explicit)
	if err != nil {
		t.Fatal(err)
	}

	if data.Paths.Root != explicit {
		t.Fatalf("Root = %q", data.Paths.Root)
	}
	if _, err := os.Stat(filepath.Join(home, ".oneshot")); err != nil {
		t.Fatal("an explicit --data-dir must never trigger adoption")
	}
}

func TestOpenCreatesSecureFileLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".onecatch")
	data, err := localdata.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()

	for _, dir := range []string{data.Paths.Root, data.Paths.Workspaces, data.Paths.Tasks, data.Paths.Workflows, data.Paths.Runs, data.Paths.Locks, data.Paths.Logs} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("mode %s = %o, want 700", dir, info.Mode().Perm())
		}
	}
	if _, err := os.Stat(filepath.Join(root, "onecatch.db")); !os.IsNotExist(err) {
		t.Fatalf("pure file store unexpectedly created onecatch.db: %v", err)
	}
}

func TestResolvePathsRejectsNamedHome(t *testing.T) {
	if _, err := localdata.ResolvePaths("~someone/.onecatch"); err == nil {
		t.Fatal("ResolvePaths() accepted a named home path")
	}
}
