package localapp

import (
	"context"
	"fmt"
	"math"
	"testing"
)

// benchSeed builds a run of the given shape once and reuses it across iterations.
func benchSeed(b *testing.B, rounds, eventsPerRound int) (*App, string) {
	b.Helper()
	// seedRunWithRounds wants a *testing.T; it only uses TempDir/Fatal, which a
	// throwaway harness provides.
	t := &testing.T{}
	app, _ := newLocalTestApp(t, completingEngine{})
	runID := seedRunWithRounds(t, app, rounds, eventsPerRound)
	if t.Failed() {
		b.Fatal("seeding the run failed")
	}
	return app, runID
}

// BenchmarkGetRunDetail measures opening a run through the identical code path,
// switching only the transcript budget: the bounded read that ships today versus
// the unbounded read it replaced. Everything else GetRunDetail does (run,
// workflow, task, workspace, step runs, workflow events, instructions) is
// included in both arms, so the delta is attributable to the transcript alone.
func BenchmarkGetRunDetail(b *testing.B) {
	shapes := []struct{ rounds, events int }{
		{5, 100},  // 500 events
		{15, 200}, // 3k events
		{30, 300}, // 9k events
	}
	original := transcriptEventBudget
	b.Cleanup(func() { transcriptEventBudget = original })

	for _, shape := range shapes {
		app, runID := benchSeed(b, shape.rounds, shape.events)
		ctx := context.Background()
		name := fmt.Sprintf("%drounds_x%devents", shape.rounds, shape.events)

		b.Run("bounded/"+name, func(b *testing.B) {
			transcriptEventBudget = original
			b.ReportAllocs()
			for b.Loop() {
				if _, err := app.GetRunDetail(ctx, runID); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("unbounded/"+name, func(b *testing.B) {
			transcriptEventBudget = math.MaxInt32 // the pre-pagination behaviour
			b.ReportAllocs()
			for b.Loop() {
				if _, err := app.GetRunDetail(ctx, runID); err != nil {
					b.Fatal(err)
				}
			}
		})
		transcriptEventBudget = original
	}
}
