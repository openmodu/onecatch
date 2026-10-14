package keeldesktop

import (
	"fmt"
	"slices"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
)

const parallelLaneHeight = 420

type conversationLane struct {
	entries    []*entry
	keys       []string
	signatures map[string]string
	rendered   map[string]bool
	scroller   *kit.MessageScrollerView
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
	for _, row := range a.conversationRows {
		if !isParallelBody(row) {
			continue
		}
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
	for id := range a.conversation.lanes {
		if !seen[id] {
			delete(a.conversation.lanes, id)
		}
	}
}
