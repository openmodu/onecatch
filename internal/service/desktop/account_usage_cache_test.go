package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestAccountUsageCachePersistsAndExpiresAfterThirtyMinutes(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	cache := newAccountUsageCache(root)
	cache.now = func() time.Time { return now }
	usage := agentrun.AccountUsage{
		Runtime:    agentrun.RuntimeCodex,
		FetchedAt:  now,
		DailyUsage: []agentrun.AccountDailyUsage{{StartDate: "2026-09-03", Tokens: 42}},
	}
	if err := cache.put(agentrun.RuntimeCodex, usage); err != nil {
		t.Fatal(err)
	}

	restarted := newAccountUsageCache(root)
	restarted.now = func() time.Time { return now.Add(29 * time.Minute) }
	got, ok, fresh := restarted.get(agentrun.RuntimeCodex, accountUsageSyncInterval)
	if !ok || !fresh || len(got.DailyUsage) != 1 || got.DailyUsage[0].Tokens != 42 {
		t.Fatalf("restarted cache = %+v, ok=%v fresh=%v", got, ok, fresh)
	}
	restarted.now = func() time.Time { return now.Add(30 * time.Minute) }
	if _, ok, fresh := restarted.get(agentrun.RuntimeCodex, accountUsageSyncInterval); !ok || fresh {
		t.Fatalf("30-minute entry should remain available but stale: ok=%v fresh=%v", ok, fresh)
	}
	info, err := os.Stat(filepath.Join(root, accountUsageCacheFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("cache permissions = %o", info.Mode().Perm())
	}
}

type cachedUsageRunner struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (r *cachedUsageRunner) Runtime() agentrun.Runtime { return agentrun.RuntimeCodex }
func (r *cachedUsageRunner) Available() bool           { return true }
func (r *cachedUsageRunner) Run(context.Context, agentrun.Request, agentrun.Sink) (agentrun.Result, error) {
	return agentrun.Result{}, nil
}
func (r *cachedUsageRunner) ReadAccountUsage(context.Context, string, []string) (agentrun.AccountUsage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return agentrun.AccountUsage{}, r.err
	}
	return agentrun.AccountUsage{
		Runtime:    agentrun.RuntimeCodex,
		FetchedAt:  time.Date(2026, 9, 4, 12, r.calls, 0, 0, time.UTC),
		DailyUsage: []agentrun.AccountDailyUsage{{StartDate: "2026-09-03", Tokens: int64(r.calls)}},
	}, nil
}
func (r *cachedUsageRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}
func (r *cachedUsageRunner) fail(err error) {
	r.mu.Lock()
	r.err = err
	r.mu.Unlock()
}

func TestAccountUsageReadCachesAndManualSyncBypassesCache(t *testing.T) {
	app, _ := newLocalTestApp(t, completingEngine{})
	runner := &cachedUsageRunner{}
	app.runtimes.mu.Lock()
	app.runtimes.engine = agentrun.NewEngineWithRunners(runner)
	app.runtimes.mu.Unlock()
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	app.accountUsageCache.now = func() time.Time { return now }

	first, err := app.GetAccountUsage(context.Background(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.GetAccountUsage(context.Background(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	if runner.callCount() != 1 || first.DailyUsage[0].Tokens != 1 || second.DailyUsage[0].Tokens != 1 {
		t.Fatalf("ordinary reads did not share cache: calls=%d first=%+v second=%+v", runner.callCount(), first, second)
	}
	manual, err := app.SyncAccountUsage(context.Background(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	if runner.callCount() != 2 || manual.DailyUsage[0].Tokens != 2 {
		t.Fatalf("manual sync did not bypass cache: calls=%d usage=%+v", runner.callCount(), manual)
	}

	// Automatic reads can fall back to a stale persisted snapshot when Codex is
	// temporarily offline; an explicit sync still reports that failure.
	now = now.Add(31 * time.Minute)
	runner.fail(errors.New("offline"))
	stale, err := app.GetAccountUsage(context.Background(), "codex")
	if err != nil || stale.DailyUsage[0].Tokens != 2 {
		t.Fatalf("stale fallback = %+v, %v", stale, err)
	}
	if _, err := app.SyncAccountUsage(context.Background(), "codex"); err == nil {
		t.Fatal("manual sync should surface the provider failure")
	}
}
