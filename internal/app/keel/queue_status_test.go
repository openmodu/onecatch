package keeldesktop

import (
	"fmt"
	"testing"
	"time"

	"github.com/dyike/keel/ui/el"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/desktop/runstream"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestQueueStatusCoalescesHistoryAndLiveUpdates(t *testing.T) {
	tx := newTranscript()
	at := time.Now()
	for i := 1; i <= 80; i++ {
		tx.frame(runstream.Frame{StepRunID: "implement", Seq: int64(i), Event: agentrun.Event{Kind: agentrun.KindQueueStatus, Text: fmt.Sprintf(`{"state":"waiting","position":%d}`, 126-i), Revision: 1, At: at.Add(time.Duration(i) * time.Second)}})
	}
	rows := buildConversationEntries(tx.visible())
	if len(rows) != 1 || rows[0].kind != agentrun.KindQueueStatus || len(rows[0].process) != 0 || queueStatusLabel(rows[0].text) != "排队中 · 当前第 46 位" {
		t.Fatalf("queue notifications became process records: %+v", rows)
	}
	key, started := rows[0].key, rows[0].at
	beforeSignature := rowSignature(rows[0], false, false)
	updated := *rows[0]
	updated.text = `{"state":"waiting","position":45}`
	if rowSignature(&updated, false, false) == beforeSignature {
		t.Fatal("equal-length queue update did not invalidate its row")
	}
	// A late history load must not roll the live queue position backwards.
	tx.merge(entry{key: eventKey("implement", "", 20, agentrun.KindQueueStatus), step: "implement", seq: 20, kind: agentrun.KindQueueStatus, text: `{"state":"waiting","position":106}`, revision: 1, at: at.Add(20 * time.Second)}, false)
	tx.frame(runstream.Frame{StepRunID: "implement", Seq: 81, Event: agentrun.Event{Kind: agentrun.KindQueueStatus, Text: `{"state":"ready"}`, Revision: 1, At: at.Add(81 * time.Second)}})
	tx.frame(runstream.Frame{StepRunID: "implement", Seq: 82, Event: agentrun.Event{Kind: agentrun.KindToolUse, StreamID: "shell", Text: "go test ./...", Revision: 1, At: at.Add(82 * time.Second)}})
	tx.frame(runstream.Frame{StepRunID: "review", Seq: 1, Event: agentrun.Event{Kind: agentrun.KindQueueStatus, Text: `{"state":"waiting","position":9}`, Revision: 1, At: at.Add(83 * time.Second)}})
	rows = buildConversationEntries(tx.visible())
	if len(rows) != 3 || rows[0].key != key || rows[0].at != started || queueStatusLabel(rows[0].text) != "排队结束" || len(rows[1].process) != 1 || rows[2].kind != agentrun.KindQueueStatus {
		t.Fatalf("queue update reordered or mixed step runs: %+v", rows)
	}
	if got := queueEntryLabel(rows[0]); got != "排队结束 · 等待 1分20秒" {
		t.Fatalf("completed live wait lost duration: %s", got)
	}
}

func TestQueueStatusRendersOutsideToolCards(t *testing.T) {
	a := conversationFixture(t)
	a.entries = buildConversationEntries([]*entry{{key: "queue-event", step: "implement", kind: agentrun.KindQueueStatus, text: `{"state":"waiting","position":125}`}})
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().WFull().HFull().Child(a.message(cx, 0)) }))
	router, frame, _ := alignmentRouter(root, 800, 300, 1)
	frame()
	if _, ok := alignmentNode(router.AppendSemantics(nil), "排队中 · 当前第 125 位", false); !ok {
		t.Fatal("missing readable queue status")
	}
	if _, ok := alignmentNode(router.AppendSemantics(nil), "已处理", true); ok {
		t.Fatal("queue status rendered as a tool group")
	}
	if queueStatusLabel(`{"state":"queued","position":0,"operation":"contextCompaction"}`) != "上下文压缩排队中 · 当前第 0 位" || queueStatusLabel(`{"state":"waiting"}`) != "排队中" {
		t.Fatal("optional queue fields lost")
	}
}

func TestQueueDurationSurvivesHistoryReload(t *testing.T) {
	at := time.Date(2026, 10, 13, 13, 0, 0, 0, time.UTC)
	tx := newTranscript()
	tx.history([]desktopservice.RuntimeEventView{{StepRunID: "step", Seq: 1, Kind: string(agentrun.KindQueueStatus), Text: `{"state":"ready"}`, At: at.Format(time.RFC3339Nano), QueueUpdatedAt: at.Add(125 * time.Second).Format(time.RFC3339Nano)}})
	row := buildConversationEntries(tx.visible())[0]
	if got := queueEntryLabel(row); got != "排队结束 · 等待 2分5秒" {
		t.Fatalf("history wait: %s", got)
	}
	row.queueUpdatedAt = time.Time{}
	if got := queueEntryLabel(row); got != "排队结束" {
		t.Fatalf("invented historical timing: %s", got)
	}
}
