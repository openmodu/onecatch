package keeldesktop

import (
	"image"
	"testing"
	"time"

	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/gio/layout"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/gio/unit"
	"github.com/dyike/keel/ui/el"
	desktop "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestQuestionCardRequiresAnswersAndPreventsDuplicateClicks(t *testing.T) {
	a := &app{detail: &desktop.RunDetail{Active: true}, answers: map[string]string{}, pendingResponses: map[string]bool{}}
	e := &entry{step: "step", question: &agentrun.UserInputRequest{ID: "input", Async: true, Questions: []agentrun.UserInputQuestion{{ID: "scope", Question: "Which scope?", Options: []agentrun.UserInputOption{{Label: "Small"}, {Label: "Large"}}}}}}
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return a.requestMessage(cx, e) }))
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(680, 420)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	var find func([]input.SemanticNode, string) (image.Rectangle, bool)
	find = func(nodes []input.SemanticNode, label string) (image.Rectangle, bool) {
		for _, n := range nodes {
			if n.Desc.Label == label && !n.Desc.Disabled && n.Desc.Gestures&input.ClickGesture != 0 {
				return n.Desc.Bounds, true
			}
			if b, ok := find(n.Children, label); ok {
				return b, true
			}
		}
		return image.Rectangle{}, false
	}
	frame()
	if _, ok := find(router.AppendSemantics(nil), "提交回答"); ok {
		t.Fatal("empty question was submittable")
	}
	box, ok := find(router.AppendSemantics(nil), "Small")
	if !ok {
		t.Fatal("suggested choice missing")
	}
	pos := f32.Pt(float32(box.Min.X+box.Dx()/2), float32(box.Min.Y+box.Dy()/2))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
	frame()
	frame()
	if a.answers[requestKey("step", "input")+"/scope"] != "Small" {
		t.Fatal("choice click did not fill answer")
	}
	if _, ok := find(router.AppendSemantics(nil), "提交回答"); !ok {
		t.Fatal("complete answer cannot be submitted")
	}
	a.pendingResponses["input"] = true
	frame()
	for _, label := range []string{"提交回答", "跳过", "Large"} {
		if _, ok := find(router.AppendSemantics(nil), label); ok {
			t.Fatalf("%s remained enabled during submission", label)
		}
	}
	delete(a.pendingResponses, "input")
	a.detail.Active = false
	frame()
	if _, ok := find(router.AppendSemantics(nil), "提交回答"); ok {
		t.Fatal("ended turn still accepts answer")
	}
}
