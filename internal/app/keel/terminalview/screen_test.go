package terminalview

import (
	"image"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/gio/layout"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/gio/unit"
	"github.com/dyike/keel/ui/theme"
)

func expectTerminalInput(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	var got strings.Builder
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	for got.Len() < len(want) {
		select {
		case s := <-ch:
			got.WriteString(s)
		case <-timeout.C:
			t.Fatalf("terminal input %q, want %q", got.String(), want)
		}
	}
	if got.String() != want {
		t.Fatalf("terminal input %q, want %q", got.String(), want)
	}
}
func TestTerminalNativeInputCompositionAndControlKeys(t *testing.T) {
	out := make(chan string, 128)
	s := New(func(_ string, b []byte) error { out <- string(b); return nil })
	s.AttachSession("test")
	defer s.Close()
	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		s.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: r.Source(), Constraints: layout.Exact(image.Pt(640, 240)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		r.Frame(&ops)
	}
	frame()
	r.Source().Execute(key.FocusCmd{Tag: s})
	frame()
	frame()
	// Ordinary text and Space's raw key share a frame; it must be emitted once.
	r.Queue(key.EditEvent{Text: "hello"}, key.Event{Name: key.NameSpace, State: key.Press}, key.EditEvent{Text: " ", Range: key.Range{Start: 5, End: 5}}, key.Event{Name: key.NameReturn, State: key.Press})
	frame()
	expectTerminalInput(t, out, "hello \r")
	// macOS may send the edit before its composition range in the same frame.
	r.Queue(key.EditEvent{Text: "zhong"}, key.CompositionEvent{Start: 0, End: 5})
	frame()
	if string(s.IME.Text) != "zhong" || !s.IME.composing() {
		t.Fatalf("missing preedit: %+v", s.IME)
	}
	select {
	case v := <-out:
		t.Fatalf("preedit leaked into shell: %q", v)
	case <-time.After(20 * time.Millisecond):
	}
	r.Queue(key.EditEvent{Range: key.Range{End: 5}, Text: "中文"}, key.CompositionEvent{Start: -1, End: -1})
	frame()
	expectTerminalInput(t, out, "中文")
	r.Queue(key.Event{Name: "C", Modifiers: key.ModCtrl, State: key.Press}, key.Event{Name: key.NameUpArrow, State: key.Press})
	frame()
	expectTerminalInput(t, out, "\x03\x1b[A")
	if runtime.GOOS == "darwin" {
		r.Queue(key.Event{Name: "E", Modifiers: key.ModAlt, State: key.Press}, key.EditEvent{Text: "é"})
		frame()
		expectTerminalInput(t, out, "é")
	}
	s.Write([]byte("\x1b[?2004h"))
	s.Paste("one\ntwo")
	expectTerminalInput(t, out, "\x1b[200~one\ntwo\x1b[201~")
	s.Writable = false
	r.Queue(key.EditEvent{Text: "discard"}, key.Event{Name: key.NameReturn, State: key.Press})
	frame()
	select {
	case v := <-out:
		t.Fatalf("input sent to exited session: %q", v)
	case <-time.After(20 * time.Millisecond):
	}
}
func TestTerminalANSIWideUnicodeScrollbackAndSearch(t *testing.T) {
	s := New(nil)
	s.AttachSession("")
	defer s.Close()
	s.Emulator.Resize(30, 3)
	s.Write([]byte("\x1b[31mRED\x1b[0m \xe4\xb8"))
	s.Write([]byte("\xad文\r\nsecond\r\nthird\r\nfourth"))
	if s.historyLen() != 1 {
		t.Fatalf("history lines %d", s.historyLen())
	}
	if c := s.cellAt(4, 0); c == nil || c.Content != "中" || c.Width != 2 {
		t.Fatalf("wide cell: %+v", c)
	}
	if c := s.cellAt(0, 0); c.Style.Fg == nil {
		t.Fatal("ANSI foreground missing")
	}
	if !s.Search("中文", false) || s.match.From.X != 4 || s.match.To.X != 8 || s.Scroll != 1 {
		t.Fatalf("wide search: %+v, scroll %d", s.match, s.Scroll)
	}
	s.Anchor, s.Selection, s.HasSelection = terminalPoint{0, 0}, terminalPoint{8, 0}, true
	if text := s.selectionText(); text != "RED 中文" {
		t.Fatalf("selection %q", text)
	}
	s.Write([]byte("\x1b[?1049h\x1b[2J\x1b[Halternate"))
	if s.historyLen() != 0 || s.cellAt(0, 0).Content != "a" {
		t.Fatal("alternate screen was not isolated")
	}
	s.Write([]byte("\x1b[?1049l"))
	if s.historyLen() != 1 || s.cellAt(0, 0).Content != "R" {
		t.Fatal("primary screen not restored")
	}
	s.Emulator.Resize(30, 5)
	s.Write([]byte("\x1b[Hİ hello 中文"))
	if !s.Search("HELLO", false) || s.match.From.X != 2 {
		t.Fatalf("casefold column mapping: %+v", s.match)
	}
}
func TestTerminalLinksKeepOriginalWideColumnMapping(t *testing.T) {
	s := New(nil)
	s.AttachSession("")
	defer s.Close()
	s.Write([]byte("中文 https://example.com/path?q=one.\r\n\x1b]8;;https://example.org\x07label\x1b]8;;\x07"))
	if got := s.linkAt(terminalPoint{7, 0}); got != "https://example.com/path?q=one" {
		t.Fatalf("plain link %q", got)
	}
	if got := s.linkAt(terminalPoint{1, 1}); got != "https://example.org" {
		t.Fatalf("OSC 8 link %q", got)
	}
	if terminalWebURL("javascript:alert(1)") != "" || terminalWebURL("file:///etc/passwd") != "" {
		t.Fatal("non-web terminal link accepted")
	}
}

func TestTerminalPaletteRetintsANSIWithoutChangingTrueColor(t *testing.T) {
	s := New(nil)
	s.AttachSession("")
	defer s.Close()
	s.SetPalette("system", false)
	s.Write([]byte("\x1b[31mR\x1b[38;2;20;40;60mT"))
	red, exact := s.cellAt(0, 0).Style.Fg, s.cellAt(1, 0).Style.Fg
	s.SetPalette("paper", false)
	if s.resolveColor(red, theme.Text) != s.Palette.Colors[1] || s.resolveColor(exact, theme.Text) != theme.RGB(0x14283c) {
		t.Fatal("palette did not distinguish indexed from true color")
	}
	s.SetPalette("midnight", true)
	if s.resolveColor(red, theme.Text) != s.Palette.Colors[1] {
		t.Fatal("existing ANSI cell kept stale palette")
	}
}
func TestTerminalLightIndexedTextRemainsReadable(t *testing.T) {
	old := theme.Current()
	defer theme.Apply(old)
	theme.Apply(theme.Light())
	s := New(nil)
	defer s.Close()
	s.SetPalette("system", false)
	for _, idx := range []ansi.IndexedColor{40, 220, 229} {
		original := s.resolveColor(idx, theme.Text)
		adjusted := s.resolveForegroundColor(idx, theme.Text)
		if adjusted == original {
			t.Fatalf("bright index %d was not adapted for white background", idx)
		}
	}
	exact := theme.RGB(0xffff00)
	if s.resolveForegroundColor(exact, theme.Text) != exact {
		t.Fatal("true color changed")
	}
}
