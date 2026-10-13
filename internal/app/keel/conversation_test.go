package keeldesktop

import (
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/window"
	tasks "github.com/openmodu/onecatch/internal/domain/tasks"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	workspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	desktop "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/desktop/runstream"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConversationParallelToolResultsRemainDistinct(t *testing.T) {
	tx := newTranscript()
	at := time.Now()
	for i, v := range []struct {
		step, id string
		kind     agentrun.EventKind
		text     string
		failed   bool
	}{
		{"first", "a", agentrun.KindToolUse, "read a", false}, {"first", "b", agentrun.KindToolUse, "read b", false},
		{"first", "a", agentrun.KindToolResult, "a result", false}, {"first", "b", agentrun.KindToolResult, "", true},
		{"second", "a", agentrun.KindToolUse, "other step", false}, {"second", "a", agentrun.KindToolResult, "other result", false},
	} {
		tx.frame(runstream.Frame{StepRunID: v.step, Seq: int64(i + 1), Event: agentrun.Event{Kind: v.kind, StreamID: v.id, Text: v.text, Failed: v.failed, Revision: 1, At: at.Add(time.Duration(i) * time.Second)}})
	}
	rows := buildConversationEntries(tx.visible())
	if len(rows) != 2 || len(rows[0].process) != 2 || len(rows[1].process) != 1 {
		t.Fatalf("process grouping: %+v", rows)
	}
	a, b, other := rows[0].process[0], rows[0].process[1], rows[1].process[0]
	if a.text != "read a" || a.details[0].text != "a result" || a.failed || !a.settled {
		t.Fatalf("call a: %+v", a)
	}
	if !b.failed || !b.settled || b.details[0].text != "" {
		t.Fatalf("silent failed call: %+v", b)
	}
	if other.details[0].text != "other result" {
		t.Fatalf("cross-step call: %+v", other)
	}
}

func conversationFixture(t *testing.T) *app {
	a, _ := skillTestApp(t)
	a.page = ""
	a.workflow.SetValue("single_agent")
	a.runtime.SetValue("pi")
	a.workspaces = []workspaces.Workspace{{ID: "modu", Name: "modu", Path: "~/Code/modu", DefaultSandbox: "full"}, {ID: "onecatch", Name: "Onecatch", Path: "~/Code/onecatch"}, {ID: "app", Name: "onecatch_app", Path: "~/Code/onecatch_app"}, {ID: "h5000", Name: "H5000M", Path: "~/Code/remote", RemoteFS: &workspaces.RemoteFS{}}}
	a.workspaceID = "modu"
	a.captionGit = &workspaces.GitSnapshot{IsRepo: true, Branch: "main"}
	a.runtimes = []desktop.RuntimeInfo{{ID: "pi", Name: "Pi", Available: true, Enabled: true}}
	at := time.Date(2026, 10, 9, 7, 57, 0, 0, time.Local)
	a.tasks["modu"] = []tasks.Task{{ID: "hello", Title: "Say hello", Prompt: "今天是什么日子", WorkspaceID: "modu", WorkflowID: "single_agent", Harness: "pi", Sandbox: "full", Status: "completed", CreatedAt: at}, {ID: "weather", Title: "查询北京未来7天天气", Harness: "grok", Status: "completed"}, {ID: "greeting", Title: "Respond to hello greeting", Harness: "grok", Status: "completed"}}
	a.task = &a.tasks["modu"][0]
	a.taskID = a.task.ID
	a.ui.Expanded["modu"] = true
	a.runID = "preview"
	a.detail = &desktop.RunDetail{Task: *a.task, Run: workflows.Run{ID: "preview", Status: workflows.RunCompleted}, Workspace: a.workspaces[0]}
	items := []entry{
		{key: "prompt", user: true, text: "今天是什么日子", at: at},
		{key: "tool", step: "one", stream: "date", seq: 1, kind: agentrun.KindToolUse, text: "date +%F", at: at.Add(time.Second)},
		{key: "result", step: "one", stream: "date", seq: 2, kind: agentrun.KindToolResult, text: "2026-09-10", at: at.Add(2 * time.Second)},
		{key: "answer", step: "one", kind: agentrun.KindMessage, text: "今天是 **2026 年 9 月 10 日，星期四**（根据系统时间）。\n\n另外，9 月 10 日也是中国的**教师节** 👩🏫。\n\n> ⚠️ 不过我注意到一个问题：刚才查天气时，数据源返回的「今天」是 9 月 2 日，而系统时间是 9 月 10 日，两者对不上。可能是天气接口的数据延迟或缓存问题，之前给你的预报日期可能不准确，需要的话我可以重新核实。", at: at.Add(3 * time.Second)},
		{key: "followup", user: true, text: "hello", at: at.Add(4 * time.Second)},
		{key: "reply", step: "two", kind: agentrun.KindMessage, text: "你好！我在。需要我做什么？", at: at.Add(5 * time.Second)},
	}
	for i := range items {
		e := items[i]
		a.transcript.items[e.key] = &e
	}
	a.syncMessages()
	return a
}

func TestConversationNativeDisclosure(t *testing.T) {
	a := conversationFixture(t)
	root := el.Root(a)
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1288, 804)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	frame()
	frame()
	box, ok := findSkillControl(router.AppendSemantics(nil), "运行了 1 个工具")
	if !ok {
		t.Fatal("missing collapsed process")
	}
	click := func(b image.Rectangle) {
		p := f32.Pt(float32(b.Min.X+b.Dx()/2), float32(b.Min.Y+b.Dy()/2))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
		frame()
		frame()
	}
	click(box)
	tool, ok := findSkillControl(router.AppendSemantics(nil), "执行命令 · date +%F")
	if !ok {
		t.Fatal("process did not expose tool")
	}
	click(tool)
	if !a.conversation.open["tool"] {
		t.Fatal("tool did not expand")
	}
	// Expanding the tool can scroll its group header behind the fixed title bar.
	// Scroll back before testing a second click on the visible disclosure.
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(800, 200), Scroll: f32.Pt(0, -400)})
	frame()
	frame()
	box, ok = findSkillControl(router.AppendSemantics(nil), "运行了 1 个工具")
	if !ok {
		t.Fatal("expanded process header missing")
	}
	click(box)
	if a.conversation.open["process/tool"] {
		t.Fatal("process did not collapse")
	}
	a.syncMessages()
	if a.conversationOpen("process/tool", false) {
		t.Fatal("stream refresh discarded explicit collapse")
	}
}

