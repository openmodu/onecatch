package keeldesktop

import (
	"github.com/dyike/keel/ui/theme"
	"image/color"
)

type terminalPalette struct {
	Background, Foreground, Cursor, Selection color.NRGBA
	Colors                                    [16]color.NRGBA
}

func resolveNativeTerminalPalette(id string, dark bool) terminalPalette {
	p := terminalPalette{Background: theme.Bg, Foreground: theme.Text, Cursor: theme.Primary, Selection: theme.SubtleHover}
	base := [16]uint32{0x000000, 0xcd0000, 0x00cd00, 0xcdcd00, 0x0000ee, 0xcd00cd, 0x00cdcd, 0xe5e5e5, 0x7f7f7f, 0xff0000, 0x00ff00, 0xffff00, 0x5c5cff, 0xff00ff, 0x00ffff, 0xffffff}
	for i, c := range base {
		p.Colors[i] = theme.RGB(c)
	}
	switch id {
	case "paper":
		p.Background, p.Foreground, p.Cursor, p.Selection = theme.RGB(0xf4f0e6), theme.RGB(0x28251f), theme.RGB(0x785a2b), theme.RGB(0xd9ceb9)
		base = [16]uint32{0x28251f, 0xa33f35, 0x507245, 0x9a6a22, 0x396c83, 0x765384, 0x39756e, 0xe9e2d4, 0x756f65, 0xff0000, 0x00ff00, 0xffff00, 0x5c5cff, 0xff00ff, 0x00ffff, 0xfffaf0}
	case "midnight":
		p.Background, p.Foreground, p.Cursor, p.Selection = theme.RGB(0x111821), theme.RGB(0xd8dee8), theme.RGB(0x89b4a6), theme.RGB(0x344453)
		base = [16]uint32{0x111821, 0xd47770, 0x8fb38a, 0xd0aa6d, 0x78a6c8, 0xaa8abd, 0x74b8b0, 0xd8dee8, 0x64717f, 0xff0000, 0x00ff00, 0xffff00, 0x5c5cff, 0xff00ff, 0x00ffff, 0xf2f5f8}
	case "contrast":
		p.Background, p.Foreground, p.Cursor, p.Selection = theme.RGB(0x000000), theme.RGB(0xffffff), theme.RGB(0xffd75f), theme.RGB(0x365f91)
		base = [16]uint32{0x000000, 0xff6b6b, 0x7dff8a, 0xffe66d, 0x7db7ff, 0xd99cff, 0x71f6e5, 0xeeeeee, 0x8a8a8a, 0xff0000, 0x00ff00, 0xffff00, 0x5c5cff, 0xff00ff, 0x00ffff, 0xffffff}
	default:
		p.Colors[0], p.Colors[7], p.Colors[8], p.Colors[15] = theme.Text, theme.Bg, theme.Muted, theme.Surface
		return p
	}
	for i, c := range base {
		p.Colors[i] = theme.RGB(c)
	}
	return p
}
