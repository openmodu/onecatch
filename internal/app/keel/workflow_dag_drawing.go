package keeldesktop

import (
	"image/color"
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

// dagEdge is one drawn edge, routed as horizontal and vertical segments so
// loops stay readable. Dependencies leave the source's right side and enter
// the target's left side; rework edges drop from the checking step's bottom,
// run along their own lane underneath, and rise into the target's bottom.
type dagEdge struct {
	Source, Target, Kind string
	Points               []f32.Point
}

const (
	dagEdgeStub     = 20 // straight run out of and into a port
	dagEdgeLaneGap  = 28 // clearance between the steps and the first lane
	dagEdgeLaneStep = 14 // spacing between parallel lanes
)

func (e dagEdge) same(other *dagEdge) bool {
	return other != nil && e.Source == other.Source && e.Target == other.Target && e.Kind == other.Kind
}

func (e dagEdge) from() f32.Point { return e.Points[0] }
func (e dagEdge) to() f32.Point   { return e.Points[len(e.Points)-1] }

func segmentLength(a, b f32.Point) float32 {
	d := b.Sub(a)
	return float32(math.Hypot(float64(d.X), float64(d.Y)))
}

// midpoint is where the delete control sits: halfway along the route.
func (e dagEdge) midpoint() f32.Point { return e.point(.5) }

// point returns the position at fraction t of the route's length.
func (e dagEdge) point(t float32) f32.Point {
	total := float32(0)
	for i := 1; i < len(e.Points); i++ {
		total += segmentLength(e.Points[i-1], e.Points[i])
	}
	remaining := max(0, min(1, t)) * total
	for i := 1; i < len(e.Points); i++ {
		length := segmentLength(e.Points[i-1], e.Points[i])
		if remaining <= length && length > 0 {
			return e.Points[i-1].Add(e.Points[i].Sub(e.Points[i-1]).Mul(remaining / length))
		}
		remaining -= length
	}
	return e.to()
}

// hit ignores the ends of the edge: they sit under the step ports, and a click
// meant to start a connection must not delete the edge beneath it.
func (e dagEdge) hit(p f32.Point) bool {
	for _, end := range []f32.Point{e.from(), e.to()} {
		if d := p.Sub(end); d.X*d.X+d.Y*d.Y <= 14*14 {
			return false
		}
	}
	for i := 1; i < len(e.Points); i++ {
		a, v := e.Points[i-1], e.Points[i].Sub(e.Points[i-1])
		length := v.X*v.X + v.Y*v.Y
		t := float32(0)
		if length > 0 {
			d := p.Sub(a)
			t = max(float32(0), min(float32(1), (d.X*v.X+d.Y*v.Y)/length))
		}
		if d := p.Sub(a.Add(v.Mul(t))); d.X*d.X+d.Y*d.Y <= 36 {
			return true
		}
	}
	return false
}

// dagEdges routes the editor's edges between its fixed-size step cards.
func dagEdges(d domainworkflows.Definition) []dagEdge {
	rects := make(map[string]flowRect, len(d.Steps))
	for _, s := range d.Steps {
		p := dagPosition(d, s.ID)
		rects[s.ID] = flowRect{X: float32(p.X), Y: float32(p.Y), W: dagNodeWidth, H: dagNodeHeight}
	}
	return routeFlow(d, rects, false)
}

func dagExtent(d domainworkflows.Definition) (float32, float32) {
	w, h := float32(dagCanvasWidth), float32(dagCanvasHeight)
	for _, s := range d.Steps {
		p := dagPosition(d, s.ID)
		w = max(w, float32(p.X)+224)
		h = max(h, float32(p.Y)+106)
	}
	// Lanes run below the steps; keep the lowest one inside the scroll area.
	for _, e := range dagEdges(d) {
		for _, p := range e.Points {
			h = max(h, p.Y+24)
		}
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
					source, target, kind := selected.Source, selected.Target, selected.Kind
					core.Call(gtx, func() { a.removeDAGEdge(source, target, kind) })
				}
				state.Pressed = nil
			case pointer.Cancel:
				state.Pressed = nil
			}
			if (oldHover == nil) != (state.Hover == nil) || oldHover != nil && !oldHover.same(state.Hover) {
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
	paintEdges(gtx, edges, state.Hover, scale)
	return core.D{Size: gtx.Constraints.Max}
}

// drawArrowHead marks direction at the target end, which matters for rework
// edges: they run right to left underneath the steps.
func drawArrowHead(gtx core.C, e dagEdge, fg color.NRGBA, scale float32) {
	tip := e.to()
	dir := tip.Sub(e.Points[len(e.Points)-2])
	length := float32(math.Hypot(float64(dir.X), float64(dir.Y)))
	if length == 0 {
		return
	}
	dir = dir.Mul(1 / length)
	normal := f32.Pt(-dir.Y, dir.X)
	base := tip.Sub(dir.Mul(8))
	var path clip.Path
	path.Begin(gtx.Ops)
	path.MoveTo(tip.Mul(scale))
	path.LineTo(base.Add(normal.Mul(4)).Mul(scale))
	path.LineTo(base.Sub(normal.Mul(4)).Mul(scale))
	path.Close()
	paint.FillShape(gtx.Ops, fg, clip.Outline{Path: path.End()}.Op())
}

// Positions with invalid floating-point data must never reach Gio's rasterizer.
func dagFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
