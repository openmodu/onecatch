package keeldesktop

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/service/worker/hosting"
)

type mobileAccessService interface {
	HostedWorker(context.Context) (hosting.Status, error)
	StartHostedWorker(context.Context) (hosting.Status, error)
	StopHostedWorker(context.Context) (hosting.Status, error)
	PairHostedWorker(context.Context) (hosting.Status, error)
}

type mobileAccessState struct {
	api       mobileAccessService
	status    hosting.Status
	loaded    bool
	busy, err string
	version   uint64
	toggle    *filledToggle
	now       time.Time
	copied    map[string]time.Time
}

// Pairing expiry matches the original floor-to-seconds display. An expired
// code is removed even when the settings page hasn't queried the host again.
func mobilePairingCountdown(pair *hosting.Pairing, now time.Time) string {
	if pair == nil || !pair.ExpiresAt.After(now) {
		return ""
	}
	seconds := int(pair.ExpiresAt.Sub(now) / time.Second)
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func (p *settingsPage) mobileService() mobileAccessService {
	if p.mobile.api != nil {
		return p.mobile.api
	}
	return p.app.backend
}

func (p *settingsPage) beginMobileAccess(action string) (uint64, bool) {
	if p.workers.closed || p.app.closed || p.mobile.busy != "" {
		return 0, false
	}
	if action != "load" && !p.mobile.loaded {
		return 0, false
	}
	p.mobile.version++
	p.mobile.busy, p.mobile.err = action, ""
	return p.mobile.version, true
}

// A state-file write may fail after start/stop succeeded. Query the actual
// listener on errors so the switch cannot lie about the resulting access.
func performMobileAccess(ctx context.Context, api mobileAccessService, action string) (hosting.Status, error) {
	var status hosting.Status
	var err error
	switch action {
	case "load":
		return api.HostedWorker(ctx)
	case "start":
		status, err = api.StartHostedWorker(ctx)
	case "stop":
		status, err = api.StopHostedWorker(ctx)
	case "pair":
		status, err = api.PairHostedWorker(ctx)
	default:
		return status, fmt.Errorf("未知手机连接操作")
	}
	if err != nil && ctx.Err() == nil {
		if live, e := api.HostedWorker(ctx); e == nil {
			status = live
		}
	}
	return status, err
}

func (p *settingsPage) applyMobileAccess(version uint64, status hosting.Status, err error) {
	if p.workers.closed || p.app.closed || p.mobile.version != version {
		return
	}
	action := p.mobile.busy
	p.mobile.busy, p.mobile.err, p.mobile.now = "", safeWorkerError(err, ""), time.Now()
	if action == "load" && err != nil {
		return // A failed refresh must not invent a stopped listener.
	}
	p.mobile.loaded = true
	status.Addresses = append([]string(nil), status.Addresses...)
	if status.Pairing != nil {
		pair := *status.Pairing
		status.Pairing = &pair
	}
	p.mobile.status = status
	p.mobile.copied = nil
}

func (p *settingsPage) runMobileAccess(action string) {
	version, ok := p.beginMobileAccess(action)
	if !ok {
		return
	}
	api := p.mobileService()
	ctx, cancel := context.WithTimeout(p.workerContext(), 20*time.Second)
	go func() {
		defer cancel()
		status, err := performMobileAccess(ctx, api, action)
		core.Update(func() { p.applyMobileAccess(version, status, err) })
	}()
}

func (p *settingsPage) loadMobileAccess() {
	p.mobile.now = time.Now()
	p.runMobileAccess("load")
}

func (p *settingsPage) watchMobileAccess() {
	ctx := p.workerContext()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				core.Update(func() {
					if !p.workers.closed && !p.app.closed && p.section == "mobile" {
						p.mobile.now = now
					}
				})
			}
		}
	}()
}

func (p *settingsPage) copyMobileValue(key, value string) {
	if p.workers.closed || p.app.closed || value == "" {
		return
	}
	if key == "pairing" {
		pair := p.mobile.status.Pairing
		if mobilePairingCountdown(pair, time.Now()) == "" || strings.ReplaceAll(pair.Code, "-", "") != value {
			return
		}
	}
	el.WriteClipboard(value)
	if p.mobile.copied == nil {
		p.mobile.copied = map[string]time.Time{}
	}
	now := time.Now()
	p.mobile.now = now
	p.mobile.copied[key] = now.Add(1600 * time.Millisecond)
}

func mobileSectionHeading(title, description string) *el.DivEl {
	return el.Div().Gap(6).Px(2).Child(el.Text(title).TextSize(15).Bold().LineHeight(1.25), el.Text(description).TextSize(12).LineHeight(1.625).TextColor(theme.Muted))
}

func mobileKicker(text string) el.Element {
	return el.Text(text).TextSize(11).Bold().TextColor(theme.Muted)
}

func (p *settingsPage) mobileCopyButton(cx *el.Context, key, label, value string) el.Element {
	icon := "clipboard"
	if p.mobile.copied[key].After(p.mobile.now) {
		icon = "check"
	}
	control := button("", func() { p.copyMobileValue(key, value) }, kit.ButtonSecondary).Name(label).Size(28).Compact(true).Content(el.ViewFunc(func(cx *el.Context) el.Element {
		return lucide(cx, icon, 14, theme.Muted)
	})).Render(cx)
	if row, ok := control.(*el.DivEl); ok {
		row.Size(el.Dp(28)).P(0).NoShrink().Rounded(8)
	}
	return control
}

