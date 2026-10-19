package keeldesktop

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/draw"
	"sync"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/markdown"
	_ "github.com/dyike/keel/ui/netimage"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	gmtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Keep Markdown's image layout and selection ranges. Only bare images gain a
// link action; explicitly linked images retain their author's destination.
type conversationMarkdownImages struct {
	doc         *markdown.Doc
	links       map[string]conversationMarkdownImage
	openLink    func(string)
	showPreview func(*attachmentImagePreview)
	preview     *attachmentImagePreview
	// The loader runs off the UI thread. These are shared references to the
	// document's decoded pixels, not a second decode or a process-wide cache.
	mu     sync.Mutex
	pixels map[string]image.Image
}
type conversationMarkdownImage struct{ source, name string }

func newConversationMarkdownImages(doc *markdown.Doc, openLink func(string), showPreview func(*attachmentImagePreview)) *conversationMarkdownImages {
	return &conversationMarkdownImages{doc: doc, links: map[string]conversationMarkdownImage{}, openLink: openLink, showPreview: showPreview, pixels: map[string]image.Image{}}
}
func (p *conversationMarkdownImages) load(ctx context.Context, source string) (image.Image, error) {
	img, err := markdown.DecodeImage(ctx, source)
	if err == nil && img != nil {
		// ImageOp keeps RGBA directly, but copies other decoder formats. Make
		// that conversion once off-thread so thumbnail and preview share it.
		if _, ok := img.(*image.RGBA); !ok {
			rgba := image.NewRGBA(image.Rectangle{Max: img.Bounds().Size()})
			draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
			img = rgba
		}
		p.mu.Lock()
		p.pixels[source] = img
		p.mu.Unlock()
	}
	return img, err
}
func (p *conversationMarkdownImages) open(target string) {
	item, ok := p.links[target]
	if !ok {
		p.openLink(target)
		return
	}
	p.mu.Lock()
	img := p.pixels[item.source]
	p.mu.Unlock()
	if img == nil {
		return
	}
	// Reuse full-resolution pixels already loaded by Markdown.
	p.preview = &attachmentImagePreview{dialog: kit.Dialog(item.name).CloseButton(true), image: kit.Image(img, item.name), natural: img.Bounds().Size(), name: item.name}
	p.preview.dialog.OnClose(func() { p.preview = nil })
	p.preview.dialog.SetValue(true)
	p.showPreview(p.preview)
}

// Dialogs belong to the outer window, not the embedded Markdown root. This
// keeps the preview outside the message's clip and virtualized viewport.
func (a *app) conversationImagePreview(cx *el.Context) el.Element {
	if p := a.conversation.markdownPreview; p != nil {
		if p.dialog.Value() {
			return p.render(cx)
		}
		a.conversation.markdownPreview = nil
	}
	return el.Div()
}
func (p *conversationMarkdownImages) plugin() markdown.Plugin {
	return markdown.Plugin{Local: true, Extensions: []goldmark.Extender{conversationImageLinks{p}}}
}

type conversationImageLinks struct{ images *conversationMarkdownImages }

func (p conversationImageLinks) Extend(md goldmark.Markdown) {
	md.Parser().AddOptions(parser.WithASTTransformers(util.Prioritized(p, 200)))
}
func (p conversationImageLinks) Transform(doc *ast.Document, reader gmtext.Reader, _ parser.Context) {
	// Collect before moving nodes so walking cannot revisit an inserted parent.
	var images []*ast.Image
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if img, ok := node.(*ast.Image); entering && ok {
			for parent := img.Parent(); parent != nil; parent = parent.Parent() {
				if _, linked := parent.(*ast.Link); linked {
					return ast.WalkSkipChildren, nil
				}
			}
			images = append(images, img)
		}
		return ast.WalkContinue, nil
	})
	for _, img := range images {
		name := string(img.Text(reader.Source()))
		if name == "" {
			name = uiText("图片")
		}
		item := conversationMarkdownImage{source: string(img.Destination), name: name}
		// Deterministic keys keep actions stable when the last streaming chunk is
		// reparsed. Only registered image actions are handled as previews.
		target := fmt.Sprintf("onecatch-image:%x", sha256.Sum256([]byte(item.source+"\x00"+item.name)))
		p.images.links[target] = item
		link := ast.NewLink()
		link.Destination = []byte(target)
		parent := img.Parent()
		parent.InsertBefore(parent, img, link)
		parent.RemoveChild(parent, img)
		link.AppendChild(link, img)
	}
}
