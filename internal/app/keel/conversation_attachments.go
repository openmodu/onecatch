package keeldesktop

import (
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
)

type conversationAttachment struct {
	Path, Name, MIMEType string
}

func instructionAttachments(paths []string) []conversationAttachment {
	items := make([]conversationAttachment, 0, len(paths))
	for _, path := range paths {
		name := filepath.Base(path)
		// Stored attachment filenames carry an ID before the original name.
		if strings.HasPrefix(name, "attachment_") {
			if _, original, ok := strings.Cut(name, "-"); ok {
				name = original
			}
		}
		items = append(items, conversationAttachment{Path: path, Name: name})
	}
	return items
}

func (item conversationAttachment) isImage() bool {
	if strings.HasPrefix(item.MIMEType, "image/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(item.Path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tif", ".tiff":
		return true
	}
	return false
}

func (a *app) conversationAttachments(cx *el.Context, items []conversationAttachment, id string, compact bool) el.Element {
	row := el.Div().Row().Wrap().Gap(8)
	for _, item := range items {
		name := item.Name
		if name == "" {
			name = filepath.Base(item.Path)
		}
		if item.isImage() {
			if a.conversation.images == nil {
				a.conversation.images = map[string]*kit.ImageView{}
			}
			key := id + "/" + item.Path
			view := a.conversation.images[key]
			if view == nil {
				width, height := float32(176), float32(132)
				if compact {
					width, height = 72, 54
				}
				// A fixed viewport keeps cached message heights stable while the
				// full image loads asynchronously, including restored history.
				view = kit.Image(nil, name).Size(width, height).Fit(kit.ImageContain).Rounded(8).
					Fallback(el.ViewFunc(func(cx *el.Context) el.Element {
						return el.Text("图片不可用：" + name).TextSize(11).TextColor(theme.Muted).MaxLines(2)
					})).Source(item.Path)
				a.conversation.images[key] = view
			}
			if a.conversation.imagePreviews == nil {
				a.conversation.imagePreviews = map[string]*attachmentImagePreview{}
			}
			preview := a.conversation.imagePreviews[key]
			if preview == nil {
				preview = newAttachmentImagePreview(view, item.Path, name)
				a.conversation.imagePreviews[key] = preview
			}
			card := el.Div().Gap(4).MaxW(el.Dp(176)).Child(view.Render(cx))
			if !compact {
				card.Child(el.Text(name).TextSize(11).TextColor(theme.Muted).MaxLines(1))
			}
			card.Child(preview.render(cx))
			row.Child(card)
		} else {
			row.Child(el.Div().Row().Items(el.Center).Gap(5).Px(8).Py(5).Rounded(8).Bg(theme.SubtleHover).MaxW(el.Dp(240)).Child(
				lucide(cx, "paperclip", 14, theme.Muted), el.Text(name).TextSize(11).TextColor(theme.Muted).MaxLines(1).MaxW(el.Dp(200)),
			))
		}
	}
	return row
}

type attachmentImagePreview struct {
	dialog  *kit.DialogView
	image   *kit.ImageView
	natural image.Point
	name    string
}

func newAttachmentImagePreview(view *kit.ImageView, path, name string) *attachmentImagePreview {
	p := &attachmentImagePreview{dialog: kit.Dialog(name).CloseButton(true), name: name}
	view.OnClick(func() {
		// Reuse the already decoded full image; the preview needs no second load.
		preview := *view
		p.image = preview.OnClick(nil)
		p.natural = image.Pt(900, 600)
		if file, err := os.Open(path); err == nil {
			config, _, err := image.DecodeConfig(io.LimitReader(file, attachmentSize+1))
			file.Close()
			if err == nil && config.Width > 0 && config.Height > 0 {
				p.natural = image.Pt(config.Width, config.Height)
			}
		}
		p.dialog.SetValue(true)
	})
	return p
}

func (p *attachmentImagePreview) render(cx *el.Context) el.Element {
	if !p.dialog.Value() || p.image == nil {
		return p.dialog.Render(cx)
	}
	w, h := cx.ViewportSize()
	maxW, maxH := min(float32(960), max(float32(1), w-80)), max(float32(1), h-160)
	scale := min(float32(1), maxW/float32(p.natural.X), maxH/float32(p.natural.Y))
	width, height := max(float32(1), float32(p.natural.X)*scale), max(float32(1), float32(p.natural.Y)*scale)
	// Include the dialog padding and its native scroll containers' tracks so
	// they do not steal width from the image and introduce letterboxing.
	p.dialog.Width(max(float32(280), width+44)).Body(el.ViewFunc(func(cx *el.Context) el.Element {
		content := p.image.Size(width, height).Rounded(0).Render(cx)
		if box, ok := content.(*el.DivEl); ok {
			box.Name("预览图片：" + p.name)
		}
		return el.Div().WFull().Items(el.Center).Child(content)
	}))
	return p.dialog.Render(cx)
}
