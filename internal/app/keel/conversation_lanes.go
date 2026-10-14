package keeldesktop

import (
	"fmt"
	"image"
	"slices"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
)

const parallelLaneHeight = 420

type conversationLane struct {
	entries    []*entry
	keys       []string
	signatures map[string]string
	rendered   map[string]bool
	scroller   *kit.MessageScrollerView
	root       *el.RootWidget
}

func isParallelBody(e *entry) bool {
	return len(e.columns) > 0 && e.kind != kindTimelineHeader
}

func parallelLaneID(row *entry, column int) string {
	return fmt.Sprintf("parallel/%s/%d", row.key, column)
}

// Each lane keeps its own virtual list and reading position. Appending to one
// lane neither grows the outer timeline nor scrolls a reader in another lane.
func (a *app) syncParallelLanes() {
	if a.conversation.lanes == nil {
		a.conversation.lanes = make(map[string]*conversationLane)
	}
	seen := make(map[string]bool)
	groups := make(map[string]bool)
	for _, row := range a.conversationRows {
		if !isParallelBody(row) {
			continue
		}
		groups[row.pageGroup] = true
		for i, cell := range row.columns {
			id := parallelLaneID(row, i)
			seen[id] = true
			lane := a.conversation.lanes[id]
			if lane == nil {
				lane = &conversationLane{signatures: make(map[string]string), rendered: make(map[string]bool)}
				lane.scroller = kit.MessageScroller(nil, 140, func(cx *el.Context, index int) el.Element {
					lane.rendered[lane.entries[index].key] = true
					return a.messageEntry(cx, lane.entries[index], -1)
				})
				lane.root = el.Embed(el.ViewFunc(func(cx *el.Context) el.Element {
					clear(lane.rendered)
					return el.Div().WFull().HFull().Child(lane.scroller.Render(cx))
				}))
				a.conversation.lanes[id] = lane
			}
			lane.entries = nil
			if cell != nil {
				lane.entries = cell.stack
				if len(lane.entries) == 0 {
					lane.entries = []*entry{cell}
				}
			}
			keys := make([]string, len(lane.entries))
			signatures := make(map[string]string, len(lane.entries))
			var changed []string
			for j, e := range lane.entries {
				keys[j] = e.key
				sig := rowSignature(e, a.detail != nil && a.detail.Active, false)
				signatures[e.key] = sig
				if lane.signatures[e.key] != sig && !lane.rendered[e.key] {
					changed = append(changed, e.key)
				}
			}
			if !slices.Equal(lane.keys, keys) {
				lane.scroller.SetKeys(keys)
				lane.keys = keys
			}
			if len(changed) > 0 {
				lane.scroller.Invalidate(changed...)
			}
			lane.signatures = signatures
		}
	}
	for group := range a.conversation.pages {
		if !groups[group] {
			delete(a.conversation.pages, group)
		}
	}
	for id := range a.conversation.lanes {
		if !seen[id] {
			delete(a.conversation.lanes, id)
		}
	}
}

// The outer height does not change when switching pages. Hidden pages retain
// their virtual scrollers, so background output cannot move the visible page.
func (a *app) parallelPage(row *entry) int {
	page := a.conversation.pages[row.pageGroup]
	if page < 0 || page >= len(row.columns) {
		return 0
	}
	return page
}

func (a *app) selectParallelPage(row *entry, page int) {
	if page < 0 || page >= len(row.columns) {
		return
	}
	if a.conversation.pages == nil {
		a.conversation.pages = make(map[string]int)
	}
	a.conversation.pages[row.pageGroup] = page
}

func (a *app) parallelPages(cx *el.Context, row *entry) el.Element {
	page := a.parallelPage(row)
	if row.kind == kindTimelineHeader {
		header := el.Div().WFull().MaxW(el.Dp(920)).H(el.Dp(62)).NoShrink().Gap(6).Py(4)
		if cell := row.columns[page]; cell != nil {
			header.Child(el.Text(cell.stepLabel).TextSize(14).Bold().MaxLines(1), el.Text(cell.stepContext).TextSize(12).TextColor(theme.Muted).MaxLines(1))
		}
		return el.Div().NoShrink().Items(el.Center).Child(header)
	}
	// Pages share a vertical baseline and expose a numbered strip on either
	// side of the selected sheet. Keep every tab above the front sheet.
	const offset = 36
	deck := el.Div().WFull().MaxW(el.Dp(920)).H(el.Dp(parallelLaneHeight + offset)).NoShrink()
	sheet := func(i int) *el.DivEl {
		return el.Div().Absolute().Top(0).Left(float32(i*offset)).Right(float32((len(row.columns)-1-i)*offset)).Bottom(0).
			Rounded(10).Bg(theme.Subtle).Border(1, seamColor)
	}
	for i := range row.columns {
		if i != page {
			deck.Child(sheet(i))
		}
	}
	front := sheet(page).Bg(theme.Bg).Pt(offset)
	body := el.Div().WFull().H(el.Dp(parallelLaneHeight)).NoShrink().
		ID(parallelLaneID(row, page)).Name(fmt.Sprintf("并行任务 %d", page+1))
	if lane := a.conversation.lanes[parallelLaneID(row, page)]; lane != nil {
		body.Child(el.Widget(core.ViewportFunc(func(gtx core.C, visible image.Rectangle) core.D {
			return lane.root.LayoutIn(gtx, visible, cx)
		})).WFull().HFull())
	}
	front.Child(body)
	deck.Child(front)
	for i := range row.columns {
		tab := button(fmt.Sprint(i+1), func() { a.selectParallelPage(row, i) }, kit.ButtonSecondary).
			Name(fmt.Sprintf("切换任务页面 %d", i+1)).Selected(page == i).Size(28)
		holder := el.Div().Absolute().Top(4).W(el.Dp(28)).H(el.Dp(28))
		if i <= page {
			holder.Left(float32(i*offset + 4))
		} else {
			holder.Right(float32((len(row.columns)-1-i)*offset + 4))
		}
		deck.Child(holder.Child(tab.Render(cx)))
	}
	return el.Div().NoShrink().Items(el.Center).Child(deck)
}
