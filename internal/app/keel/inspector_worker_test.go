package keeldesktop

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/window"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/worker"
)

func inspectorWorkerDetail() *desktopservice.RunDetail {
	return &desktopservice.RunDetail{
		Run:      domainworkflows.Run{ID: "read-run", CurrentStepID: "write", Status: domainworkflows.RunRunning},
		Workflow: domainworkflows.Definition{Steps: []domainworkflows.Step{{ID: "read", Runtime: "codex", Sandbox: "read-only", WorkerID: "worker-a"}, {ID: "write", Runtime: "codex", Sandbox: "workspace-write", WorkerID: "worker-b"}}},
		StepRuns: []domainworkflows.StepRun{{StepID: "read", Status: domainworkflows.StepRunRunning}},
	}
}

func TestInspectorWorkerDefaultsFollowLatestReadOnlyStep(t *testing.T) {
	a := dagTestApp(t)
	a.workspaceID = "workspace"
	a.workspaces = []domainworkspaces.Workspace{{ID: a.workspaceID}}
	a.task = &domaintasks.Task{ID: "task"}
	a.detail = inspectorWorkerDetail()
	a.workers = []worker.Info{{ID: "worker-a", Name: "Mac mini", Enabled: true}, {ID: "worker-b", Name: "Build Mac", Enabled: true}, {ID: "off", Name: "Disabled", Enabled: false}}
	if !a.syncInspectorGitSource() || a.workbench.Source != "worker-a" || len(a.workbench.Workers) != 2 {
		t.Fatal("latest read-only worker was not selected")
	}
	if got := activeInspectorWorkerID(a.detail); got != "worker-a" {
		t.Fatal("run.currentStepId incorrectly took precedence over latest attempt")
	}
	// A manual override survives token growth and a periodic refresh.
	a.workbench.Source = "worker-b"
	a.workbench.SourcePicker.SetValue("worker-b")
	a.detail.StepRuns[0].InputTokens = 123
	if a.syncInspectorGitSource() || a.workbench.Source != "worker-b" {
		t.Fatal("token update discarded manual selection")
	}
	a.detail.StepRuns = append(a.detail.StepRuns, domainworkflows.StepRun{StepID: "write"})
	if !a.syncInspectorGitSource() || a.workbench.Source != "local" {
		t.Fatal("writable step did not return to delivered local changes")
	}
	a.detail.StepRuns = nil
	a.detail.Run.CurrentStepID = "read"
	a.syncInspectorGitSource()
	if a.workbench.Source != "worker-a" {
		t.Fatal("missing step runs did not fall back to currentStepId")
	}
	a.workers[0].Enabled = false
	a.syncInspectorGitSource()
	if a.workbench.Source != "local" {
		t.Fatal("disabled worker remained the inspection source")
	}
	a.workers[0].Enabled = true
	a.syncInspectorGitSource()
	a.task = nil // a draft never follows the prior run
	a.syncInspectorGitSource()
	if a.workbench.Source != "local" {
		t.Fatal("draft retained the preceding read-only worker")
	}
	if activeInspectorWorkerID(nil) != "" {
		t.Fatal("nil run selected a worker")
	}
}

