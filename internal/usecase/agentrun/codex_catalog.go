package agentrun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The app-server model list determines which models can be selected. Its
// response omits context limits in older versions, so enrich those entries
// from the same Codex home's catalog without inventing model capabilities.
func mergeCodexCatalog(configuration *CodexConfiguration, cwd string, environment []string, provider, catalogPath string) {
	if catalogPath == "" && provider != "" && provider != "openai" {
		return // A custom provider must not inherit OpenAI's cached catalog.
	}
	custom := catalogPath != ""
	if custom {
		if !filepath.IsAbs(catalogPath) {
			catalogPath = filepath.Join(cwd, catalogPath)
		}
	} else {
		if environment == nil {
			environment = os.Environ()
		}
		home := environmentValue(environment, "CODEX_HOME")
		if home == "" {
			userHome := environmentValue(environment, "HOME")
			if userHome == "" {
				userHome, _ = os.UserHomeDir()
			}
			if userHome == "" {
				return
			}
			home = filepath.Join(userHome, ".codex")
		}
		if !filepath.IsAbs(home) {
			home = filepath.Join(cwd, home)
		}
		catalogPath = filepath.Join(home, "models_cache.json")
	}
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return
	}
	var catalog struct {
		FetchedAt time.Time `json:"fetched_at"`
		Models    []struct {
			Slug             string `json:"slug"`
			ContextWindow    int    `json:"context_window"`
			MaxContextWindow int    `json:"max_context_window"`
		} `json:"models"`
	}
	if json.Unmarshal(data, &catalog) != nil {
		return
	}
	// Do not apply limits from an abandoned installation's stale cache.
	if !custom && (catalog.FetchedAt.IsZero() || time.Since(catalog.FetchedAt) > 24*time.Hour) {
		return
	}
	indices := make(map[string]int, len(configuration.Models))
	for i, model := range configuration.Models {
		indices[model.Model] = i
	}
	for _, item := range catalog.Models {
		if item.Slug == "" {
			continue
		}
		if index, ok := indices[item.Slug]; ok {
			model := &configuration.Models[index]
			if model.ContextWindow == 0 {
				model.ContextWindow = item.ContextWindow
			}
			if model.MaxContextWindow == 0 {
				model.MaxContextWindow = item.MaxContextWindow
			}
			continue
		}
	}
}

func codexMaxContextWindowOverride(configuration CodexConfiguration, selected string) (string, bool) {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		selected = configuration.Model
	}
	for _, model := range configuration.Models {
		if (selected != "" && (model.Model == selected || model.ID == selected)) || (selected == "" && model.IsDefault) {
			if model.MaxContextWindow > model.ContextWindow && model.MaxContextWindow > 0 {
				return fmt.Sprintf("model_context_window=%d", model.MaxContextWindow), true
			}
			break
		}
	}
	return "", false
}
