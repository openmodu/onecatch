package keeldesktop

import (
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/markdown"
	"github.com/dyike/keel/ui/theme"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	gmtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"strings"
	"unicode/utf8"
)

// Gio's rich Markdown painter draws outlines, while native el.Text also paints
// color bitmap glyphs. Use the documented InlineObject extension for emoji,
// retaining the original Unicode cluster in selection and copied text.
var conversationEmojiKind = ast.NewNodeKind("ConversationEmoji")

type conversationEmoji struct {
	ast.BaseInline
	value string
}

func (n *conversationEmoji) Kind() ast.NodeKind { return conversationEmojiKind }
func (n *conversationEmoji) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

type conversationEmojiExtension struct{}

func (conversationEmojiExtension) Extend(md goldmark.Markdown) {
	md.Parser().AddOptions(parser.WithASTTransformers(util.Prioritized(conversationEmojiTransformer{}, 49)))
}

type conversationEmojiTransformer struct{}

func emojiBase(r rune) bool { return r >= 0x1f000 && r <= 0x1faff || r >= 0x2600 && r <= 0x27bf }
func emojiEnd(line []byte, start int) int {
	_, size := utf8.DecodeRune(line[start:])
	end := start + size
	for end < len(line) {
		next, n := utf8.DecodeRune(line[end:])
		if next == 0xfe0f || next == 0xfe0e || next >= 0x1f3fb && next <= 0x1f3ff {
			end += n
			continue
		}
		if next == 0x200d {
			joined, m := utf8.DecodeRune(line[end+n:])
			if emojiBase(joined) {
				end += n + m
				continue
			}
		}
		break
	}
	return end
}
func (conversationEmojiTransformer) Transform(doc *ast.Document, reader gmtext.Reader, _ parser.Context) {
	source := reader.Source()
	var leaves []*ast.Text
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if n.Kind() == ast.KindCodeSpan || n.Kind() == ast.KindFencedCodeBlock {
				return ast.WalkSkipChildren, nil
			}
			if leaf, ok := n.(*ast.Text); ok {
				leaves = append(leaves, leaf)
			}
		}
		return ast.WalkContinue, nil
	})
	for _, leaf := range leaves {
		data := leaf.Segment.Value(source)
		offset := leaf.Segment.Start
		last := 0
		changed := false
		parent := leaf.Parent()
		for i := 0; i < len(data); {
			r, size := utf8.DecodeRune(data[i:])
			if !emojiBase(r) {
				i += size
				continue
			}
			end := emojiEnd(data, i)
			if i > last {
				parent.InsertBefore(parent, leaf, ast.NewTextSegment(gmtext.NewSegment(offset+last, offset+i)))
			}
			parent.InsertBefore(parent, leaf, &conversationEmoji{value: string(data[i:end])})
			changed = true
			last = end
			i = end
		}
		if changed {
			tail := ast.NewTextSegment(gmtext.NewSegment(offset+last, leaf.Segment.Stop))
			tail.SetSoftLineBreak(leaf.SoftLineBreak())
			tail.SetHardLineBreak(leaf.HardLineBreak())
			parent.InsertBefore(parent, leaf, tail)
			parent.RemoveChild(parent, leaf)
		}
	}
}
func conversationEmojiObject(node ast.Node, _ []byte) markdown.InlineObject {
	value := node.(*conversationEmoji).value
	return markdown.InlineObject{Text: value, Widget: el.Embed(el.ViewFunc(func(*el.Context) el.Element { return el.Text(value).TextSize(float32(theme.BodySize)).LineHeight(1.65) }))}
}
func conversationQuote(node ast.Node, source []byte) el.View {
	var body strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() != ast.KindParagraph {
			return nil
		}
		lines := child.Lines()
		for i := 0; i < lines.Len(); i++ {
			segment := lines.At(i)
			line := string(segment.Value(source))
			body.WriteString(strings.TrimPrefix(strings.TrimLeft(line, " \t"), "> "))
		}
		body.WriteString("\n\n")
	}
	// Empty/nested quotes retain the standard parser rather than losing content.
	if strings.TrimSpace(body.String()) == "" {
		return nil
	}
	doc := newConversationDocument(body.String())
	view := el.Embed(el.ViewFunc(func(cx *el.Context) el.Element { return doc.Render(cx) }))
	return el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().Px(12).Py(2).Opacity(.84).Child(el.Widget(view))
	})
}
