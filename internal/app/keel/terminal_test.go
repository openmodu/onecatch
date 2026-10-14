package keeldesktop

import (
	"context"
	"encoding/base64"
	"image"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/gio/layout"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/gio/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/app/keel/terminalview"
	terminalservice "github.com/openmodu/onecatch/internal/service/terminal"
)

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
		s := terminalview.New(nil)
		s.AttachSession("")
		a.terminal.Panes[id] = &terminalPane{ID: id, Screen: s}
	}
	tab := &terminalTab{ID: "original", Root: terminalLeaf("original")}
	tab.Root.split("original", "sibling", "vertical", "split")
	a.terminal.Tabs = []*terminalTab{tab}
	a.terminal.Active, a.terminal.Focused = "original", "original"
	a.terminal.Open = true
	a.removeTerminalPane("original")
	if len(a.terminal.Tabs) != 1 || a.terminal.Active != "original" || a.terminal.Focused != "sibling" || a.terminal.Panes["sibling"].Screen.Closed() {
		t.Fatal("closing original pane destroyed its tab group")
	}
	a.terminal.Sessions["live"] = "sibling"
	output.SessionID = "live"
	a.terminalEvent(terminalservice.OutputEvent, output)
	if !a.terminal.Panes["sibling"].Screen.Search("first output", false) {
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
	s := terminalview.New(func(id string, data []byte) error {
		return b.terminal.Write(id, base64.StdEncoding.EncodeToString(data))
	})
	s.AttachSession(session.ID)
	defer s.Close()
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
				s.Write(data)
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
	if !s.Search("VALUE:hello中文", false) {
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
		s := terminalview.New(nil)
		s.AttachSession("")
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

func TestTerminalCanvasAlwaysFollowsAppearance(t *testing.T) {
	defer applyPalette(false)
	for _, dark := range []bool{false, true} {
		applyPalette(dark)
		for _, id := range []string{"system", "paper", "midnight", "contrast"} {
			p := terminalview.ResolvePalette(id, dark)
			if p.Background != theme.Surface || p.Foreground != theme.Text {
				t.Fatalf("%s dark=%v overrides application canvas", id, dark)
			}
		}
	}
}

func TestTerminalPanelCloseDisposesAllTabs(t *testing.T) {
	a := alignmentFixture(t)
	panes := make([]*terminalPane, 0, len(a.terminal.Panes))
	for _, p := range a.terminal.Panes {
		panes = append(panes, p)
	}
	a.closeTerminalPanel()
	if len(a.terminal.Tabs) != 0 || len(a.terminal.Panes) != 0 || a.terminal.Open {
		t.Fatal("close retained terminal state")
	}
	for _, p := range panes {
		if !p.Removed {
			t.Fatal("close did not dispose pane")
		}
	}
}
