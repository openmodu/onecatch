package keeldesktop

import (
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/dyike/keel/gio/font"
	"github.com/dyike/keel/gio/layout"
	"github.com/dyike/keel/gio/text"
	"github.com/dyike/keel/gio/unit"
	"github.com/dyike/keel/ui/theme"
	"golang.org/x/image/math/fixed"
)

// Bounds and clipboard checks retain the full text even when the shaper puts
// a non-whitespace glyph past MaxWidth. Exercise Markdown's one-line shaping
// parameters, including its invisible truncator, before the root clips ink.
func TestConversationWrappedLinesRespectContentWidth(t *testing.T) {
	_ = conversationFixture(t)
	for _, size := range []string{"small", "medium", "large", "extra-large"} {
		for _, scale := range []int{1, 2} {
			for _, width := range []int{340, 700, 860} {
				t.Run(fmt.Sprintf("size=%s/scale=%d/width=%d", size, scale, width), func(t *testing.T) {
					gtx := layout.Context{Metric: unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}}
					gtx.Metric.PxPerSp *= chatFontSize(size) / float32(theme.BodySize)
					maxWidth := width * scale
					content := longWorkflowSummary
					for line := 0; content != ""; line++ {
						shaper := theme.Material.Shaper
						shaper.LayoutString(text.Parameters{
							Font: font.Font{Typeface: theme.Material.Face}, PxPerEm: fixed.I(gtx.Sp(theme.BodySize)),
							MaxWidth: maxWidth, MaxLines: 1, Truncator: "\u200b", WrapPolicy: text.WrapHeuristically,
						}, content)
						count, advance := 0, 0
						var firstX fixed.Int26_6
						for glyph, ok := shaper.NextGlyph(); ok; glyph, ok = shaper.NextGlyph() {
							if glyph.Flags&text.FlagTruncator != 0 {
								continue
							}
							if count == 0 {
								firstX = glyph.X
							}
							count += int(glyph.Runes)
							advance = max(advance, (glyph.X + glyph.Advance - firstX).Ceil())
						}
						if count <= 0 || count > utf8.RuneCountInString(content) {
							t.Fatalf("line %d made invalid progress: runes=%d", line, count)
						}
						n := 0
						for range count {
							_, bytes := utf8.DecodeRuneInString(content[n:])
							n += bytes
						}
						if advance > maxWidth {
							t.Fatalf("line %d places text past the card edge: advance=%d width=%d text=%q", line, advance, maxWidth, content[:n])
						}
						content = content[n:]
					}
				})
			}
		}
	}
}
