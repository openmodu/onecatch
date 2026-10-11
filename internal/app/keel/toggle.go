package keeldesktop

import (
	"fmt"
	"strconv"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
)

// Filled choices remain visible when decorative borders are disabled. Deferred
// form choices use a checkbox; immediately applied settings use a switch.
type filledToggle struct {
	label                     string
	value, disabled, checkbox bool
	hideLabel                 bool
	onChange                  func(bool)
}

func newFilledToggle(label string, value, checkbox bool) *filledToggle {
	return &filledToggle{label: label, value: value, checkbox: checkbox}
}
func (v *filledToggle) Value() bool            { return v.value }
func (v *filledToggle) SetValue(value bool)    { v.value = value }
func (v *filledToggle) SetDisabled(value bool) { v.disabled = value }
func (v *filledToggle) Render(cx *el.Context) el.Element {
	role := "switch"
	track := theme.SubtleHover
	if v.value {
		track = theme.Primary
	}
	var mark el.Element
	if v.checkbox {
		role = "checkbox"
		box := el.Div().Size(el.Dp(18)).NoShrink().Rounded(5).Bg(track).Center()
		if v.value {
			box.Child(lucide(cx, "check", 14, theme.OnColor))
		}
		mark = box
	} else {
		knob := el.Div().Size(el.Dp(16)).NoShrink().Rounded(8).Bg(theme.Surface)
		left := float32(0)
		if v.value {
			left = 16
		}
		mark = el.Div().W(el.Dp(36)).H(el.Dp(20)).NoShrink().Rounded(10).Bg(track).Px(2).Row().Items(el.Center).Child(el.Div().W(el.Dp(left)).NoShrink(), knob)
	}
	toggle := func() {
		if !v.disabled {
			if v.onChange != nil {
				v.onChange(!v.value)
			} else {
				v.value = !v.value
			}
		}
	}
	row := el.Div().ID(fmt.Sprintf("filled-toggle/%p", v)).Role(role).Name(uiText(v.label)).Value(strconv.FormatBool(v.value)).Selected(v.value).Disabled(v.disabled).Focusable(true).CursorPointer().Row().Items(el.Center).Gap(10).Py(4).Rounded(6).OnClick(toggle).OnKey(func(e el.KeyEvent) bool {
		if e.State == el.KeyPress && (e.Name == "Space" || e.Name == "Return" || e.Name == "Enter") {
			toggle()
			return true
		}
		return false
	}).FocusStyle(neutralInputFocus).Child(mark)
	if !v.hideLabel {
		row.Child(el.Text(uiText(v.label)).TextSize(13))
	}
	return row
}
