package keeldesktop

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/openmodu/onecatch/internal/service/desktop/runstream"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

// benchAnswer is a typical agent reply: paragraphs, a list and code.
const benchAnswer = "今天（2026-10-10，周六）上海天气：\n\n- **天气**：毛毛雨\n- **温度**：19~25°C\n- **降水概率**：90%\n\n今天有小雨，气温 25°C 左右，比较凉爽，出门记得带伞，可以加件薄外套。\n\n```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\n\n（顺便说明：之前几次查询返回的是 9 月初的数据，现在数据源日期已与系统时间一致，本次结果应为准确的。）\n\n"

// conversationBench is a run with 40 finished rounds and a live answer.
func conversationBench(b *testing.B) (*app, func()) {
	a, frame, _ := conversationScrollBench(b, false)
	return a, frame
}

func conversationScrollBench(b *testing.B, longReply bool) (*app, func(), func(float32)) {
	t := &testing.T{}
	a := conversationFixture(t)
	b.Cleanup(a.close)
	at := time.Date(2026, 10, 10, 8, 0, 0, 0, time.Local)
	for i := range 40 {
		step := fmt.Sprintf("s%d", i)
		user := entry{key: "u" + step, user: true, text: fmt.Sprintf("第 %d 个问题：今天上海的天气怎么样", i), at: at.Add(time.Duration(i) * time.Minute)}
		reply := entry{key: "a" + step, step: step, kind: agentrun.KindMessage, text: benchAnswer, at: at.Add(time.Duration(i)*time.Minute + time.Second)}
		a.transcript.items[user.key], a.transcript.items[reply.key] = &user, &reply
	}
	if longReply {
		e := &entry{key: "long-answer", kind: agentrun.KindMessage, text: strings.Repeat(benchAnswer, 80), at: at.Add(time.Hour)}
		a.transcript.items[e.key] = e
	}
	a.detail.Active = true
	a.postUpdate = func(func()) {} // the benchmark applies queued frames itself
	a.syncMessages()
	root := el.Root(a)
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1288*2, 804*2)), Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}})
		router.Frame(&ops)
	}
	a.scroller.ScrollToEnd()
	for range 3 {
		frame()
	}
	scroll := func(dy float32) {
		router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(1600, 600), Scroll: f32.Pt(0, dy)})
		frame()
	}
	return a, frame, scroll
}

func BenchmarkConversationScroll(b *testing.B) {
	for _, longReply := range []bool{false, true} {
		name := "many-rows"
		if longReply {
			name = "long-reply"
		}
		b.Run(name, func(b *testing.B) {
			a, _, scroll := conversationScrollBench(b, longReply)
			a.scroller.SetFollow(false)
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				dy := float32(-96)
				if i/80%2 != 0 {
					dy = -dy
				}
				scroll(dy)
			}
		})
	}
}

var benchTokens = strings.Fields(strings.Repeat("这是 流式 输出 的 一段 文字 ，包含 **加粗** 和 `代码` 。\n\n", 40))

func BenchmarkConversationStreamToken(b *testing.B) {
	a, frame := conversationBench(b)
	b.ResetTimer()
	for i := range b.N {
		a.receiveFrame(benchFrame(i))
		a.applyLiveFrames()
		frame()
	}
}

func BenchmarkConversationTyping(b *testing.B) {
	a, frame := conversationBench(b)
	b.ResetTimer()
	for i := range b.N {
		a.prompt += "字"
		if i%200 == 0 {
			a.prompt = ""
		}
		frame()
	}
}

func BenchmarkConversationIdleFrame(b *testing.B) {
	_, frame := conversationBench(b)
	b.ResetTimer()
	for range b.N {
		frame()
	}
}

// benchFrame is the i-th delta of live answers of about 600 tokens (3 KB),
// a long but ordinary reply; each answer starts a new message.
func benchFrame(i int) runstream.Frame {
	const tokens = 600
	phase := agentrun.StreamDelta
	if i%tokens == 0 {
		phase = agentrun.StreamStart
	}
	id := fmt.Sprintf("live%d", i/tokens)
	f := runstream.Frame{RunID: "preview", StepRunID: id, Seq: int64(999 + i/tokens)}
	f.Kind, f.StreamID, f.Phase, f.Revision, f.Text, f.At = agentrun.KindMessage, id, phase, uint64(i+1), benchTokens[i%len(benchTokens)]+" ", time.Date(2026, 10, 10, 9, 0, 0, 0, time.Local).Add(time.Duration(i/tokens)*time.Minute)
	return f
}

// A provider often delivers a burst of small deltas between two frames.
func BenchmarkConversationStreamBurst(b *testing.B) {
	a, frame := conversationBench(b)
	n := 0
	b.ResetTimer()
	for range b.N {
		for range 30 {
			a.receiveFrame(benchFrame(n))
			n++
		}
		a.applyLiveFrames() // the one queued UI update
		frame()
	}
}
