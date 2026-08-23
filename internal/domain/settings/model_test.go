package settings

import "testing"

func TestDefaultsAreSafeAndValid(t *testing.T) {
	got := Defaults()
	if got.Runtimes["modu"].Integration != "sdk" || got.Runtimes["modu"].ConfigSource != "shared" || got.Runtimes["codex"].Integration != "cli" {
		t.Fatalf("runtime integrations = %#v", got.Runtimes)
	}
	if err := Validate(got); err != nil {
		t.Fatal(err)
	}
	if got.Security.AllowFullSandbox {
		t.Fatal("full sandbox must default to disabled")
	}
	if !got.Security.ConfirmFullSandboxEveryRun {
		t.Fatal("full sandbox confirmation must default to enabled")
	}
	if got.Experimental.RemoteWorkersEnabled {
		t.Fatal("remote workers must default to disabled")
	}
	for _, id := range []string{"codex", "claude", "modu", "pi", "grok", "dsh"} {
		if !got.HarnessEnabled(id) {
			t.Fatalf("runtime %q must default to enabled", id)
		}
	}
	for _, id := range []string{"codex", "claude", "modu"} {
		if !got.HarnessRemoteFSEnabled(id) {
			t.Fatalf("runtime %q must default to remote FS enabled", id)
		}
	}
	for _, id := range []string{"pi", "grok", "dsh"} {
		if got.HarnessRemoteFSEnabled(id) {
			t.Fatalf("runtime %q must not support remote FS", id)
		}
	}
	if got.Terminal.Theme != "system" || got.Terminal.Shell != "" {
		t.Fatalf("terminal defaults = %+v", got.Terminal)
	}
}

