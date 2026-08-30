package desktop

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
)

func TestRecentTrayConversationsUsesActivityOrderAndVisibleWorkspaces(t *testing.T) {
	base := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	tasks := []domaintasks.Task{
		{ID: "pinned-old", WorkspaceID: "visible", Title: "旧置顶会话", Pinned: true, UpdatedAt: base},
		{ID: "latest", WorkspaceID: "visible", Title: "最新会话", UpdatedAt: base.Add(4 * time.Minute)},
		{ID: "hidden", WorkspaceID: "removed", Title: "已移除项目", UpdatedAt: base.Add(5 * time.Minute)},
		{ID: "second", WorkspaceID: "visible", Title: "第二条", UpdatedAt: base.Add(3 * time.Minute)},
		{ID: "third", WorkspaceID: "visible", Title: "第三条", UpdatedAt: base.Add(2 * time.Minute)},
		{ID: "fourth", WorkspaceID: "visible", Title: "第四条", UpdatedAt: base.Add(time.Minute)},
	}
	workspaces := []domainworkspaces.Workspace{{ID: "visible", Name: "OneCatch"}}

	got := recentTrayConversations(tasks, workspaces, 3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for index, want := range []string{"latest", "second", "third"} {
		if got[index].TaskID != want {
			t.Fatalf("item %d = %q, want %q", index, got[index].TaskID, want)
		}
	}
	if got[0].Label != "最新会话" {
		t.Fatalf("label = %q", got[0].Label)
	}
}

func TestTrayConversationLabelUsesCompactDisplayWidth(t *testing.T) {
	got := trayConversationLabel(strings.Repeat("会话", 20))
	if got != strings.Repeat("会话", 7)+"…" {
		t.Fatalf("CJK label = %q", got)
	}
	got = trayConversationLabel(strings.Repeat("a", 40))
	if got != strings.Repeat("a", 28)+"…" {
		t.Fatalf("ASCII label = %q", got)
	}
}

func TestTrayLabelsFollowLanguage(t *testing.T) {
	if got := labelsForLanguage("en-US"); got.new != "New Conversation" || got.quit != "Quit OneCatch" {
		t.Fatalf("English labels = %#v", got)
	}
	if got := labelsForLanguage("zh-CN"); got.new != "新建会话" || got.quit != "退出 OneCatch" {
		t.Fatalf("Chinese labels = %#v", got)
	}
}

func TestMacTrayTemplateIconDropsDarkTileAndKeepsBrightMark(t *testing.T) {
	input := image.NewNRGBA(image.Rect(0, 0, 12, 12))
	for y := range 12 {
		for x := range 12 {
			input.SetNRGBA(x, y, color.NRGBA{R: 12, G: 24, B: 24, A: 255})
		}
	}
	for index := 3; index < 9; index++ {
		input.SetNRGBA(index, 6, color.NRGBA{R: 245, G: 250, B: 248, A: 255})
		input.SetNRGBA(6, index, color.NRGBA{R: 24, G: 205, B: 220, A: 255})
	}
	var source bytes.Buffer
	if err := png.Encode(&source, input); err != nil {
		t.Fatal(err)
	}
	encoded, err := macTrayTemplateIcon(source.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	result, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	var transparent, opaque int
	for y := result.Bounds().Min.Y; y < result.Bounds().Max.Y; y++ {
		for x := result.Bounds().Min.X; x < result.Bounds().Max.X; x++ {
			_, _, _, alpha := result.At(x, y).RGBA()
			if alpha == 0 {
				transparent++
			} else if alpha == 0xffff {
				opaque++
			}
		}
	}
	if transparent == 0 || opaque == 0 {
		t.Fatalf("transparent = %d, opaque = %d", transparent, opaque)
	}
}
