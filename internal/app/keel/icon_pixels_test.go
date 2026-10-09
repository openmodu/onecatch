package keeldesktop

import (
	"bytes"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"image"
	"strings"
	"testing"
)

func TestRuntimeMarksHaveVisiblePixels(t *testing.T) {
	entries, _ := iconAssets.ReadDir("runtimeicons")
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".svg") {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			data, _ := iconAssets.ReadFile("runtimeicons/" + entry.Name())
			data, err := nativeSVG(data)
			if err != nil {
				t.Fatal(err)
			}
			icon, err := oksvg.ReadReplacingCurrentColor(bytes.NewReader(data), "black", oksvg.StrictErrorMode)
			if err != nil {
				t.Fatal(err)
			}
			img := image.NewRGBA(image.Rect(0, 0, 48, 48))
			icon.SetTarget(0, 0, 48, 48)
			icon.Draw(rasterx.NewDasher(48, 48, rasterx.NewScannerGV(48, 48, img, img.Bounds())), 1)
			visible := 0
			for y := 0; y < 48; y++ {
				for x := 0; x < 48; x++ {
					if img.RGBAAt(x, y).A > 0 {
						visible++
					}
				}
			}
			if visible < 40 {
				t.Fatalf("mark is empty: %d visible pixels, %d paths", visible, len(icon.SVGPaths))
			}
		})
	}
}
