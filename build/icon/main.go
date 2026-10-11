// Command icon generates the runtime icon from the full-resolution package artwork.
package main

import (
	"flag"
	"image"
	"image/png"
	"log"
	"os"

	"golang.org/x/image/draw"
)

func main() {
	icns := flag.String("optimize-icns", "", "losslessly recompress PNG representations in an ICNS file")
	flag.Parse()
	if *icns != "" {
		if err := optimizeICNSFile(*icns); err != nil {
			log.Fatal(err)
		}
		return
	}

	source, err := os.Open("build/keel/appicon.png")
	if err != nil {
		log.Fatal(err)
	}
	original, err := png.Decode(source)
	source.Close()
	if err != nil {
		log.Fatal(err)
	}
	// 512px covers the runtime Dock/taskbar icon and the 22dp in-app logo.
	// The packager continues using the full-resolution source independently.
	resized := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	draw.CatmullRom.Scale(resized, resized.Bounds(), original, original.Bounds(), draw.Src, nil)
	output, err := os.Create("internal/app/keel/appicon.png")
	if err != nil {
		log.Fatal(err)
	}
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(output, resized); err != nil {
		output.Close()
		log.Fatal(err)
	}
	if err := output.Close(); err != nil {
		log.Fatal(err)
	}
}
