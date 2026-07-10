package localdata_test

import (
	"os"
	"path/filepath"
	"testing"

	localdata "github.com/openmodu/oneshot/internal/data/local"
)

func TestDefaultRootUsesDotOneshotInHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := localdata.DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root != filepath.Join(home, ".oneshot") {
		t.Fatalf("DefaultRoot() = %q", root)
	}
}

func TestOpenCreatesSecureFileLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".oneshot")
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
	if _, err := os.Stat(filepath.Join(root, "oneshot.db")); !os.IsNotExist(err) {
		t.Fatalf("pure file store unexpectedly created oneshot.db: %v", err)
	}
}

func TestResolvePathsRejectsNamedHome(t *testing.T) {
	if _, err := localdata.ResolvePaths("~someone/.oneshot"); err == nil {
		t.Fatal("ResolvePaths() accepted a named home path")
	}
}
