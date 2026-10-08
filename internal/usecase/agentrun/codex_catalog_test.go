package agentrun

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeCodexCatalog(t *testing.T, home string, age time.Duration) {
	t.Helper()
	catalog := map[string]any{"fetched_at": time.Now().Add(-age), "models": []map[string]any{
		{"slug": "rpc-model", "visibility": "list", "display_name": "stale name", "context_window": 100, "max_context_window": 200},
		{"slug": "new-model", "visibility": "list", "display_name": "New model", "default_reasoning_level": "high", "supported_reasoning_levels": []map[string]string{{"effort": "high"}}, "additional_speed_tiers": []string{"fast"}, "context_window": 100, "max_context_window": 400},
		{"slug": "hidden-model", "visibility": "hide"},
	}}
	data, _ := json.Marshal(catalog)
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCodexCatalogEnrichesAdvertisedModelsWithoutReplacingDefaults(t *testing.T) {
	home := t.TempDir()
	writeCodexCatalog(t, home, time.Minute)
	config := CodexConfiguration{Model: "rpc-model", Models: []CodexModelInfo{{Model: "rpc-model", DisplayName: "RPC name", IsDefault: true, MaxContextWindow: 250}, {Model: "new-model"}}}
	mergeCodexCatalog(&config, t.TempDir(), []string{"CODEX_HOME=" + home}, "openai", "")
	if len(config.Models) != 2 || config.Model != "rpc-model" {
		t.Fatalf("configuration = %+v", config)
	}
	if got := config.Models[0]; got.DisplayName != "RPC name" || !got.IsDefault || got.MaxContextWindow != 250 || got.ContextWindow != 100 {
		t.Fatalf("RPC model changed: %+v", got)
	}
	if got := config.Models[1]; got.Model != "new-model" || got.IsDefault || got.MaxContextWindow != 400 {
		t.Fatalf("new model = %+v", got)
	}
	mergeCodexCatalog(&config, "", []string{"CODEX_HOME=" + home}, "openai", "")
	if len(config.Models) != 2 {
		t.Fatal("duplicate catalog models")
	}
}

func TestCodexCatalogRespectsProviderHomeAndFreshness(t *testing.T) {
	home := t.TempDir()
	writeCodexCatalog(t, home, time.Minute)
	for _, tc := range []struct{ name, provider, home string }{
		{"custom provider", "private", home}, {"different home", "openai", t.TempDir()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := CodexConfiguration{Models: []CodexModelInfo{{Model: "new-model"}}}
			mergeCodexCatalog(&config, "", []string{"CODEX_HOME=" + tc.home}, tc.provider, "")
			if config.Models[0].MaxContextWindow != 0 {
				t.Fatalf("unexpected limits: %+v", config.Models)
			}
		})
	}
	writeCodexCatalog(t, home, 48*time.Hour)
	config := CodexConfiguration{Models: []CodexModelInfo{{Model: "new-model"}}}
	mergeCodexCatalog(&config, "", []string{"CODEX_HOME=" + home}, "openai", "")
	if config.Models[0].MaxContextWindow != 0 {
		t.Fatal("stale cache used")
	}
	// An explicitly configured catalog is not a timestamped cache.
	config.Models = []CodexModelInfo{{Model: "new-model"}}
	mergeCodexCatalog(&config, home, nil, "private", "models_cache.json")
	if config.Models[0].MaxContextWindow != 400 {
		t.Fatal("custom catalog not used")
	}
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	mergeCodexCatalog(&config, home, nil, "private", "models_cache.json")
	if len(config.Models) != 1 || config.Models[0].MaxContextWindow != 400 {
		t.Fatal("invalid catalog changed existing data")
	}
}

func TestCodexConfigurationReadsAllPagesAndWorkspace(t *testing.T) {
	workspace := t.TempDir()
	capture := filepath.Join(t.TempDir(), "requests")
	bin := stubBinary(t, "", "", 0)
	script := `#!/bin/sh
while IFS= read -r line; do
  printf '%s\n' "$line" >> "$ONECATCH_REQUESTS"
  case "$line" in
    *'"method":"initialize"'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    *'"method":"config/read"'*) printf '%s\n' '{"id":2,"result":{"config":{"model":"last-page-model"}}}' ;;
    *'"cursor":"page-2"'*) printf '%s\n' '{"id":3,"result":{"data":[{"id":"last-page-model","model":"last-page-model"}],"nextCursor":null}}' ;;
    *'"method":"model/list"'*) printf '%s\n' '{"id":3,"result":{"data":[{"id":"first-page-model","model":"first-page-model"}],"nextCursor":"page-2"}}' ;;
  esac
done
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	config, err := NewCodexRunner(bin).InspectConfiguration(ctx, workspace, []string{"CODEX_HOME=" + t.TempDir(), "ONECATCH_REQUESTS=" + capture})
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Models) != 2 || config.Models[1].Model != "last-page-model" {
		t.Fatalf("models = %+v", config.Models)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var request map[string]any
	for _, line := range strings.Split(string(data), "\n") {
		if json.Unmarshal([]byte(line), &request) == nil && request["method"] == "config/read" {
			if request["params"].(map[string]any)["cwd"] != workspace {
				t.Fatal("workspace missing from config/read")
			}
			return
		}
	}
	t.Fatal("config/read missing")
}
