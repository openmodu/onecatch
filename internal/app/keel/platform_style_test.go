package keeldesktop

import (
	"strings"
	"testing"

	"github.com/dyike/keel/ui/window"
)

func TestPlatformChrome(t *testing.T) {
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
	if platform == "windows" {
		return window.MenuDisplayHidden
	}
	return window.MenuDisplayAuto
}

func TestWindowsFontFamiliesAndShortcuts(t *testing.T) {
	for _, family := range []string{"Microsoft YaHei UI", "SimSun"} {
		if !strings.Contains(string(platformUIFace("windows")), family) || !strings.Contains(string(platformTerminalFace("windows")), family) {
			t.Fatal("missing Chinese fallback", family)
		}
	}
	if got := windowsShortcutLabel("mod+shift+enter"); got != "Ctrl+Shift+Enter" {
		t.Fatal(got)
	}
}
