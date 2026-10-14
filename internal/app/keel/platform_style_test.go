package keeldesktop

import (
	"strings"
	"testing"

	"github.com/dyike/keel/ui/window"
	"github.com/openmodu/onecatch/internal/app/keel/terminalview"
)

func TestPlatformChrome(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	for _, platform := range []string{"windows", "linux", "darwin"} {
		opts := platformWindowOptions(platform, window.Options{Width: 900, TrafficLightLayout: &window.TrafficLightLayout{Height: 52}})
		mac := platform == "darwin"
		if opts.Frameless != (mac || platform == "windows") || opts.NativeTrafficLights != mac || (opts.TrafficLightLayout != nil) != mac || opts.Width != 900 || opts.MenuDisplay != expectedMenuDisplay(platform) {
			t.Fatalf("%s: %+v", platform, opts)
		}
	}
	if !strings.HasPrefix(string(platformUIFace("windows")), "Segoe UI,") {
		t.Fatal("Windows should prefer Segoe UI")
	}
}

func expectedMenuDisplay(platform string) window.MenuDisplay {
	if platform == "windows" || platform == "linux" {
		return window.MenuDisplayHidden
	}
	return window.MenuDisplayAuto
}

func TestWindowsFontFamiliesAndShortcuts(t *testing.T) {
	for _, family := range []string{"Microsoft YaHei UI", "SimSun"} {
		if !strings.Contains(string(platformUIFace("windows")), family) || !strings.Contains(string(terminalview.Typeface("windows")), family) {
			t.Fatal("missing Chinese fallback", family)
		}
	}
	if got := windowsShortcutLabel("mod+shift+enter"); got != "Ctrl+Shift+Enter" {
		t.Fatal(got)
	}
}

func TestLinuxWaylandChrome(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	opts := platformWindowOptions("linux", window.Options{})
	if !opts.Frameless || opts.NativeTrafficLights || opts.MenuDisplay != window.MenuDisplayHidden {
		t.Fatalf("Wayland chrome: %+v", opts)
	}
}
