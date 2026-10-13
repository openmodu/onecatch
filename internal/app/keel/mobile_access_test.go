package keeldesktop

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"net"
	"os"
	"path/filepath"
	"strconv"
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
	"github.com/openmodu/onecatch/internal/service/worker"
	"github.com/openmodu/onecatch/internal/service/worker/hosting"
)

type mobileFixtureService struct {
	status hosting.Status
	err    error
	called string
}

func (s *mobileFixtureService) HostedWorker(context.Context) (hosting.Status, error) {
	s.called += "/load"
	return s.status, nil
}
func (s *mobileFixtureService) StartHostedWorker(context.Context) (hosting.Status, error) {
	s.called += "/start"
	return s.status, s.err
}
func (s *mobileFixtureService) StopHostedWorker(context.Context) (hosting.Status, error) {
	s.called += "/stop"
	return s.status, s.err
}
func (s *mobileFixtureService) PairHostedWorker(context.Context) (hosting.Status, error) {
	s.called += "/pair"
	return s.status, s.err
}

func mobileTestSettings(t *testing.T) *settingsPage {
	t.Helper()
	p := workerTestSettings(dagTestApp(t))
	p.section = "mobile"
	p.mobile.loaded, p.mobile.now = true, time.Now()
	t.Cleanup(p.closeWorkers)
	return p
}

func TestMobilePairingExpiry(t *testing.T) {
	now := time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		remaining time.Duration
		want      string
	}{{10 * time.Minute, "10:00"}, {61 * time.Second, "1:01"}, {5900 * time.Millisecond, "0:05"}, {500 * time.Millisecond, "0:00"}, {0, ""}, {-time.Second, ""}} {
		pair := &hosting.Pairing{Code: "ABCD-2345", ExpiresAt: now.Add(test.remaining)}
		if got := mobilePairingCountdown(pair, now); got != test.want {
			t.Fatalf("remaining %v: %q, want %q", test.remaining, got, test.want)
		}
	}
	if mobilePairingCountdown(nil, now) != "" {
		t.Fatal("missing pairing exposed a code")
	}
}

func TestMobileCopyRejectsExpiredOrReplacedCode(t *testing.T) {
	p := mobileTestSettings(t)
	p.mobile.status = mobilePreviewStatus(time.Now())
	p.copyMobileValue("pairing", "old-code")
	if len(p.mobile.copied) != 0 {
		t.Fatal("stale button copied a replaced code")
	}
	p.mobile.status.Pairing.ExpiresAt = time.Now().Add(-time.Second)
	p.copyMobileValue("pairing", "ABCD2345")
	if len(p.mobile.copied) != 0 {
		t.Fatal("expired code was copied between countdown frames")
	}
	p.mobile.status.Pairing = nil
	p.copyMobileValue("pairing", "ABCD2345")
	if len(p.mobile.copied) != 0 {
		t.Fatal("stopped host copied a code")
	}
}

func TestMobileAccessRecoversActualListenerStateAfterWriteFailure(t *testing.T) {
	for _, action := range []string{"start", "stop", "pair"} {
		p := mobileTestSettings(t)
		p.mobile.status.Running = true
		version, ok := p.beginMobileAccess(action)
		if !ok {
			t.Fatal("request rejected")
		}
		if _, ok := p.beginMobileAccess("pair"); ok {
			t.Fatal("overlapping request accepted")
		}
		api := &mobileFixtureService{status: hosting.Status{Running: action != "stop", Port: 9232}, err: errors.New("write hosting state: permission denied")}
		status, err := performMobileAccess(context.Background(), api, action)
		p.applyMobileAccess(version, status, err)
		if api.called != "/"+action+"/load" || p.mobile.status.Running != (action != "stop") || p.mobile.busy != "" || p.mobile.err == "" {
			t.Fatalf("%s did not recover actual listener state", action)
		}
	}
}

