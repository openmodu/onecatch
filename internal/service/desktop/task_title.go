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
