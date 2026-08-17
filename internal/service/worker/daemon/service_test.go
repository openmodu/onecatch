package workerdaemon

import (
	"strings"
	"testing"
)

func testConfig() Config {
	return Config{
		Binary: "/Applications/OneCatch.app/Contents/Resources/bin/onecatch-worker",
		Listen: "0.0.0.0:9231", ID: "build-mac", Name: "Build & Test Mac",
		DataDir: "/Users/worker/.onecatch-worker", TLSCert: "/Users/worker/certs/server.pem",
		TLSKey: "/Users/worker/certs/server-key.pem", MaxConcurrency: 4,
		PathEnvironment: "/opt/homebrew/bin:/usr/bin:/bin",
	}
}

func TestRenderLaunchdEscapesValuesAndNeverEmbedsToken(t *testing.T) {
	payload, err := RenderLaunchd(testConfig(), "/Users/worker/Library/Logs/onecatch-worker.log")
	if err != nil {
		t.Fatal(err)
	}
	value := string(payload)
	for _, expected := range []string{
		"<string>Build &amp; Test Mac</string>",
		"<string>--data-dir</string>",
		"<string>/Users/worker/.onecatch-worker</string>",
		"<key>PATH</key>",
	} {
		if !strings.Contains(value, expected) {
			t.Fatalf("launchd payload missing %q:\n%s", expected, value)
		}
	}
	if strings.Contains(value, "TOKEN") || strings.Contains(value, "--workspace") {
		t.Fatalf("launchd payload contains a legacy secret or mapping:\n%s", value)
	}
}

func TestRenderSystemdQuotesArgumentsAndRestarts(t *testing.T) {
	value := string(RenderSystemd(testConfig()))
	for _, expected := range []string{
		`ExecStart="/Applications/OneCatch.app/Contents/Resources/bin/onecatch-worker"`,
		`"--name" "Build & Test Mac"`,
		`"--data-dir" "/Users/worker/.onecatch-worker"`,
		"Restart=on-failure",
		"WantedBy=default.target",
	} {
		if !strings.Contains(value, expected) {
			t.Fatalf("systemd payload missing %q:\n%s", expected, value)
		}
	}
}
