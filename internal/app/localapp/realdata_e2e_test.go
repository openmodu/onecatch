package localapp

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/openmodu/oneshot/internal/gitinspect"
	localdata "github.com/openmodu/oneshot/internal/data/local"
	"github.com/openmodu/oneshot/internal/workspacelock"
	workflowuc "github.com/openmodu/oneshot/internal/usecase/workflows"
)

// TestRealDataOpenRunCost measures opening every run in a real ~/.oneshot copy:
// wall time for GetRunDetail plus the size of the JSON payload that actually
// crosses the Wails bridge to the webview. Set ONESHOT_E2E_ROOT to a COPY of a
// data dir; the test is skipped otherwise so CI stays hermetic.
func TestRealDataOpenRunCost(t *testing.T) {
	root := os.Getenv("ONESHOT_E2E_ROOT")
	if root == "" {
		t.Skip("set ONESHOT_E2E_ROOT to a copy of a real ~/.oneshot to run this")
	}
	store, err := localdata.OpenStore(root)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	runtimes, err := NewRuntimeRegistry(root)
	if err != nil {
		t.Fatalf("runtime registry: %v", err)
	}
	orchestrator := workflowuc.NewUsecase(store.Repos.Tasks, store.Repos.Workflows, completingEngine{}, workspacelock.New(store.Data.Paths.Locks), gitinspect.New(""))
	app := New(store, orchestrator, runtimes, gitinspect.New(""))
	defer app.Close()
	ctx := context.Background()

	workspaces, err := app.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	type row struct {
		runID    string
		events   int
		rounds   int
		loaded   int
		trunc    bool
		bounded  time.Duration
		unbound  time.Duration
		boundedB int
		unboundB int
	}
	var rows []row
	original := transcriptByteBudget
	defer func() { transcriptByteBudget = original }()

	for _, workspace := range workspaces {
		page, err := app.ListRuns(ctx, ListRunsInput{WorkspaceID: workspace.ID, Limit: 200})
		if err != nil {
			continue
		}
		for _, item := range page.Items {
			measure := func(budget int) (time.Duration, int, RunDetail) {
				transcriptByteBudget = budget
				// Warm the page cache so we measure steady-state, not first read.
				if _, err := app.GetRunDetail(ctx, item.Run.ID); err != nil {
					t.Fatalf("GetRunDetail warmup %s: %v", item.Run.ID, err)
				}
				best := time.Hour
				var detail RunDetail
				for range 5 {
					start := time.Now()
					d, err := app.GetRunDetail(ctx, item.Run.ID)
					elapsed := time.Since(start)
					if err != nil {
						t.Fatalf("GetRunDetail %s: %v", item.Run.ID, err)
					}
					if elapsed < best {
						best, detail = elapsed, d
					}
				}
				payload, err := json.Marshal(detail)
				if err != nil {
					t.Fatalf("marshal detail: %v", err)
				}
				return best, len(payload), detail
			}
			budget := original
			if override := os.Getenv("ONESHOT_E2E_BUDGET"); override != "" {
				parsed, err := strconv.Atoi(override)
				if err != nil {
					t.Fatalf("bad ONESHOT_E2E_BUDGET: %v", err)
				}
				budget = parsed
			}
			bDur, bBytes, bDetail := measure(budget)
			uDur, uBytes, uDetail := measure(1 << 30)
			if dir := os.Getenv("ONESHOT_E2E_DUMP"); dir != "" {
				payload, _ := json.Marshal(uDetail)
				if err := os.WriteFile(dir+"/"+item.Run.ID+".json", payload, 0o600); err != nil {
					t.Fatalf("dump payload: %v", err)
				}
			}
			rows = append(rows, row{
				runID: item.Run.ID[:12], events: len(uDetail.RuntimeEvents), rounds: len(uDetail.StepRuns),
				loaded: len(bDetail.LoadedStepRunIDs), trunc: bDetail.TranscriptTruncated,
				bounded: bDur, unbound: uDur, boundedB: bBytes, unboundB: uBytes,
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].unboundB > rows[j].unboundB })

	t.Log("REAL DATA — cost of opening a run (best of 5, warm cache)")
	t.Log("run            rounds  events | bounded payload  time | full payload   time | truncated")
	t.Log("---------------------------------------------------------------------------------------")
	var sumB, sumU int
	for _, r := range rows {
		sumB += r.boundedB
		sumU += r.unboundB
		t.Logf("%-14s %5d %7d | %9s %8s | %9s %8s | %v",
			r.runID, r.rounds, r.events,
			humanBytes(r.boundedB), r.bounded.Round(100*time.Microsecond),
			humanBytes(r.unboundB), r.unbound.Round(100*time.Microsecond), r.trunc)
	}
	t.Logf("TOTAL payload across %d runs: bounded %s vs full %s", len(rows), humanBytes(sumB), humanBytes(sumU))
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return formatFloat(float64(n)/(1<<20)) + "MB"
	case n >= 1<<10:
		return formatFloat(float64(n)/(1<<10)) + "KB"
	default:
		return formatFloat(float64(n)) + "B"
	}
}

func formatFloat(value float64) string {
	out, _ := json.Marshal(float64(int(value*10)) / 10)
	return string(out)
}
