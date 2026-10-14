package keeldesktop

import (
	"cmp"
	"fmt"
	"image/color"
	"slices"

	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/op/clip"
	"github.com/dyike/keel/gio/op/paint"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

// flowRect is a step's box in a flow drawing, in dp.
type flowRect struct{ X, Y, W, H float32 }

func (r flowRect) right() float32  { return r.X + r.W }
func (r flowRect) bottom() float32 { return r.Y + r.H }

// routeFlow draws every edge as horizontal and vertical segments around the
// given step boxes. In a horizontal flow dependencies run left to right and
// loops (rework, or a dependency pointing backwards) run in lanes underneath;
// in a vertical flow dependencies run top to bottom and loops run in lanes to
// the right. Each loop gets its own lane, the shortest nearest the steps, so
// nested loops never cross, and several loops at one step use separate slots.
func routeFlow(d domainworkflows.Definition, rects map[string]flowRect, vertical bool) []dagEdge {
	edges := make([]dagEdge, len(d.Edges))
	type loop struct {
		index      int
		start, end f32.Point
		span       float32
		bypass     bool
	}
	var loops []loop
	outSlot, inSlot := map[string]int{}, map[string]int{}
	edge := float32(0) // the far side of every step: where lanes begin
	for _, r := range rects {
		if vertical {
			edge = max(edge, r.right())
		} else {
			edge = max(edge, r.bottom())
		}
	}
	for i, e := range d.Edges {
		from, to := rects[e.From], rects[e.To]
		edges[i] = dagEdge{Source: e.From, Target: e.To, Kind: e.Kind}
		if e.Kind == domainworkflows.EdgeRework {
			// Leaving loops use one half of the step's far side and arriving
			// loops the other half, one slot each.
			out := float32(outSlot[e.From]) * dagEdgeLaneStep
			in := float32(inSlot[e.To]) * dagEdgeLaneStep
			outSlot[e.From]++
			inSlot[e.To]++
			var start, end f32.Point
			if vertical {
				start = f32.Pt(from.right(), from.Y+from.H/2+6+out)
				end = f32.Pt(to.right(), to.Y+to.H/2-6-in)
			} else {
				start = f32.Pt(from.X+from.W/2+18+out, from.bottom())
				end = f32.Pt(to.X+to.W/2-18-in, to.bottom())
			}
			loops = append(loops, loop{index: i, start: start, end: end})
			continue
		}
		edges[i].Kind = domainworkflows.EdgeNext
		if vertical {
			start, end := f32.Pt(from.X+from.W/2, from.bottom()), f32.Pt(to.X+to.W/2, to.Y)
			if end.Y-start.Y >= 2*dagEdgeStub {
				mid := (start.Y + end.Y) / 2
				edges[i].Points = []f32.Point{start, f32.Pt(start.X, mid), f32.Pt(end.X, mid), end}
				continue
			}
			loops = append(loops, loop{index: i, start: f32.Pt(from.right(), from.Y+from.H/2), end: f32.Pt(to.right(), to.Y+to.H/2), bypass: true})
			continue
		}
		start, end := f32.Pt(from.right(), from.Y+from.H/2), f32.Pt(to.X, to.Y+to.H/2)
		if end.X-start.X >= 2*dagEdgeStub {
			mid := (start.X + end.X) / 2
			edges[i].Points = []f32.Point{start, f32.Pt(mid, start.Y), f32.Pt(mid, end.Y), end}
			continue
		}
		loops = append(loops, loop{index: i, start: start, end: end, bypass: true})
	}
	for i := range loops {
		if vertical {
			loops[i].span = abs32(loops[i].start.Y - loops[i].end.Y)
		} else {
			loops[i].span = abs32(loops[i].start.X - loops[i].end.X)
		}
	}
	slices.SortStableFunc(loops, func(a, b loop) int { return cmp.Compare(a.span, b.span) })
	for lane, l := range loops {
		at := edge + dagEdgeLaneGap + float32(lane)*dagEdgeLaneStep
		switch {
		case vertical:
			edges[l.index].Points = []f32.Point{l.start, f32.Pt(at, l.start.Y), f32.Pt(at, l.end.Y), l.end}
		case l.bypass:
			out, in := l.start.X+dagEdgeStub, l.end.X-dagEdgeStub
			edges[l.index].Points = []f32.Point{l.start, f32.Pt(out, l.start.Y), f32.Pt(out, at), f32.Pt(in, at), f32.Pt(in, l.end.Y), l.end}
		default:
			edges[l.index].Points = []f32.Point{l.start, f32.Pt(l.start.X, at), f32.Pt(l.end.X, at), l.end}
		}
	}
	return edges
}

func abs32(v float32) float32 { return max(v, -v) }

// paintEdges strokes routed edges; hover marks the one a click would delete.
func paintEdges(gtx core.C, edges []dagEdge, hover *dagEdge, scale float32) {
	for _, e := range edges {
		if len(e.Points) < 2 {
			continue
		}
		var path clip.Path
		path.Begin(gtx.Ops)
		path.MoveTo(e.from().Mul(scale))
		for _, p := range e.Points[1:] {
			path.LineTo(p.Mul(scale))
		}
		fg, width := theme.Muted, float32(1.5)
		if e.Kind == domainworkflows.EdgeRework {
			fg = theme.Warning
		}
		if e.same(hover) {
			fg, width = theme.DangerText, 3
		}
		paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: path.End(), Width: width * scale}.Op())
		drawArrowHead(gtx, e, fg, scale)
	}
}

