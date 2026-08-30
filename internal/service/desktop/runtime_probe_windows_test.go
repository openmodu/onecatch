//go:build windows

package desktop

import "testing"

func TestRuntimeVersionProbeDoesNotLaunchWindowsCLI(t *testing.T) {
	if version := probeRuntimeVersion(`C:\\tools\\codex.cmd`); version != "" {
		t.Fatalf("Windows runtime version = %q, want no external probe", version)
	}
}