func TestRenderConversationReference(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	locale.Apply(locale.Chinese())
	applyPalette(false)
	a := conversationFixture(t)
	for _, v := range []struct {
		name string
		w, h int
	}{{"conversation-reference", 1288, 804}, {"conversation-reference-narrow", 860, 600}} {
		if err := window.Screenshot(el.Root(a), v.w, v.h, filepath.Join(dir, v.name+".png")); err != nil {
			t.Fatal(err)
		}
	}
	a.conversation.open = map[string]bool{"process/tool": true, "tool": true}
	a.conversation.overridden = map[string]bool{"process/tool": true, "tool": true}
	if err := window.Screenshot(el.Root(a), 1288, 804, filepath.Join(dir, "conversation-tools-open.png")); err != nil {
		t.Fatal(err)
	}
	a.dark = true
	applyPalette(true)
	if err := window.Screenshot(el.Root(a), 1288, 804, filepath.Join(dir, "conversation-reference-dark.png")); err != nil {
		t.Fatal(err)
	}
	applyPalette(false)
}

func TestConversationCopyReadsLatestAndClearsFeedback(t *testing.T) {
	a := conversationFixture(t)
	e := a.transcript.items["reply"]
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().Child(el.Div().ID("message/" + e.key).H(el.Dp(40)).WFull().Child(a.messageActions(cx, e, false)))
	}))
	var router input.Router
	var ops op.Ops
	now := time.Now()
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: now, Source: router.Source(), Constraints: layout.Exact(image.Pt(400, 120)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(20, 20)})
	frame()
	frame()
	// The CopyButton instance is retained while the streamed document changes.
	a.docs[e.key] = newConversationDocument("更新后的回复 👩🏫")
	box, ok := findSkillControl(router.AppendSemantics(nil), "复制消息")
	if !ok {
		t.Fatal("copy action missing")
	}
	p := f32.Pt(float32(box.Min.X+box.Dx()/2), float32(box.Min.Y+box.Dy()/2))
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: p})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
	frame()
	if _, data, ok := router.WriteClipboard(); !ok || string(data) != "更新后的回复 👩🏫" {
		t.Fatalf("copy did not read latest Unicode document: %q, %v copied=%v", data, ok, a.conversation.copyButtons[e.key].Copied())
	}
	if !a.conversation.copyButtons[e.key].Copied() {
		t.Fatal("copy feedback did not start")
	}
	now = now.Add(2 * time.Second)
	frame()
	frame()
	if a.conversation.copyButtons[e.key].Copied() {
		t.Fatal("custom styling broke the native CopyButton feedback timer")
	}
}

