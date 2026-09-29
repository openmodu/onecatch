package agentrun

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTraeLocalDailyBreakdown(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".trae", "cli", "sessions")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	first := time.Date(2026, 9, 27, 23, 59, 0, 0, time.Local)
	second := first.Add(2 * time.Minute)
	sample := func(at time.Time, input, output, read, write, reasoning int64) string {
		record := map[string]any{"type": "event_msg", "timestamp": at, "payload": map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": traeLocalTokens{input, output, read, write, reasoning}}}}
		raw, _ := json.Marshal(record)
		return string(raw) + "\n"
	}
	a := sample(first, 100, 20, 30, 50, 5)
	b := sample(second, 180, 30, 90, 60, 8)
	data := "{\"type\":\"session_meta\",\"payload\":{\"id\":\"session\"}}\n" + a + a + b + "{broken\n"
	for _, name := range []string{"original.jsonl", "copy.jsonl"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// A fork includes its parent's history before its own new request.
	fork := "{\"type\":\"session_meta\",\"payload\":{\"id\":\"fork\"}}\n" + a + b + sample(second.Add(time.Minute), 200, 35, 100, 60, 9)
	if err := os.WriteFile(filepath.Join(root, "fork.jsonl"), []byte(fork), 0600); err != nil {
		t.Fatal(err)
	}
	usage, err := readTraeLocalUsage(context.Background(), []string{"HOME=" + home}, second)
	if err != nil {
		t.Fatal(err)
	}
	if usage.DailyUsageScope != AccountUsageScopeDevice || len(usage.DailyUsage) != 2 || *usage.Summary.LifetimeTokens != 235 {
		t.Fatalf("usage: %+v", usage)
	}
	want := []AccountTokenBreakdown{{Input: 100, Output: 20, CacheRead: 30, CacheWrite: 50, Reasoning: 5}, {Input: 100, Output: 15, CacheRead: 70, CacheWrite: 10, Reasoning: 4}}
	for i, day := range usage.DailyUsage {
		if day.Breakdown == nil || *day.Breakdown != want[i] || day.Tokens != want[i].Input+want[i].Output {
			t.Fatalf("day %d: %+v / %+v", i, day, day.Breakdown)
		}
	}
}

func TestTraeLocalUsageEmptyAndCancelled(t *testing.T) {
	env := []string{"HOME=" + t.TempDir()}
	usage, err := readTraeLocalUsage(context.Background(), env, time.Now())
	if err != nil || len(usage.DailyUsage) != 0 {
		t.Fatalf("missing history: %+v %v", usage, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = readTraeLocalUsage(ctx, env, time.Now())
	if err == nil {
		t.Fatal("cancelled scan succeeded")
	}
}
