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

// Preserve typed line breaks without rewriting the source or bypassing Markdown.
// The transform is block-local, so appends retain incremental parsing.
func userMessageMarkdownPlugin() markdown.Plugin {
	return markdown.Plugin{Local: true, Extensions: []goldmark.Extender{userMessageLineBreaks{}}}
}

type userMessageLineBreaks struct{}

func (userMessageLineBreaks) Extend(md goldmark.Markdown) {
	md.Parser().AddOptions(parser.WithASTTransformers(util.Prioritized(userMessageLineBreaks{}, 100)))
}

func (userMessageLineBreaks) Transform(doc *ast.Document, _ gmtext.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if text, ok := node.(*ast.Text); entering && ok && text.SoftLineBreak() {
			text.SetSoftLineBreak(false)
			text.SetHardLineBreak(true)
		}
		return ast.WalkContinue, nil
	})
}
