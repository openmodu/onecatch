package desktop

import (
	"context"
	"strings"
	"testing"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type taskTitleRunner struct{ request *agentrun.Request }

func (r *taskTitleRunner) Runtime() agentrun.Runtime { return agentrun.RuntimeCodex }
func (r *taskTitleRunner) Available() bool           { return true }
func (r *taskTitleRunner) Run(_ context.Context, request agentrun.Request, _ agentrun.Sink) (agentrun.Result, error) {
	r.request = &request
	return agentrun.Result{Succeeded: true, FinalMessage: "修复最小窗口状态栏。"}, nil
}

func TestNormalizeGeneratedTaskTitle(t *testing.T) {
	tests := map[string]string{
		"plain":    "修复最小窗口状态栏",
		"label":    "标题：修复最小窗口状态栏。",
		"quoted":   `"Improve task title generation"`,
		"markdown": "```\nAI 生成任务标题\n```",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if got := normalizeGeneratedTaskTitle(input); got != map[string]string{
				"plain": "修复最小窗口状态栏", "label": "修复最小窗口状态栏", "quoted": "Improve task title generation", "markdown": "AI 生成任务标题",
			}[name] {
				t.Fatalf("normalizeGeneratedTaskTitle(%q) = %q", input, got)
			}
		})
	}
}

func TestTaskTitleFallbackMatchesFrontendBehavior(t *testing.T) {
	if got := taskTitleFromPrompt("\n## 增加语法高亮\n并支持多个文件", "新建任务"); got != "增加语法高亮" {
		t.Fatalf("taskTitleFromPrompt() = %q", got)
	}
	if got := taskTitleFromPrompt("", "新建任务"); got != "新建任务" {
		t.Fatalf("empty fallback = %q", got)
	}
	if got := taskTitleFromPrompt(strings.Repeat("长", 60), "新建任务"); len([]rune(got)) != maxGeneratedTaskTitleCharacters {
		t.Fatalf("truncated title length = %d", len([]rune(got)))
	}
}

func TestTaskTitleAgentPromptRequiresTitleOnly(t *testing.T) {
	prompt := taskTitleAgentPrompt("修复状态栏")
	for _, requirement := range []string{"same language", "Return only the title", "Do not inspect files", "修复状态栏"} {
		if !strings.Contains(prompt, requirement) {
			t.Fatalf("prompt does not contain %q: %s", requirement, prompt)
		}
	}
}

func TestCreateTaskGeneratesMissingTitleWithSelectedAgent(t *testing.T) {
	app, _ := newLocalTestApp(t, completingEngine{})
	runner := &taskTitleRunner{}
	app.runtimes.mu.Lock()
	app.runtimes.engine = agentrun.NewEngineWithRunners(runner)
	app.runtimes.mu.Unlock()

	workspace, err := app.AddWorkspace(context.Background(), AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	task, err := app.CreateTask(context.Background(), CreateTaskInput{
		WorkspaceID: workspace.ID,
		WorkflowID:  directAgentWorkflowID,
		Harness:     string(agentrun.RuntimeCodex),
		Prompt:      "窗口最小时状态栏右侧按钮不见了，请修复。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "修复最小窗口状态栏" {
		t.Fatalf("generated title = %q", task.Title)
	}
	if runner.request == nil || runner.request.Sandbox != agentrun.SandboxReadOnly {
		t.Fatalf("title request = %+v", runner.request)
	}
}