type flowLines struct{ edges []dagEdge }

func (v flowLines) Layout(gtx core.C) core.D {
	scale := gtx.Metric.PxPerDp
	if scale <= 0 {
		scale = 1
	}
	paintEdges(gtx, v.edges, nil, scale)
	return core.D{Size: gtx.Constraints.Max}
}

// flowChartOptions configures a read-only flow chart. Nodes carries live step
// states for a run; without it the chart shows the definition only.
type flowChartOptions struct {
	ID       string
	Vertical bool
	// Width bounds a vertical chart; a horizontal chart grows to fit.
	Width    float32
	Nodes    map[string]domainworkflows.NodeState
	Rounds   map[string]int
	Selected string
	OnSelect func(stepID string)
}

const (
	flowNodeW, flowNodeH   = 148, 50
	flowColGap, flowRowGap = 44, 18
	flowPad                = 6
)

// flowLayout places steps by dependency depth: columns in a horizontal chart,
// rows in a vertical one. Rework edges do not affect placement.
func flowLayout(d domainworkflows.Definition, o flowChartOptions) (map[string]flowRect, float32, float32) {
	levels := map[string]int{}
	for _, id := range dagLevelOrder(d) {
		for _, dep := range domainworkflows.Upstream(d, id) {
			levels[id] = max(levels[id], levels[dep]+1)
		}
	}
	byLevel := map[int][]string{}
	depth := 0
	for _, s := range d.Steps {
		byLevel[levels[s.ID]] = append(byLevel[levels[s.ID]], s.ID)
		depth = max(depth, levels[s.ID]+1)
	}
	loops := 0
	for _, e := range d.Edges {
		if e.Kind == domainworkflows.EdgeRework {
			loops++
		}
	}
	laneRoom := float32(0)
	if loops > 0 {
		laneRoom = dagEdgeLaneGap + float32(loops)*dagEdgeLaneStep
	}
	rects := map[string]flowRect{}
	if o.Vertical {
		usable := max(120, o.Width-2*flowPad-laneRoom)
		for level := 0; level < depth; level++ {
			ids := byLevel[level]
			n := float32(len(ids))
			w := min(220, (usable-(n-1)*12)/n)
			x := flowPad + (usable-(w*n+(n-1)*12))/2
			for i, id := range ids {
				rects[id] = flowRect{X: x + float32(i)*(w+12), Y: flowPad + float32(level)*(flowNodeH+flowColGap), W: w, H: flowNodeH}
			}
		}
		return rects, max(o.Width, usable+2*flowPad+laneRoom), flowPad*2 + float32(depth)*(flowNodeH+flowColGap) - flowColGap
	}
	rows := 0
	for level := 0; level < depth; level++ {
		for i, id := range byLevel[level] {
			rects[id] = flowRect{X: flowPad + float32(level)*(flowNodeW+flowColGap), Y: flowPad + float32(i)*(flowNodeH+flowRowGap), W: flowNodeW, H: flowNodeH}
			rows = max(rows, i+1)
		}
	}
	width := flowPad*2 + float32(depth)*(flowNodeW+flowColGap) - flowColGap
	height := flowPad*2 + float32(rows)*(flowNodeH+flowRowGap) - flowRowGap + laneRoom
	return rects, width, height
}

// dagLevelOrder returns steps so that every step follows its dependencies.
func dagLevelOrder(d domainworkflows.Definition) []string {
	var order []string
	seen := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		for _, dep := range domainworkflows.Upstream(d, id) {
			visit(dep)
		}
		order = append(order, id)
	}
	for _, s := range d.Steps {
		visit(s.ID)
	}
	return order
}

func flowNodeColors(node domainworkflows.NodeState, live bool) (stroke, dot color.NRGBA) {
	if !live {
		return seamColor, theme.Muted
	}
	switch node.Status {
	case domainworkflows.NodeCompleted:
		return theme.Success, theme.Success
	case domainworkflows.NodeRunning:
		return theme.Info, theme.Info
	case domainworkflows.NodePaused, domainworkflows.NodeFailed:
		return theme.Warning, theme.Warning
	default:
		if node.Verdict == domainworkflows.VerdictRework || len(node.Inbox) > 0 {
			return theme.Warning, theme.Warning
		}
		return seamColor, theme.Muted
	}
}

