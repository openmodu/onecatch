package desktop

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type reopenWindowSpy struct {
	calls []string
}

func (window *reopenWindowSpy) UnMinimise() {
	window.calls = append(window.calls, "unminimise")
}

func (window *reopenWindowSpy) Show() application.Window {
	window.calls = append(window.calls, "show")
	return nil
}

func (window *reopenWindowSpy) Restore() {
	window.calls = append(window.calls, "restore")
}

func (window *reopenWindowSpy) Focus() {
	window.calls = append(window.calls, "focus")
}

func TestReopenMainWindowRestoresOnlyWhenNothingIsVisible(t *testing.T) {
	window := &reopenWindowSpy{}
	if handled := reopenMainWindow(false, window); !handled {
		t.Fatal("reopenMainWindow() = false, want true")
	}
	want := []string{"unminimise", "show", "restore", "focus"}
	if len(window.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", window.calls, want)
	}
	for index := range want {
		if window.calls[index] != want[index] {
			t.Fatalf("calls = %v, want %v", window.calls, want)
		}
	}
}

func TestReopenMainWindowLeavesVisibleWindowsAlone(t *testing.T) {
	window := &reopenWindowSpy{}
	if handled := reopenMainWindow(true, window); handled {
		t.Fatal("reopenMainWindow() = true, want false")
	}
	if len(window.calls) != 0 {
		t.Fatalf("calls = %v, want none", window.calls)
	}
}

func TestReopenMainWindowIgnoresMissingMainWindow(t *testing.T) {
	if handled := reopenMainWindow(false, nil); handled {
		t.Fatal("reopenMainWindow() = true, want false")
	}
}
