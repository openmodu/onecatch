package desktop

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/openmodu/onecatch/internal/service/skillmanager"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type SkillDebugInput struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

type SkillDebugEvent struct {
	Kind   string    `json:"kind"`
	Text   string    `json:"text,omitempty"`
	Failed bool      `json:"failed,omitempty"`
	At     time.Time `json:"at"`
}

type SkillDebugResult struct {
	Succeeded  bool              `json:"succeeded"`
	Output     string            `json:"output"`
	SessionID  string            `json:"sessionId,omitempty"`
	DurationMS int64             `json:"durationMs"`
	Usage      agentrun.Usage    `json:"usage"`
	Events     []SkillDebugEvent `json:"events"`
}

func (a *Service) skillManager() (*skillmanager.Manager, error) {
	a.skillsMu.Lock()
	defer a.skillsMu.Unlock()
	if a.skills == nil && a.skillsErr == nil {
		a.skills, a.skillsErr = skillmanager.New("")
	}
	if a.skillsErr != nil {
		return nil, coded("skills_unavailable", a.skillsErr.Error())
	}
	if a.skills == nil {
		return nil, coded("skills_unavailable", "skill manager is not configured")
	}
	return a.skills, nil
}

// ListManagedSkills returns the skills OneCatch itself manages on disk. It is
// distinct from Service.ListSkills, which asks a runtime for the catalog it
// resolves at a workspace.
func (a *Service) ListManagedSkills() ([]skillmanager.Skill, error) {
	manager, err := a.skillManager()
	if err != nil {
		return nil, err
	}
	return manager.List()
}

func (a *Service) ListSkillFiles(directory string) ([]skillmanager.SkillFileEntry, error) {
	manager, err := a.skillManager()
	if err != nil {
		return nil, err
	}
	return manager.ListFiles(directory)
}

func (a *Service) GetSkill(name string) (skillmanager.SkillDocument, error) {
	manager, err := a.skillManager()
	if err != nil {
		return skillmanager.SkillDocument{}, err
	}
	return manager.Get(name)
}

func (a *Service) CreateSkill(input skillmanager.SaveSkillInput) (skillmanager.SkillDocument, error) {
	manager, err := a.skillManager()
	if err != nil {
		return skillmanager.SkillDocument{}, err
	}
	return manager.Create(input)
}

func (a *Service) UpdateSkill(input skillmanager.SaveSkillInput) (skillmanager.SkillDocument, error) {
	manager, err := a.skillManager()
	if err != nil {
		return skillmanager.SkillDocument{}, err
	}
	return manager.Update(input)
}

func (a *Service) DeleteSkill(name string) error {
	manager, err := a.skillManager()
	if err != nil {
		return err
	}
	return manager.Delete(name)
}

func (a *Service) ScanSkillSyncTargets() ([]skillmanager.SyncTarget, error) {
	manager, err := a.skillManager()
	if err != nil {
		return nil, err
	}
	return manager.ScanTargets()
}

func (a *Service) AddSkillSyncTarget(input skillmanager.AddTargetInput) (skillmanager.SyncTarget, error) {
	manager, err := a.skillManager()
	if err != nil {
		return skillmanager.SyncTarget{}, err
	}
	return manager.AddTarget(input)
}

func (a *Service) RemoveSkillSyncTarget(id string) error {
	manager, err := a.skillManager()
	if err != nil {
		return err
	}
	return manager.RemoveTarget(strings.TrimSpace(id))
}

func (a *Service) SyncSkills(ctx context.Context, id string) (skillmanager.SyncResult, error) {
	manager, err := a.skillManager()
	if err != nil {
		return skillmanager.SyncResult{}, err
	}
	return manager.Sync(ctx, strings.TrimSpace(id))
}

// DebugSkill creates an isolated Modu SDK session whose AgentDir is
// ~/.onecatch. Modu consequently discovers the managed library at
// ~/.onecatch/skills, while the slash invocation guarantees that the selected
// skill body is injected even when model invocation is disabled in metadata.
func (a *Service) DebugSkill(ctx context.Context, input SkillDebugInput) (SkillDebugResult, error) {
	manager, err := a.skillManager()
	if err != nil {
		return SkillDebugResult{}, err
	}
	document, err := manager.Get(strings.TrimSpace(input.Name))
	if err != nil {
		return SkillDebugResult{}, coded("skill_not_found", err.Error())
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return SkillDebugResult{}, coded("skill_debug_prompt_required", "enter a test prompt")
	}
	if a.runtimes == nil {
		return SkillDebugResult{}, coded("skill_debug_unavailable", "Modu runtime is not configured")
	}
	a.runtimes.mu.RLock()
	config := a.runtimes.config
	moduSettings := a.runtimes.settings[string(agentrun.RuntimeModu)]
	a.runtimes.mu.RUnlock()
	if !moduSettings.Enabled {
		return SkillDebugResult{}, coded("runtime_disabled", "Modu is disabled in Settings")
	}
	runner := agentrun.NewModuSDKRunner(agentrun.ModuSDKOptions{
		ConfigPath: config.ModuConfigPath,
		AgentDir:   filepath.Dir(manager.Root()),
	})
	if !runner.Available() {
		return SkillDebugResult{}, coded("skill_debug_unavailable", "configure the Modu native SDK and a model before debugging skills")
	}

	debugContext, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	startedAt := time.Now()
	events := make([]SkillDebugEvent, 0, 32)
	streamIndexes := make(map[string]int)
	sink := func(event agentrun.Event) {
		if event.Kind == agentrun.KindUsage || event.Kind == agentrun.KindStarted || event.Kind == agentrun.KindResult {
			return
		}
		text := event.Text
		if text == "" && event.Phase != agentrun.StreamStart {
			return
		}
		if event.StreamID != "" {
			if index, ok := streamIndexes[event.StreamID]; ok {
				switch event.Phase {
				case agentrun.StreamDelta:
					events[index].Text += text
				case agentrun.StreamSnapshot:
					events[index].Text = text
				}
				return
			}
		}
		if len(events) >= 200 {
			return
		}
		if event.StreamID != "" {
			streamIndexes[event.StreamID] = len(events)
		}
		events = append(events, SkillDebugEvent{Kind: string(event.Kind), Text: text, Failed: event.Failed, At: event.At})
	}
	result, runErr := runner.Run(debugContext, agentrun.Request{
		Runtime:   agentrun.RuntimeModu,
		Workspace: filepath.Dir(document.Path),
		Prompt:    "/" + document.Name + " " + prompt,
		Provider:  moduSettings.Provider,
		Model:     moduSettings.DefaultModel,
		Sandbox:   agentrun.SandboxReadOnly,
	}, sink)
	debugResult := SkillDebugResult{
		Succeeded:  result.Succeeded,
		Output:     result.FinalMessage,
		SessionID:  result.SessionID,
		DurationMS: time.Since(startedAt).Milliseconds(),
		Usage:      result.Usage,
		Events:     events,
	}
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			return debugResult, runErr
		}
		return debugResult, fmt.Errorf("debug skill with Modu: %w", runErr)
	}
	return debugResult, nil
}
