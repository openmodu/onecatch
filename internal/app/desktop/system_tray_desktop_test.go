//go:build windows || linux

package desktop

import (
	"strings"
	"testing"
	"time"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
)

func TestLatestActiveTrayTasksReturnsLatestThreeRegardlessOfPins(t *testing.T) {
	base := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	tasks := []domaintasks.Task{
		{ID: "completed-newer", Status: domaintasks.StatusCompleted, UpdatedAt: base.Add(5 * time.Minute)},
		{ID: "old-pinned", Pinned: true, Status: domaintasks.StatusPaused, UpdatedAt: base},
		{ID: "newest", Status: domaintasks.StatusRunning, UpdatedAt: base.Add(4 * time.Minute)},
		{ID: "third", Status: domaintasks.StatusReady, UpdatedAt: base.Add(2 * time.Minute)},
		{ID: "second", Status: domaintasks.StatusQueued, UpdatedAt: base.Add(3 * time.Minute)},
	}
	got := latestActiveTrayTasks(tasks, 3)
	if len(got) != 3 || got[0].ID != "newest" || got[1].ID != "second" || got[2].ID != "third" {
		t.Fatalf("latestActiveTrayTasks() = %+v", got)
	}
	if tasks[0].ID != "completed-newer" {
		t.Fatal("latestActiveTrayTasks mutated its input")
	}
}

func TestTraySessionLabelShowsActiveStatus(t *testing.T) {
	for _, test := range []struct {
		status domaintasks.Status
		want   string
	}{
		{status: domaintasks.StatusRunning, want: "修复登录  · 运行中"},
		{status: domaintasks.StatusPaused, want: "修复登录  · 已暂停"},
		{status: domaintasks.StatusQueued, want: "修复登录  · 排队中"},
		{status: domaintasks.StatusReady, want: "修复登录  · 就绪"},
		{status: domaintasks.StatusCompleted, want: "修复登录"},
	} {
		task := domaintasks.Task{Title: "修复登录", Status: test.status}
		if got := traySessionLabel(task); got != test.want {
			t.Fatalf("traySessionLabel(%q) = %q, want %q", test.status, got, test.want)
		}
	}
}

func TestTraySessionLabelTruncatesByDisplayWidth(t *testing.T) {
	task := domaintasks.Task{Title: strings.Repeat("会", 20), Status: domaintasks.StatusRunning}
	got := traySessionLabel(task)
	want := strings.Repeat("会", 14) + "…  · 运行中"
	if got != want {
		t.Fatalf("traySessionLabel() = %q, want %q", got, want)
	}
}
