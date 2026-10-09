package keeldesktop

import (
	"math"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/theme"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

type dagEdge struct {
	Source, Target string
	From, To       f32.Point
}

func (e dagEdge) midpoint() f32.Point { return e.From.Add(e.To).Mul(.5) }
func (e dagEdge) point(t float32) f32.Point {
	u := 1 - t
	return e.From.Mul(u * u * u).Add(e.From.Add(f32.Pt(70, 0)).Mul(3 * u * u * t)).Add(e.To.Add(f32.Pt(-70, 0)).Mul(3 * u * t * t)).Add(e.To.Mul(t * t * t))
}
func (e dagEdge) hit(p f32.Point) bool {
	previous := e.From
	for i := 1; i <= 48; i++ {
		next := e.point(float32(i) / 48)
		v := next.Sub(previous)
		length := v.X*v.X + v.Y*v.Y
		t := float32(0)
		if length > 0 {
			d := p.Sub(previous)
			t = max(float32(0), min(float32(1), (d.X*v.X+d.Y*v.Y)/length))
		}
		d := p.Sub(previous.Add(v.Mul(t)))
		if d.X*d.X+d.Y*d.Y <= 36 {
			return true
		}
		previous = next
	}
	return false
}
func dagEdges(d domainworkflows.Definition) []dagEdge {
	var edges []dagEdge
	for _, s := range d.Steps {
		for _, source := range s.DependsOn {
			from, to := dagPosition(d, source), dagPosition(d, s.ID)
			edges = append(edges, dagEdge{source, s.ID, f32.Pt(float32(from.X)+204, float32(from.Y)+47), f32.Pt(float32(to.X), float32(to.Y)+47)})
		}
	}
	return edges
}
func dagExtent(d domainworkflows.Definition) (float32, float32) {
	w, h := float32(dagCanvasWidth), float32(dagCanvasHeight)
	for _, s := range d.Steps {
		p := dagPosition(d, s.ID)
		w = max(w, float32(p.X)+224)
		h = max(h, float32(p.Y)+106)
	}
	return w, h
}

type dagEdgeInput struct{ Pressed, Hover *dagEdge }
type dagDrawing struct{ App *app }

// The drawing is native Gio. Hit testing follows the actual Bézier path rather
// than its bounding rectangle, so empty canvas clicks do not delete an edge.
func (v dagDrawing) Layout(gtx core.C) core.D {
	a := v.App
	state := &a.ui.DAG.Edges
	scale := gtx.Metric.PxPerDp
	if scale <= 0 {
		scale = 1
	}
	edges := dagEdges(a.ui.WorkflowDraft)
	if gtx.Enabled() {
		for {
			ev, ok := gtx.Event(pointer.Filter{Target: state, Kinds: pointer.Press | pointer.Release | pointer.Cancel | pointer.Move | pointer.Enter | pointer.Leave})
			if !ok {
				break
			}
			e, ok := ev.(pointer.Event)
			if !ok {
				continue
			}
			p := e.Position.Mul(1 / scale)
			oldHover := state.Hover
			switch e.Kind {
			case pointer.Move, pointer.Enter:
				state.Hover = nil
				for i := len(edges) - 1; i >= 0; i-- {
					if edges[i].hit(p) {
						selected := edges[i]
						state.Hover = &selected
						break
					}
				}
			case pointer.Leave:
				state.Hover = nil
			case pointer.Press:
				state.Pressed = nil
				if e.Buttons != pointer.ButtonPrimary {
					continue
				}
				for i := len(edges) - 1; i >= 0; i-- {
					if edges[i].hit(p) {
						selected := edges[i]
						state.Pressed = &selected
						break
					}
				}
			case pointer.Release:
				if selected := state.Pressed; selected != nil && selected.hit(p) {
					source, target := selected.Source, selected.Target
					core.Call(gtx, func() { a.removeDAGEdge(source, target) })
				}
				state.Pressed = nil
			case pointer.Cancel:
				state.Pressed = nil
			}
			if (oldHover == nil) != (state.Hover == nil) || oldHover != nil && state.Hover != nil && (oldHover.Source != state.Hover.Source || oldHover.Target != state.Hover.Target) {
				gtx.Execute(op.InvalidateCmd{})
			}
		}
		event.Op(gtx.Ops, state)
		if state.Hover != nil {
			pointer.CursorPointer.Add(gtx.Ops)
		}
	}
	// One batched path keeps the dotted grid inexpensive even for a large graph.
	var dots clip.Path
	dots.Begin(gtx.Ops)
	w, h := float32(gtx.Constraints.Max.X)/scale, float32(gtx.Constraints.Max.Y)/scale
	// Imported layouts may be far beyond the normal canvas. Bound only the dot
	// mesh; node positions and scroll extent remain intact.
	for y := float32(9); y < min(h, 3000); y += 18 {
		for x := float32(9); x < min(w, 3000); x += 18 {
			p := f32.Pt(x*scale, y*scale)
			r := float32(.7) * scale
			dots.MoveTo(p.Add(f32.Pt(-r, 0)))
			dots.QuadTo(p.Add(f32.Pt(-r, -r)), p.Add(f32.Pt(0, -r)))
			dots.QuadTo(p.Add(f32.Pt(r, -r)), p.Add(f32.Pt(r, 0)))
			dots.QuadTo(p.Add(f32.Pt(r, r)), p.Add(f32.Pt(0, r)))
			dots.QuadTo(p.Add(f32.Pt(-r, r)), p.Add(f32.Pt(-r, 0)))
			dots.Close()
		}
	}
	paint.FillShape(gtx.Ops, mixColor(seamColor, theme.Bg, .72), clip.Outline{Path: dots.End()}.Op())
	for _, e := range edges {
		var path clip.Path
		path.Begin(gtx.Ops)
		path.MoveTo(e.From.Mul(scale))
		path.CubeTo(e.From.Add(f32.Pt(70, 0)).Mul(scale), e.To.Add(f32.Pt(-70, 0)).Mul(scale), e.To.Mul(scale))
		fg, width := theme.Muted, float32(1.5)
		if state.Hover != nil && state.Hover.Source == e.Source && state.Hover.Target == e.Target {
			fg, width = theme.DangerText, 3
		}
		paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: path.End(), Width: width * scale}.Op())
	}
	return core.D{Size: gtx.Constraints.Max}
}

// Positions with invalid floating-point data must never reach Gio's rasterizer.
func dagFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
