package desktop

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/openmodu/onecatch/internal/service/skillmanager"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

// SkillDebugEventName is the channel a debug run streams over while it is
// still running. The call that started the run still returns the whole result;
// these frames only make the wait legible.
const SkillDebugEventName = "onecatch:skill-debug"

type SkillDebugInput struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
	// RunID is chosen by the caller because frames start arriving before the
	// call returns — there is no earlier moment to hand an identifier back.
	// An empty RunID runs without streaming and cannot be stopped.
	RunID string `json:"runId,omitempty"`
}

type SkillDebugEvent struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
	// Streaming marks an event whose text is still growing, so the renderer
	// can show a caret instead of presenting a half-written message as final.
	Streaming bool      `json:"streaming,omitempty"`
	Failed    bool      `json:"failed,omitempty"`
	At        time.Time `json:"at"`
}

// SkillDebugFrame is one incremental update from a running debug session.
// Index addresses a slot in the run's event list, so a message arriving as
// token deltas is replaced in place rather than appended once per chunk.
type SkillDebugFrame struct {
	RunID string          `json:"runId"`
	Index int             `json:"index"`
	Event SkillDebugEvent `json:"event"`
}

type SkillDebugResult struct {
	Succeeded bool   `json:"succeeded"`
	Output    string `json:"output"`
	// Stopped separates a run the user interrupted from one that failed. Both
	// end early, but only a failure is worth reporting as an error.
	Stopped    bool              `json:"stopped,omitempty"`
	SessionID  string            `json:"sessionId,omitempty"`
	DurationMS int64             `json:"durationMs"`
	Usage      agentrun.Usage    `json:"usage"`
	Events     []SkillDebugEvent `json:"events"`
}

// skillDebugRun is the handle StopSkillDebug reaches a live run through.
type skillDebugRun struct {
	cancel  context.CancelFunc
	stopped atomic.Bool
}

// SetSkillDebugEmitter installs the transport that carries streamed frames to
// the UI. It stays nil outside the desktop app, where a debug run simply
// delivers its whole transcript on completion.
func (a *Service) SetSkillDebugEmitter(emit func(SkillDebugFrame)) {
	a.skillDebugMu.Lock()
	a.skillDebugEmit = emit
	a.skillDebugMu.Unlock()
}

func (a *Service) skillDebugEmitter() func(SkillDebugFrame) {
	a.skillDebugMu.Lock()
	defer a.skillDebugMu.Unlock()
	return a.skillDebugEmit
}

func (a *Service) registerSkillDebug(runID string, run *skillDebugRun) {
	a.skillDebugMu.Lock()
	if a.skillDebugRuns == nil {
		a.skillDebugRuns = make(map[string]*skillDebugRun)
	}
	a.skillDebugRuns[runID] = run
	a.skillDebugMu.Unlock()
}

func (a *Service) releaseSkillDebug(runID string) {
	a.skillDebugMu.Lock()
	delete(a.skillDebugRuns, runID)
	a.skillDebugMu.Unlock()
}

// StopSkillDebug interrupts a streaming run. DebugSkill then returns the
// transcript collected so far instead of an error: the user asked for it to
// end, and the partial output is usually the point of stopping.
func (a *Service) StopSkillDebug(runID string) {
	a.skillDebugMu.Lock()
	run := a.skillDebugRuns[strings.TrimSpace(runID)]
	a.skillDebugMu.Unlock()
	if run == nil {
		return
	}
	run.stopped.Store(true)
	run.cancel()
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

func (a *Service) ReadSkillFile(path string) (skillmanager.SkillFileContent, error) {
	manager, err := a.skillManager()
	if err != nil {
		return skillmanager.SkillFileContent{}, err
	}
	return manager.ReadFile(path)
}

func (a *Service) WriteSkillFile(input skillmanager.SaveSkillFileInput) (skillmanager.SkillFileContent, error) {
	manager, err := a.skillManager()
	if err != nil {
		return skillmanager.SkillFileContent{}, err
	}
	return manager.WriteFile(input.Path, input.Content)
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

	runID := strings.TrimSpace(input.RunID)
	debugContext, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	run := &skillDebugRun{cancel: cancel}
	if runID != "" {
		a.registerSkillDebug(runID, run)
		defer a.releaseSkillDebug(runID)
	}
	publish := a.skillDebugEmitter()
	emit := func(index int, event SkillDebugEvent) {
		if publish == nil || runID == "" {
			return
		}
		publish(SkillDebugFrame{RunID: runID, Index: index, Event: event})
	}

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
		streaming := event.StreamID != "" && event.Phase != agentrun.StreamEnd
		if event.StreamID != "" {
			if index, ok := streamIndexes[event.StreamID]; ok {
				switch event.Phase {
				case agentrun.StreamDelta:
					events[index].Text += text
				case agentrun.StreamSnapshot:
					events[index].Text = text
				}
				events[index].Streaming = streaming
				emit(index, events[index])
				return
			}
		}
		if len(events) >= 200 {
			return
		}
		if event.StreamID != "" {
			streamIndexes[event.StreamID] = len(events)
		}
		events = append(events, SkillDebugEvent{Kind: string(event.Kind), Text: text, Streaming: streaming, Failed: event.Failed, At: event.At})
		emit(len(events)-1, events[len(events)-1])
	}
	result, runErr := runner.Run(debugContext, agentrun.Request{
		Runtime:   agentrun.RuntimeModu,
		Workspace: filepath.Dir(document.Path),
		Prompt:    "/" + document.Name + " " + prompt,
		Provider:  moduSettings.Provider,
		Model:     moduSettings.DefaultModel,
		Sandbox:   agentrun.SandboxReadOnly,
	}, sink)
	// The run is over, so no event can still be growing. Leaving a stream
	// marked live would leave a caret blinking under a finished transcript.
	for index := range events {
		events[index].Streaming = false
	}
	debugResult := SkillDebugResult{
		Succeeded:  result.Succeeded,
		Output:     result.FinalMessage,
		SessionID:  result.SessionID,
		DurationMS: time.Since(startedAt).Milliseconds(),
		Usage:      result.Usage,
		Events:     events,
	}
	stopped := run.stopped.Load() && errors.Is(runErr, context.Canceled)
	if stopped {
		debugResult.Stopped = true
		debugResult.Succeeded = false
	}
	// A failed or interrupted run is the transcript most worth keeping, so the
	// record is written before any error is returned. A history that cannot be
	// written must not fail the run the user just watched.
	// The transcript is already on screen and the run itself is over, so a
	// history that cannot be written is not worth turning into a failed call.
	_ = a.appendSkillDebugRun(SkillDebugRecord{
		RunID:     runID,
		Skill:     document.Name,
		Prompt:    prompt,
		StartedAt: startedAt,
		Result:    debugResult,
	})
	if runErr != nil && !stopped {
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			return debugResult, runErr
		}
		return debugResult, fmt.Errorf("debug skill with Modu: %w", runErr)
	}
	return debugResult, nil
}
