package keeldesktop

import (
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/window"
	"os"
	"path/filepath"
	"testing"

	"github.com/dyike/keel/ui/el"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

func TestAnalyzedTaskMetadataUpdatesAllVisibleCopies(t *testing.T) {
	a := conversationFixture(t)
	original := *a.task
	a.detail.Task = original
	a.tasks[original.WorkspaceID] = append(a.tasks[original.WorkspaceID], original)
	updated := original
	updated.Title = "分析后的标题"
	updated.Category = "coding"
	updated.CategorySource = "ai"
	// Applying metadata must not replace a more recent task execution state.
	updated.Status = "failed"
	a.applyAnalyzedTaskDetails(updated)
	if a.task.Title != updated.Title || a.task.Category != updated.Category || a.detail.Task.Title != updated.Title {
		t.Fatal("analysis did not update the active session")
	}
	if a.task.Status != original.Status {
		t.Fatal("metadata update overwrote execution state")
	}
	for _, group := range a.tasks {
		for _, task := range group {
			if task.ID == original.ID && (task.Title != updated.Title || task.CategorySource != "ai") {
				t.Fatal("stale sidebar metadata")
			}
		}
	}
}

func TestTaskMenuOffersAIAnalysis(t *testing.T) {
	a := conversationFixture(t)
	menu := a.taskMenu(*a.task)
	menu.SetValue(true)
	root := el.Root(a)
	router, frame, _ := alignmentRouter(root, 1120, 800, 1)
	frame()
	if _, ok := findSkillControl(router.AppendSemantics(nil), "AI 分析标题和类型"); !ok {
		t.Fatal("missing AI analysis menu entry")
	}
	if dir := os.Getenv("ONECATCH_KEEL_PREVIEWS"); dir != "" {
		widget := core.Func(func(gtx core.C) core.D { gtx.Source = router.Source(); return root.Layout(gtx) })
		if err := window.Screenshot(screenshotRoot{widget}, 1120, 800, filepath.Join(dir, "task-actions-menu.png")); err != nil {
			t.Fatal(err)
		}
	}
	a.ui.TaskAnalyzing = map[string]bool{a.task.ID: true}
	// Duplicate invocations are ignored rather than starting another generation.
	a.analyzeTaskDetails(a.task.ID)
	if !a.ui.TaskAnalyzing[a.task.ID] {
		t.Fatal("analysis state was lost")
	}
}

func TestAnalyzedTaskMetadataAfterNavigation(t *testing.T) {
	for _, state := range []string{"no-session", "task-without-run", "other-session"} {
		t.Run(state, func(t *testing.T) {
			original := domaintasks.Task{ID: "analyzed", WorkspaceID: "project", Title: "old", Status: "running"}
			updated := original
			updated.Title, updated.Category, updated.CategorySource = "new title", "coding", "ai"
			a := &app{tasks: map[string][]domaintasks.Task{"project": {original}}}
			switch state {
			case "task-without-run":
				current := original
				a.task = &current
			case "other-session":
				a.task = &domaintasks.Task{ID: "other", Title: "untouched"}
				a.detail = &desktopservice.RunDetail{Task: *a.task}
			}
			a.applyAnalyzedTaskDetails(updated)
			saved := a.tasks["project"][0]
			if saved.Title != updated.Title || saved.Category != updated.Category || saved.Status != original.Status {
				t.Fatalf("incorrect sidebar metadata: %+v", saved)
			}
			if state == "task-without-run" && a.task.Title != updated.Title {
				t.Fatal("task without run not updated")
			}
			if state == "other-session" && (a.task.Title != "untouched" || a.detail.Task.Title != "untouched") {
				t.Fatal("changed another session")
			}
		})
	}
}
