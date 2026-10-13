package keeldesktop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/gio/io/semantic"
	"github.com/dyike/keel/gio/layout"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/gio/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/window"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/worker"
)

func workerTestSettings(a *app) *settingsPage {
	p := &settingsPage{app: a, runtime: kit.Select("").Appearance(false), enabled: newFilledToggle("启用此 Agent", true, true), section: "experimental"}
	p.runtime.SetValue("codex")
	p.selectRuntime("codex")
	return p
}

func TestWorkerPairEditHealthAndTaskPersistence(t *testing.T) {
	a := dagTestApp(t)
	p := workerTestSettings(a)
	t.Cleanup(p.closeWorkers)
	ctx := context.Background()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pair":
			var body worker.PairRequest
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Code != "ABCD2345" {
				http.Error(w, "invalid code", http.StatusForbidden)
				return
			}
			_ = json.NewEncoder(w).Encode(worker.PairResult{WorkerID: "test-worker", Name: "Test Mac mini", Token: "ephemeral-test-token", ProtocolVersion: 1})
		case "/v1/health":
			if r.Header.Get("Authorization") != "Bearer ephemeral-test-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(worker.Health{WorkerID: "test-worker", Name: "Test Mac mini", ProtocolVersion: 1, Runtimes: map[string]bool{"codex": true}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	p.openWorker(nil)
	p.workers.form.kind.SetValue("worker")
	p.workers.form.input.BaseURL, p.workers.form.code = server.URL, "ABCD2345"
	info, err := saveWorkerConnection(ctx, a.backend.service, p.workers.form)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasToken || len(info.ServerCertificateSHA256) != 64 {
		t.Fatal("pairing didn't retain token and TLS certificate pin")
	}
	public, _ := json.Marshal(info)
	if strings.Contains(string(public), "ephemeral-test-token") {
		t.Fatal("pairing credential leaked to UI")
	}
	p.openWorker(&info)
	p.workers.form.input.Name = "Renamed Mac mini"
	edited, err := saveWorkerConnection(ctx, a.backend.service, p.workers.form)
	if err != nil || edited.Name != "Renamed Mac mini" || !edited.HasToken || edited.ServerCertificateSHA256 != info.ServerCertificateSHA256 {
		t.Fatal("edit lost pairing or pin")
	}
	status, err := a.backend.service.CheckWorker(ctx, info.ID)
	if err != nil || status.Health.WorkerID != info.ID {
		t.Fatalf("authenticated health check: %v", err)
	}
	// A TLS pin change must take effect; a stale earlier success must not overwrite it.
	p.workers.health = map[string]workerHealth{info.ID: {checking: true, version: 7}}
	p.invalidateWorkerHealth(info.ID)
	p.applyWorkerHealth(info.ID, 7, status, nil)
	if p.workers.health[info.ID].ok {
		t.Fatal("stale health survived a changed configuration")
	}
	h := p.workers.health[info.ID]
	p.applyWorkerHealth(info.ID, h.version, status, errors.New("connection refused"))
	if p.workers.health[info.ID].ok || p.workers.health[info.ID].err == "" {
		t.Fatal("failure displayed as connected")
	}
	p.workers.form.input.ServerCertificateSHA256 = strings.Repeat("0", 64)
	if _, err := saveWorkerConnection(ctx, a.backend.service, p.workers.form); err != nil {
		t.Fatal(err)
	}
	if _, err := a.backend.service.CheckWorker(ctx, info.ID); err == nil {
		t.Fatal("health accepted wrong TLS pin")
	}
	// Restore pin and create a real persisted remote task through the same input as Send.
	p.workers.form.input.ServerCertificateSHA256 = info.ServerCertificateSHA256
	if _, err := saveWorkerConnection(ctx, a.backend.service, p.workers.form); err != nil {
		t.Fatal(err)
	}
	a.settings.Experimental.RemoteWorkersEnabled = true
	a.settings, err = a.backend.service.UpdateExperimentalSettings(ctx, a.settings.Experimental, a.settings.Revision)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := a.backend.service.AddWorkspace(ctx, desktopservice.AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	a.workspaces, a.workspaceID = []domainworkspaces.Workspace{workspace}, workspace.ID
	a.workers, _ = a.backend.service.ListWorkers(ctx)
	if !a.selectConversationWorker(info.ID, "codex") {
		t.Fatal("connected worker cannot be selected")
	}
	created, err := a.backend.service.CreateTask(ctx, a.createTaskInput("review the code"))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := a.backend.store.Repos.Tasks.GetTask(ctx, created.ID)
	if err != nil || stored.WorkerID != info.ID || stored.Harness != "codex" || stored.Worktree != nil {
		t.Fatal("native Send didn't persist remote execution target")
	}
	if err := a.backend.service.DeleteWorker(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	items, err := a.backend.service.ListWorkers(ctx)
	if err != nil || len(items) != 0 {
		t.Fatal("delete didn't remove registry entry")
	}
}

func TestWorkerConversationRestrictions(t *testing.T) {
	a := dagTestApp(t)
	a.workflow.SetValue("single_agent")
	a.settings.Experimental.RemoteWorkersEnabled = true
	a.workspaces = []domainworkspaces.Workspace{{ID: "project", AutoWorktree: true}}
	a.workspaceID = "project"
	a.workers = []worker.Info{{ID: "mac", Name: "Mac mini", Enabled: true}, {ID: "sandbox", Name: "Volcengine Sandbox · example", Enabled: true, Provider: worker.ProviderVolcengineSandbox}, {ID: "disabled", Name: "Unavailable"}}
	a.sandbox.SetValue("full")
	a.model = "local-only-profile"
	if !a.selectConversationWorker("sandbox", "codex") || a.model != "" || a.sandbox.Value() == "full" || a.effectiveWorktreeMode() != "project" || a.attachmentsEnabled() {
		t.Fatal("remote restrictions not applied")
	}
	if a.conversationLabel("Codex") != "codex volc_sandbox" {
		t.Fatal("sandbox label differs from original")
	}
	if a.selectConversationWorker("sandbox", "claude") || a.selectConversationWorker("disabled", "codex") || a.selectConversationWorker("missing", "codex") {
		t.Fatal("unsupported worker accepted")
	}
	a.selectLocalConversation("claude", "single_agent")
	if a.remoteConversation() || a.model != "" || !a.attachmentsEnabled() {
		t.Fatal("local switch kept remote profile")
	}
	a.attachments.Items = []selectedAttachment{{Path: "/test/image.png"}}
	if a.selectConversationWorker("mac", "codex") {
		t.Fatal("local attachments crossed remote boundary")
	}
	a.attachments.Items = nil
	a.workspaces[0].RemoteFS = &domainworkspaces.RemoteFS{}
	if a.selectConversationWorker("mac", "codex") {
		t.Fatal("Remote FS accepted remote Worker")
	}
	a.workspaces[0].RemoteFS = nil
	a.settings.Experimental.RemoteWorkersEnabled = false
	if a.selectConversationWorker("mac", "codex") {
		t.Fatal("disabled experiment accepted remote Worker")
	}
	a.settings.Experimental.RemoteWorkersEnabled = true
	if !a.selectConversationWorker("mac", "codex") {
		t.Fatal("standard Worker rejected")
	}
	a.selectWorktree("new", nil)
	if a.effectiveWorktreeMode() != "project" {
		t.Fatal("remote target accepted local Worktree")
	}
	a.workers[0].Enabled = false
	if a.conversationWorkerReady() || a.workerID != "mac" {
		t.Fatal("unavailable Worker was silently replaced or accepted")
	}
	a.workers[0].Enabled = true
	a.task = &domaintasks.Task{ID: "persisted", WorkerID: "mac"}
	if a.selectConversationWorker("sandbox", "codex") {
		t.Fatal("existing task changed Worker")
	}
	a.selectLocalConversation("claude", "single_agent")
	if a.workerID != "mac" || a.runtime.Value() != "codex" {
		t.Fatal("existing task switched execution target")
	}
	a.task = nil
	a.selectLocalConversation("codex", "parallel_review")
	if a.remoteConversation() || a.workflow.Value() != "parallel_review" {
		t.Fatal("workflow retained conversation worker")
	}
}

func TestWorkerAgentPopoverNativeSelection(t *testing.T) {
	a := dagTestApp(t)
	icon, err := png.Decode(bytes.NewReader(appIcon))
	if err != nil {
		t.Fatal(err)
	}
	a.logo = kit.Image(icon, "OneCatch").Size(22, 22)
	a.workflow.SetValue("single_agent")
	a.runtime.SetValue("codex")
	a.settings.Experimental.RemoteWorkersEnabled = true
	a.workspaces = []domainworkspaces.Workspace{{ID: "project", Name: "Onecatch"}}
	a.workspaceID = "project"
	a.workers = []worker.Info{{ID: "mac", Name: "Mac mini", Enabled: true}}
	a.buildExecutorMenu()
	root := el.Root(a)
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1120, 760)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	var find func([]input.SemanticNode, string) (image.Rectangle, bool)
	find = func(nodes []input.SemanticNode, label string) (image.Rectangle, bool) {
		for _, n := range nodes {
			if n.Desc.Label == label && !n.Desc.Disabled && n.Desc.Class == semantic.Button {
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
		if !ok || b.Empty() {
			t.Fatalf("no enabled Agent control %q", label)
		}
		pos := f32.Pt(float32(b.Min.X+b.Dx()/2), float32(b.Min.Y+b.Dy()/2))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
		frame()
		frame()
	}
	frame()
	click("Codex")
	click("Codex · Mac mini")
	if a.workerID != "mac" || a.model != "" || a.ui.Executor.Value() {
		t.Fatal("remote row didn't select and close Agent menu")
	}
	click("Codex · Mac mini")
	click("Claude Code")
	if a.remoteConversation() || a.runtime.Value() != "claude" || a.ui.Executor.Value() {
		t.Fatal("local row retained remote target or open menu")
	}
	click("Claude Code")
	for _, d := range a.definitions {
		if d.ID == "parallel_review" {
			click(d.Name)
			break
		}
	}
	if a.workflow.Value() != "parallel_review" || a.remoteConversation() || a.ui.Executor.Value() {
		t.Fatal("workflow row retained remote conversation")
	}
}

func TestWorkerModalNativeKeyboardAndMinimumViewport(t *testing.T) {
	a := dagTestApp(t)
	p := workerTestSettings(a)
	t.Cleanup(p.closeWorkers)
	a.workers = []worker.Info{{ID: "mac", Name: "Mac mini", BaseURL: "http://192.168.1.20:9231", Enabled: true}, {ID: "sandbox", Name: "volc sandbox", Enabled: true, Provider: worker.ProviderVolcengineSandbox}}
	root := el.Root(p)
	var router input.Router
	var ops op.Ops
	size := image.Pt(960, 800)
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	var find func([]input.SemanticNode, string) (image.Rectangle, bool)
	find = func(nodes []input.SemanticNode, label string) (image.Rectangle, bool) {
		for _, n := range nodes {
			if n.Desc.Label == label && !n.Desc.Disabled && n.Desc.Class != semantic.Unknown {
				return n.Desc.Bounds, true
			}
			if b, ok := find(n.Children, label); ok {
				return b, true
			}
		}
		return image.Rectangle{}, false
	}
	box := func(label string) image.Rectangle {
		t.Helper()
		b, ok := find(router.AppendSemantics(nil), label)
		if !ok || b.Empty() {
			t.Fatalf("no enabled %q", label)
		}
		return b
	}
	click := func(label string) {
		t.Helper()
		b := box(label)
		pos := f32.Pt(float32(b.Min.X+b.Dx()/2), float32(b.Min.Y+b.Dy()/2))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
		frame()
	}
	frame()
	if _, ok := find(router.AppendSemantics(nil), "添加环境"); ok {
		t.Fatal("configuration exposed before unlock")
	}
	click("远端 Worker")
	router.Queue(key.Event{Name: "S", Modifiers: key.ModCtrl | key.ModAlt, State: key.Press})
	frame()
	frame()
	if !p.workersVisible() || a.settings.Experimental.RemoteWorkersEnabled {
		t.Fatalf("unlock state: section=%q page=%q unlocked=%v persisted=%v", p.section, p.view.Value(), p.workers.unlocked, a.settings.Experimental.RemoteWorkersEnabled)
	}
	click("添加环境")
	frame()
	if p.workers.form.kind.Value() != "sandbox" {
		t.Fatal("modal default differs from source")
	}
	click("沙箱连接 URL")
	router.Queue(key.Event{Name: "A", Modifiers: key.ModShortcut, State: key.Press})
	frame()
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 8}, Text: "https://example.volceapi.com/?faasInstanceName=test"})
	frame()
	frame()
	if !p.workers.form.canSave() {
		t.Fatal("valid sandbox URL typed in native field not accepted")
	}
	click("同步本地项目")
	if !p.workers.form.sync.Value() {
		t.Fatal("native switch didn't toggle")
	}
	for _, s := range []image.Point{{960, 800}, {860, 600}} {
		size = s
		frame()
		for _, label := range []string{"取消", "连接沙箱"} {
			if !box(label).In(image.Rectangle{Max: s}) {
				t.Fatalf("%s clipped at %v", label, s)
			}
		}
	}
	click("取消")
	if p.workers.form.open || p.workers.form.input.BaseURL != "" || p.workers.form.code != "" {
		t.Fatal("cancel didn't retire sensitive form state")
	}
	p.openWorker(&a.workers[0])
	frame()
	click("TLS 与设备配对")
	frame()
	box("服务端证书 SHA-256（可选）")
	box("mTLS 客户端私钥")
	if !box("保存修改").In(image.Rectangle{Max: size}) {
		t.Fatal("expanded TLS pushed action offscreen")
	}
}

