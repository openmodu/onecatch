package keeldesktop

import (
	"github.com/dyike/keel/ui/theme"
	"image/color"
	"math"
)

type terminalPalette struct {
	Background, Foreground, Cursor, Selection color.NRGBA
	Colors                                    [16]color.NRGBA
}

func resolveNativeTerminalPalette(id string, dark bool) terminalPalette {
	p := terminalPalette{Background: theme.Surface, Foreground: theme.Text, Cursor: theme.Primary, Selection: theme.SubtleHover}
	base := [16]uint32{0x000000, 0xcd0000, 0x00cd00, 0xcdcd00, 0x0000ee, 0xcd00cd, 0x00cdcd, 0xe5e5e5, 0x7f7f7f, 0xff0000, 0x00ff00, 0xffff00, 0x5c5cff, 0xff00ff, 0x00ffff, 0xffffff}
	if !dark {
		base = [16]uint32{0x202020, 0xb42318, 0x168329, 0x927500, 0x006bd6, 0x8b43b5, 0x007f87, 0x707070, 0x666666, 0xcf3434, 0x189b32, 0xa18300, 0x0875e1, 0x9957c1, 0x008d96, 0x888888}
	}
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
		p.Background, p.Foreground, p.Cursor, p.Selection = theme.RGB(0x242424), theme.RGB(0xffffff), theme.RGB(0xffd75f), theme.RGB(0x365f91)
		base = [16]uint32{0x000000, 0xff6b6b, 0x7dff8a, 0xffe66d, 0x7db7ff, 0xd99cff, 0x71f6e5, 0xeeeeee, 0x8a8a8a, 0xff0000, 0x00ff00, 0xffff00, 0x5c5cff, 0xff00ff, 0x00ffff, 0xffffff}
	default:
		if dark {
			p.Colors[0], p.Colors[7], p.Colors[8], p.Colors[15] = theme.Text, theme.Text, theme.Muted, theme.Text
		}
		return p
	}
	for i, c := range base {
		p.Colors[i] = theme.RGB(c)
	}
	// The canvas and default text always follow the application appearance.
	p.Background, p.Foreground, p.Cursor = theme.Surface, theme.Text, theme.Primary
	return p
}

// Indexed shell colors often target black terminals. Retain their hue while
// making foreground text readable on a light canvas; true-color output is untouched.
func terminalIndexedForeground(c, bg color.NRGBA) color.NRGBA {
	if bg.R < 128 || bg.G < 128 || bg.B < 128 {
		return c
	}
	luminance := func(c color.NRGBA) float64 {
		channel := func(v uint8) float64 {
			x := float64(v) / 255
			if x <= .04045 {
				return x / 12.92
			}
			return math.Pow((x+.055)/1.055, 2.4)
		}
		return .2126*channel(c.R) + .7152*channel(c.G) + .0722*channel(c.B)
	}
	background := luminance(bg)
	for (background+.05)/(luminance(c)+.05) < 3.5 {
		c.R, c.G, c.B = uint8(float32(c.R)*.92), uint8(float32(c.G)*.92), uint8(float32(c.B)*.92)
	}
	return c
}
