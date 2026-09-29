package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTraeAccountUsageUsesOwnEndpoint(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "requests.jsonl")
	t.Setenv("ONECATCH_TRAE_CAPTURE", capture)
	t.Setenv("ONECATCH_TRAE_USAGE", `{"id":2,"result":{"usage":{"usage_percent":25,"quota_remaining":225,"quota_limit":300,"is_depleted":false,"reset_time":1791129599}}}`)
	t.Setenv("HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	usage, err := NewTraeRunner(stubTraeAppServer(t)).ReadAccountUsage(ctx, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if usage.Runtime != RuntimeTrae || usage.Scope != AccountUsageScopeMixed || len(usage.RateLimits) != 1 {
		t.Fatalf("usage: %+v", usage)
	}
	limit := usage.RateLimits[0]
	if limit.Primary.UsedPercent != 25 || limit.Primary.WindowDurationMins != 10080 || limit.Primary.ResetsAt != 1791129599 || *limit.QuotaRemaining != 225 || *limit.QuotaLimit != 300 {
		t.Fatalf("limit: %+v", limit)
	}
	if len(usage.DailyUsage) != 0 || usage.Summary.LifetimeTokens == nil || *usage.Summary.LifetimeTokens != 0 {
		t.Fatalf("quota must not become tokens: %+v", usage)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "model/internalUsage/read") || strings.Contains(string(data), "account/") || strings.Contains(string(data), "turn/start") {
		t.Fatalf("unexpected requests: %s", data)
	}
}

func TestTraeAccountUsageMissingAndZero(t *testing.T) {
	for _, raw := range []string{`{}`, `{"usage":null}`, `invalid`} {
		if _, err := decodeTraeAccountUsage([]byte(raw), time.Now()); err == nil {
			t.Fatalf("expected unavailable/error for %s", raw)
		}
	}
	usage, err := decodeTraeAccountUsage([]byte(`{"usage":{"usage_percent":100,"quota_remaining":0,"quota_limit":300,"is_depleted":true}}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	limit := usage.RateLimits[0]
	if limit.QuotaRemaining == nil || *limit.QuotaRemaining != 0 || !*limit.SpendControlReached {
		t.Fatalf("zero/depletion lost: %+v", limit)
	}
	usage, err = decodeTraeAccountUsage([]byte(`{"usage":{"quota_remaining":null,"quota_limit":null}}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	limit = usage.RateLimits[0]
	if limit.Primary != nil || limit.QuotaRemaining != nil || limit.QuotaLimit != nil || limit.SpendControlReached != nil {
		t.Fatalf("missing became zero: %+v", limit)
	}
}

func TestLiveTraeAccountUsage(t *testing.T) {
	if os.Getenv("ONECATCH_LIVE") != "1" {
		t.Skip("set ONECATCH_LIVE=1 to read the installed TRAE account")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	usage, err := NewTraeRunner("traecli").ReadAccountUsage(ctx, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if usage.Runtime != RuntimeTrae || len(usage.RateLimits) != 1 || usage.RateLimits[0].Primary == nil {
		t.Fatalf("missing weekly usage: %+v", usage)
	}
	t.Logf("weekly usage: %d%%; daily records: %d", usage.RateLimits[0].Primary.UsedPercent, len(usage.DailyUsage))
}

func TestTraeQuotaUnavailableKeepsLocalHistory(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".trae", "cli", "sessions")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	record := `{"type":"event_msg","timestamp":"2026-09-29T05:38:12Z","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"output_tokens":20,"cached_input_tokens":40}}}}`
	if err := os.WriteFile(filepath.Join(root, "session.jsonl"), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("ONECATCH_TRAE_USAGE", `{"id":2,"result":{"usage":null}}`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	usage, err := NewTraeRunner(stubTraeAppServer(t)).ReadAccountUsage(ctx, t.TempDir(), os.Environ())
	if err != nil || usage.Warning == "" || len(usage.DailyUsage) != 1 || usage.DailyUsage[0].Tokens != 120 || usage.Scope != AccountUsageScopeDevice {
		t.Fatalf("local fallback: %+v %v", usage, err)
	}
}
