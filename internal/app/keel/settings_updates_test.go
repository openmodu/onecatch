package keeldesktop

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/locale"
	"github.com/openmodu/onecatch/internal/service/appupdate"
)

type testDesktopUpdater struct {
	status appupdate.Status
	calls  []string
	err    error
}

func (s *testDesktopUpdater) Check(context.Context) (appupdate.Status, error) {
	s.calls = append(s.calls, "check")
	s.status.State, s.status.AvailableVersion = "available", "0.5.2"
	return s.status, s.err
}
func (s *testDesktopUpdater) Download(context.Context) (appupdate.Status, error) {
	s.calls = append(s.calls, "download")
	s.status.State = "ready"
	return s.status, s.err
}
func (s *testDesktopUpdater) Apply(context.Context) error {
	s.calls = append(s.calls, "apply")
	return s.err
}
func (s *testDesktopUpdater) Close() {}

func TestUpdateControlsCheckDownloadAndInstall(t *testing.T) {
	locale.Apply(locale.Chinese())
	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			a := dagTestApp(t)
			service := &testDesktopUpdater{status: appupdate.Status{CurrentVersion: "0.5.1", State: "idle", AutomaticSupported: true, VerificationEnabled: true}}
			a.updates.service, a.updates.status = service, service.status
			pending := make(chan func(), 8)
			a.postUpdate = func(fn func()) { pending <- fn }
			p := workerTestSettings(a)
			p.section = "updates"
			router, frame, click := alignmentRouter(el.Root(p), 960, 800, scale)
			has := func(label string) bool { _, ok := findSkillControl(router.AppendSemantics(nil), label); return ok }
			if has("保存") || has("恢复默认值") || has("下载更新") || has("重启并安装") {
				t.Fatal("update page has unrelated or premature actions")
			}
			invoke := func(label, want string) {
				t.Helper()
				bounds, ok := findSkillControl(router.AppendSemantics(nil), label)
				if !ok {
					t.Fatalf("missing %s", label)
				}
				click(bounds)
				if !a.updates.busy {
					t.Fatal("operation did not start")
				}
				select {
				case fn := <-pending:
					fn()
				case <-time.After(5 * time.Second):
					t.Fatal("update operation timed out")
				}
				frame()
				frame()
				if a.updates.busy || service.calls[len(service.calls)-1] != want {
					t.Fatalf("operation failed: %+v", service.calls)
				}
			}
			invoke("检查更新", "check")
			if !has("下载更新") || has("重启并安装") {
				t.Fatal("available release cannot be downloaded")
			}
			invoke("下载更新", "download")
			if !has("重启并安装") || len(service.calls) != 2 {
				t.Fatal("ready release not offered, or installed automatically")
			}
			service.err = errors.New("installer launch failed")
			invoke("重启并安装", "apply")
			if a.updates.err != service.err.Error() || !has("重启并安装") {
				t.Fatal("install error is hidden or cannot be retried")
			}
		})
	}
}

func TestBackgroundUpdateNoticePreservesComposerFocus(t *testing.T) {
	a := dagTestApp(t)
	a.logo = kit.Image(image.NewRGBA(image.Rect(0, 0, 22, 22)), "OneCatch").Size(22, 22)
	var focused bool
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		view := el.Div().Row().WFull().HFull().Child(a.sidebar(cx), el.Div().Grow().W(el.Dp(0)).Child(a.composer(cx)))
		focused = cx.FocusWithin("prompt")
		return view
	}))
	router, frame, click := alignmentRouter(root, 800, 400, 1)
	a.updates.status = appupdate.Status{State: "available", AvailableVersion: "0.5.2"}
	frame()
	frame()
	if !focused {
		t.Fatal("background update stole composer focus")
	}
	if _, ok := findSkillControl(router.AppendSemantics(nil), "发现新版本 0.5.2"); !ok {
		t.Fatal("background release has no sidebar button")
	}
	notice, noticeOK := alignmentNode(router.AppendSemantics(nil), "发现新版本 0.5.2", false)
	menu, menuOK := alignmentNode(router.AppendSemantics(nil), "OneCatch 菜单", true)
	if !noticeOK || !menuOK || notice.Bounds.Max.Y > menu.Bounds.Min.Y || notice.Bounds.Max.X > menu.Bounds.Max.X || menu.Bounds.Min.Y < 300 {
		t.Fatalf("update button is not above the sidebar footer: notice=%v menu=%v", notice.Bounds, menu.Bounds)
	}
	close, ok := findSkillControl(router.AppendSemantics(nil), "暂不更新")
	if !ok {
		t.Fatal("notification cannot be dismissed")
	}
	click(close)
	frame()
	if _, ok := findSkillControl(router.AppendSemantics(nil), "发现新版本 0.5.2"); ok {
		t.Fatal("dismissed release shown again")
	}
	a.updates.status.AvailableVersion = "0.5.3"
	frame()
	frame()
	if _, ok := findSkillControl(router.AppendSemantics(nil), "发现新版本 0.5.3"); !ok {
		t.Fatal("dismissal hid a newer release")
	}
}

func TestRenderUpdateSettings(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	locale.Apply(locale.Chinese())
	for _, dark := range []bool{false, true} {
		applyPalette(dark)
		a := dagTestApp(t)
		a.updates.status = appupdate.Status{CurrentVersion: "0.5.1", State: "ready", AvailableVersion: "0.5.2", AutomaticSupported: true, VerificationEnabled: true, Notes: "修复空输入框光标和首次聚焦。\n补齐软件更新入口。"}
		p := workerTestSettings(a)
		p.section = "updates"
		if err := screenshotWithFocus(el.Root(p), 960, 800, filepath.Join(dir, fmt.Sprintf("updates-dark-%v.png", dark))); err != nil {
			t.Fatal(err)
		}
		main := alignmentFixture(t)
		main.updates.status = a.updates.status
		if err := screenshotWithFocus(el.Root(main), 1120, 760, filepath.Join(dir, fmt.Sprintf("sidebar-update-dark-%v.png", dark))); err != nil {
			t.Fatal(err)
		}
	}
	applyPalette(false)
}
