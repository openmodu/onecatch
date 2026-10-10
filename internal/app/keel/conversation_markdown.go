package keeldesktop

import (
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/markdown"
	"github.com/dyike/keel/ui/theme"
	"github.com/yuin/goldmark/ast"
)

func renderConversationDocument(cx *el.Context, doc *markdown.Doc) el.Element {
	// Content dividers remain visible without restoring control/card outlines.
	p := theme.Current()
	p.Border = seamColor
	return cx.Themed(p, el.ViewFunc(func(cx *el.Context) el.Element {
		root := doc.Render(cx)
		if column, ok := root.(*el.DivEl); ok {
			column.Gap(16)
		}
		return root
	}))
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
	view := el.Embed(el.ViewFunc(func(cx *el.Context) el.Element { return renderConversationDocument(cx, doc) }))
	return el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().Px(12).Py(2).Opacity(.84).Child(el.Widget(view))
	})
}
