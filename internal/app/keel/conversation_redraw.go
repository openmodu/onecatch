package keeldesktop

import (
	"encoding/binary"
	"hash/maphash"
)

// visibleConversationSignature hashes what the conversation currently draws:
// the row order, and the content of the rows mounted in the last frame. A live
// frame that leaves it unchanged needs no window redraw. Keel repaints the
// whole window for any update, so skipping these is what keeps a long-running
// command with folded output from holding the window in its active state.
func (a *app) visibleConversationSignature() uint64 {
	var h maphash.Hash
	h.SetSeed(rowSignatureSeed)
	rows := a.displayedConversationRows()
	active := a.detail != nil && a.detail.Active
	for i, e := range rows {
		h.WriteString(e.key)
		h.WriteByte(0)
		// Rows outside the mounted range are re-estimated by syncMessages;
		// their content does not reach the screen until they are scrolled in.
		if a.conversation.renderedRows[e.key] {
			writeSignatureNumber(&h, a.visibleRowSignature(e, active, i == len(rows)-1))
		}
	}
	return h.Sum64()
}

// visibleRowSignature is rowSignature without the content a folded process
// group or tool hides. Anything else falls back to the full row signature.
func (a *app) visibleRowSignature(e *entry, active, last bool) uint64 {
	if len(e.process) == 0 || len(e.columns) > 0 || len(e.stack) > 0 {
		return rowSignature(e, active, last)
	}
	var h maphash.Hash
	h.SetSeed(rowSignatureSeed)
	str := func(s string) {
		h.WriteString(s)
		h.WriteByte(0)
	}
	num := func(n uint64) { writeSignatureNumber(&h, n) }
	flag := func(v bool) {
		if v {
			num(1)
		} else {
			num(0)
		}
	}
	groupActive, count, thoughts := a.processState(e)
	open := a.conversationOpen(e.key, groupActive)
	str(e.key)
	str(e.stepLabel)
	str(e.stepContext)
	flag(groupActive)
	num(uint64(count))
	num(uint64(thoughts))
	flag(open)
	if !open {
		return h.Sum64()
	}
	num(uint64(len(e.process)))
	for _, item := range e.process {
		if item.permission != nil || item.question != nil {
			num(rowSignature(item, active, last))
			continue
		}
		// The folded row draws the call's own text (title or reasoning
		// preview), its state and its timing; results only when opened.
		str(item.key)
		str(string(item.kind))
		num(item.revision)
		num(uint64(len(item.text)))
		flag(item.streaming)
		flag(item.failed)
		flag(item.settled)
		num(uint64(len(item.details)))
		if n := len(item.details); n > 0 {
			num(uint64(item.details[n-1].at.UnixNano()))
		}
		itemOpen := a.conversationOpen(item.key, false)
		flag(itemOpen)
		if itemOpen {
			for _, d := range item.details {
				str(d.key)
				num(d.revision)
				num(uint64(len(d.text)))
				flag(d.failed)
			}
		}
	}
	return h.Sum64()
}

func writeSignatureNumber(h *maphash.Hash, n uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], n)
	h.Write(b[:])
}

// liveFramesNeedRedraw reports whether frames just applied changed what is on
// screen.
func (a *app) liveFramesNeedRedraw(before uint64) bool {
	return a.visibleConversationSignature() != before
}
