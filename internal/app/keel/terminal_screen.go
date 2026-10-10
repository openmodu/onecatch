package keeldesktop

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/io/transfer"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/theme"
	"golang.org/x/image/math/fixed"
)

type terminalPoint struct{ X, Y int }
type terminalMatch struct{ From, To terminalPoint }

type terminalScreen struct {
	Emulator                          *vt.Emulator
	ctx                               context.Context
	cancel                            context.CancelFunc
	readDone                          chan struct{}
	attach                            chan string
	input                             chan []byte
	closed                            bool
	Writable                          bool
	scrollRemainder                   float32
	Error                             string
	Palette                           terminalPalette
	IndexedForeground                 [256]color.NRGBA
	paletteID                         string
	focused, focusNext, cursorVisible bool
	cursorStyle                       vt.CursorStyle
	blink                             bool
	cursorChanged                     time.Time
	mouseModes                        map[ansi.Mode]bool
	Scroll                            int
	Anchor, Selection                 terminalPoint
	HasSelection, dragging            bool
	press                             f32.Point
	pressTime                         time.Duration
	IME                               terminalIME
	OnActivate, OnSearch              func()
	OnResize                          func(int, int)
	OnTitle                           func(string)
	OnError                           func(error)
	OnLink                            func(string)
	hoverURL                          string
	mouseButton                       uv.MouseButton
	glyphs                            []text.Glyph
	painter                           theme.GlyphPainter
	cellWidth, lineHeight, baseline   float32
	match                             *terminalMatch
}

