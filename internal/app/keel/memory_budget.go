package keeldesktop

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime/debug"

	"golang.org/x/image/draw"
)

// Bound Go's retained heap in the desktop process; native/GPU allocations are
// outside this soft limit. An explicit GOMEMLIMIT always takes precedence.
func configureMemoryBudget() {
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(96 << 20)
	}
}

// Packaged macOS builds already carry the Dock artwork. Installing it again
// creates a second decoded native image; development binaries still need it.
func hasPackagedIcon(goos, executable string) bool {
	if goos != "darwin" || filepath.Base(filepath.Dir(executable)) != "MacOS" {
		return false
	}
	contents := filepath.Dir(filepath.Dir(executable))
	if filepath.Base(contents) != "Contents" {
		return false
	}
	info, err := os.Stat(filepath.Join(contents, "Resources", "icon.icns"))
	return err == nil && !info.IsDir() && info.Size() > 0
}

// The in-window logo and tray never need the 1024px Dock bitmap. Keep enough
// pixels for their 22dp display at up to 4x scale without retaining 4MiB.
func decodeStatusIcon(data []byte) (image.Image, error) {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if src.Bounds().Dx() <= 96 && src.Bounds().Dy() <= 96 {
		return src, nil
	}
	dst := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst, nil
}
