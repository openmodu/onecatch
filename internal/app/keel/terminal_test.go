package keeldesktop

import (
	"context"
	"encoding/base64"
	"image"
	"runtime"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	terminalservice "github.com/openmodu/onecatch/internal/service/terminal"
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
	s := newTerminalScreen(func(_ string, b []byte) error { out <- string(b); return nil })
	s.attachSession("test")
	defer s.close()
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
	s.write([]byte("\x1b[?2004h"))
	s.paste("one\ntwo")
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
	s := newTerminalScreen(nil)
	s.attachSession("")
	defer s.close()
	s.Emulator.Resize(30, 3)
	s.write([]byte("\x1b[31mRED\x1b[0m \xe4\xb8"))
	s.write([]byte("\xad文\r\nsecond\r\nthird\r\nfourth"))
	if s.historyLen() != 1 {
		t.Fatalf("history lines %d", s.historyLen())
	}
	if c := s.cellAt(4, 0); c == nil || c.Content != "中" || c.Width != 2 {
		t.Fatalf("wide cell: %+v", c)
	}
	if c := s.cellAt(0, 0); c.Style.Fg == nil {
		t.Fatal("ANSI foreground missing")
	}
	if !s.search("中文", false) || s.match.From.X != 4 || s.match.To.X != 8 || s.Scroll != 1 {
		t.Fatalf("wide search: %+v, scroll %d", s.match, s.Scroll)
	}
	s.Anchor, s.Selection, s.HasSelection = terminalPoint{0, 0}, terminalPoint{8, 0}, true
	if text := s.selectionText(); text != "RED 中文" {
		t.Fatalf("selection %q", text)
	}
	s.write([]byte("\x1b[?1049h\x1b[2J\x1b[Halternate"))
	if s.historyLen() != 0 || s.cellAt(0, 0).Content != "a" {
		t.Fatal("alternate screen was not isolated")
	}
	s.write([]byte("\x1b[?1049l"))
	if s.historyLen() != 1 || s.cellAt(0, 0).Content != "R" {
		t.Fatal("primary screen not restored")
	}
	s.Emulator.Resize(30, 5)
	s.write([]byte("\x1b[Hİ hello 中文"))
	if !s.search("HELLO", false) || s.match.From.X != 2 {
		t.Fatalf("casefold column mapping: %+v", s.match)
	}
}
func TestTerminalSplitsRetainSiblingsWhenOriginalPaneCloses(t *testing.T) {
	n := terminalLeaf("first")
	n.split("first", "second", "vertical", "one")
	n.split("second", "third", "horizontal", "two")
	n = n.remove("first")
	if ids := strings.Join(n.panes(), ","); ids != "second,third" {
		t.Fatalf("lost surviving panes: %s", ids)
	}
	n = n.remove("second")
	if n.Pane != "third" {
		t.Fatal("nested close did not promote sibling")
	}
}

