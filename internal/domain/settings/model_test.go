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

func TestValidateRejectsDangerousEnvironmentKeys(t *testing.T) {
	for _, key := range []string{"PATH", "DYLD_INSERT_LIBRARIES", "LD_PRELOAD", "bad-key"} {
		input := Defaults()
		input.Runtimes["codex"] = RuntimeSettings{EnvironmentAllowlist: []string{key}}
		if err := Validate(input); err == nil {
			t.Fatalf("expected %s to be rejected", key)
		}
	}
}