func TestLegacyTraeOutputAttachesToCallInsteadOfBecomingCommand(t *testing.T) {
	at := time.Now()
	entries := buildConversationEntries([]*entry{
		{key: "read", step: "a", kind: agentrun.KindToolUse, text: "Read", at: at},
		{key: "read-result", step: "a", kind: agentrun.KindToolResult, stream: "trae-tool_result-call_a", text: "65 func example()", at: at.Add(time.Second)},
		{key: "other", step: "b", kind: agentrun.KindToolUse, stream: "modern", text: "pwd", at: at},
		{key: "unmatched", step: "b", kind: agentrun.KindToolResult, stream: "different", text: "other output", at: at},
	})
	if len(entries[0].process) != 1 || len(entries[0].process[0].details) != 1 || !entries[0].process[0].settled {
		t.Fatal("legacy output should settle its own call")
	}
	if len(entries[1].process) != 2 || len(entries[1].process[0].details) != 0 {
		t.Fatal("mismatched modern IDs must not pair")
	}
	title, body, _ := toolDisplay(&entry{kind: agentrun.KindToolUse, text: `Read {"file_path":"/repo/a.go","offset":65,"limit":10}`})
	if !strings.Contains(title, "读取文件") || !strings.Contains(title, "/repo/a.go") || !strings.Contains(body, `"offset": 65`) {
		t.Fatalf("unreadable tool display: %q, %q", title, body)
	}
	title, _, _ = toolDisplay(&entry{kind: agentrun.KindToolResult, text: "65 func example()"})
	if title != "工具结果 · 未找到对应调用" {
		t.Fatalf("orphan result presented as command: %q", title)
	}
}

func TestLegacyParallelToolsDoNotGuessResultOwnership(t *testing.T) {
	rows := buildConversationEntries([]*entry{
		{key: "a", step: "s", kind: agentrun.KindToolUse, text: "read a"},
		{key: "b", step: "s", kind: agentrun.KindToolUse, text: "read b"},
		{key: "ra", step: "s", kind: agentrun.KindToolResult, stream: "legacy-result-a", text: "a content"},
		{key: "rb", step: "s", kind: agentrun.KindToolResult, stream: "legacy-result-b", text: "b content"},
		{key: "c", step: "s", kind: agentrun.KindToolUse, text: "read c"},
		{key: "rc", step: "s", kind: agentrun.KindToolResult, stream: "legacy-result-c", text: "c content"},
	})
	items := rows[0].process
	if len(items) != 5 || len(items[0].details) != 0 || len(items[1].details) != 0 || len(items[4].details) != 1 {
		t.Fatalf("ambiguous legacy output assigned to wrong command: %+v", items)
	}
	if items[4].details[0].text != "c content" {
		t.Fatal("later sequential call lost its result")
	}
}

func TestCodexParallelCommandOutputsShowParametersOnTheirCalls(t *testing.T) {
	at := time.Unix(100, 0)
	rows := buildConversationEntries([]*entry{
		{key: "a", step: "impl", kind: agentrun.KindToolUse, stream: "codex-tool-output-a", text: "go test ./...", at: at},
		{key: "b", step: "impl", kind: agentrun.KindToolUse, stream: "codex-tool-output-b", text: `files.Read {"file_path":"/repo/a.go","offset":65}`, at: at},
		{key: "rb", step: "impl", kind: agentrun.KindToolResult, stream: "codex-tool-output-b", text: "65 func example()", at: at.Add(time.Second)},
		{key: "ra", step: "impl", kind: agentrun.KindToolResult, stream: "codex-tool-output-a", text: "test failed", failed: true, at: at.Add(2 * time.Second)},
	})
	if len(rows) != 1 || len(rows[0].process) != 2 {
		t.Fatalf("orphaned Codex results: %+v", rows)
	}
	command, read := rows[0].process[0], rows[0].process[1]
	if len(command.details) != 1 || command.details[0].text != "test failed" || !command.failed || len(read.details) != 1 || read.details[0].text != "65 func example()" || read.failed {
		t.Fatal("parallel results paired to wrong calls")
	}
	_, body, _ := toolDisplay(command)
	sections := toolCodeSections(command, "命令", body)
	if len(sections) != 1 || sections[0].language != "bash" || sections[0].text != "go test ./..." {
		t.Fatal("command parameters not available")
	}
	_, body, _ = toolDisplay(read)
	if !strings.Contains(body, `"offset": 65`) || !strings.Contains(body, "/repo/a.go") {
		t.Fatalf("read parameters missing: %s", body)
	}
}
