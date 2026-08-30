package desktop

import (
	"context"
	"regexp"
	"strings"
	"time"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

const maxGeneratedTaskTitleCharacters = 48
const maxTaskTitlePromptCharacters = 8000

var taskTitlePrefix = regexp.MustCompile(`(?i)^(?:task\s+title|title|任务标题|标题)\s*[:：]\s*`)

type pendingTaskTitle struct {
	provisionalTitle string
	workspace        string
	definition       domainworkflows.Definition
	input            CreateTaskInput
}

// queueTaskTitleRefinement remembers the AI title request without starting a
// second Agent beside the user's first turn. On Linux, two simultaneous Codex
// app-server cold starts were effectively serial and delayed the real message
// by the entire title-generation turn.
func (a *Service) queueTaskTitleRefinement(taskID, provisionalTitle, workspace string, definition domainworkflows.Definition, input CreateTaskInput) {
	a.titleMu.Lock()
	a.pendingTitles[taskID] = pendingTaskTitle{provisionalTitle: provisionalTitle, workspace: workspace, definition: definition, input: input}
	a.titleMu.Unlock()
}

// refineTaskTitleAfterRun starts the queued title request only after the first
// real run has left the active set. The provisional prompt title is visible in
// the meantime, and this background work can no longer delay the first answer.
func (a *Service) refineTaskTitleAfterRun(runID string) {
	if a.rootCtx.Err() != nil {
		return
	}
	run, err := a.store.Repos.Workflows.GetRun(a.rootCtx, runID)
	if err != nil {
		return
	}
	a.titleMu.Lock()
	pending, ok := a.pendingTitles[run.TaskID]
	if ok {
		delete(a.pendingTitles, run.TaskID)
	}
	a.titleMu.Unlock()
	if !ok {
		return
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		title := a.generateTaskTitle(a.rootCtx, pending.workspace, pending.definition, pending.input)
		if title == pending.provisionalTitle || a.rootCtx.Err() != nil {
			return
		}
		_, _ = a.store.Repos.Tasks.UpdateTaskTitle(a.rootCtx, run.TaskID, pending.provisionalTitle, title, time.Now().UTC())
	}()
}

func (a *Service) cancelTaskTitleRefinement(taskID string) {
	a.titleMu.Lock()
	delete(a.pendingTitles, taskID)
	a.titleMu.Unlock()
}

func (a *Service) generateTaskTitle(ctx context.Context, workspace string, definition domainworkflows.Definition, input CreateTaskInput) string {
	fallback := taskTitleFromPrompt(input.Prompt, "新建任务")
	if strings.TrimSpace(input.Prompt) == "" {
		return fallback
	}
	runtime := agentrun.Runtime(strings.TrimSpace(input.Harness))
	model := strings.TrimSpace(input.Model)
	if runtime == "" {
		for _, step := range definition.Steps {
			if step.ID == definition.EntryStepID {
				runtime = agentrun.Runtime(strings.TrimSpace(step.Runtime))
				if model == "" {
					model = strings.TrimSpace(step.Model)
				}
				break
			}
		}
	}
	if !runtime.Valid() || !a.runtimes.Available(runtime) {
		return fallback
	}

	generateCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := a.runtimes.Run(generateCtx, agentrun.Request{
		Runtime:         runtime,
		Workspace:       workspace,
		Prompt:          taskTitleAgentPrompt(input.Prompt),
		Model:           model,
		ReasoningEffort: strings.TrimSpace(input.ReasoningEffort),
		ServiceTier:     strings.TrimSpace(input.ServiceTier),
		Sandbox:         agentrun.SandboxReadOnly,
	}, nil)
	if err != nil || !result.Succeeded {
		return fallback
	}
	if title := normalizeGeneratedTaskTitle(result.FinalMessage); title != "" {
		return title
	}
	return fallback
}

func taskTitleAgentPrompt(prompt string) string {
	characters := []rune(strings.TrimSpace(prompt))
	if len(characters) > maxTaskTitlePromptCharacters {
		characters = characters[:maxTaskTitlePromptCharacters]
	}
	return strings.Join([]string{
		"Generate a concise task title for the user's request below.",
		"Use the same language as the request. Preserve important product, file, and code names.",
		"Return only the title: no quotes, Markdown, label, explanation, or ending punctuation.",
		"Keep it within 24 Chinese characters or 48 characters in other languages.",
		"Do not inspect files and do not call tools.",
		"",
		string(characters),
	}, "\n")
}

func normalizeGeneratedTaskTitle(value string) string {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(value), "```", ""), "\n")
	title := ""
	for _, line := range lines {
		if candidate := strings.TrimSpace(line); candidate != "" {
			title = candidate
			break
		}
	}
	title = taskTitlePrefix.ReplaceAllString(title, "")
	title = strings.TrimSpace(strings.Trim(title, "\"'`“”‘’"))
	title = strings.TrimRight(title, "。.!！?？;；")
	return truncateTaskTitle(title)
}

func taskTitleFromPrompt(value, fallback string) string {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		line = regexp.MustCompile(`^(?:#{1,6}|[-*+]|\d+[.)])\s+`).ReplaceAllString(line, "")
		return truncateTaskTitle(strings.Join(strings.Fields(line), " "))
	}
	return fallback
}

func truncateTaskTitle(value string) string {
	characters := []rune(strings.TrimSpace(value))
	if len(characters) <= maxGeneratedTaskTitleCharacters {
		return string(characters)
	}
	return string(characters[:maxGeneratedTaskTitleCharacters-1]) + "…"
}