func TestInspectorWorkerSourceRespectsContextAndRejectsOldResults(t *testing.T) {
	a := dagTestApp(t)
	a.workspaceID = "first"
	a.workspaces = []domainworkspaces.Workspace{{ID: "first"}, {ID: "ssh", RemoteFS: &domainworkspaces.RemoteFS{Host: "test-host"}}}
	a.task = &domaintasks.Task{ID: "task"}
	a.detail = inspectorWorkerDetail()
	a.workers = []worker.Info{{ID: "worker-a", Name: "Mac mini", Enabled: true}}
	a.syncInspectorGitSource()
	version := a.workbench.Version
	a.workbench.NewBranch, a.workbench.Error = "old-branch", "old-error"
	a.workspaceID = "ssh"
	a.syncInspectorGitSource()
	if a.workbench.Source != "local" || len(a.workbench.Workers) != 0 || a.workbench.NewBranch != "" || a.workbench.Error != "" {
		t.Fatal("Remote FS did not clear remote inspection context")
	}
	a.applyInspectorGit(version, "first", "worker-a", inspectorGitResult{snapshot: domainworkspaces.GitSnapshot{Branch: "stale"}})
	if a.workbench.Snapshot != nil {
		t.Fatal("old worker response survived context change")
	}
	a.workspaceID = "first"
	a.task.Worktree = &domainworkspaces.Worktree{Path: "/temporary-checkout"}
	a.syncInspectorGitSource()
	if a.workbench.Source != "local" || len(a.workbench.Workers) != 0 {
		t.Fatal("bound task permitted remote clone inspection")
	}
	a.task.Worktree = nil
	a.workbench.Context = "old-context"
	a.syncInspectorGitSource()
	// Entering the inspector after source synchronization must not reset it.
	a.workbench.Tab = "status"
	a.loadInspector()
	if a.workbench.Source != "worker-a" {
		t.Fatal("inspector context initialization reset the selected worker")
	}
	version = a.workbench.Version
	a.workbench.Source = "local"
	a.applyInspectorGit(version, "first", "worker-a", inspectorGitResult{snapshot: domainworkspaces.GitSnapshot{Branch: "old-remote"}})
	if a.workbench.Snapshot != nil {
		t.Fatal("source change accepted a late remote snapshot")
	}
}

func TestInspectorReadsPinnedWorkerGitAndKeepsLocalCaption(t *testing.T) {
	a := dagTestApp(t)
	ctx := context.Background()
	workspace, err := a.backend.service.AddWorkspace(ctx, desktopservice.AddWorkspaceInput{Path: gitFixture(t), Name: "Inspection"})
	if err != nil {
		t.Fatal(err)
	}
	var reads atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/pair" {
			_ = json.NewEncoder(w).Encode(worker.PairResult{WorkerID: "worker-a", Name: "Mac mini", Token: "test-inspector-token", ProtocolVersion: 1})
			return
		}
		if r.URL.Path != "/v1/workspaces/"+workspace.ID+"/git" || r.Header.Get("Authorization") != "Bearer test-inspector-token" {
			http.Error(w, "invalid inspection request", http.StatusForbidden)
			return
		}
		reads.Add(1)
		_ = json.NewEncoder(w).Encode(domainworkspaces.GitSnapshot{IsRepo: true, Branch: "worker-clone", Head: "019f5ed", Files: []domainworkspaces.GitFile{{Path: "main.go", Worktree: "M"}}})
	}))
	defer server.Close()
	info, err := a.backend.service.PairWorker(ctx, server.URL, "ABCD2345")
	if err != nil {
		t.Fatal(err)
	}
	a.workspaces, a.workspaceID = []domainworkspaces.Workspace{workspace}, workspace.ID
	a.workers = []worker.Info{info}
	a.task = &domaintasks.Task{ID: "task"}
	a.detail = inspectorWorkerDetail()
	a.syncInspectorGitSource()
	a.captionGit = &domainworkspaces.GitSnapshot{Branch: "main"}
	result := readInspectorGit(ctx, a.backend.service, workspace.ID, a.workbench.Source)
	if result.err != nil || result.snapshot.Branch != "worker-clone" || len(result.branches) != 0 || reads.Load() != 1 {
		t.Fatal("remote source did not read the registered pinned worker")
	}
	a.applyInspectorGit(a.workbench.Version, workspace.ID, a.workbench.Source, result)
	if a.workbench.Snapshot.Branch != "worker-clone" || a.captionGit.Branch != "main" {
		t.Fatal("worker snapshot overwrote local project caption")
	}
	a.changeBranch("must-not-be-created", true)
	if a.workbench.Mutating || strings.Contains(runGit(t, workspace.Path, "branch"), "must-not-be-created") {
		t.Fatal("remote inspection allowed a local branch mutation")
	}
	a.detail.Workflow.Steps[0].Sandbox = "workspace-write"
	a.syncInspectorGitSource()
	result = readInspectorGit(ctx, a.backend.service, workspace.ID, a.workbench.Source)
	if result.err != nil || result.snapshot.Branch != "main" || len(result.branches) == 0 || reads.Load() != 1 {
		t.Fatal("writable step still queried the remote clone")
	}
	a.applyInspectorGit(a.workbench.Version, workspace.ID, "local", result)
	if a.workbench.Snapshot.Branch != "main" {
		t.Fatal("local result not applied")
	}
}