func TestWorkerErrorRedaction(t *testing.T) {
	raw := "https://example.volceapi.com/?faasInstanceName=test&Authorization=secret"
	for _, err := range []error{errors.New("request failed: " + raw), errors.New(`Get "` + raw + `": EOF`)} {
		if strings.Contains(safeWorkerError(err, ""), "secret") || strings.Contains(safeWorkerError(err, raw), "secret") {
			t.Fatal("gateway credential in status")
		}
	}
}

func TestRenderWorkerPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	a := dagTestApp(t)
	p := workerTestSettings(a)
	applyPalette(false)
	root := el.Root(p)
	shot := func(name string, w, h int) {
		t.Helper()
		if err := window.Screenshot(root, w, h, filepath.Join(dir, name+".png")); err != nil {
			t.Fatal(err)
		}
	}
	shot("worker-locked", 960, 800)
	p.workers.unlocked = true
	a.workers = []worker.Info{{ID: "mac", Name: "Mac mini", BaseURL: "http://192.168.1.20:9231", Enabled: true}, {ID: "sandbox", Name: "volc sandbox", Enabled: true, Provider: worker.ProviderVolcengineSandbox}}
	p.workers.health = map[string]workerHealth{"mac": {ok: true}, "sandbox": {ok: true}}
	shot("worker-settings", 960, 800)
	p.openWorker(nil)
	shot("worker-sandbox-modal", 960, 800)
	p.workers.form.kind.SetValue("worker")
	shot("worker-pair-modal", 960, 800)
	p.openWorker(&a.workers[0])
	shot("worker-edit-modal", 960, 800)
	p.workers.form.tls = true
	shot("worker-edit-tls", 960, 800)
	shot("worker-edit-tls-narrow", 860, 600)
	a.dark = true
	applyPalette(true)
	shot("worker-edit-dark", 960, 800)
	applyPalette(false)
}
