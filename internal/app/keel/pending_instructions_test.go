package keeldesktop

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/gio/layout"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/gio/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type instructionEngine struct{ started chan agentrun.Request }

func (*instructionEngine) Available(agentrun.Runtime) bool { return true }
func (e *instructionEngine) Run(ctx context.Context, request agentrun.Request, _ agentrun.Sink) (agentrun.Result, error) {
	select {
	case e.started <- request:
	case <-ctx.Done():
		return agentrun.Result{}, ctx.Err()
	}
	<-ctx.Done()
	return agentrun.Result{}, ctx.Err()
}

func instructionTestRun(t *testing.T) (*backend, domaintasks.Task, domainworkflows.Run, *instructionEngine) {
	t.Helper()
	ctx := context.Background()
	engine := &instructionEngine{started: make(chan agentrun.Request, 8)}
	b, err := openBackendWithEngine(ctx, t.TempDir(), engine)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.close() })
	w, err := b.service.AddWorkspace(ctx, desktopservice.AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	task, err := b.service.CreateTask(ctx, desktopservice.CreateTaskInput{WorkspaceID: w.ID, Title: "Pending messages", Prompt: "initial task", WorkflowID: "single_agent", Harness: "codex", Sandbox: "read-only", WorktreeMode: "project"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := b.service.StartRun(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-engine.started:
	case <-time.After(3 * time.Second):
		t.Fatal("initial turn did not start")
	}
	return b, task, run, engine
}

func TestPendingInstructionSteeringPreservesIdentityAndAttachments(t *testing.T) {
	b, _, run, engine := instructionTestRun(t)
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(file, []byte("attached requirement"), 0600); err != nil {
		t.Fatal(err)
	}
	later, err := b.service.QueueFollowUp(ctx, run.ID, desktopservice.InstructionInput{Content: "later work"})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := b.service.QueueFollowUp(ctx, run.ID, desktopservice.InstructionInput{Content: "urgent correction", AttachmentPaths: []string{file}})
	if err != nil || len(queued.Attachments) != 1 {
		t.Fatalf("queue: %+v %v", queued, err)
	}
	if err := applyInstructionAction(ctx, b.service, instructionAction{run.ID, queued.ID, "steer"}); err != nil {
		t.Fatal(err)
	}
	select {
	case request := <-engine.started:
		urgentAt, ordinaryAt := strings.Index(request.Prompt, "urgent correction"), strings.Index(request.Prompt, "later work")
		if urgentAt < 0 || ordinaryAt >= 0 {
			t.Fatalf("wrong resumed instruction: %s", request.Prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("steering did not restart the turn")
	}
	detail, err := b.service.GetRunDetail(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Instructions) != 2 {
		t.Fatalf("steering duplicated instructions: %+v", detail.Instructions)
	}
	found := false
	for _, item := range detail.Instructions {
		if item.ID == queued.ID {
			found = true
			if !item.Priority || item.FollowUp || item.Status != domainworkflows.InstructionApplied || len(item.Attachments) != 1 || item.Attachments[0] != queued.Attachments[0] {
				t.Fatalf("identity/attachment promotion: %+v", item)
			}
		}
		if item.ID == later.ID && (item.Priority || !item.FollowUp || item.Status != domainworkflows.InstructionPending) {
			t.Fatal("steering changed an unrelated instruction's mode")
		}
	}
	if !found {
		t.Fatal("original instruction identity disappeared")
	}
}

func TestPendingInstructionDeleteThroughNativeControlKeepsDraft(t *testing.T) {
	b, task, run, _ := instructionTestRun(t)
	ctx := context.Background()
	queued, err := b.service.QueueFollowUp(ctx, run.ID, desktopservice.InstructionInput{Content: "remove me"})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(b)
	t.Cleanup(a.close)
	a.task, a.taskID, a.runID = &task, task.ID, run.ID
	detail, err := b.service.GetRunDetail(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	a.detail, a.prompt = &detail, "draft stays here"
	path := filepath.Join(t.TempDir(), "draft.txt")
	os.WriteFile(path, []byte("draft attachment"), 0600)
	a.attachments.Items = []selectedAttachment{{Path: path}}
	root := el.Root(el.ViewFunc(a.pendingInstructionList))
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(640, 168)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	var find func([]input.SemanticNode, string) (image.Rectangle, bool)
	find = func(nodes []input.SemanticNode, name string) (image.Rectangle, bool) {
		for _, n := range nodes {
			if n.Desc.Label == name && !n.Desc.Disabled && n.Desc.Gestures&input.ClickGesture != 0 {
				return n.Desc.Bounds, true
			}
			if b, ok := find(n.Children, name); ok {
				return b, true
			}
		}
		return image.Rectangle{}, false
	}
	frame()
	if _, ok := find(router.AppendSemantics(nil), "调整方向"); !ok {
		t.Fatal("running follow-up omitted steering control")
	}
	box, ok := find(router.AppendSemantics(nil), "删除后续消息")
	if !ok {
		t.Fatal("pending message omitted delete control")
	}
	pos := f32.Pt(float32(box.Min.X+box.Dx()/2), float32(box.Min.Y+box.Dy()/2))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
	frame()
	if !a.instructionBusy() {
		t.Fatal("native deletion did not serialize actions")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		next, err := b.service.GetRunDetail(ctx, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range next.Instructions {
			found = found || item.ID == queued.ID
		}
		if !found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("native deletion did not reach the store")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if a.prompt != "draft stays here" || len(a.attachments.Items) != 1 {
		t.Fatal("pending action cleared composer draft")
	}
	frame()
	if _, ok := find(router.AppendSemantics(nil), "调整方向"); ok {
		t.Fatal("steering remained enabled during deletion")
	}
	a.instructionAction = instructionAction{}
	a.detail.Run.Status = domainworkflows.RunPaused
	frame()
	if _, ok := find(router.AppendSemantics(nil), "调整方向"); ok {
		t.Fatal("paused task offered immediate steering")
	}
	if _, ok := find(router.AppendSemantics(nil), "删除后续消息"); !ok {
		t.Fatal("paused task could not remove its pending message")
	}
	a.detail.Instructions[0].Status = domainworkflows.InstructionApplied
	frame()
	if _, ok := find(router.AppendSemantics(nil), "删除后续消息"); ok {
		t.Fatal("applied message remained deletable")
	}
}

func TestSteeringContinuationKeepsFailedInsertAvailable(t *testing.T) {
	b, _, run, engine := instructionTestRun(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	persisted, err := b.service.SubmitRunMessage(ctx, run.ID, desktopservice.RunMessageInput{Resume: desktopservice.ResumeRunInput{Instruction: "retry this"}, Steer: true})
	if err == nil || persisted {
		t.Fatal("failed steering cleared a retryable draft")
	}
	persisted, err = b.service.SubmitRunMessage(context.Background(), run.ID, desktopservice.RunMessageInput{Resume: desktopservice.ResumeRunInput{Instruction: "correct direction"}, Steer: true})
	if err != nil || !persisted {
		t.Fatalf("steering submit: %v %v", persisted, err)
	}
	select {
	case request := <-engine.started:
		if !strings.Contains(request.Prompt, "correct direction") {
			t.Fatal("steering did not reach resumed turn")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("steering continuation did not restart")
	}
}

func TestPausedComposerCanResumeWithoutAnotherMessage(t *testing.T) {
	b, task, run, engine := instructionTestRun(t)
	ctx := context.Background()
	if _, err := b.service.InterruptRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	var detail desktopservice.RunDetail
	deadline := time.Now().Add(3 * time.Second)
	for {
		var err error
		detail, err = b.service.GetRunDetail(ctx, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !detail.Active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("interrupted run did not stop")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if detail.Run.Status != domainworkflows.RunPaused {
		t.Fatalf("interrupted status: %s", detail.Run.Status)
	}
	a := newApp(b)
	t.Cleanup(a.close)
	a.task, a.taskID, a.runID, a.workspaceID = &task, task.ID, run.ID, task.WorkspaceID
	a.detail = &detail
	a.runtime.SetValue("codex")
	root := el.Root(el.ViewFunc(a.composer))
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(640, 240)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	var find func([]input.SemanticNode) (image.Rectangle, bool)
	find = func(nodes []input.SemanticNode) (image.Rectangle, bool) {
		for _, n := range nodes {
			if n.Desc.Label == "恢复运行" && !n.Desc.Disabled && n.Desc.Gestures&input.ClickGesture != 0 {
				return n.Desc.Bounds, true
			}
			if bounds, found := find(n.Children); found {
				return bounds, true
			}
		}
		return image.Rectangle{}, false
	}
	frame()
	bounds, found := find(router.AppendSemantics(nil))
	if !found {
		t.Fatal("empty paused composer disabled resume")
	}
	pos := f32.Pt(float32(bounds.Min.X+bounds.Dx()/2), float32(bounds.Min.Y+bounds.Dy()/2))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
	frame()
	select {
	case <-engine.started:
	case <-time.After(3 * time.Second):
		t.Fatalf("native empty resume did not reach engine: %s", a.errorText)
	}
}

func TestRenderPendingInstructionsPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	a := dagTestApp(t)
	a.runID = "preview-run"
	a.task = &domaintasks.Task{ID: "preview-task", WorkflowID: "single_agent"}
	a.detail = &desktopservice.RunDetail{Run: domainworkflows.Run{ID: a.runID, Status: domainworkflows.RunRunning}, Active: true, Instructions: []domainworkflows.Instruction{{ID: "preview-1", Content: "补上暂停和恢复的状态反馈", Status: domainworkflows.InstructionPending, FollowUp: true}, {ID: "preview-2", Attachments: []string{"notes.md"}, Status: domainworkflows.InstructionPending, FollowUp: true}}}
	a.runtime.SetValue("codex")
	applyPalette(false)
	defer applyPalette(false)
	view := el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().P(16).Child(a.composer(cx)) })
	if err := window.Screenshot(el.Root(view), 860, 320, filepath.Join(dir, "pending-instructions.png")); err != nil {
		t.Fatal(err)
	}
	if err := window.Screenshot(el.Root(view), 420, 320, filepath.Join(dir, "pending-instructions-narrow.png")); err != nil {
		t.Fatal(err)
	}
	applyPalette(true)
	if err := window.Screenshot(el.Root(view), 860, 320, filepath.Join(dir, "pending-instructions-dark.png")); err != nil {
		t.Fatal(err)
	}
	applyPalette(false)
	a.beginInstructionEdit("preview-1")
	if err := window.Screenshot(el.Root(view), 860, 360, filepath.Join(dir, "pending-instructions-edit.png")); err != nil {
		t.Fatal(err)
	}
	if err := window.Screenshot(el.Root(view), 420, 360, filepath.Join(dir, "pending-instructions-edit-narrow.png")); err != nil {
		t.Fatal(err)
	}
}

func TestPendingInstructionEditThroughNativeControlPreservesQueueAndDraft(t *testing.T) {
	b, task, run, _ := instructionTestRun(t)
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "attached.txt")
	if err := os.WriteFile(file, []byte("keep attachment"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := b.service.QueueFollowUp(ctx, run.ID, desktopservice.InstructionInput{Content: "first", AttachmentPaths: []string{file}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.service.QueueFollowUp(ctx, run.ID, desktopservice.InstructionInput{Content: "second"})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(b)
	t.Cleanup(a.close)
	a.task, a.taskID, a.runID = &task, task.ID, run.ID
	detail, err := b.service.GetRunDetail(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	a.detail, a.prompt = &detail, "unrelated composer draft"
	a.attachments.Items = []selectedAttachment{{Path: file}}
	updates := make(chan func(), 8)
	a.postUpdate = func(fn func()) { updates <- fn }
	root := el.Root(el.ViewFunc(a.pendingInstructionList))
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(700, 300)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	var find func([]input.SemanticNode, string) (image.Rectangle, bool)
	find = func(nodes []input.SemanticNode, label string) (image.Rectangle, bool) {
		for _, node := range nodes {
			if node.Desc.Label == label && !node.Desc.Disabled && node.Desc.Gestures&input.ClickGesture != 0 {
				return node.Desc.Bounds, true
			}
			if box, ok := find(node.Children, label); ok {
				return box, true
			}
		}
		return image.Rectangle{}, false
	}
	click := func(label string) {
		t.Helper()
		frame()
		box, ok := find(router.AppendSemantics(nil), label)
		if !ok {
			t.Fatalf("missing enabled control %q", label)
		}
		pos := f32.Pt(float32(box.Min.X+box.Dx()/2), float32(box.Min.Y+box.Dy()/2))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
		frame()
	}
	complete := func() {
		t.Helper()
		select {
		case fn := <-updates:
			fn()
		case <-time.After(3 * time.Second):
			t.Fatal("edit did not complete")
		}
	}
	click("编辑后续消息")
	if a.instructionEdit.InstructionID != first.ID {
		t.Fatal("edit opened wrong queue item")
	}
	frame() // Apply the editor's one-time focus request.
	router.Queue(key.Event{Name: "A", Modifiers: key.ModShortcut, State: key.Press})
	frame()
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 5}, Text: "edited first"})
	frame()
	if a.instructionEdit.Content != "edited first" {
		t.Fatalf("native edit not accepted: %q", a.instructionEdit.Content)
	}
	click("保存修改")
	complete()
	updated, err := b.service.GetRunDetail(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Instructions) != 2 || updated.Instructions[0].ID != first.ID || updated.Instructions[0].Content != "edited first" || !updated.Instructions[0].CreatedAt.Equal(first.CreatedAt) || len(updated.Instructions[0].Attachments) != 1 || updated.Instructions[0].Attachments[0] != first.Attachments[0] || updated.Instructions[1].ID != second.ID {
		t.Fatalf("edit replaced or reordered queue: %+v", updated.Instructions)
	}
	if a.instructionEdit.InstructionID != "" || a.prompt != "unrelated composer draft" || len(a.attachments.Items) != 1 {
		t.Fatal("save lost editor or composer state")
	}
	a.detail = &updated
	a.beginInstructionEdit(second.ID)
	a.instructionEdit.Content = "unsaved cancellation"
	click("取消编辑")
	if a.instructionEdit.InstructionID != "" {
		t.Fatal("cancel retained editing state")
	}
	a.beginInstructionEdit(second.ID)
	a.instructionEdit.Content = "keep this if delivery wins"
	// Model deletion/delivery winning the repository lock before Save arrives.
	if err := b.service.RemoveInstruction(ctx, run.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	click("保存修改")
	complete()
	if a.errorText == "" || a.instructionEdit.Content != "keep this if delivery wins" {
		t.Fatal("stale edit silently succeeded or lost draft")
	}
}