// labelPoint is where an edge's label sits: the middle of a dependency, or
// the middle of a loop's lane so it reads along the lane it names.
func (e dagEdge) labelPoint() f32.Point {
	if e.Kind == domainworkflows.EdgeRework && len(e.Points) == 4 {
		return e.Points[1].Add(e.Points[2]).Mul(.5)
	}
	return e.point(.5)
}

// flowEdgeLabel says what moves work along an edge: a pass for a dependency,
// a rework verdict for a loop with its round budget, and live progress when
// run state is given.
func flowEdgeLabel(d domainworkflows.Definition, e dagEdge, nodes map[string]domainworkflows.NodeState, rounds map[string]int) (string, color.NRGBA) {
	if e.Kind != domainworkflows.EdgeRework {
		if nodes != nil && nodes[e.Source].Status == domainworkflows.NodeCompleted {
			return "通过", theme.Success
		}
		return "通过", theme.Muted
	}
	limit := d.Policy.MaxReworkRounds
	for _, edge := range d.Edges {
		if edge.Kind == domainworkflows.EdgeRework && edge.From == e.Source && edge.To == e.Target && edge.MaxRounds > 0 {
			limit = edge.MaxRounds
		}
	}
	if nodes == nil {
		return fmt.Sprintf("退回 · 最多 %d 轮", limit), theme.Warning
	}
	return fmt.Sprintf("退回 %d/%d", rounds[domainworkflows.EdgeKey(e.Source, e.Target)], limit), theme.Warning
}

// edgeLabels places a small pill on every edge naming its transition.
func edgeLabels(d domainworkflows.Definition, edges []dagEdge, nodes map[string]domainworkflows.NodeState, rounds map[string]int) []el.Element {
	labels := make([]el.Element, 0, len(edges))
	for _, e := range edges {
		if len(e.Points) < 2 {
			continue
		}
		text, fg := flowEdgeLabel(d, e, nodes, rounds)
		p := e.labelPoint()
		labels = append(labels, el.Div().Absolute().Left(p.X-48).Top(p.Y-8).W(el.Dp(96)).H(el.Dp(16)).Center().
			// The pill is a container around the text rather than a text
			// element with its own background, as elsewhere in the app.
			Child(el.Div().Bg(theme.Bg).Rounded(4).Px(4).Child(el.Text(text).TextSize(9).TextColor(fg).MaxLines(1))))
	}
	return labels
}

// flowChart renders the whole workflow graph read-only: every step, every
// dependency and every rework loop, coloured by live state when a run is given.
func flowChart(cx *el.Context, d domainworkflows.Definition, o flowChartOptions) el.Element {
	rects, w, h := flowLayout(d, o)
	edges := routeFlow(d, rects, o.Vertical)
	chart := el.Div().ID(o.ID).W(el.Dp(w)).H(el.Dp(h)).NoShrink().Child(el.Widget(flowLines{edges: edges}).WFull().HFull())
	chart.Child(edgeLabels(d, edges, o.Nodes, o.Rounds)...)
	live := o.Nodes != nil
	for _, step := range d.Steps {
		s := step
		r := rects[s.ID]
		node := o.Nodes[s.ID]
		stroke, dot := flowNodeColors(node, live)
		ring := float32(1)
		if o.Selected == s.ID {
			ring = 2
		}
		subtitle := stepAgentLabel(s)
		if live {
			subtitle, _ = stepStatusLabel(node)
		}
		card := el.Div().WFull().HFull().Rounded(8).Bg(theme.Surface).Px(10).Py(7).Gap(3).
			ID(o.ID+"/"+s.ID).Name("步骤 "+s.Name).
			Child(el.Div().Row().Items(el.Center).Gap(6).Child(el.Div().Size(el.Dp(6)).Rounded(3).Bg(dot).NoShrink(), el.Text(s.Name).TextSize(12).Medium().MaxLines(1)),
				el.Text(subtitle).TextSize(10).TextColor(theme.Muted).MaxLines(1))
		if o.OnSelect != nil {
			card.Role("button").Focusable(true).CursorPointer().OnClick(func() { o.OnSelect(s.ID) })
		}
		chart.Child(el.Div().Absolute().Left(r.X - ring).Top(r.Y - ring).W(el.Dp(r.W + 2*ring)).H(el.Dp(r.H + 2*ring)).Rounded(8 + ring).Bg(stroke).P(ring).Child(card))
	}
	return chart
}
