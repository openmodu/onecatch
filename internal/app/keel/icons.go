package keeldesktop

import (
	"bytes"
	"embed"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

// The same Lucide path data and runtime marks used by the React desktop.
//
//go:embed icons/*.svg runtimeicons/*
var iconAssets embed.FS

var iconNames = map[kit.IconName]string{
	kit.IconFolder: "folder", kit.IconArchive: "workflow", kit.IconSettings: "settings-2",
	kit.IconPlus: "plus", kit.IconSearch: "search", kit.IconChevronDown: "chevron-down",
	kit.IconChevronLeft: "chevron-left", kit.IconChevronRight: "chevron-right",
	kit.IconEdit: "square-pen", kit.IconMore: "ellipsis", kit.IconFile: "file-text",
	kit.IconLock: "shield", kit.IconHome: "hard-drive", kit.IconTrash: "trash-2",
	kit.IconRetry: "refresh-cw", kit.IconInbox: "blocks", kit.IconReceipt: "chart-no-axes-column",
	kit.IconUser: "briefcase-business", kit.IconExternalLink: "arrow-up", kit.IconBan: "square",
}

type iconKey struct {
	name     string
	size     float32
	color    color.NRGBA
	original bool
}

var iconCache = make(map[iconKey]*kit.IconView)
var rasterIconCache = make(map[iconKey]*kit.ImageView)

func lucide(cx *el.Context, name string, size float32, fg color.NRGBA) el.Element {
	return assetIcon(cx, "icons/"+name+".svg", size, fg, false)
}
func outlineIcon(cx *el.Context, name kit.IconName, size float32, fg color.NRGBA) el.Element {
	if path, ok := iconNames[name]; ok {
		return lucide(cx, path, size, fg)
	}
	return kit.Icon(name).Size(size).Color(fg).Render(cx)
}

// All callers run under Keel's frame lock.
func assetIcon(cx *el.Context, path string, size float32, fg color.NRGBA, original bool) el.Element {
	key := iconKey{name: path, size: size, color: fg, original: original}
	view := iconCache[key]
	if view == nil {
		data, err := iconAssets.ReadFile(path)
		if err != nil {
			return kit.Icon(kit.IconFile).Size(size).Color(fg).Render(cx)
		}
		data, err = nativeSVG(data)
		if err != nil {
			return kit.Icon(kit.IconFile).Size(size).Color(fg).Render(cx)
		}
		view, err = kit.SVGIcon(data)
		if err != nil {
			return kit.Icon(kit.IconFile).Size(size).Color(fg).Render(cx)
		}
		view.Size(size).Color(fg).OriginalColors(original)
		iconCache[key] = view
	}
	return view.Render(cx)
}

func rasterIcon(cx *el.Context, path string, size float32, tint ...color.NRGBA) el.Element {
	key := iconKey{name: path, size: size}
	if len(tint) > 0 {
		key.color = tint[0]
	}
	view := rasterIconCache[key]
	if view == nil {
		data, err := iconAssets.ReadFile(path)
		if err != nil {
			return lucide(cx, "bot", size, color.NRGBA{A: 255})
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return lucide(cx, "bot", size, color.NRGBA{A: 255})
		}
		if len(tint) > 0 {
			monochrome := image.NewNRGBA(img.Bounds())
			draw.DrawMask(monochrome, img.Bounds(), image.NewUniform(tint[0]), image.Point{}, img, img.Bounds().Min, draw.Src)
			img = monochrome
		}
		view = kit.Image(img, "").Size(size, size)
		rasterIconCache[key] = view
	}
	return view.Render(cx)
}