func TestMobileAccessRefreshAndCloseRetainCorrectState(t *testing.T) {
	p := mobileTestSettings(t)
	p.mobile.status = hosting.Status{Running: true, Port: 9232}
	version, _ := p.beginMobileAccess("load")
	p.applyMobileAccess(version, hosting.Status{}, errors.New("read failed"))
	if !p.mobile.status.Running || p.mobile.err == "" {
		t.Fatal("failed refresh claimed access was off")
	}
	version, _ = p.beginMobileAccess("pair")
	p.applyMobileAccess(version-1, hosting.Status{}, nil)
	if p.mobile.busy != "pair" || !p.mobile.status.Running {
		t.Fatal("stale response overwrote current request")
	}
	p.closeWorkers()
	p.applyMobileAccess(version, hosting.Status{Running: true, Pairing: &hosting.Pairing{Code: "dummy"}}, nil)
	if p.mobile.status.Pairing != nil {
		t.Fatal("closed settings retained late pairing data")
	}
	if _, ok := p.beginMobileAccess("start"); ok {
		t.Fatal("closed page allowed another request")
	}
}

// Exercise the real desktop service in a temporary store, with an ephemeral
// test port. The user's persisted host state and default 9232 listener are
// untouched. This validates TLS pairing; it does not claim a physical phone
// or local-network-permission dialog was exercised.
func TestMobileHostedServicePairRotateRestoreAndShutdown(t *testing.T) {
	root := t.TempDir()
	b, err := openBackendWithEngine(context.Background(), root, testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()
	ctx := context.Background()
	if err := b.restoreHostedWorker(ctx); err != nil {
		t.Fatal(err)
	}
	if s, err := b.HostedWorker(ctx); err != nil || s.Running || s.Pairing != nil {
		t.Fatal("fresh client did not default to closed")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	hostRoot := filepath.Join(root, "host-worker")
	if err := os.MkdirAll(hostRoot, 0700); err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(map[string]any{"enabled": true, "port": port})
	if err := os.WriteFile(filepath.Join(hostRoot, "hosting.json"), state, 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.restoreHostedWorker(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := b.StartHostedWorker(ctx)
	if err != nil || !first.Running || first.Port != port || first.Fingerprint == "" {
		t.Fatal("restored listener unavailable")
	}
	paired, err := b.PairHostedWorker(ctx)
	if err != nil || paired.Pairing == nil {
		t.Fatal("pairing code not issued")
	}
	oldCode := paired.Pairing.Code
	paired, err = b.PairHostedWorker(ctx)
	if err != nil || paired.Pairing == nil || paired.Pairing.Code == oldCode {
		t.Fatal("pairing code not rotated")
	}
	baseURL := "https://127.0.0.1:" + strconv.Itoa(port)
	client := worker.NewClient()
	if _, err := client.Pair(ctx, baseURL, oldCode); err == nil {
		t.Fatal("rotated-away code still works")
	}
	result, err := client.Pair(ctx, baseURL, paired.Pairing.Code)
	if err != nil || result.Token == "" || result.ServerCertificateSHA256 != first.Fingerprint {
		t.Fatal("TLS pairing failed to preserve certificate pin")
	}
	if _, err := client.Pair(ctx, baseURL, paired.Pairing.Code); err == nil {
		t.Fatal("pairing code accepted twice")
	}
	if status, err := b.StopHostedWorker(ctx); err != nil || status.Running || status.Pairing != nil || len(status.Addresses) != 0 {
		t.Fatal("stop continued advertising access")
	}
	if err := b.restoreHostedWorker(ctx); err != nil {
		t.Fatal(err)
	}
	if s, _ := b.HostedWorker(ctx); s.Running {
		t.Fatal("explicitly stopped host was restored")
	}
	if s, err := b.StartHostedWorker(ctx); err != nil || s.Fingerprint != first.Fingerprint {
		t.Fatal("restart invalidated the existing phone pin")
	}
	if err := b.close(); err != nil {
		t.Fatal(err)
	}
	for _, action := range []func(context.Context) (hosting.Status, error){b.HostedWorker, b.StartHostedWorker, b.PairHostedWorker, b.StopHostedWorker} {
		if _, err := action(ctx); err == nil {
			t.Fatal("closed client permitted a late host action")
		}
	}
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), time.Second); err == nil {
		conn.Close()
		t.Fatal("client shutdown left the listener open")
	}
	// The application's shutdown preserves the prior opt-in for next launch.
	data, err := os.ReadFile(filepath.Join(hostRoot, "hosting.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct{ Enabled bool }
	if json.Unmarshal(data, &saved) != nil || !saved.Enabled {
		t.Fatal("shutdown erased the user's hosting preference")
	}
}

func TestMobileHostedRequestCancelledBeforeLockCannotStart(t *testing.T) {
	b, err := openBackendWithEngine(context.Background(), t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()
	ctx, cancel := context.WithCancel(context.Background())
	b.hostMu.Lock()
	done := make(chan error, 1)
	go func() { _, err := b.StartHostedWorker(ctx); done <- err }()
	cancel()
	b.hostMu.Unlock()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled start was not rejected")
	}
	if s, err := b.HostedWorker(context.Background()); err != nil || s.Running {
		t.Fatal("cancelled request opened a listener")
	}
}

func TestMobileNativeSwitchAndExpiryAtMinimumSize(t *testing.T) {
	p := mobileTestSettings(t)
	// Inspect the source-equivalent immediate-action switch through Gio. Keep
	// the callback local here; real service requests are covered above.
	root := el.Root(p)
	var router input.Router
	var ops op.Ops
	size := image.Pt(860, 600)
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
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
	frame()
	requested := false
	p.mobile.toggle.onChange = func(on bool) { requested = on }
	b, ok := find(router.AppendSemantics(nil), "允许手机连接")
	if !ok || !b.In(image.Rectangle{Max: size}) {
		t.Fatal("mobile switch missing or clipped")
	}
	pos := f32.Pt(float32(b.Min.X+b.Dx()/2), float32(b.Min.Y+b.Dy()/2))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
	frame()
	if !requested || p.mobile.toggle.Value() {
		t.Fatal("switch failed to request action, or changed before success")
	}
	requested = false
	router.Queue(key.Event{Name: "Space", State: key.Press}, key.Event{Name: "Space", State: key.Release})
	frame()
	if !requested {
		t.Fatal("keyboard switch action unavailable")
	}
	p.mobile.status = mobilePreviewStatus(time.Now())
	frame()
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(700, 500), Scroll: f32.Pt(0, 300)})
	frame()
	frame()
	if _, ok := find(router.AppendSemantics(nil), "复制配对码"); !ok {
		t.Fatal("live pairing copy missing")
	}
	p.mobile.status.Pairing.ExpiresAt = time.Now().Add(-time.Second)
	p.mobile.now = time.Now()
	frame()
	if _, ok := find(router.AppendSemantics(nil), "复制配对码"); ok {
		t.Fatal("expired pairing remained actionable")
	}
	if _, ok := find(router.AppendSemantics(nil), "生成配对码"); !ok {
		t.Fatal("expired pairing cannot be replaced")
	}
}

func mobilePreviewStatus(now time.Time) hosting.Status {
	return hosting.Status{Running: true, Port: 9232, Addresses: []string{"https://192.168.1.20:9232"}, Fingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Pairing: &hosting.Pairing{Code: "ABCD-2345", ExpiresAt: now.Add(10 * time.Minute)}}
}

func TestRenderMobileAccessPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	p := mobileTestSettings(t)
	applyPalette(false)
	defer applyPalette(false)
	root := el.Root(p)
	shot := func(name string, w, h int) {
		t.Helper()
		if err := window.Screenshot(root, w, h, filepath.Join(dir, name+".png")); err != nil {
			t.Fatal(err)
		}
	}
	shot("mobile-off", 960, 800)
	p.mobile.status = mobilePreviewStatus(p.mobile.now)
	shot("mobile-pair", 960, 800)
	shot("mobile-pair-narrow", 860, 600)
	p.mobile.status.Pairing.ExpiresAt = p.mobile.now.Add(-time.Second)
	shot("mobile-expired", 960, 800)
	p.mobile.err = "write hosting state: permission denied"
	shot("mobile-error", 960, 800)
	p.mobile.err = ""
	applyPalette(true)
	// A fresh root captures the settled dark theme. Reusing the light root
	// intentionally starts Keel's 120 ms background transition on this frame.
	root = el.Root(p)
	shot("mobile-dark", 960, 800)
}
