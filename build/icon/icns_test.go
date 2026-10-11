package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestICNSOptimizationPreservesRepresentationsAndPixels(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			source.SetNRGBA(x, y, color.NRGBA{uint8(x * 7), uint8(y * 7), 128, uint8((x + y) * 4)})
		}
	}
	var buffer bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.NoCompression}
	if err := encoder.Encode(&buffer, source); err != nil {
		t.Fatal(err)
	}
	input := []byte("icns\x00\x00\x00\x00")
	payloads := [][]byte{buffer.Bytes(), buffer.Bytes(), []byte("opaque legacy metadata")}
	ids := []string{"ic07", "ic12", "info"}
	for i, payload := range payloads {
		input = append(input, ids[i]...)
		input = binary.BigEndian.AppendUint32(input, uint32(len(payload)+8))
		input = append(input, payload...)
	}
	binary.BigEndian.PutUint32(input[4:8], uint32(len(input)))
	output, err := optimizeICNS(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(output) >= len(input) {
		t.Fatal("uncompressed PNG was not reduced")
	}
	offset := 8
	for i, id := range ids {
		if string(output[offset:offset+4]) != id {
			t.Fatal("representation identity changed")
		}
		length := int(binary.BigEndian.Uint32(output[offset+4 : offset+8]))
		payload := output[offset+8 : offset+length]
		if id == "info" {
			if !bytes.Equal(payload, payloads[i]) {
				t.Fatal("metadata changed")
			}
		} else {
			decoded, err := png.Decode(bytes.NewReader(payload))
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Bounds() != source.Bounds() {
				t.Fatal("dimensions changed")
			}
			for y := 0; y < 32; y++ {
				for x := 0; x < 32; x++ {
					r, g, b, a := decoded.At(x, y).RGBA()
					rr, gg, bb, aa := source.At(x, y).RGBA()
					if r != rr || g != gg || b != bb || a != aa {
						t.Fatalf("pixel changed at %d,%d", x, y)
					}
				}
			}
		}
		offset += length
	}
	if offset != len(output) {
		t.Fatal("extra representations")
	}
}

func TestICNSRejectsMalformedData(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("not icon"), []byte("icns\x00\x00\x00\x10ic07\x00\x00\x00\x00"), []byte("icns\x00\x00\x00\x10ic07\xff\xff\xff\xff")} {
		if _, err := optimizeICNS(data); err == nil {
			t.Fatal("malformed ICNS accepted")
		}
	}
}
