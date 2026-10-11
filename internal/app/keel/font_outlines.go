package keeldesktop

import (
	"encoding/binary"
	"fmt"
)

// Adapted from keel-rex font_prepare.go: prefer outlines over unsupported monochrome strikes.
// Only the in-memory SFNT table directory is changed; installed font files
// remain untouched. Keep color emoji tables and bitmap-only fonts intact.
func preferFontOutlines(data []byte) error {
	if len(data) < 12 {
		return fmt.Errorf("truncated font header")
	}
	offsets := []uint32{0}
	if string(data[:4]) == "ttcf" {
		count := uint64(binary.BigEndian.Uint32(data[8:12]))
		if count > uint64(len(data)-12)/4 {
			return fmt.Errorf("truncated font collection")
		}
		offsets = nil
		for i := uint64(0); i < count; i++ {
			offsets = append(offsets, binary.BigEndian.Uint32(data[12+4*i:16+4*i]))
		}
	}
	for _, offset := range offsets {
		start := uint64(offset)
		if start+12 > uint64(len(data)) {
			return fmt.Errorf("invalid font directory offset")
		}
		count := uint64(binary.BigEndian.Uint16(data[start+4 : start+6]))
		if start+12+16*count > uint64(len(data)) {
			return fmt.Errorf("truncated font directory")
		}
		hasOutline := false
		var bitmapTags []uint64
		for i := uint64(0); i < count; i++ {
			at := start + 12 + 16*i
			switch string(data[at : at+4]) {
			case "glyf", "CFF ", "CFF2":
				hasOutline = true
			case "EBDT", "EBLC", "EBSC", "bdat", "bloc":
				bitmapTags = append(bitmapTags, at)
			}
		}
		if hasOutline {
			for _, at := range bitmapTags {
				// Unknown tags are ignored by the OpenType parser. Their table
				// offsets and shared TTC data do not need to be rewritten.
				copy(data[at:at+4], "OC  ")
			}
		}
	}
	return nil
}