func (p *settingsPage) mobileAccess(cx *el.Context) el.Element {
	s := &p.mobile
	if s.now.IsZero() {
		s.now = time.Now()
	}
	port := s.status.Port
	if port == 0 {
		port = hosting.DefaultPort
	}
	if s.toggle == nil {
		s.toggle = newFilledToggle("允许手机连接", false, false)
		s.toggle.hideLabel = true
		s.toggle.onChange = func(on bool) {
			action := "stop"
			if on {
				action = "start"
			}
			p.runMobileAccess(action)
		}
	}
	s.toggle.SetValue(s.status.Running)
	s.toggle.SetDisabled(!s.loaded || s.busy != "")
	heading := mobileSectionHeading("作为远程 Worker", "开启后，同一局域网里的 OneCatch 手机端可以连上这台电脑，使用这里配置好的 Harness 跑任务。连接走 HTTPS，手机在配对时会记住这台电脑的证书指纹。")
	header := el.Div().Row().Items(el.Start).Gap(24).Child(heading.Grow().W(el.Dp(0)))
	if s.status.Running {
		header.Child(el.Div().Row().Items(el.Center).Gap(6).Px(8).Py(3).Child(lucide(cx, "smartphone", 12, theme.Text), el.Text("已开启").TextSize(12)))
	}
	root := el.Div().Gap(24)
	root.Child(el.Div().Gap(14).Child(header, workerSwitch(cx, "允许手机连接", fmt.Sprintf("在端口 %d 上监听局域网连接。默认关闭，关掉后已配对的手机也连不上。", port), s.toggle)))
	if s.err != "" {
		retry := button("重试", p.loadMobileAccess, kit.ButtonGhost).Size(24)
		retry.SetDisabled(s.busy != "")
		root.Child(el.Div().Gap(8).Child(el.Text(s.err).TextSize(12).TextColor(theme.Danger), retry.Render(cx)))
	}
	if !s.status.Running {
		return root
	}
	pairSection := el.Div().Gap(14).Child(mobileSectionHeading("配对手机", "让手机和电脑连接同一局域网，在手机「连接电脑」中选择发现的电脑，再输入下方配对码。未发现时可手动填地址。配对码 10 分钟内有效，且只能用一次。"))
	content := el.Div().P(16).Gap(20).Rounded(12).Bg(mixColor(theme.Surface, theme.Bg, .72)).Child(el.Text("手机打开「连接电脑」后会自动搜索；没找到时点「重新搜索」。请允许两端的本地网络权限。").TextSize(14).LineHeight(1.625).TextColor(theme.Muted))
	addresses := el.Div().Gap(8).Child(mobileKicker("Worker 地址"))
	if len(s.status.Addresses) == 0 {
		addresses.Child(el.Text("没有检测到局域网地址，请确认这台电脑已连接 Wi-Fi 或有线网络。").TextSize(12).TextColor(theme.Muted))
	} else {
		list := el.Div().Rounded(8).Bg(mixColor(theme.Bg, theme.Surface, .55))
		for i, address := range s.status.Addresses {
			if i > 0 {
				list.Child(el.Div().H(el.Dp(1)).Bg(seamColor))
			}
			list.Child(el.Div().Row().Items(el.Center).Gap(12).Px(12).Py(10).MinH(el.Dp(40)).Child(el.Text(address).Mono().TextSize(13).MaxLines(1).Grow().W(el.Dp(0)), p.mobileCopyButton(cx, address, "复制地址", address)))
		}
		addresses.Child(list)
	}
	content.Child(addresses)
	countdown := mobilePairingCountdown(s.status.Pairing, s.now)
	action := "生成配对码"
	if countdown != "" {
		action = "换一个配对码"
	}
	newCode := button(action, func() { p.runMobileAccess("pair") }, kit.ButtonSecondary).Name(action).Size(24).Compact(true).Content(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().Row().Items(el.Center).Gap(6).Child(lucide(cx, "refresh-cw", 12, theme.Text), el.Text(action).TextSize(12).Medium())
	}))
	newCode.SetDisabled(s.busy != "")
	codes := el.Div().Gap(8).Child(el.Div().Row().Items(el.Center).Justify(el.SpaceBetween).Child(mobileKicker("配对码"), newCode.Render(cx)))
	if countdown == "" {
		codes.Child(el.Text("还没有生成配对码。").TextSize(12).TextColor(theme.Muted))
	} else {
		code := strings.ReplaceAll(s.status.Pairing.Code, "-", "")
		glyphs := el.Div().Row().Gap(3.6).Grow().W(el.Dp(0))
		for _, ch := range code {
			glyphs.Child(el.Text(string(ch)).Mono().TextSize(20).Bold())
		}
		codes.Child(el.Div().Row().Items(el.Center).Gap(12).MinH(el.Dp(56)).Px(14).Py(12).Rounded(8).Bg(mixColor(theme.Subtle, theme.Bg, .3)).Child(glyphs, el.Text(countdown+" 后失效").TextSize(12).TextColor(theme.Muted), p.mobileCopyButton(cx, "pairing", "复制配对码", code)))
	}
	content.Child(codes)
	if s.status.Fingerprint != "" {
		fingerprint := s.status.Fingerprint
		if len(fingerprint) > 16 {
			fingerprint = fingerprint[:16]
		}
		content.Child(el.Div().Gap(12).Child(el.Div().H(el.Dp(1)).Bg(seamColor), el.Div().Row().Items(el.Start).Gap(8).Child(lucide(cx, "fingerprint", 14, theme.Muted), el.Text("证书指纹 "+strings.ToUpper(fingerprint)+"…").TextSize(12).TextColor(theme.Muted))))
	}
	root.Child(pairSection.Child(content))
	return root
}
