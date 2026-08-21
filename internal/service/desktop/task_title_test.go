package desktop

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type taskTitleRunner struct {
	requests chan agentrun.Request
	release  chan struct{}
}

func (r *taskTitleRunner) Runtime() agentrun.Runtime { return agentrun.RuntimeCodex }
func (r *taskTitleRunner) Available() bool           { return true }
func (r *taskTitleRunner) Run(ctx context.Context, request agentrun.Request, _ agentrun.Sink) (agentrun.Result, error) {
	r.requests <- request
	select {
	case <-ctx.Done():
		return agentrun.Result{}, ctx.Err()
	case <-r.release:
		return agentrun.Result{Succeeded: true, FinalMessage: "修复最小窗口状态栏。"}, nil
	}
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

func TestCreateTaskReturnsPromptTitleBeforeSelectedAgentRefinesIt(t *testing.T) {
	app, _ := newLocalTestApp(t, completingEngine{})
	runner := &taskTitleRunner{requests: make(chan agentrun.Request, 1), release: make(chan struct{})}
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
	if task.Title != "窗口最小时状态栏右侧按钮不见了，请修复。" {
		t.Fatalf("provisional title = %q", task.Title)
	}
	var request agentrun.Request
	select {
	case request = <-runner.requests:
	case <-time.After(time.Second):
		t.Fatal("title refinement did not start")
	}
	if request.Sandbox != agentrun.SandboxReadOnly {
		t.Fatalf("title request = %+v", request)
	}
	close(runner.release)
	deadline := time.Now().Add(time.Second)
	for {
		stored, getErr := app.store.Repos.Tasks.GetTask(context.Background(), task.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if stored.Title == "修复最小窗口状态栏" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("refined title = %q", stored.Title)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAsyncTaskTitleDoesNotOverwriteManualRename(t *testing.T) {
	app, _ := newLocalTestApp(t, completingEngine{})
	runner := &taskTitleRunner{requests: make(chan agentrun.Request, 1), release: make(chan struct{})}
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
		Prompt:      "修复状态栏按钮",
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.requests:
	case <-time.After(time.Second):
		t.Fatal("title refinement did not start")
	}
	if _, err := app.RenameTask(context.Background(), task.ID, "我命名的标题"); err != nil {
		t.Fatal(err)
	}
	close(runner.release)
	app.wg.Wait()
	stored, err := app.store.Repos.Tasks.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Title != "我命名的标题" {
		t.Fatalf("manual title was overwritten: %q", stored.Title)
	}
}
