package desktop

import (
	"context"
	"strings"
	"testing"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type taskAnalysisRunner struct {
	output string
	onRun  func(agentrun.Request)
}

func (r *taskAnalysisRunner) Runtime() agentrun.Runtime { return agentrun.RuntimeCodex }
func (r *taskAnalysisRunner) Available() bool           { return true }
func (r *taskAnalysisRunner) Run(_ context.Context, request agentrun.Request, _ agentrun.Sink) (agentrun.Result, error) {
	if r.onRun != nil {
		r.onRun(request)
	}
	return agentrun.Result{Succeeded: true, FinalMessage: r.output}, nil
}

func TestAnalyzeTaskDetails(t *testing.T) {
	for _, scenario := range []string{"success", "invalid", "concurrent edit"} {
		t.Run(scenario, func(t *testing.T) {
			app, _ := newLocalTestApp(t, completingEngine{})
			ctx := context.Background()
			workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: directAgentWorkflowID, Harness: string(agentrun.RuntimeCodex), Prompt: "修复登录后页面空白"})
			if err != nil {
				t.Fatal(err)
			}
			runner := &taskAnalysisRunner{output: `{"title":"修复登录白屏","category":"fix"}`}
			runner.onRun = func(request agentrun.Request) {
				if request.Sandbox != agentrun.SandboxReadOnly || request.ResumeSessionID != "" || !strings.Contains(request.Prompt, task.Prompt) {
					t.Fatalf("unexpected analysis request: %+v", request)
				}
				if scenario == "concurrent edit" {
					if _, err := app.UpdateTaskDetails(ctx, task.ID, "用户修改", "feat"); err != nil {
						t.Fatal(err)
					}
				}
			}
			if scenario == "invalid" {
				runner.output = `{"title":"错误输出","category":"unknown"}`
			}
			app.runtimes.mu.Lock()
			app.runtimes.engine = agentrun.NewEngineWithRunners(runner)
			app.runtimes.mu.Unlock()
			result, err := app.AnalyzeTaskDetails(ctx, task.ID)
			saved, readErr := app.store.Repos.Tasks.GetTask(ctx, task.ID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			switch scenario {
			case "success":
				if err != nil || result.Title != "修复登录白屏" || saved.Category != "fix" || saved.CategorySource != "ai" || saved.Status != task.Status {
					t.Fatalf("result=%+v saved=%+v err=%v", result, saved, err)
				}
				updated, err := app.UpdateTaskDetails(ctx, task.ID, saved.Title, "docs")
				if err != nil || updated.CategorySource != "" {
					t.Fatalf("manual override source: %+v %v", updated, err)
				}
			case "invalid":
				if err == nil || saved.Title != task.Title || saved.Category != task.Category {
					t.Fatalf("invalid output changed task: %+v %v", saved, err)
				}
			case "concurrent edit":
				if err == nil || saved.Title != "用户修改" || saved.Category != "feat" {
					t.Fatalf("AI overwrote edit: %+v %v", saved, err)
				}
			}
		})
	}
}

func TestParseAnalyzedTaskDetails(t *testing.T) {
	for _, input := range []string{`{"title":"Fix login","category":"fix"}`, "```json\n{\"title\":\"Fix login\",\"category\":\"fix\"}\n```"} {
		if _, err := parseAnalyzedTaskDetails(input); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []string{`{}`, `{"title":"","category":"fix"}`, `{"title":"Fix login","category":"invalid"}`, `{"title":"Fix login","category":""}`, `not JSON`} {
		if _, err := parseAnalyzedTaskDetails(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}
