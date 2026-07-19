package settings

import "testing"

func TestDefaultsAreSafeAndValid(t *testing.T) {
	got := Defaults()
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
