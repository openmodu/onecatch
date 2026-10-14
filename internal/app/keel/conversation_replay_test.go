package keeldesktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"bufio"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktop "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/desktop/runstream"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

// Optional read-only replay of a real run. Never executes tools or writes its data.
func TestReplayWorkflowTranscript(t *testing.T) {
	root := os.Getenv("ONECATCH_KEEL_REPLAY_RUN")
	if root == "" {
		t.Skip("set ONECATCH_KEEL_REPLAY_RUN to replay stored history")
	}
	a := conversationFixture(t)
	a.transcript = newTranscript()
	a.detail = &desktop.RunDetail{}
	read := func(path string, value any) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(data, value); err != nil {
			t.Fatal(err)
		}
	}
	read(filepath.Join(root, "workflow.json"), &a.detail.Workflow)
	read(filepath.Join(root, "run.json"), &a.detail.Run)
	paths, err := filepath.Glob(filepath.Join(root, "steps", "*", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		var step workflows.StepRun
		read(path, &step)
		a.detail.StepRuns = append(a.detail.StepRuns, step)
		file, err := os.Open(filepath.Join(filepath.Dir(path), "events.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		scan := bufio.NewScanner(file)
		scan.Buffer(make([]byte, 4096), 8<<20)
		for scan.Scan() {
			var record struct {
				Seq     int64          `json:"seq"`
				Payload agentrun.Event `json:"payload"`
			}
			if err := json.Unmarshal(scan.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			record.Payload.Revision = uint64(record.Seq)
			a.transcript.frame(runstream.Frame{StepRunID: step.ID, Seq: record.Seq, Event: record.Payload})
		}
		err = scan.Err()
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	a.syncMessages()
	columns := 0
	orphaned := 0
	for _, row := range a.displayedConversationRows() {
		if len(row.columns) > 0 {
			columns++
		}
	}
	for _, row := range a.entries {
		for _, item := range row.process {
			if item.kind == agentrun.KindToolResult {
				orphaned++
			}
		}
	}
	if columns == 0 {
		t.Fatal("parallel history did not create columns")
	}
	t.Logf("replayed %d executions, %d display rows, %d parallel rows, %d unpaired results", len(paths), len(a.displayedConversationRows()), columns, orphaned)
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	rows := a.displayedConversationRows()
	view := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		box := el.Div().WFull().HFull().P(20)
		for i := 0; i < min(6, len(rows)); i++ {
			box.Child(a.message(cx, i))
		}
		return box
	}))
	if err := window.Screenshot(screenshotRoot{core.Func(view.Layout)}, 1288, 900, filepath.Join(dir, "workflow-replay.png")); err != nil {
		t.Fatal(err)
	}
}
