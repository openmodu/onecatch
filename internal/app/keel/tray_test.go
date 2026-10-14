package keeldesktop

import (
	"bytes"
	"errors"
	"image/color"
	"image/png"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dyike/keel/native/tray"
	"github.com/dyike/keel/ui/locale"
	"github.com/mattn/go-runewidth"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
)

type testTrayIcon struct {
	menus  [][]tray.MenuItem
	closes int
	err    error
}

func (s *testTrayIcon) SetMenu(items []tray.MenuItem) error {
	s.menus = append(s.menus, items)
	return s.err
}
func (s *testTrayIcon) Close() error { s.closes++; return nil }

func trayTaskIDs(items []tray.MenuItem) []string {
	var ids []string
	for _, item := range items {
		if strings.HasPrefix(item.ID, "task/") {
			ids = append(ids, strings.TrimPrefix(item.ID, "task/"))
		}
	}
	return ids
}

func TestTrayShowsFiveRecentSessionsAcrossVisibleProjects(t *testing.T) {
	locale.Apply(locale.Chinese())
	a := conversationFixture(t)
	base := time.Now()
	a.workspaces = []domainworkspaces.Workspace{{ID: "first", Name: "第一个项目"}, {ID: "second", Name: "第二个项目"}, {ID: "hidden", Hidden: true}}
	a.tasks = map[string][]domaintasks.Task{
		"first": {
			{ID: "pin", WorkspaceID: "first", Pinned: true, UpdatedAt: base.Add(-10 * time.Hour)},
			{ID: "old-running", WorkspaceID: "first", Status: domaintasks.StatusRunning, UpdatedAt: base.Add(-5 * time.Hour)},
			{ID: "third", WorkspaceID: "first", CreatedAt: base.Add(-time.Minute)},
		},
		"second": {
			{ID: "new-running", WorkspaceID: "second", Title: "正在执行", Status: domaintasks.StatusRunning, UpdatedAt: base.Add(-time.Hour)},
			{ID: "newest", WorkspaceID: "second", Title: "最新\n 会话", UpdatedAt: base},
			{ID: "fourth", WorkspaceID: "second", UpdatedAt: base.Add(-2 * time.Minute)},
			{ID: "old", WorkspaceID: "second", UpdatedAt: base.Add(-6 * time.Hour)},
		},
		"hidden": {{ID: "hidden-task", WorkspaceID: "hidden", UpdatedAt: base.Add(time.Hour), Status: domaintasks.StatusRunning}},
	}
	items := a.trayMenu()
	if ids := trayTaskIDs(items); !slices.Equal(ids, []string{"new-running", "old-running", "newest", "third", "fourth"}) {
		t.Fatalf("recent sessions = %v", ids)
	}
	for _, item := range items {
		if item.ID == "task/new-running" && !strings.Contains(item.Title, "第二个项目 · 正在执行") {
			t.Fatal("session label omitted project/title")
		}
		if item.ID == "task/newest" && strings.Contains(item.Title, "\n") {
			t.Fatal("session title can span native menu lines")
		}
	}
	if a.tasks["second"][0].ID != "new-running" || a.tasks["first"][0].ID != "pin" {
		t.Fatal("tray changed stored task order")
	}
	a.tasks["first"][1].Status = domaintasks.StatusCompleted
	if ids := trayTaskIDs(a.trayMenu()); !slices.Equal(ids, []string{"new-running", "newest", "third", "fourth", "old-running"}) {
		t.Fatalf("completion did not reorder sessions: %v", ids)
	}
}

func TestTrayMenuRefreshesOnChangesAndRetriesFailures(t *testing.T) {
	locale.Apply(locale.Chinese())
	defer locale.Apply(locale.Chinese())
	a := conversationFixture(t)
	status := &testTrayIcon{}
	a.tray.icon = status
	a.refreshTrayMenu()
	a.refreshTrayMenu()
	if len(status.menus) != 1 {
		t.Fatal("unchanged menu was rewritten")
	}
	a.tasks["modu"][0].Title = "修改后的会话"
	a.refreshTrayMenu()
	locale.Apply(locale.English())
	a.refreshTrayMenu()
	if len(status.menus) != 3 || status.menus[2][0].Title != "Show OneCatch" {
		t.Fatal("title or language change did not reach tray")
	}
	previous := a.tray.model
	status.err = errors.New("native menu unavailable")
	a.tasks["modu"] = nil
	a.refreshTrayMenu()
	if a.tray.model != previous || a.errorText == "" {
		t.Fatal("failed update committed a stale menu model")
	}
	status.err = nil
	a.refreshTrayMenu()
	if a.tray.model == previous || len(trayTaskIDs(status.menus[len(status.menus)-1])) != 0 {
		t.Fatal("failed menu update was not retried")
	}
}

