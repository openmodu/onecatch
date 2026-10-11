package keeldesktop

import (
	"bytes"
	"encoding/binary"
	gotext "github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"golang.org/x/image/font/gofont/gomono"
	"slices"
	"testing"
)

// A normal outline font with one unsupported monochrome strike, like Windows
// CJK fonts. Alias two Chinese codepoints to A so this regression is portable
// and does not redistribute a Windows system font.
func legacyBitmapFont(t *testing.T) []byte {
	t.Helper()
	loader, err := ot.NewLoader(bytes.NewReader(gomono.TTF))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := gotext.NewFont(loader)
	if err != nil {
		t.Fatal(err)
	}
	face := gotext.NewFace(parsed)
	gid, _ := face.NominalGlyph('A')
	var groups [][3]uint32
	for r := rune(0); r < 128; r++ {
		if g, ok := face.NominalGlyph(r); ok {
			groups = append(groups, [3]uint32{uint32(r), uint32(r), uint32(g)})
		}
	}
	for _, r := range "为啥" {
		groups = append(groups, [3]uint32{uint32(r), uint32(r), uint32(gid)})
	}
	slices.SortFunc(groups, func(a, b [3]uint32) int { return int(a[0]) - int(b[0]) })
	cmap := make([]byte, 12+16+len(groups)*12)
	put16 := func(b []byte, at int, v uint16) { binary.BigEndian.PutUint16(b[at:], v) }
	put32 := func(b []byte, at int, v uint32) { binary.BigEndian.PutUint32(b[at:], v) }
	put16(cmap, 2, 1)
	put16(cmap, 4, 3)
	put16(cmap, 6, 10)
	put32(cmap, 8, 12)
	put16(cmap, 12, 12)
	put32(cmap, 16, uint32(len(cmap)-12))
	put32(cmap, 24, uint32(len(groups)))
	for i, group := range groups {
		for j, v := range group {
			put32(cmap, 28+i*12+j*4, v)
		}
	}
	eblc := make([]byte, 84)
	put32(eblc, 0, 0x20000)
	put32(eblc, 4, 1)
	put32(eblc, 8, 56)
	put32(eblc, 12, 28)
	put32(eblc, 16, 1)
	put16(eblc, 48, uint16(gid))
	put16(eblc, 50, uint16(gid))
	eblc[52], eblc[53], eblc[54], eblc[55] = 12, 12, 1, 1
	put16(eblc, 56, uint16(gid))
	put16(eblc, 58, uint16(gid))
	put32(eblc, 60, 8)
	put16(eblc, 64, 1)
	put16(eblc, 66, 1)
	put32(eblc, 68, 4)
	put32(eblc, 80, 13)
	ebdt := []byte{0, 2, 0, 0, 8, 8, 0, 8, 8, 255, 255, 255, 255, 255, 255, 255, 255}
	var tables []ot.Table
	for _, tag := range loader.Tables() {
		data, err := loader.RawTable(tag)
		if err != nil {
			t.Fatal(err)
		}
		if tag == ot.MustNewTag("cmap") {
			data = cmap
		}
		tables = append(tables, ot.Table{Tag: tag, Content: data})
	}
	tables = append(tables, ot.Table{Tag: ot.MustNewTag("EBLC"), Content: eblc}, ot.Table{Tag: ot.MustNewTag("EBDT"), Content: ebdt})
	slices.SortFunc(tables, func(a, b ot.Table) int {
		if a.Tag < b.Tag {
			return -1
		}
		if a.Tag > b.Tag {
			return 1
		}
		return 0
	})
	return ot.WriteTTF(tables)
}

func TestWindowsFontOutlines(t *testing.T) {
	files := [][]byte{legacyBitmapFont(t)}
	bitmaps := 0
	for _, data := range files {
		before, err := ot.NewLoaders(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		for _, loader := range before {
			parsed, err := gotext.NewFont(loader)
			if err != nil {
				t.Fatal(err)
			}
			face := gotext.NewFace(parsed)
			for _, r := range "为啥我是说这对比显示中文" {
				gid, ok := face.NominalGlyph(r)
				if !ok {
					continue
				}
				if bitmap, ok := face.GlyphData(gid).(gotext.GlyphBitmap); ok && (bitmap.Format == gotext.BlackAndWhite || bitmap.Format == gotext.BlackAndWhiteByteAligned) {
					bitmaps++
				}
			}
		}
		if err := preferFontOutlines(data); err != nil {
			t.Fatal(err)
		}
		after, err := ot.NewLoaders(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		for _, loader := range after {
			parsed, err := gotext.NewFont(loader)
			if err != nil {
				t.Fatal(err)
			}
			face := gotext.NewFace(parsed)
			for _, r := range "为啥我是说这对比显示中文" {
				gid, ok := face.NominalGlyph(r)
				if !ok {
					continue
				}
				if bitmap, ok := face.GlyphData(gid).(gotext.GlyphBitmap); ok && (bitmap.Format == gotext.BlackAndWhite || bitmap.Format == gotext.BlackAndWhiteByteAligned) {
					t.Fatalf("%c still hides its outline behind a bitmap", r)
				}
			}
		}
	}
	if bitmaps == 0 {
		t.Fatal("fixture did not reproduce unsupported Chinese bitmap glyphs")
	}
	t.Logf("replaced %d unsupported bitmap selections with outlines", bitmaps)
}
