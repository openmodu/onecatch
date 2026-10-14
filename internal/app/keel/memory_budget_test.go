package keeldesktop

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
)

func TestMemoryBudgetHonorsExplicitLimit(t *testing.T) {
	old := debug.SetMemoryLimit(256 << 20)
	defer debug.SetMemoryLimit(old)
	t.Setenv("GOMEMLIMIT", "256MiB")
	configureMemoryBudget()
	if got := debug.SetMemoryLimit(-1); got != 256<<20 {
		t.Fatalf("overrode explicit budget: %d", got)
	}
	t.Setenv("GOMEMLIMIT", "")
	configureMemoryBudget()
	if got := debug.SetMemoryLimit(-1); got != 96<<20 {
		t.Fatalf("default budget: %d", got)
	}
}

func TestPackagedIconDetection(t *testing.T) {
	contents := filepath.Join(t.TempDir(), "OneCatch.app", "Contents")
	exe := filepath.Join(contents, "MacOS", "OneCatch")
	if hasPackagedIcon("darwin", exe) {
		t.Fatal("missing icon accepted")
	}
	if err := os.MkdirAll(filepath.Join(contents, "Resources"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contents, "Resources", "icon.icns"), []byte("icns"), 0600); err != nil {
		t.Fatal(err)
	}
	if !hasPackagedIcon("darwin", exe) {
		t.Fatal("packaged icon not detected")
	}
	if hasPackagedIcon("linux", exe) || hasPackagedIcon("darwin", filepath.Join(contents, "OneCatch")) {
		t.Fatal("development/non-Mac binary incorrectly skipped")
	}
}

func TestStatusIconHasBoundedPixels(t *testing.T) {
	icon, err := decodeStatusIcon(appIcon)
	if err != nil {
		t.Fatal(err)
	}
	if icon.Bounds().Dx() > 96 || icon.Bounds().Dy() > 96 {
		t.Fatalf("status icon retains Dock-sized bitmap: %v", icon.Bounds())
	}
}