func TestNormalizeMigratesHarnessSwitchesFromSchemaV1(t *testing.T) {
	input := Defaults()
	input.SchemaVersion = 1
	for id, runtime := range input.Runtimes {
		runtime.Enabled = false
		runtime.RemoteFSEnabled = false
		input.Runtimes[id] = runtime
	}
	got, err := Normalize(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != CurrentSchemaVersion || !got.HarnessEnabled("pi") || !got.HarnessRemoteFSEnabled("codex") || got.HarnessRemoteFSEnabled("pi") {
		t.Fatalf("migrated switches = %+v", got.Runtimes)
	}
}

func TestValidateRejectsRemoteFSForUnsupportedHarness(t *testing.T) {
	input := Defaults()
	pi := input.Runtimes["pi"]
	pi.RemoteFSEnabled = true
	input.Runtimes["pi"] = pi
	if err := Validate(input); err == nil {
		t.Fatal("Pi remote FS setting was accepted")
	}
}

func TestNormalizeMigratesRuntimeIntegrations(t *testing.T) {
	input := Defaults()
	input.Runtimes["codex"] = RuntimeSettings{}
	input.Runtimes["claude"] = RuntimeSettings{}
	input.Runtimes["modu"] = RuntimeSettings{}
	got, err := Normalize(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Runtimes["codex"].Integration != "cli" || got.Runtimes["claude"].Integration != "cli" || got.Runtimes["modu"].Integration != "sdk" || got.Runtimes["modu"].ConfigSource != "shared" {
		t.Fatalf("normalized integrations = %#v", got.Runtimes)
	}
	if err := Validate(got); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeAndValidateModuConfigSource(t *testing.T) {
	input := Defaults()
	input.Runtimes["modu"] = RuntimeSettings{Integration: "sdk", ConfigSource: " OneCatch ", ConfigPath: " /tmp/onecatch-modu/config.toml "}
	got, err := Normalize(input)
	if err != nil {
		t.Fatal(err)
	}
	modu := got.Runtimes["modu"]
	if modu.ConfigSource != "onecatch" || modu.ConfigPath != "/tmp/onecatch-modu/config.toml" {
		t.Fatalf("modu config = %+v", modu)
	}
	if err := Validate(got); err != nil {
		t.Fatal(err)
	}
	modu.ConfigSource = "unknown"
	got.Runtimes["modu"] = modu
	if err := Validate(got); err == nil {
		t.Fatal("invalid Modu config source was accepted")
	}
}

func TestNormalizeAndValidateTerminalSettings(t *testing.T) {
	input := Defaults()
	input.Terminal = TerminalSettings{Shell: " zsh ", Arguments: []string{" -l ", "", " --flag=value with spaces "}, Theme: " MIDNIGHT "}
	got, err := Normalize(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Terminal.Shell != "zsh" || got.Terminal.Theme != "midnight" || len(got.Terminal.Arguments) != 2 || got.Terminal.Arguments[1] != "--flag=value with spaces" {
		t.Fatalf("terminal settings = %+v", got.Terminal)
	}
	if err := Validate(got); err != nil {
		t.Fatal(err)
	}
	got.Terminal.Theme = "neon"
	if err := Validate(got); err == nil {
		t.Fatal("invalid terminal theme was accepted")
	}
}

func TestNormalizeEnvironmentAllowlist(t *testing.T) {
	input := Defaults()
	input.Runtimes["codex"] = RuntimeSettings{EnvironmentAllowlist: []string{" openai_api_key ", "OPENAI_API_KEY"}}
	got, err := Normalize(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Runtimes["codex"].EnvironmentAllowlist) != 1 || got.Runtimes["codex"].EnvironmentAllowlist[0] != "OPENAI_API_KEY" {
		t.Fatalf("unexpected allowlist: %#v", got.Runtimes["codex"].EnvironmentAllowlist)
	}
}

func TestNormalizeAddsModuAndNormalizesProvider(t *testing.T) {
	input := Defaults()
	delete(input.Runtimes, "modu")
	got, err := Normalize(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Runtimes["modu"]; !ok {
		t.Fatal("Normalize did not add Modu runtime")
	}
	got.Runtimes["modu"] = RuntimeSettings{Provider: " Anthropic "}
	got, err = Normalize(got)
	if err != nil || got.Runtimes["modu"].Provider != "anthropic" {
		t.Fatalf("normalized = %+v, err = %v", got.Runtimes["modu"], err)
	}
	if err := Validate(got); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsInvalidRuntimeProvider(t *testing.T) {
	input := Defaults()
	input.Runtimes["modu"] = RuntimeSettings{Provider: "custom"}
	if err := Validate(input); err == nil {
		t.Fatal("expected custom Modu provider to be rejected")
	}
	input = Defaults()
	input.Runtimes["codex"] = RuntimeSettings{Provider: "openai"}
	if err := Validate(input); err == nil {
		t.Fatal("expected provider on Codex to be rejected")
	}
}

func TestNormalizeAndValidateCodexModelSettings(t *testing.T) {
	input := Defaults()
	input.Runtimes["codex"] = RuntimeSettings{ReasoningEffort: " XHIGH ", ServiceTier: " Priority "}
	got, err := Normalize(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Runtimes["codex"].ReasoningEffort != "xhigh" || got.Runtimes["codex"].ServiceTier != "priority" {
		t.Fatalf("codex settings = %+v", got.Runtimes["codex"])
	}
	if err := Validate(got); err != nil {
		t.Fatal(err)
	}

	got.Runtimes["codex"] = RuntimeSettings{ReasoningEffort: "impossible"}
	if err := Validate(got); err == nil {
		t.Fatal("invalid Codex reasoning effort was accepted")
	}
	got = Defaults()
	got.Runtimes["claude"] = RuntimeSettings{ReasoningEffort: "high"}
	if err := Validate(got); err != nil {
		t.Fatalf("Claude Code reasoning effort was rejected: %v", err)
	}
	got.Runtimes["claude"] = RuntimeSettings{ReasoningEffort: "ultra"}
	if err := Validate(got); err == nil {
		t.Fatal("invalid Claude Code reasoning effort was accepted")
	}
	got.Runtimes["claude"] = RuntimeSettings{ServiceTier: "fast"}
	if err := Validate(got); err == nil {
		t.Fatal("Codex service tier was accepted for Claude")
	}
}

func TestValidateRejectsDangerousEnvironmentKeys(t *testing.T) {
	for _, key := range []string{"PATH", "DYLD_INSERT_LIBRARIES", "LD_PRELOAD", "bad-key"} {
		input := Defaults()
		input.Runtimes["codex"] = RuntimeSettings{EnvironmentAllowlist: []string{key}}
		if err := Validate(input); err == nil {
			t.Fatalf("expected %s to be rejected", key)
		}
	}
}

func TestDefaultsIncludeEveryHarness(t *testing.T) {
	got := Defaults()
	for _, id := range []string{"codex", "claude", "modu", "pi", "grok", "dsh"} {
		if _, ok := got.Runtimes[id]; !ok {
			t.Fatalf("runtime %q missing from defaults", id)
		}
	}
	if err := Validate(got); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeAddsNewHarnessesToExistingSettings(t *testing.T) {
	// Settings saved before these harnesses existed must gain them rather than
	// failing validation on the way in.
	input := Defaults()
	for _, id := range []string{"pi", "grok", "dsh"} {
		delete(input.Runtimes, id)
	}
	got, err := Normalize(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"pi", "grok", "dsh"} {
		if got.Runtimes[id].Integration != "cli" {
			t.Fatalf("runtime %q = %+v, want cli integration", id, got.Runtimes[id])
		}
	}
	if err := Validate(got); err != nil {
		t.Fatal(err)
	}
}

func TestValidateReasoningEffortPerHarness(t *testing.T) {
	for _, testCase := range []struct {
		runtime string
		effort  string
		valid   bool
	}{
		// Grok's own catalog tops out at xhigh; pi adds "off" to disable
		// thinking outright and has no "max".
		{"grok", "xhigh", true},
		{"grok", "max", false},
		{"pi", "off", true},
		{"pi", "max", false},
		// DeepSeek Harness exposes no reasoning control to its CLI.
		{"dsh", "", true},
		{"dsh", "high", false},
	} {
		input := Defaults()
		input.Runtimes[testCase.runtime] = RuntimeSettings{Integration: "cli", ReasoningEffort: testCase.effort}
		err := Validate(input)
		if testCase.valid && err != nil {
			t.Fatalf("%s effort %q rejected: %v", testCase.runtime, testCase.effort, err)
		}
		if !testCase.valid && err == nil {
			t.Fatalf("%s effort %q was accepted", testCase.runtime, testCase.effort)
		}
	}
}

func TestValidateProviderAndServiceTierPerHarness(t *testing.T) {
	input := Defaults()
	input.Runtimes["dsh"] = RuntimeSettings{Integration: "cli", Provider: "deepseek-official"}
	if err := Validate(input); err != nil {
		t.Fatalf("dsh provider rejected: %v", err)
	}
	input.Runtimes["dsh"] = RuntimeSettings{Integration: "cli", Provider: "openai"}
	if err := Validate(input); err == nil {
		t.Fatal("an unregistered dsh provider was accepted")
	}

	input = Defaults()
	input.Runtimes["grok"] = RuntimeSettings{Integration: "cli", Provider: "xai"}
	if err := Validate(input); err == nil {
		t.Fatal("grok does not select a provider and must reject one")
	}

	// Codex remains the only harness with a speed/processing tier.
	input = Defaults()
	input.Runtimes["pi"] = RuntimeSettings{Integration: "cli", ServiceTier: "priority"}
	if err := Validate(input); err == nil {
		t.Fatal("pi does not support a service tier and must reject one")
	}
}