func TestTrayWindowCloseKeepsSessionUntilExplicitCleanup(t *testing.T) {
	a := conversationFixture(t)
	status := &testTrayIcon{}
	a.tray.icon = status
	a.prompt = "未发送的草稿"
	transcript, taskID := a.transcript, a.taskID
	a.mainWindowClosed()
	if a.closed || a.ctx.Err() != nil || status.closes != 0 || a.transcript != transcript || a.taskID != taskID || a.prompt != "未发送的草稿" {
		t.Fatal("closing resident window discarded session or stopped backend")
	}
	a.close()
	a.close()
	if !a.closed || a.ctx.Err() == nil || status.closes != 1 {
		t.Fatal("explicit cleanup did not run exactly once")
	}
}

func TestTrayUnavailableKeepsCloseToExitBehavior(t *testing.T) {
	a := conversationFixture(t)
	a.mainWindowClosed()
	if !a.closed || a.ctx.Err() == nil {
		t.Fatal("no-tray window close left an unreachable background process")
	}
}

func TestTrayActionsMarshalAndIgnoreCallbacksAfterShutdown(t *testing.T) {
	a := conversationFixture(t)
	var pending []func()
	a.postUpdate = func(fn func()) { pending = append(pending, fn) }
	called := 0
	callback := a.trayAction(func() { called++ })
	callback()
	if called != 0 || len(pending) != 1 {
		t.Fatal("native callback mutated UI state directly")
	}
	pending[0]()
	callback()
	a.close()
	pending[1]()
	if called != 1 {
		t.Fatal("queued tray callback ran after shutdown")
	}
}

func TestTrayTemplateIconKeepsMarkAndRemovesDarkTile(t *testing.T) {
	icon, err := png.Decode(bytes.NewReader(appIcon))
	if err != nil {
		t.Fatal(err)
	}
	mask := trayTemplateIcon(icon)
	clear, ink := 0, 0
	for y := mask.Bounds().Min.Y; y < mask.Bounds().Max.Y; y++ {
		for x := mask.Bounds().Min.X; x < mask.Bounds().Max.X; x++ {
			pixel := color.NRGBAModel.Convert(mask.At(x, y)).(color.NRGBA)
			if pixel.A == 0 {
				clear++
			} else if pixel.A > 150 {
				ink++
			}
			if pixel.R != 0 || pixel.G != 0 || pixel.B != 0 {
				t.Fatal("template icon retained colored pixels")
			}
		}
	}
	if clear == 0 || ink == 0 {
		t.Fatalf("template has no transparent tile or visible mark: %d/%d", clear, ink)
	}
}

func TestTrayMenuKeepsVisibilityActionsTogetherAndBoundsSessionLabels(t *testing.T) {
	locale.Apply(locale.Chinese())
	a := conversationFixture(t)
	a.workspaces[0].Name = strings.Repeat("项目名", 20)
	a.tasks["modu"][0].Title = strings.Repeat("中文标题 English ", 30)
	items := a.trayMenu()
	if items[0].ID != "show" || items[1].ID != "hide" {
		t.Fatal("show/hide actions are separated")
	}
	for _, item := range items {
		if item.ID != "task/hello" {
			continue
		}
		parts := strings.Split(item.Title, " · ")
		if len(parts) != 2 || !strings.HasSuffix(parts[0], "…") || !strings.HasSuffix(parts[1], "…") || runewidth.StringWidth(parts[0]) > 10 || runewidth.StringWidth(parts[1]) > 28 {
			t.Fatalf("project/title exceed independent width budgets: %q", item.Title)
		}
		return
	}
	t.Fatal("long session was omitted")
}
