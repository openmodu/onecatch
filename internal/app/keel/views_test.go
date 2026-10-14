package keeldesktop

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/window"
	"github.com/openmodu/onecatch/internal/app/keel/terminalview"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/skillmanager"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

// Optional render QA uses the production views and Keel's offscreen renderer.
// It needs a working graphics driver and is kept out of headless CI by default.
// ONECATCH_KEEL_PREVIEWS=bin/keel-qa go test ./internal/app/keel -run TestRenderPreview
func TestRenderPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to render native UI previews")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	icon, err := png.Decode(bytes.NewReader(appIcon))
	if err != nil {
		t.Fatal(err)
	}
	b, err := openBackendWithEngine(context.Background(), t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(b)
	defer a.unsubscribe()
	defer a.cancel()
	defer b.close()
	a.logo = kit.Image(icon, "OneCatch").Size(22, 22).Rounded(5)
	a.workspaces = []domainworkspaces.Workspace{{ID: "project", Name: "Onecatch", Path: "~/Code/onecatch", DefaultSandbox: "workspace-write"}}
	a.workspaceID = "project"
	a.captionGit = &domainworkspaces.GitSnapshot{IsRepo: true, Branch: "main", Head: "a81c79e"}
	a.ui.Expanded["project"] = true
	a.runtimes = []desktopservice.RuntimeInfo{{ID: "codex", Name: "Codex", Available: true, Enabled: true}, {ID: "claude", Name: "Claude Code", Available: true, Enabled: true}}
	a.buildExecutorMenu()
	a.tasks["project"] = []domaintasks.Task{{ID: "task1", Title: "用 Go 构建新的桌面客户端", WorkspaceID: "project", Status: "completed", Harness: "codex", WorkflowID: "single_agent"}, {ID: "task2", Title: "整理工作流与 Agent 配置", WorkspaceID: "project", Status: "ready", Harness: "claude", WorkflowID: "single_agent"}}
	a.runtime.SetEntries(kit.SelectOption{Value: "codex", Label: "Codex"}, kit.SelectOption{Value: "claude", Label: "Claude Code"})
	a.runtime.SetValue("codex")
	a.workflow.SetEntries(kit.SelectOption{Value: "single_agent", Label: "单 Agent 完成"})
	a.workflow.SetValue("single_agent")
	locale.Apply(locale.Chinese())
	applyPalette(false)
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "welcome.png")); err != nil {
		t.Fatal(err)
	}

	a.workbench.Visible = true
	a.workbench.Tab = "git"
	a.workbench.Context = "project"
	a.workbench.Snapshot = &domainworkspaces.GitSnapshot{IsRepo: true, Branch: "feature/workbench", Head: "019f5ed", Files: []domainworkspaces.GitFile{{Path: "src/app/App.jsx", Index: " ", Worktree: "M"}}}
	a.workbench.BranchPicker.SetEntries(kit.SelectOption{Value: "feature/workbench", Label: "feature/workbench"}, kit.SelectOption{Value: "main", Label: "main"})
	a.workbench.BranchPicker.SetValue("feature/workbench")
	for _, size := range []struct {
		name string
		w, h int
	}{{"inspector-git", 1120, 760}, {"inspector-git-narrow", 860, 560}} {
		if err := window.Screenshot(el.Root(a), size.w, size.h, filepath.Join(dir, size.name+".png")); err != nil {
			t.Fatal(err)
		}
	}
	a.workbench.Tab = "files"
	files := a.fileState("project")
	files.Entries = []desktopservice.WorkspaceFileEntry{{Name: "frontend", Path: "frontend", Directory: true}, {Name: "README.md", Path: "README.md"}}
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "inspector-files.png")); err != nil {
		t.Fatal(err)
	}
	files.Buffers["main.go"] = newFileBuffer(desktopservice.WorkspaceFileDocument{Path: "main.go", Content: "package main\n\nfunc main() {\n    println(\"Hello from Keel\")\n}\n"})
	files.Active = "main.go"
	files.Buffers["main.go"].Editor.SetValue("package main\n\nfunc main() {\n    println(\"Hello from Go\")\n}\n")
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "inspector-file-editor.png")); err != nil {
		t.Fatal(err)
	}
	a.workbench.Tab = "review"
	review := a.reviewState("project")
	review.Files = buildGitReview(domainworkspaces.GitSnapshot{Files: []domainworkspaces.GitFile{{Path: "frontend/src/app/components/Composer.jsx", Index: " ", Worktree: "M"}, {Path: "frontend/src/index.css", Index: " ", Worktree: "M"}}}, "", "diff --git a/frontend/src/app/components/Composer.jsx b/frontend/src/app/components/Composer.jsx\nindex 2cc67d1..b5aa194 100644\n--- a/frontend/src/app/components/Composer.jsx\n+++ b/frontend/src/app/components/Composer.jsx\n@@ -42,3 +42,4 @@ export default function Composer() {\n   const canSend = Boolean(draft.trim());\n-  const label = \"继续\";\n+  const label = \"发送消息\";\n+  const directAgent = workflowId === \"single_agent\";\n   return <div>{label}</div>;\ndiff --git a/frontend/src/index.css b/frontend/src/index.css\nindex 88231a0..1be5412 100644\n--- a/frontend/src/index.css\n+++ b/frontend/src/index.css\n@@ -1234,3 +1234,4 @@\n .conversation-message-actions {\n-  height: 24px;\n+  height: 27px;\n+  padding-top: 3px;\n }\n", nil)
	review.Selected = "frontend/src/app/components/Composer.jsx"
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "inspector-review.png")); err != nil {
		t.Fatal(err)
	}
	a.workbench.Maximized = true
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "inspector-review-wide.png")); err != nil {
		t.Fatal(err)
	}
	a.workbench.Maximized = false
	a.workbench.Visible = false
	a.attachments.Items = []selectedAttachment{{Name: "task-notes.md", Path: "/preview/task-notes.md", Size: 4096}}
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "attachments.png")); err != nil {
		t.Fatal(err)
	}
	a.attachments.Items = nil
	a.workbench.Worktrees = []domainworkspaces.Worktree{{ContextID: "main", Path: "~/Code/openmodu/onecatch", Root: "~/Code/openmodu/onecatch", Branch: "main", Main: true}, {ContextID: "login", Path: "~/Code/openmodu/onecatch-login", Root: "~/Code/openmodu/onecatch-login", Branch: "feat/login"}, {ContextID: "review", Path: "~/Code/openmodu/onecatch-review", Root: "~/Code/openmodu/onecatch-review", Branch: "fix/sidebar"}, {ContextID: "old", Path: "~/Code/openmodu/onecatch-old", Root: "~/Code/openmodu/onecatch-old", Branch: "old", Unavailable: true}}
	a.buildWorktreeMenu()
	a.workbench.WorktreeMenu.SetValue(true)
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "worktree-menu.png")); err != nil {
		t.Fatal(err)
	}
	a.workbench.WorktreeMenu.SetValue(false)
	a.editProject("project")
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "project-dialog.png")); err != nil {
		t.Fatal(err)
	}
	a.projectDialog.SetValue(false)
	a.task = &a.tasks["project"][0]
	a.taskID = "task1"
	a.transcript.items["prompt"] = &entry{key: "prompt", user: true, text: "帮我做一版纯 Go 的桌面客户端。", at: time.Now()}
	a.transcript.items["answer"] = &entry{key: "answer", kind: agentrun.KindMessage, text: "界面由 **Keel** 直接绘制。\n\n- 项目与任务保存在本地\n- Agent 消息实时更新\n- 窗口只保留一条竖向分隔线\n\n```go\nwindow.Open(window.Options{\n    Title: \"OneCatch\",\n    Content: el.Root(app),\n})\n```", at: time.Now().Add(time.Second)}
	a.syncMessages()
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "conversation.png")); err != nil {
		t.Fatal(err)
	}
	a.dark = true
	applyPalette(true)
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "conversation-dark.png")); err != nil {
		t.Fatal(err)
	}
	a.dark = false
	applyPalette(false)
	originalRuntimes := a.runtimes
	a.runtimes = []desktopservice.RuntimeInfo{{ID: "codex", Name: "Codex", Available: true, Enabled: true, Version: "CLI"}}
	a.settings, err = b.service.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := &settingsPage{app: a, runtime: kit.Select("").Appearance(false), enabled: newFilledToggle("启用此 Agent", true, true)}
	p.runtime.SetEntries(kit.SelectOption{Value: "codex", Label: "Codex"})
	p.runtime.SetValue("codex")
	p.selectRuntime("codex")
	if err := window.Screenshot(el.Root(p), 960, 800, filepath.Join(dir, "settings.png")); err != nil {
		t.Fatal(err)
	}
	for _, section := range []string{"harness", "execution", "security", "storage", "terminal"} {
		p.section = section
		if err := window.Screenshot(el.Root(p), 960, 800, filepath.Join(dir, "settings-"+section+".png")); err != nil {
			t.Fatal(err)
		}
	}
	a.runtimes = originalRuntimes
	a.page = "workflows"
	a.definitions, err = b.service.ListDefinitions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range a.definitions {
		if d.ID != "single_agent" {
			a.ui.SelectedWorkflow = d.ID
			break
		}
	}
	if err := window.Screenshot(el.Root(workflowView{a}), 1040, 760, filepath.Join(dir, "workflows.png")); err != nil {
		t.Fatal(err)
	}
	for _, d := range a.definitions {
		if d.ID == a.ui.SelectedWorkflow {
			a.selectWorkflow(d.ID, true)
			break
		}
	}
	if err := window.Screenshot(el.Root(workflowView{a}), 1040, 760, filepath.Join(dir, "workflow-form.png")); err != nil {
		t.Fatal(err)
	}
	a.ui.WorkflowRaw = true
	if err := window.Screenshot(el.Root(workflowView{a}), 1040, 760, filepath.Join(dir, "workflow-source.png")); err != nil {
		t.Fatal(err)
	}
	a.ui.WorkflowSource = false
	a.page = "templates"
	a.library.Material = "func add(a,b int) int { return a+b }"
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "templates.png")); err != nil {
		t.Fatal(err)
	}
	a.page = "skills"
	a.library.Skills = []skillmanager.Skill{{Name: "review", Description: "审查项目中的改动，查找可验证的缺陷。"}}
	a.library.SkillName = "review"
	a.library.SkillDoc = &skillmanager.SkillDocument{Skill: skillmanager.Skill{Name: "review", Path: "skills/review/SKILL.md"}, Content: "---\nname: review\ndescription: 审查项目中的改动\n---\n\n# Review\n\n结合现有测试验证改动。"}
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "skills.png")); err != nil {
		t.Fatal(err)
	}
	a.page = "usage"
	lifetime := int64(12972034323)
	a.library.Usage = []agentrun.AccountUsage{{Runtime: agentrun.RuntimeCodex, Scope: agentrun.AccountUsageScopeAccount, Summary: agentrun.AccountUsageSummary{LifetimeTokens: &lifetime}, DailyUsage: []agentrun.AccountDailyUsage{{StartDate: time.Now().Format("2006-01-02"), Tokens: 182000}}, RateLimits: []agentrun.AccountRateLimit{{ID: "codex", Name: "Codex", Primary: &agentrun.AccountRateLimitWindow{UsedPercent: 27, WindowDurationMins: 300}}}}}
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "usage.png")); err != nil {
		t.Fatal(err)
	}
	a.page = ""
	a.task = nil
	a.taskID = ""
	a.ui.More.SetValue(true)
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "more-menu.png")); err != nil {
		t.Fatal(err)
	}
	a.ui.More.SetValue(false)
	a.ui.Footer.SetValue(true)
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "app-menu.png")); err != nil {
		t.Fatal(err)
	}
	a.ui.Footer.SetValue(false)
	a.ui.Customize.SetValue(true)
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "sidebar-customization.png")); err != nil {
		t.Fatal(err)
	}
	a.ui.Customize.SetValue(false)
	if err := window.Screenshot(el.Root(a), 860, 560, filepath.Join(dir, "main-narrow.png")); err != nil {
		t.Fatal(err)
	}
	a.ui.Preferences.Collapsed = true
	if err := window.Screenshot(el.Root(a), 860, 560, filepath.Join(dir, "compact.png")); err != nil {
		t.Fatal(err)
	}
	a.ui.Preferences.Collapsed = false
	a.page = ""
	a.terminal.Open = true
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "terminal-empty.png")); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"terminal-left", "terminal-right"} {
		s := terminalview.New(nil)
		s.AttachSession("")
		defer s.Close()
		s.Write([]byte("\x1b[32mityike@Mac-mini\x1b[0m: \x1b[33m~/Code/onecatch\x1b[0m (main)\r\n$ printf 'Hello from Go\\n'\r\nHello from Go\r\n\x1b[36m终端已就绪 · Go 原生渲染\x1b[0m\r\n$ "))
		a.terminal.Panes[id] = &terminalPane{ID: id, Title: "zsh", Running: true, Screen: s}
	}
	tab := &terminalTab{ID: "terminal-left", Root: terminalLeaf("terminal-left")}
	a.terminal.Tabs = []*terminalTab{tab}
	a.terminal.Active, a.terminal.Focused = tab.ID, "terminal-left"
	for _, size := range []struct {
		name string
		w, h int
	}{{"terminal", 1120, 760}, {"terminal-narrow", 860, 560}} {
		if err := window.Screenshot(el.Root(a), size.w, size.h, filepath.Join(dir, size.name+".png")); err != nil {
			t.Fatal(err)
		}
	}
	tab.Root.split("terminal-left", "terminal-right", "vertical", "fixture")
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "terminal-split.png")); err != nil {
		t.Fatal(err)
	}
	a.terminal.SearchOpen, a.terminal.Query = true, "Hello"
	a.searchTerminal(false)
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "terminal-search.png")); err != nil {
		t.Fatal(err)
	}
	a.terminal.SearchOpen, a.terminal.Maximized = false, true
	a.settings.Terminal.Theme = "contrast"
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "terminal-maximized.png")); err != nil {
		t.Fatal(err)
	}
	a.settings.Terminal.Theme = "system"
	a.dark = true
	applyPalette(true)
	if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, "terminal-dark.png")); err != nil {
		t.Fatal(err)
	}
	applyPalette(false)

}
