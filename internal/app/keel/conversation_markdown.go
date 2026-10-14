package keeldesktop

import (
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/markdown"
	"github.com/dyike/keel/ui/theme"
)

func (a *app) openConversationLink(url string) {
	open := a.openURL
	if open == nil {
		open = core.OpenURL
	}
	a.fail(open(url))
}

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
