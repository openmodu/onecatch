package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// TraeRunner owns TRAE's app-server protocol independently of other harnesses.
// Each turn opens its own process; continuation explicitly uses thread/resume.
type TraeRunner struct{ binary string }

func NewTraeRunner(binary string) *TraeRunner {
	if binary == "" {
		binary = "traecli"
	}
	return &TraeRunner{binary: binary}
}
func (r *TraeRunner) Runtime() Runtime { return RuntimeTrae }
func (r *TraeRunner) Available() bool  { _, err := exec.LookPath(r.binary); return err == nil }
func (r *TraeRunner) SupportsInteractivePermissions(sandbox Sandbox) bool {
	return sandbox == "" || sandbox == SandboxReadOnly || sandbox == SandboxWorkspaceWrite
}

func (r *TraeRunner) ListSkills(ctx context.Context, cwd string, environment []string) ([]Skill, error) {
	p, err := r.connect(ctx, cwd, environment)
	if err != nil {
		return nil, err
	}
	defer p.close()
	raw, err := p.call("skills/list", map[string]any{"cwds": []string{cwd}, "forceReload": true})
	if err != nil {
		return nil, err
	}
	return traeSkills(raw)
}

func traeSkills(raw json.RawMessage) ([]Skill, error) {
	var response struct {
		Data []struct {
			Skills []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Path        string `json:"path"`
				Scope       string `json:"scope"`
				Enabled     bool   `json:"enabled"`
				Interface   struct {
					DisplayName      string `json:"displayName"`
					ShortDescription string `json:"shortDescription"`
				} `json:"interface"`
			} `json:"skills"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode TRAE skills: %w", err)
	}
	var skills []Skill
	for _, group := range response.Data {
		for _, item := range group.Skills {
			if item.Enabled && item.Name != "" && item.Path != "" {
				skills = append(skills, Skill{Name: item.Name, Description: item.Description, Path: item.Path, Scope: item.Scope, DisplayName: item.Interface.DisplayName, ShortDescription: item.Interface.ShortDescription})
			}
		}
	}
	return skills, nil
}

func (r *TraeRunner) InspectConfiguration(ctx context.Context, cwd string, environment []string) (HarnessConfiguration, error) {
	p, err := r.connect(ctx, cwd, environment)
	if err != nil {
		return HarnessConfiguration{}, err
	}
	defer p.close()
	var configuration HarnessConfiguration
	var cursor string
	seen := make(map[string]bool)
	for {
		params := map[string]any{"includeHidden": false}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := p.call("model/list", params)
		if err != nil {
			return HarnessConfiguration{}, fmt.Errorf("list TRAE models: %w", err)
		}
		var response struct {
			Data []struct {
				ID                     string `json:"id"`
				Model                  string `json:"model"`
				ConfigName             string `json:"configName"`
				DisplayName            string `json:"displayName"`
				Description            string `json:"description"`
				ContextWindow          int    `json:"contextWindow"`
				IsDefault              bool   `json:"isDefault"`
				DefaultReasoningEffort string `json:"defaultReasoningEffort"`
				BusinessMetadata       struct {
					Load *struct {
						Percent   *int   `json:"load_percent"`
						QueueSize *int64 `json:"queue_size"`
					} `json:"load"`
				} `json:"businessMetadata"`
				SupportedReasoningEfforts []struct {
					ReasoningEffort string `json:"reasoningEffort"`
				} `json:"supportedReasoningEfforts"`
			} `json:"data"`
			NextCursor string `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &response); err != nil {
			return HarnessConfiguration{}, fmt.Errorf("decode TRAE models: %w", err)
		}
		for _, item := range response.Data {
			model := item.ConfigName
			if model == "" {
				model = item.ID
			}
			if model == "" {
				model = item.Model
			}
			if model == "" {
				continue
			}
			entry := HarnessModel{Model: model, DisplayName: item.DisplayName, Description: item.Description, ContextWindow: item.ContextWindow, DefaultEffort: item.DefaultReasoningEffort}
			if load := item.BusinessMetadata.Load; load != nil {
				entry.Load = &HarnessModelLoad{Percent: load.Percent, QueueSize: load.QueueSize}
			}
			if entry.DisplayName == "" {
				entry.DisplayName = model
			}
			for _, effort := range item.SupportedReasoningEfforts {
				entry.Efforts = append(entry.Efforts, effort.ReasoningEffort)
			}
			configuration.Models = append(configuration.Models, entry)
			if item.IsDefault {
				configuration.Model = model
			}
		}
		cursor = response.NextCursor
		if cursor == "" {
			break
		}
		if seen[cursor] {
			return HarnessConfiguration{}, fmt.Errorf("TRAE model catalog repeated cursor %q", cursor)
		}
		seen[cursor] = true
	}
	return configuration, nil
}
