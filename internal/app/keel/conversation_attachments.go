package keeldesktop

import (
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
				view = kit.Image(nil, name).Size(width, height).Fit(kit.ImageContain).Rounded(8).Preview().
					Fallback(el.ViewFunc(func(cx *el.Context) el.Element {
						return el.Text("图片不可用：" + name).TextSize(11).TextColor(theme.Muted).MaxLines(2)
					})).Source(item.Path)
				a.conversation.images[key] = view
			}
			card := el.Div().Gap(4).MaxW(el.Dp(176)).Child(view.Render(cx))
			if !compact {
				card.Child(el.Text(name).TextSize(11).TextColor(theme.Muted).MaxLines(1))
			}
			row.Child(card)
		} else {
			row.Child(el.Div().Row().Items(el.Center).Gap(5).Px(8).Py(5).Rounded(8).Bg(theme.SubtleHover).MaxW(el.Dp(240)).Child(
				lucide(cx, "paperclip", 14, theme.Muted), el.Text(name).TextSize(11).TextColor(theme.Muted).MaxLines(1).MaxW(el.Dp(200)),
			))
		}
	}
	return row
}
