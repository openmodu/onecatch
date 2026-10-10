package keeldesktop

import (
	"image"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/openmodu/onecatch/internal/service/desktop/runstream"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

const streamedTableAnswer = "明天（2026-10-11，周日）两地天气对比：\n\n| 城市 | 天气 | 温度 | 降水概率 |\n|------|------|------|----------|\n| **上海** | 阴 | 18~26°C | 4% |\n| **北京** | 阴 | 16~26°C | 0% |\n\n**总结：**\n- 明天上海和北京都是**阴天**，基本不下雨，比今天上海的毛毛雨（90%）好很多\n- 气温接近，白天 26°C 左右，早晚偏凉（16~19°C），建议穿长袖加件外套\n- 北京无降水，上海降水概率也很低（4%），出行都不受影响"

// Streaming an answer with a table: the view follows to the end, and the
// finished message is as tall as its content. The table used to be measured
// at its natural width (one character per line), leaving a large blank gap,
// and following stopped once the growing row outran last frame's measure.
func TestConversationStreamsTableAnswer(t *testing.T) {
	a := conversationFixture(t)
	a.detail.Active = true
	var queued []func()
	a.postUpdate = func(fn func()) { queued = append(queued, fn) }
	root := el.Root(a)
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1288, 1000)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	frame()
	latestShown := func() bool {
		_, ok := findSkillControl(router.AppendSemantics(nil), "回到最新")
		return ok
	}
	rev := uint64(0)
	send := func(phase agentrun.StreamPhase, text string) {
		rev++
		f := runstream.Frame{RunID: a.runID, StepRunID: "s3", Seq: 70}
		f.Kind, f.StreamID, f.Phase, f.Revision, f.Text, f.At = agentrun.KindMessage, "m", phase, rev, text, time.Date(2026, 10, 10, 10, 43, 0, 0, time.Local)
		a.receiveFrame(f)
		for _, fn := range queued {
			fn()
		}
		queued = nil
		frame()
	}
	send(agentrun.StreamStart, "")
	for i := 0; i < len(streamedTableAnswer); {
		n := 3
		for i+n < len(streamedTableAnswer) && !utf8.RuneStart(streamedTableAnswer[i+n]) {
			n++
		}
		n = min(n, len(streamedTableAnswer)-i)
		send(agentrun.StreamDelta, streamedTableAnswer[i:i+n])
		i += n
		if latestShown() {
			t.Fatalf("stopped following after %d of %d bytes", i, len(streamedTableAnswer))
		}
	}
	send(agentrun.StreamEnd, streamedTableAnswer)
	a.detail.Active = false
	a.syncMessages()
	for range 4 {
		frame()
	}
	if latestShown() {
		t.Fatal("the finished answer is not followed to the end")
	}
	// The message's actions sit right below it; its last line must be close.
	var last, actions image.Rectangle
	var walk func(n input.SemanticNode)
	walk = func(n input.SemanticNode) {
		if strings.HasPrefix(n.Desc.Label, "北京无降水") {
			last = n.Desc.Bounds
		}
		if n.Desc.Label == "复制消息" && n.Desc.Bounds.Min.Y > actions.Min.Y {
			actions = n.Desc.Bounds
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, n := range router.AppendSemantics(nil) {
		walk(n)
	}
	if last.Empty() || actions.Empty() {
		t.Fatal("missing the answer's last line or its actions")
	}
	if gap := actions.Min.Y - last.Max.Y; gap > 40 {
		t.Fatalf("%dpx of blank space between the answer and its actions", gap)
	}
}
