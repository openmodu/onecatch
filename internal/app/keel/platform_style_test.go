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
		if opts.Frameless != mac || opts.NativeTrafficLights != mac || (opts.TrafficLightLayout != nil) != mac || opts.Width != 900 || opts.MenuDisplay != window.MenuDisplayAuto {
			t.Fatalf("%s: %+v", platform, opts)
		}
	}
	if !strings.HasPrefix(string(platformUIFace("windows")), "Segoe UI,") {
		t.Fatal("Windows should prefer Segoe UI")
	}
}
