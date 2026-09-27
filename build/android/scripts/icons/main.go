// Generate Android launcher resources from the shared OneCatch artwork.
// Run from the repository root: go run ./build/android/scripts/icons
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
)

func main() {
	f, err := os.Open("build/desktop/appicon-source.png")
	check(err)
	src, err := png.Decode(f)
	check(err)
	check(f.Close())
	root := "build/android/app/src/main/res"
	background := color.NRGBA{R: 15, G: 24, B: 23, A: 255}
	for _, density := range []struct {
		name string
		size int
	}{
		{"mdpi", 48}, {"hdpi", 72}, {"xhdpi", 96}, {"xxhdpi", 144}, {"xxxhdpi", 192},
	} {
		dir := filepath.Join(root, "mipmap-"+density.name)
		icon := image.NewNRGBA(image.Rect(0, 0, density.size, density.size))
		xdraw.CatmullRom.Scale(icon, icon.Bounds(), src, src.Bounds(), draw.Src, nil)
		save(filepath.Join(dir, "ic_launcher.png"), icon)
		// Fit the artwork inside a circular background for pre-Android 8 launchers.
		round := image.NewNRGBA(icon.Bounds())
		draw.Draw(round, round.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
		inset := density.size / 10
		xdraw.CatmullRom.Scale(round, round.Bounds().Inset(inset), src, src.Bounds(), draw.Over, nil)
		radius := float64(density.size) / 2
		for y := 0; y < density.size; y++ {
			for x := 0; x < density.size; x++ {
				dx, dy := float64(x)+0.5-radius, float64(y)+0.5-radius
				if dx*dx+dy*dy > radius*radius {
					round.SetNRGBA(x, y, color.NRGBA{})
				}
			}
		}
		save(filepath.Join(dir, "ic_launcher_round.png"), round)
	}
	// Adaptive icons use a 108 dp canvas; keep the artwork in its central 66 dp.
	foreground := image.NewNRGBA(image.Rect(0, 0, 432, 432))
	xdraw.CatmullRom.Scale(foreground, image.Rect(84, 84, 348, 348), src, src.Bounds(), draw.Src, nil)
	save(filepath.Join(root, "drawable-nodpi/ic_launcher_foreground.png"), foreground)
	adaptive := []byte(`<?xml version="1.0" encoding="utf-8"?>
<adaptive-icon xmlns:android="http://schemas.android.com/apk/res/android">
    <background android:drawable="@color/ic_launcher_background" />
    <foreground android:drawable="@drawable/ic_launcher_foreground" />
</adaptive-icon>
`)
	for _, name := range []string{"ic_launcher.xml", "ic_launcher_round.xml"} {
		write(filepath.Join(root, "mipmap-anydpi-v26", name), adaptive)
	}
	write(filepath.Join(root, "values/ic_launcher_background.xml"), []byte(`<?xml version="1.0" encoding="utf-8"?>
<resources>
    <color name="ic_launcher_background">#0F1817</color>
</resources>
`))
}
func save(path string, img image.Image) {
	check(os.MkdirAll(filepath.Dir(path), 0755))
	f, err := os.Create(path)
	check(err)
	check(png.Encode(f, img))
	check(f.Close())
}
func write(path string, data []byte) {
	check(os.MkdirAll(filepath.Dir(path), 0755))
	check(os.WriteFile(path, data, 0644))
}
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