// The emulator is owned by the UI queue. Its pipe reader does no UI work and
// always drains responses, even if the PTY writer is temporarily blocked. This
// keeps device-status replies and bracketed paste from deadlocking a frame.
func newTerminalScreen(write func(string, []byte) error) *terminalScreen {
	ctx, cancel := context.WithCancel(context.Background())
	s := &terminalScreen{Emulator: vt.NewEmulator(80, 24), ctx: ctx, cancel: cancel, readDone: make(chan struct{}), attach: make(chan string, 1), input: make(chan []byte, 256), cursorVisible: true, cursorStyle: vt.CursorBar, blink: true, mouseModes: map[ansi.Mode]bool{}}
	s.IME.reset()
	s.Writable = true
	s.Emulator.SetScrollbackSize(10000)
	s.Emulator.SetCallbacks(vt.Callbacks{
		Title: func(title string) {
			if s.OnTitle != nil {
				s.OnTitle(title)
			}
		},
		AltScreen:        func(bool) { s.Scroll = 0; s.HasSelection = false },
		CursorVisibility: func(on bool) { s.cursorVisible = on },
		CursorStyle:      func(style vt.CursorStyle, blink bool) { s.cursorStyle, s.blink = style, blink },
		EnableMode:       func(mode ansi.Mode) { s.mouseModes[mode] = true },
		DisableMode:      func(mode ansi.Mode) { delete(s.mouseModes, mode) },
	})
	go func() {
		defer close(s.readDone)
		buf := make([]byte, 32<<10)
		for {
			n, err := s.Emulator.Read(buf)
			if n > 0 && ctx.Err() == nil {
				data := append([]byte(nil), buf[:n]...)
				select {
				case s.input <- data:
				default:
					s.cancel()
					s.reportError(fmt.Errorf("终端输入队列已满，请重新启动此终端"))
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		var id string
		select {
		case id = <-s.attach:
		case <-ctx.Done():
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case data := <-s.input:
				if write != nil {
					if err := write(id, data); err != nil {
						s.cancel()
						s.reportError(err)
						return
					}
				}
			}
		}
	}()
	return s
}
func (s *terminalScreen) reportError(err error) {
	core.Update(func() {
		if !s.closed {
			s.Error = err.Error()
			if s.OnError != nil {
				s.OnError(err)
			}
		}
	})
}
func (s *terminalScreen) attachSession(id string) {
	select {
	case s.attach <- id:
	default:
	}
}
func (s *terminalScreen) close() {
	if s.closed {
		return
	}
	s.closed = true
	s.cancel()
	// Close only the pipe first. Wait for the reader to leave before writing
	// Emulator.closed, which the upstream Read method accesses without a lock.
	_ = s.Emulator.InputPipe().(io.Closer).Close()
	<-s.readDone
	_ = s.Emulator.Close()
}
func (s *terminalScreen) write(data []byte) {
	if !s.closed {
		before := s.historyLen()
		_, _ = s.Emulator.Write(data)
		if s.Scroll > 0 {
			s.Scroll += max(0, s.historyLen()-before)
		}
		s.Scroll = min(s.Scroll, s.Emulator.ScrollbackLen())
	}
}
func (s *terminalScreen) setPalette(id string, dark bool) {
	palette := resolveNativeTerminalPalette(id, dark)
	if s.Palette == palette {
		return
	}
	s.Palette = palette
	s.Emulator.SetDefaultBackgroundColor(palette.Background)
	s.Emulator.SetDefaultForegroundColor(palette.Foreground)
	s.Emulator.SetDefaultCursorColor(palette.Cursor)
	for i, c := range palette.Colors {
		s.Emulator.SetIndexedColor(i, c)
	}
	for i := range s.IndexedForeground {
		s.IndexedForeground[i] = terminalIndexedForeground(terminalColor(s.Emulator.IndexedColor(i), palette.Foreground), palette.Background)
	}
}
func (s *terminalScreen) historyLen() int {
	if s.Emulator.IsAltScreen() {
		return 0
	}
	return s.Emulator.ScrollbackLen()
}
func (s *terminalScreen) cellAt(x, y int) *uv.Cell {
	n := s.historyLen()
	if y < n {
		return s.Emulator.ScrollbackCellAt(x, y)
	}
	return s.Emulator.CellAt(x, y-n)
}
func (s *terminalScreen) topLine() int          { return s.historyLen() - s.Scroll }
func (s *terminalScreen) scrollBy(lines int)    { s.Scroll = max(0, min(s.historyLen(), s.Scroll+lines)) }
func terminalPointLess(a, b terminalPoint) bool { return a.Y < b.Y || a.Y == b.Y && a.X < b.X }
func (s *terminalScreen) selectionBounds() (terminalPoint, terminalPoint) {
	a, b := s.Anchor, s.Selection
	if terminalPointLess(b, a) {
		a, b = b, a
	}
	return a, b
}
func (s *terminalScreen) selected(x, y int) bool {
	if !s.HasSelection {
		return false
	}
	a, b := s.selectionBounds()
	p := terminalPoint{x, y}
	return !terminalPointLess(p, a) && terminalPointLess(p, b)
}
func (s *terminalScreen) selectionText() string {
	if !s.HasSelection {
		return ""
	}
	a, b := s.selectionBounds()
	var lines []string
	for y := a.Y; y <= b.Y; y++ {
		var line strings.Builder
		left, right := 0, s.Emulator.Width()
		if y == a.Y {
			left = a.X
		}
		if y == b.Y {
			right = b.X
		}
		for x := left; x < right; x++ {
			c := s.cellAt(x, y)
			if c == nil {
				line.WriteByte(' ')
			} else if c.Width != 0 {
				line.WriteString(c.Content)
			}
		}
		lines = append(lines, strings.TrimRight(line.String(), " "))
	}
	return strings.Join(lines, "\n")
}
func (s *terminalScreen) selectAll() {
	s.Anchor = terminalPoint{0, 0}
	s.Selection = terminalPoint{s.Emulator.Width(), s.historyLen() + s.Emulator.Height() - 1}
	s.HasSelection = true
}
func (s *terminalScreen) copy(gtx core.C) {
	if v := s.selectionText(); v != "" {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(v))})
	}
}
func (s *terminalScreen) paste(v string) {
	if len(v) > 1<<20 {
		s.reportError(fmt.Errorf("终端单次粘贴不能超过 1 MB"))
		return
	}
	if !s.closed && s.Writable {
		s.Scroll = 0
		s.HasSelection = false
		s.Emulator.Paste(v)
	}
}
func (s *terminalScreen) search(query string, backward bool) bool {
	if query == "" {
		s.match = nil
		return true
	}
	start := terminalPoint{-1, -1}
	if s.match != nil {
		start = s.match.From
	}
	var matches []terminalMatch
	for y := 0; y < s.historyLen()+s.Emulator.Height(); y++ {
		var line strings.Builder
		var columns []int
		for x := 0; x < s.Emulator.Width(); x++ {
			c := s.cellAt(x, y)
			if c != nil && c.Width == 0 {
				continue
			}
			content := " "
			if c != nil && c.Content != "" {
				content = strings.ToLower(c.Content)
			}
			for range []byte(content) {
				columns = append(columns, x)
			}
			line.WriteString(content)
		}
		text := line.String()
		for offset := 0; offset < len(text); {
			query = strings.ToLower(query)
			n := strings.Index(text[offset:], query)
			if n < 0 {
				break
			}
			index := offset + n
			if index >= len(columns) {
				break
			}
			end := min(index+len(query)-1, len(columns)-1)
			cell := s.cellAt(columns[end], y)
			w := 1
			if cell != nil {
				w = max(1, cell.Width)
			}
			matches = append(matches, terminalMatch{terminalPoint{columns[index], y}, terminalPoint{columns[end] + w, y}})
			offset = index + max(1, len(query))
		}
	}
	if len(matches) == 0 {
		s.match = nil
		return false
	}
	chosen := matches[0]
	if backward {
		chosen = matches[len(matches)-1]
		for i := len(matches) - 1; i >= 0; i-- {
			if terminalPointLess(matches[i].From, start) {
				chosen = matches[i]
				break
			}
		}
	} else {
		for _, m := range matches {
			if terminalPointLess(start, m.From) {
				chosen = m
				break
			}
		}
	}
	s.match = &chosen
	s.Scroll = max(0, min(s.historyLen(), s.historyLen()-chosen.From.Y))
	return true
}
func (s *terminalScreen) pointAt(p f32.Point) terminalPoint {
	return terminalPoint{max(0, min(s.Emulator.Width(), int((p.X-10)/s.cellWidth))), max(0, min(s.historyLen()+s.Emulator.Height()-1, s.topLine()+int((p.Y-6)/s.lineHeight)))}
}
func (s *terminalScreen) mouseTracking() bool {
	for _, mode := range []ansi.Mode{ansi.ModeMouseX10, ansi.ModeMouseNormal, ansi.ModeMouseButtonEvent, ansi.ModeMouseAnyEvent} {
		if s.mouseModes[mode] {
			return true
		}
	}
	return false
}
func (s *terminalScreen) pointer(gtx core.C, e pointer.Event) {
	p := s.pointAt(e.Position)
	if e.Kind == pointer.Press {
		gtx.Execute(key.FocusCmd{Tag: s})
		if s.OnActivate != nil {
			core.Call(gtx, s.OnActivate)
		}
		s.focused = true
		s.cursorChanged = gtx.Now
		gtx.Execute(pointer.GrabCmd{Tag: s, ID: e.PointerID})
	}
	if e.Kind == pointer.Move || e.Kind == pointer.Drag || e.Kind == pointer.Press {
		s.hoverURL = s.linkAt(p)
	}
	if s.Writable && s.mouseTracking() && !e.Modifiers.Contain(key.ModShift) {
		m := uv.Mouse{X: p.X, Y: p.Y - s.topLine(), Mod: terminalModifiers(e.Modifiers), Button: uv.MouseLeft}
		m.X = min(m.X, s.Emulator.Width()-1)
		if e.Buttons&pointer.ButtonSecondary != 0 {
			m.Button = uv.MouseRight
		}
		if e.Buttons&pointer.ButtonTertiary != 0 {
			m.Button = uv.MouseMiddle
		}
		if e.Kind == pointer.Release {
			m.Button = s.mouseButton
		}
		switch e.Kind {
		case pointer.Press:
			s.mouseButton = m.Button
			s.Emulator.SendMouse(uv.MouseClickEvent(m))
		case pointer.Release:
			s.Emulator.SendMouse(uv.MouseReleaseEvent(m))
		case pointer.Drag, pointer.Move:
			s.Emulator.SendMouse(uv.MouseMotionEvent(m))
		case pointer.Scroll:
			if e.Scroll.Y < 0 {
				m.Button = uv.MouseWheelUp
			} else {
				m.Button = uv.MouseWheelDown
			}
			s.Emulator.SendMouse(uv.MouseWheelEvent(m))
		}
		return
	}
	switch e.Kind {
	case pointer.Press:
		if e.Buttons&pointer.ButtonPrimary == 0 {
			return
		}
		s.press = e.Position
		s.pressTime = e.Time
		s.Anchor, s.Selection = p, p
		s.dragging = true
		s.HasSelection = false
	case pointer.Drag, pointer.Move:
		if s.dragging && e.Buttons&pointer.ButtonPrimary == 0 {
			s.dragging = false
			return
		}
		if s.dragging {
			distance := (e.Position.X-s.press.X)*(e.Position.X-s.press.X) + (e.Position.Y-s.press.Y)*(e.Position.Y-s.press.Y)
			if distance >= 36 && e.Time-s.pressTime >= 180*time.Millisecond {
				s.HasSelection = true
				s.Selection = p
			}
		}
	case pointer.Release:
		if s.dragging && !s.HasSelection && s.hoverURL != "" && s.OnLink != nil {
			dx, dy := e.Position.X-s.press.X, e.Position.Y-s.press.Y
			if dx*dx+dy*dy < 36 {
				core.Call(gtx, func() { s.OnLink(s.hoverURL) })
			}
		}
		s.dragging = false
	case pointer.Cancel:
		s.dragging = false
	case pointer.Scroll:
		s.scrollRemainder -= e.Scroll.Y / s.lineHeight
		lines := int(s.scrollRemainder)
		s.scrollRemainder -= float32(lines)
		s.scrollBy(lines)
	}
}
func terminalModifiers(mod key.Modifiers) uv.KeyMod {
	var v uv.KeyMod
	if mod.Contain(key.ModCtrl) {
		v |= uv.ModCtrl
	}
	if mod.Contain(key.ModAlt) {
		v |= uv.ModAlt
	}
	if mod.Contain(key.ModShift) {
		v |= uv.ModShift
	}
	return v
}
func terminalKey(e key.Event) (uv.KeyPressEvent, bool) {
	if e.Name == key.NameSpace && e.Modifiers&(key.ModCtrl|key.ModAlt) == 0 {
		return uv.KeyPressEvent{}, false
	}

	codes := map[key.Name]rune{key.NameReturn: uv.KeyEnter, key.NameEnter: uv.KeyEnter, key.NameEscape: uv.KeyEscape, key.NameTab: uv.KeyTab, key.NameDeleteBackward: uv.KeyBackspace, key.NameDeleteForward: uv.KeyDelete, key.NameLeftArrow: uv.KeyLeft, key.NameRightArrow: uv.KeyRight, key.NameUpArrow: uv.KeyUp, key.NameDownArrow: uv.KeyDown, key.NameHome: uv.KeyHome, key.NameEnd: uv.KeyEnd, key.NamePageUp: uv.KeyPgUp, key.NamePageDown: uv.KeyPgDown, key.NameSpace: uv.KeySpace}
	code, ok := codes[e.Name]
	if runtime.GOOS == "darwin" && e.Modifiers.Contain(key.ModAlt) && !e.Modifiers.Contain(key.ModCtrl) && ((!ok && len([]rune(string(e.Name))) == 1) || e.Name == key.NameSpace) {
		return uv.KeyPressEvent{}, false
	}
	if !ok {
		if strings.HasPrefix(string(e.Name), "F") {
			if n, err := strconv.Atoi(strings.TrimPrefix(string(e.Name), "F")); err == nil && n >= 1 && n <= 12 {
				code = uv.KeyF1 + rune(n-1)
				ok = true
			}
		}
	}
	if !ok && e.Modifiers&(key.ModCtrl|key.ModAlt) != 0 {
		r := []rune(string(e.Name))
		if len(r) == 1 {
			code = unicode.ToLower(r[0])
			ok = true
		}
	}
	return uv.KeyPressEvent{Code: code, Mod: terminalModifiers(e.Modifiers)}, ok
}
func (s *terminalScreen) key(gtx core.C, e key.Event) {
	if e.State != key.Press {
		return
	}
	name := strings.ToUpper(string(e.Name))
	copyShortcut := runtime.GOOS == "darwin" && e.Modifiers&(key.ModSuper|key.ModCommand) != 0 && !e.Modifiers.Contain(key.ModCtrl) || runtime.GOOS != "darwin" && e.Modifiers.Contain(key.ModCtrl|key.ModShift)
	if copyShortcut && name == "C" {
		s.copy(gtx)
		return
	}
	if e.Modifiers.Contain(key.ModShortcut) && name == "V" {
		gtx.Execute(clipboard.ReadCmd{Tag: s})
		return
	}
	if (e.Modifiers&(key.ModSuper|key.ModCommand) != 0 || e.Modifiers.Contain(key.ModCtrl)) && name == "F" {
		if s.OnSearch != nil {
			core.Call(gtx, s.OnSearch)
		}
		return
	}
	if e.Modifiers&(key.ModSuper|key.ModCommand) != 0 {
		if name == "A" {
			s.selectAll()
		}
		return
	}
	if e.Modifiers.Contain(key.ModShift) && (e.Name == key.NamePageUp || e.Name == key.NamePageDown) {
		direction := 1
		if e.Name == key.NamePageDown {
			direction = -1
		}
		s.scrollBy(direction * max(1, s.Emulator.Height()-1))
		return
	}
	if s.IME.composing() {
		return
	}
	if !s.Writable {
		return
	}
	if k, ok := terminalKey(e); ok {
		s.flushIME()
		s.Scroll = 0
		s.HasSelection = false
		s.Emulator.SendKey(k)
		s.cursorChanged = gtx.Now
	}
}
func (s *terminalScreen) flushIME() {
	if !s.IME.composing() {
		if data := s.IME.commit(); data != "" {
			if !s.Writable {
				return
			}
			s.Scroll = 0
			s.HasSelection = false
			s.Emulator.SendText(data)
		}
	}
}
func (s *terminalScreen) events(gtx core.C) {
	filters := []event.Filter{key.FocusFilter{Target: s}, key.Filter{Focus: s, Optional: key.ModCtrl | key.ModAlt | key.ModShift | key.ModSuper | key.ModCommand}, key.Filter{Focus: s, Name: key.NameTab, Optional: key.ModCtrl | key.ModAlt | key.ModShift | key.ModSuper | key.ModCommand}, pointer.Filter{Target: s, Kinds: pointer.Press | pointer.Release | pointer.Drag | pointer.Move | pointer.Cancel | pointer.Scroll, ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}}, transfer.TargetFilter{Target: s, Type: "application/text"}}
	for {
		e, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		switch e := e.(type) {
		case key.FocusEvent:
			s.focused = e.Focus
			s.IME.reset()
			if e.Focus {
				s.Emulator.Focus()
			} else {
				s.Emulator.Blur()
				s.dragging = false
			}
		case key.Event:
			s.key(gtx, e)
		case key.EditEvent:
			s.IME.edit(e)
		case key.CompositionEvent:
			s.IME.Composition = key.Range(e)
		case key.SelectionEvent:
			s.IME.Selection = key.Range(e)
		case key.SnippetEvent:
		case pointer.Event:
			s.pointer(gtx, e)
		case transfer.DataEvent:
			r := e.Open()
			data, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
			r.Close()
			if err == nil {
				s.flushIME()
				s.paste(string(data))
			} else {
				s.reportError(err)
			}
		}
	}
	s.flushIME()
	for {
		action, ok := core.NextEditAction(gtx, s)
		if !ok {
			break
		}
		switch action {
		case core.EditCopy:
			s.copy(gtx)
		case core.EditPaste:
			gtx.Execute(clipboard.ReadCmd{Tag: s})
		case core.EditSelectAll:
			s.selectAll()
		}
	}
}
func (s *terminalScreen) Layout(gtx core.C) core.D {
	size := gtx.Constraints.Max
	if size.X <= 0 || size.Y <= 0 || s.closed {
		return core.D{Size: size}
	}
	params := text.Parameters{Font: font.Font{Typeface: font.Typeface(theme.MonoFace)}, PxPerEm: fixed.I(gtx.Sp(unit.Sp(12))), MaxWidth: 1 << 24, MaxLines: 1, DisableSpaceTrim: true, Locale: gtx.Locale}
	sh := theme.Material.Shaper
	sh.LayoutString(params, "M")
	advance := fixed.Int26_6(0)
	for {
		g, ok := sh.NextGlyph()
		if !ok {
			break
		}
		advance += g.Advance
	}
	s.cellWidth = max(1, float32(advance)/64)
	s.lineHeight = float32(gtx.Sp(unit.Sp(12))) * 1.24
	s.baseline = float32(gtx.Sp(unit.Sp(12)))
	cols := max(2, min(1000, int(float32(size.X-20)/s.cellWidth)))
	rows := max(1, min(500, int(float32(size.Y-12)/s.lineHeight)))
	if gtx.Enabled() {
		if cols != s.Emulator.Width() || rows != s.Emulator.Height() {
			s.Emulator.Resize(cols, rows)
			s.HasSelection = false
			if s.OnResize != nil {
				s.OnResize(rows, cols)
			}
		}
		s.events(gtx)
		if s.focusNext {
			gtx.Execute(key.FocusCmd{Tag: s})
			s.focusNext = false
		}
	}
	defer clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops).Pop()
	// Keep the terminal canvas integrated with the panel even when a shell
	// emits an OSC default-background override. Explicit cell colors still apply.
	defaultBg := s.Palette.Background
	defaultFg := terminalColor(s.Emulator.ForegroundColor(), s.Palette.Foreground)
	paint.Fill(gtx.Ops, defaultBg)
	event.Op(gtx.Ops, s)
	key.InputHintOp{Tag: s, Hint: key.HintAny}.Add(gtx.Ops)
	semantic.LabelOp("终端内容").Add(gtx.Ops)
	pointer.CursorText.Add(gtx.Ops)
	if s.hoverURL != "" {
		pointer.CursorPointer.Add(gtx.Ops)
	}
	for y := 0; y < rows; y++ {
		row := s.topLine() + y
		for x := 0; x < cols; {
			c := s.cellAt(x, row)
			w := 1
			fg, bg, attrs, content := defaultFg, defaultBg, uint8(0), ""
			if c != nil {
				w = max(1, c.Width)
				fg = s.resolveForegroundColor(c.Style.Fg, fg)
				bg = s.resolveColor(c.Style.Bg, bg)
				attrs = c.Style.Attrs
				content = c.Content
				if attrs&uv.AttrReverse != 0 {
					fg, bg = bg, fg
				}
				if attrs&uv.AttrFaint != 0 {
					fg = mixColor(fg, bg, .5)
				}
				if attrs&uv.AttrConceal != 0 {
					fg = bg
				}
			}
			if s.selected(x, row) {
				bg = s.Palette.Selection
			}
			if s.match != nil && row == s.match.From.Y && x >= s.match.From.X && x < s.match.To.X {
				bg = theme.RGB(0xe58c55)
				fg = theme.RGB(0x1a1a1a)
			}
			left, top := float32(10)+float32(x)*s.cellWidth, float32(6)+float32(y)*s.lineHeight
			if bg != defaultBg {
				paint.FillShape(gtx.Ops, bg, clip.Rect(image.Rect(int(left), int(top), int(math.Ceil(float64(left+float32(w)*s.cellWidth))), int(math.Ceil(float64(top+s.lineHeight))))).Op())
			}
			if content != "" && content != " " && c.Width != 0 {
				p := params
				if attrs&uv.AttrBold != 0 {
					p.Font.Weight = font.Bold
				}
				if attrs&uv.AttrItalic != 0 {
					p.Font.Style = font.Italic
				}
				s.drawText(gtx, p, content, left, top+s.baseline, fg)
				if c.Style.Underline != uv.UnderlineNone {
					paint.FillShape(gtx.Ops, fg, clip.Rect(image.Rect(int(left), int(top+s.baseline+1), int(left+float32(w)*s.cellWidth), int(top+s.baseline+2))).Op())
				}
				if attrs&uv.AttrStrikethrough != 0 {
					paint.FillShape(gtx.Ops, fg, clip.Rect(image.Rect(int(left), int(top+s.baseline*.55), int(left+float32(w)*s.cellWidth), int(top+s.baseline*.55+1))).Op())
				}
			}
			x += w
		}
	}
	cursor := s.Emulator.CursorPosition()
	if s.Scroll == 0 && s.cursorVisible {
		left, top := 10+float32(cursor.X)*s.cellWidth, 6+float32(cursor.Y)*s.lineHeight
		drawCursor := true
		if s.focused && s.blink {
			elapsed := gtx.Now.Sub(s.cursorChanged)
			drawCursor = elapsed%(1100*time.Millisecond) < 550*time.Millisecond
			if gtx.Enabled() {
				gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(550*time.Millisecond - elapsed%(550*time.Millisecond))})
			}
		}
		if drawCursor {
			w, h := float32(2), s.lineHeight
			if s.cursorStyle == vt.CursorBlock {
				w = s.cellWidth
			}
			if s.cursorStyle == vt.CursorUnderline {
				w, h = s.cellWidth, 2
				top += s.lineHeight - 2
			}
			color := terminalColor(s.Emulator.CursorColor(), s.Palette.Cursor)
			if !s.focused {
				color = mixColor(color, s.Palette.Background, .5)
			}
			paint.FillShape(gtx.Ops, color, clip.Rect(image.Rect(int(left), int(top), int(left+w), int(top+h))).Op())
		}
	}
	if s.IME.composing() {
		pos := s.Emulator.CursorPosition()
		s.drawText(gtx, params, string(s.IME.Text), 10+float32(pos.X)*s.cellWidth, 6+float32(pos.Y)*s.lineHeight+s.baseline, s.Palette.Foreground)
	}
	if gtx.Enabled() && gtx.Focused(s) {
		caret := s.Emulator.CursorPosition()
		caretX, caretY := 10+float32(caret.X)*s.cellWidth, 6+float32(caret.Y)*s.lineHeight
		bounds := image.Rectangle{}
		if s.IME.composing() {
			width := float32(ansi.StringWidth(string(s.IME.Text))) * s.cellWidth
			bounds = image.Rect(int(caretX), int(caretY), int(caretX+width), int(caretY+s.lineHeight))
			end := max(0, min(len(s.IME.Text), s.IME.Selection.End))
			caretX += float32(ansi.StringWidth(string(s.IME.Text[:end]))) * s.cellWidth
		}
		gtx.Execute(key.SnippetCmd{Tag: s, Snippet: key.Snippet{Text: string(s.IME.Text), Range: key.Range{End: len(s.IME.Text)}}})
		gtx.Execute(key.SelectionCmd{Tag: s, Range: s.IME.Selection, CompositionBounds: bounds, Caret: key.Caret{Pos: f32.Pt(caretX, caretY+s.baseline), Ascent: s.baseline, Descent: s.lineHeight - s.baseline}})
	}
	return core.D{Size: size}
}

var terminalURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

func terminalWebURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return raw
}
func (s *terminalScreen) linkAt(p terminalPoint) string {
	if c := s.cellAt(p.X, p.Y); c != nil && c.Link.URL != "" {
		return terminalWebURL(c.Link.URL)
	}
	var line strings.Builder
	start, end := -1, -1
	for x := 0; x < s.Emulator.Width(); x++ {
		c := s.cellAt(x, p.Y)
		if c != nil && c.Width == 0 {
			continue
		}
		text := " "
		w := 1
		if c != nil {
			if c.Content != "" {
				text = c.Content
			}
			w = max(1, c.Width)
		}
		if p.X >= x && p.X < x+w {
			start, end = line.Len(), line.Len()+len(text)
		}
		line.WriteString(text)
	}
	if start < 0 {
		return ""
	}
	text := line.String()
	for _, match := range terminalURLPattern.FindAllStringIndex(text, -1) {
		if start >= match[0] && end <= match[1] {
			raw := strings.TrimRight(text[match[0]:match[1]], ".,;:!?)\u3002\uff0c")
			return terminalWebURL(raw)
		}
	}
	return ""
}
func (s *terminalScreen) drawText(gtx core.C, p text.Parameters, value string, x, y float32, c color.NRGBA) {
	sh := theme.Material.Shaper
	sh.LayoutString(p, value)
	s.glyphs = s.glyphs[:0]
	for {
		g, ok := sh.NextGlyph()
		if !ok {
			break
		}
		s.glyphs = append(s.glyphs, g)
	}
	offset := op.Affine(f32.AffineId().Offset(f32.Pt(x, y))).Push(gtx.Ops)
	s.painter.Paint(gtx.Ops, sh, p, s.glyphs, c)
	offset.Pop()
}
func terminalColor(c color.Color, fallback color.NRGBA) color.NRGBA {
	if c == nil {
		return fallback
	}
	return color.NRGBAModel.Convert(c).(color.NRGBA)
}

func (s *terminalScreen) resolveColor(c color.Color, fallback color.NRGBA) color.NRGBA {
	switch v := c.(type) {
	case ansi.BasicColor:
		c = s.Emulator.IndexedColor(int(v))
	case ansi.IndexedColor:
		c = s.Emulator.IndexedColor(int(v))
	}
	return terminalColor(c, fallback)
}

func (s *terminalScreen) resolveForegroundColor(c color.Color, fallback color.NRGBA) color.NRGBA {
	switch v := c.(type) {
	case ansi.BasicColor:
		return s.IndexedForeground[uint8(v)]
	case ansi.IndexedColor:
		return s.IndexedForeground[uint8(v)]
	}
	return s.resolveColor(c, fallback)
}
