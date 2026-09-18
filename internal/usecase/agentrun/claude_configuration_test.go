package agentrun

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeClaudeModelSettings(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeModelConfigurationReadsThirdPartySettings(t *testing.T) {
	home := t.TempDir()
	writeClaudeModelSettings(t, filepath.Join(home, ".claude", "settings.json"), `{"model":"opus","env":{"ANTHROPIC_MODEL":"provider/main","ANTHROPIC_DEFAULT_OPUS_MODEL":"provider/reasoner","ANTHROPIC_DEFAULT_OPUS_MODEL_NAME":"Gateway Reasoner","ANTHROPIC_DEFAULT_SONNET_MODEL":"provider/main","ANTHROPIC_CUSTOM_MODEL_OPTION":"provider/custom","ANTHROPIC_CUSTOM_MODEL_OPTION_NAME":"Custom model","ANTHROPIC_AUTH_TOKEN":"secret-token","ANTHROPIC_BASE_URL":"https://private.example"}}`)
	got, err := readClaudeModelConfiguration(home, []string{"HOME=" + home, "ANTHROPIC_MODEL=shell-model"}, []ClaudeModelInfo{{Model: "opus", DisplayName: "Opus", Alias: true}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "provider/main" {
		t.Fatalf("model = %q", got.Model)
	}
	names := map[string]ClaudeModelInfo{}
	for _, model := range got.Models {
		if _, duplicate := names[model.Model]; duplicate {
			t.Fatalf("duplicate: %s", model.Model)
		}
		names[model.Model] = model
	}
	if names["opus"].DisplayName != "Opus · Gateway Reasoner" || !names["opus"].Alias {
		t.Fatalf("opus = %+v", names["opus"])
	}
	if names["provider/custom"].DisplayName != "Custom model" || names["provider/main"].Alias {
		t.Fatalf("models = %+v", got.Models)
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "secret-token") || strings.Contains(string(encoded), "private.example") {
		t.Fatal("credentials leaked")
	}
}

func TestClaudeModelConfigurationProjectAndConfigDirectory(t *testing.T) {
	home, config, project := t.TempDir(), t.TempDir(), t.TempDir()
	writeClaudeModelSettings(t, filepath.Join(home, ".claude", "settings.json"), `{"model":"wrong-config"}`)
	writeClaudeModelSettings(t, filepath.Join(config, "settings.json"), `{"model":"user","env":{"ANTHROPIC_DEFAULT_OPUS_MODEL":"user-opus"}}`)
	writeClaudeModelSettings(t, filepath.Join(project, ".claude", "settings.json"), `{"model":"project","env":{"ANTHROPIC_DEFAULT_OPUS_MODEL":"project-opus"}}`)
	writeClaudeModelSettings(t, filepath.Join(project, ".claude", "settings.local.json"), `{"model":"local"}`)
	env := []string{"HOME=" + home, "CLAUDE_CONFIG_DIR=" + config, "ANTHROPIC_DEFAULT_MODEL=fallback"}
	got, err := readClaudeModelConfiguration(project, env, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "local" {
		t.Fatalf("model = %q", got.Model)
	}
	for _, model := range got.Models {
		if model.Model == "user-opus" || model.Model == "wrong-config" {
			t.Fatalf("stale model = %s", model.Model)
		}
	}
	got, err = readClaudeModelConfiguration(project, append(env, "ANTHROPIC_MODEL=explicit-env"), nil)
	if err != nil || got.Model != "explicit-env" {
		t.Fatalf("model = %q, err = %v", got.Model, err)
	}
}

func TestClaudeModelConfigurationMissingAndInvalidSettings(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "ANTHROPIC_DEFAULT_MODEL=provider/fallback"}
	got, err := readClaudeModelConfiguration(home, env, nil)
	if err != nil || got.Model != "provider/fallback" {
		t.Fatalf("configuration = %+v, err = %v", got, err)
	}
	writeClaudeModelSettings(t, filepath.Join(home, ".claude", "settings.json"), `{"env":{"ANTHROPIC_AUTH_TOKEN":"do-not-print",`)
	_, err = readClaudeModelConfiguration(home, env, nil)
	if err == nil || strings.Contains(err.Error(), "do-not-print") {
		t.Fatalf("error = %v", err)
	}
}

func TestClaudeInspectConfigurationReturnsConfiguredThirdPartyModel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub binary uses POSIX shell")
	}
	home := t.TempDir()
	writeClaudeModelSettings(t, filepath.Join(home, ".claude", "settings.json"), `{"env":{"ANTHROPIC_MODEL":"provider/custom"}}`)
	runner := NewClaudeRunner(stubBinary(t, "Options:\n  --effort <level> (low, high)\n", "", 0))
	got, err := runner.InspectConfiguration(context.Background(), home, []string{"HOME=" + home})
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "provider/custom" || len(got.Models) != 1 || got.Models[0].Model != got.Model {
		t.Fatalf("configuration = %+v", got)
	}
	if strings.Join(got.Efforts, ",") != "low,high" {
		t.Fatalf("efforts = %v", got.Efforts)
	}
}