func TestTerminalSessionEventsAndTabLifecycle(t *testing.T) {
	b, err := openBackendWithEngine(context.Background(), t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(b)
	defer a.close()
	output := terminalservice.OutputFrame{SessionID: "early", Data: base64.StdEncoding.EncodeToString([]byte("first output"))}
	a.terminal.Creating = 1
	a.terminalEvent(terminalservice.OutputEvent, output)
	a.terminalEvent(terminalservice.ExitEvent, terminalservice.ExitFrame{SessionID: "early", ExitCode: 9})
	pending := a.terminal.Pending["early"]
	if pending == nil || string(pending.Data[0]) != "first output" || pending.Exit.ExitCode != 9 {
		t.Fatalf("startup events lost: %+v", pending)
	}
	for _, id := range []string{"original", "sibling"} {
		s := newTerminalScreen(nil)
		s.attachSession("")
		a.terminal.Panes[id] = &terminalPane{ID: id, Screen: s}
	}
	tab := &terminalTab{ID: "original", Root: terminalLeaf("original")}
	tab.Root.split("original", "sibling", "vertical", "split")
	a.terminal.Tabs = []*terminalTab{tab}
	a.terminal.Active, a.terminal.Focused = "original", "original"
	a.terminal.Open = true
	a.removeTerminalPane("original")
	if len(a.terminal.Tabs) != 1 || a.terminal.Active != "original" || a.terminal.Focused != "sibling" || a.terminal.Panes["sibling"].Screen.closed {
		t.Fatal("closing original pane destroyed its tab group")
	}
	a.terminal.Sessions["live"] = "sibling"
	output.SessionID = "live"
	a.terminalEvent(terminalservice.OutputEvent, output)
	if !a.terminal.Panes["sibling"].Screen.search("first output", false) {
		t.Fatal("mapped session missed output")
	}
	a.terminalEvent(terminalservice.ExitEvent, terminalservice.ExitFrame{SessionID: "live", ExitCode: 7})
	if a.terminal.Panes["sibling"].Screen.Writable || a.terminal.Panes["sibling"].ExitCode != 7 {
		t.Fatal("exited terminal still accepts input")
	}
	a.toggleTerminal()
	if a.terminal.Open || len(a.terminal.Tabs) != 1 {
		t.Fatal("collapse killed terminal")
	}
	a.toggleTerminal()
	if !a.terminal.Open {
		t.Fatal("existing terminal did not reopen")
	}
	a.closeTerminalTab("original")
	if len(a.terminal.Panes) != 0 || a.terminal.Open {
		t.Fatal("last tab did not close dock")
	}
}
func TestTerminalLinksKeepOriginalWideColumnMapping(t *testing.T) {
	s := newTerminalScreen(nil)
	s.attachSession("")
	defer s.close()
	s.write([]byte("中文 https://example.com/path?q=one.\r\n\x1b]8;;https://example.org\x07label\x1b]8;;\x07"))
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
	s := newTerminalScreen(nil)
	s.attachSession("")
	defer s.close()
	s.setPalette("system", false)
	s.write([]byte("\x1b[31mR\x1b[38;2;20;40;60mT"))
	red, exact := s.cellAt(0, 0).Style.Fg, s.cellAt(1, 0).Style.Fg
	s.setPalette("paper", false)
	if s.resolveColor(red, theme.Text) != s.Palette.Colors[1] || s.resolveColor(exact, theme.Text) != theme.RGB(0x14283c) {
		t.Fatal("palette did not distinguish indexed from true color")
	}
	s.setPalette("midnight", true)
	if s.resolveColor(red, theme.Text) != s.Palette.Colors[1] {
		t.Fatal("existing ANSI cell kept stale palette")
	}
}
func TestKeelTerminalRealPTYInputResizeAndExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell test")
	}
	b, err := openBackendWithEngine(context.Background(), t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()
	events := make(chan any, 128)
	b.terminal.SetEmitter(func(_ string, p any) { events <- p })
	session, err := b.createTerminal(terminalservice.CreateInput{Workspace: t.TempDir(), Shell: "/bin/sh", Arguments: []string{"-c", "printf 'READY\\r\\n'; read value; printf 'VALUE:%s\\r\\n' \"$value\"; stty size; exit 7"}, Rows: 12, Cols: 70})
	if err != nil {
		t.Fatal(err)
	}
	s := newTerminalScreen(func(id string, data []byte) error {
		return b.terminal.Write(id, base64.StdEncoding.EncodeToString(data))
	})
	s.attachSession(session.ID)
	defer s.close()
	var output strings.Builder
	var exit *terminalservice.ExitFrame
	sent := false
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for exit == nil {
		select {
		case event := <-events:
			switch e := event.(type) {
			case terminalservice.OutputFrame:
				data, err := base64.StdEncoding.DecodeString(e.Data)
				if err != nil {
					t.Fatal(err)
				}
				output.Write(data)
				s.write(data)
				if !sent && strings.Contains(output.String(), "READY") {
					if err = b.terminal.Resize(session.ID, 15, 90); err != nil {
						t.Fatal(err)
					}
					s.Emulator.SendText("hello中文")
					s.Emulator.SendText("\r")
					sent = true
				}
			case terminalservice.ExitFrame:
				exit = &e
			}
		case <-deadline.C:
			t.Fatalf("PTY did not exit: %q", output.String())
		}
	}
	if exit.ExitCode != 7 || !strings.Contains(output.String(), "VALUE:hello中文") || !strings.Contains(output.String(), "15 90") {
		t.Fatalf("PTY output/exit: %q %+v", output.String(), exit)
	}
	if !s.search("VALUE:hello中文", false) {
		t.Fatal("PTY output absent from emulator")
	}
	if err = b.close(); err != nil {
		t.Fatal(err)
	}
	if _, err = b.createTerminal(terminalservice.CreateInput{Workspace: t.TempDir()}); err == nil {
		t.Fatal("shell created after shutdown")
	}
}
func TestTerminalSplitResizeThroughNativeInput(t *testing.T) {
	b, err := openBackendWithEngine(context.Background(), t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(b)
	defer a.close()
	for _, id := range []string{"left", "right"} {
		s := newTerminalScreen(nil)
		s.attachSession("")
		a.terminal.Panes[id] = &terminalPane{ID: id, Screen: s}
	}
	n := terminalLeaf("left")
	n.split("left", "right", "vertical", "fixture")
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return a.terminalSplitView(cx, n) }))
	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: r.Source(), Constraints: layout.Exact(image.Pt(800, 280)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		r.Frame(&ops)
	}
	frame()
	frame()
	if n.Available != 800 {
		t.Fatalf("split available width %v", n.Available)
	}
	// The 9px seam must win against the terminal's adjacent pointer region.
	// Pointer events are tested below against Gio's actual hit routing.
	queueTerminalDrag(&r, frame)
	if n.Ratio < .59 || n.Ratio > .61 {
		t.Fatalf("split drag ratio %v", n.Ratio)
	}
}

func queueTerminalDrag(r *input.Router, frame func()) {
	r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(398, 140)})
	frame()
	r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(478, 140)})
	frame()
	r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(478, 140)})
	frame()
}
