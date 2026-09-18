package agentrun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Read only model metadata; credentials and gateway URLs never enter the UI response.
func readClaudeModelConfiguration(cwd string, environment []string, models []ClaudeModelInfo) (ClaudeConfiguration, error) {
	if environment == nil {
		environment = os.Environ()
	}
	home := environmentValue(environment, "HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	configRoot := environmentValue(environment, "CLAUDE_CONFIG_DIR")
	if configRoot == "" {
		configRoot = filepath.Join(home, ".claude")
	}
	paths := []string{filepath.Join(configRoot, "settings.json")}
	if cwd != "" && filepath.Clean(cwd) != filepath.Clean(home) {
		for _, root := range claudeProjectRoots(cwd) {
			paths = append(paths, filepath.Join(root, ".claude", "settings.json"), filepath.Join(root, ".claude", "settings.local.json"))
		}
	}
	env := make(map[string]string)
	for _, entry := range environment {
		if key, value, ok := strings.Cut(entry, "="); ok {
			env[key] = value
		}
	}
	var configured string
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return ClaudeConfiguration{}, fmt.Errorf("read Claude model settings %s: %w", path, err)
		}
		var settings struct {
			Model *string           `json:"model"`
			Env   map[string]string `json:"env"`
		}
		if err := json.Unmarshal(contents, &settings); err != nil {
			return ClaudeConfiguration{}, fmt.Errorf("invalid Claude model settings in %s", path)
		}
		if settings.Model != nil {
			configured = strings.TrimSpace(*settings.Model)
		}
		for key, value := range settings.Env {
			env[key] = value
		}
	}
	if value := strings.TrimSpace(env["ANTHROPIC_MODEL"]); value != "" {
		configured = value
	}
	if configured == "" {
		configured = strings.TrimSpace(env["ANTHROPIC_DEFAULT_MODEL"])
	}
	result := ClaudeConfiguration{Model: configured, Models: append([]ClaudeModelInfo{}, models...)}
	add := func(id, label string, alias bool) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if label == "" {
			label = id
		}
		for i := range result.Models {
			if result.Models[i].Model == id {
				if label != id {
					result.Models[i].DisplayName = label
				}
				return
			}
		}
		result.Models = append(result.Models, ClaudeModelInfo{Model: id, DisplayName: label, Alias: alias})
	}
	for _, alias := range []string{"fable", "opus", "sonnet", "haiku"} {
		key := "ANTHROPIC_DEFAULT_" + strings.ToUpper(alias) + "_MODEL"
		id := strings.TrimSpace(env[key])
		if id == "" {
			continue
		}
		label := strings.TrimSpace(env[key+"_NAME"])
		if label == "" {
			label = id
		}
		add(alias, strings.ToUpper(alias[:1])+alias[1:]+" · "+label, true)
		add(id, label, false)
	}
	add(env["ANTHROPIC_CUSTOM_MODEL_OPTION"], env["ANTHROPIC_CUSTOM_MODEL_OPTION_NAME"], false)
	add(configured, "", false)
	return result, nil
}