func TestInspectorWorkerSourceThroughNativePicker(t *testing.T) {
	a := dagTestApp(t)
	a.workspaceID = "fixture"
	a.workspaces = []domainworkspaces.Workspace{{ID: "fixture"}}
	a.workers = []worker.Info{{ID: "worker-a", Name: "Mac mini", Enabled: true}}
	a.task = &domaintasks.Task{ID: "task"}
	a.detail = inspectorWorkerDetail()
	a.syncInspectorGitSource()
	a.workbench.Snapshot = &domainworkspaces.GitSnapshot{IsRepo: true, Branch: "worker-clone", Files: []domainworkspaces.GitFile{{Path: "remote-only.go", Worktree: "M"}}}
	root := el.Root(el.ViewFunc(a.gitInspector))
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(380, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	var find func([]input.SemanticNode, string) (image.Rectangle, bool)
	find = func(nodes []input.SemanticNode, label string) (image.Rectangle, bool) {
		for _, n := range nodes {
			if n.Desc.Label == label && n.Desc.Gestures&input.ClickGesture != 0 && !n.Desc.Disabled {
				return n.Desc.Bounds, true
			}
			if b, ok := find(n.Children, label); ok {
				return b, true
			}
		}
		return image.Rectangle{}, false
	}
	click := func(label string) {
		t.Helper()
		b, ok := find(router.AppendSemantics(nil), label)
		if !ok {
			t.Fatalf("missing enabled %q", label)
		}
		pos := f32.Pt(float32(b.Min.X+b.Dx()/2), float32(b.Min.Y+b.Dy()/2))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
		frame()
	}
	frame()
	if _, ok := find(router.AppendSemantics(nil), "创建"); ok {
		t.Fatal("remote view offered branch creation")
	}
	if _, ok := find(router.AppendSemantics(nil), "remote-only.go"); ok {
		t.Fatal("remote file offered a local review action")
	}
	click("Git 来源")
	frame()
	click("本地")
	frame()
	if a.workbench.Source != "local" || a.workbench.SourcePicker.Value() != "local" {
		t.Fatal("native picker did not override worker source")
	}
	if a.syncInspectorGitSource() || a.workbench.Source != "local" {
		t.Fatal("manual picker choice immediately reverted")
	}
}

func TestRenderInspectorWorkerPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	a := dagTestApp(t)
	applyPalette(false)
	defer applyPalette(false)
	icon, err := png.Decode(bytes.NewReader(appIcon))
	if err != nil {
		t.Fatal(err)
	}
	a.logo = kit.Image(icon, "OneCatch").Size(22, 22).Rounded(5)
	a.workbench.Visible, a.workbench.Tab = true, "git"
	a.workspaceID = "onecatch-demo"
	a.workspaces = []domainworkspaces.Workspace{{ID: a.workspaceID, Name: "onecatch", Path: "~/Code/openmodu/onecatch"}}
	a.task = &domaintasks.Task{ID: "fixture-task", Title: "优化本地 Agent 调度流程", WorkspaceID: a.workspaceID}
	a.detail = inspectorWorkerDetail()
	a.workers = []worker.Info{{ID: "worker-a", Name: "Mac mini", Enabled: true}}
	a.syncInspectorGitSource()
	a.workbench.Snapshot = &domainworkspaces.GitSnapshot{IsRepo: true, Branch: "feature/workbench", Head: "019f5ed", Files: []domainworkspaces.GitFile{{Path: "src/app/App.jsx", Worktree: "M"}}}
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "inspector-worker.png")); err != nil {
		t.Fatal(err)
	}
	applyPalette(true)
	if err := window.Screenshot(el.Root(el.ViewFunc(a.gitInspector)), 380, 600, filepath.Join(dir, "inspector-worker-dark.png")); err != nil {
		t.Fatal(err)
	}
}
