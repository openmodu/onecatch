package keeldesktop

// A split tree keeps every pane alive independently of tab focus. Closing a
// leaf promotes its sibling; closing the original pane does not lose its group.
type terminalSplit struct {
	ID, Pane, Direction     string
	Ratio                   float32
	Available, DragPosition float32
	First, Second           *terminalSplit
}

func terminalLeaf(id string) *terminalSplit { return &terminalSplit{Pane: id} }
func (n *terminalSplit) panes() []string {
	if n == nil {
		return nil
	}
	if n.Pane != "" {
		return []string{n.Pane}
	}
	return append(n.First.panes(), n.Second.panes()...)
}
func (n *terminalSplit) split(pane, next, direction, id string) bool {
	if n == nil {
		return false
	}
	if n.Pane == pane {
		*n = terminalSplit{ID: id, Direction: direction, Ratio: .5, First: terminalLeaf(pane), Second: terminalLeaf(next)}
		return true
	}
	return n.First.split(pane, next, direction, id) || n.Second.split(pane, next, direction, id)
}
func (n *terminalSplit) remove(pane string) *terminalSplit {
	if n == nil {
		return nil
	}
	if n.Pane != "" {
		if n.Pane == pane {
			return nil
		}
		return n
	}
	n.First, n.Second = n.First.remove(pane), n.Second.remove(pane)
	if n.First == nil {
		return n.Second
	}
	if n.Second == nil {
		return n.First
	}
	return n
}

type terminalRect struct{ X, Y, W, H float32 }
type terminalPaneRect struct {
	ID   string
	Rect terminalRect
}
type terminalSplitRect struct {
	Node *terminalSplit
	Rect terminalRect
}

func (n *terminalSplit) geometry(rect terminalRect, panes *[]terminalPaneRect, splits *[]terminalSplitRect) {
	if n == nil {
		return
	}
	if n.Pane != "" {
		*panes = append(*panes, terminalPaneRect{n.Pane, rect})
		return
	}
	*splits = append(*splits, terminalSplitRect{n, rect})
	first, second := rect, rect
	if n.Direction == "vertical" {
		first.W = rect.W * n.Ratio
		second.X = rect.X + first.W
		second.W = rect.W - first.W
	} else {
		first.H = rect.H * n.Ratio
		second.Y = rect.Y + first.H
		second.H = rect.H - first.H
	}
	n.First.geometry(first, panes, splits)
	n.Second.geometry(second, panes, splits)
}
func terminalSplitRatio(ratio, available, minimum float32) float32 {
	if available <= 0 {
		return .5
	}
	limit := min(float32(.45), minimum/available)
	return max(limit, min(1-limit, ratio))
}
