package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalAccountUsageReadersAggregateHarnessSessions(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.Local)
	environment := []string{"HOME=" + home}

	writeUsageFixture(t, filepath.Join(home, ".claude", "stats-cache.json"), `{
  "lastComputedDate":"2026-09-02",
  "dailyModelTokens":[
    {"date":"2026-09-01","tokensByModel":{"claude-opus":100}},
    {"date":"2026-09-02","tokensByModel":{"claude-opus":999}}
  ]
}`)
	claudeLine := `{"uuid":"entry-1","timestamp":"2026-09-02T08:00:00Z","type":"assistant","message":{"id":"msg-1","role":"assistant","usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":5,"cache_creation_input_tokens":3}}}`
	writeUsageFixture(t, filepath.Join(home, ".claude", "projects", "project", "session.jsonl"), claudeLine+"\n"+claudeLine+"\n")

	writeUsageFixture(t, filepath.Join(home, ".pi", "agent", "sessions", "project", "session.jsonl"), `{"id":"pi-1","timestamp":"2026-09-03T08:00:00Z","type":"message","message":{"role":"assistant","model":"provider/model","usage":{"input":20,"output":4,"cacheRead":6,"cacheWrite":0,"totalTokens":30}}}`+"\n")

	writeUsageFixture(t, filepath.Join(home, ".grok", "sessions", "project", "session", "updates.jsonl"), `{"timestamp":1788422400,"method":"_x.ai/session/update","params":{"sessionId":"grok-session","update":{"sessionUpdate":"turn_completed","prompt_id":"prompt-1","usage":{"inputTokens":30,"outputTokens":5,"totalTokens":35,"cachedReadTokens":10}}}}`+"\n")

	writeUsageFixture(t, filepath.Join(home, ".modu", "sessions", "project", "session.jsonl"), `{"id":"modu-1","timestamp":"2026-09-04T08:00:00Z","type":"message","message":{"role":"assistant","model":"provider/model","usage":{"input":40,"output":6,"cacheRead":4,"cacheWrite":0,"totalTokens":50}}}`+"\n")

	tests := []struct {
		name    string
		reader  AccountUsageReader
		runtime Runtime
		want    map[string]int64
	}{
		{"claude", &ClaudeRunner{now: func() time.Time { return now }}, RuntimeClaude, map[string]int64{"2026-09-01": 100, "2026-09-02": 20}},
		{"pi", &PiRunner{now: func() time.Time { return now }}, RuntimePi, map[string]int64{"2026-09-03": 30}},
		{"grok", &GrokRunner{now: func() time.Time { return now }}, RuntimeGrok, map[string]int64{"2026-09-03": 35}},
		{"modu", &ModuRunner{now: func() time.Time { return now }}, RuntimeModu, map[string]int64{"2026-09-04": 50}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usage, err := test.reader.ReadAccountUsage(context.Background(), home, environment)
			if err != nil {
				t.Fatal(err)
			}
			if usage.Runtime != test.runtime || usage.Scope != AccountUsageScopeDevice {
				t.Fatalf("identity = (%q, %q), want (%q, device)", usage.Runtime, usage.Scope, test.runtime)
			}
			got := make(map[string]int64)
			for _, bucket := range usage.DailyUsage {
				got[bucket.StartDate] = bucket.Tokens
			}
			for day, tokens := range test.want {
				if got[day] != tokens {
					t.Fatalf("daily usage = %+v, want %s=%d", got, day, tokens)
				}
			}
			if usage.Summary.LifetimeTokens == nil || *usage.Summary.LifetimeTokens == 0 {
				t.Fatalf("summary = %+v", usage.Summary)
			}
		})
	}
}

func TestLocalAccountUsageReaderHonorsConfiguredDirectories(t *testing.T) {
	home := t.TempDir()
	piSessions := filepath.Join(home, "pi-sessions")
	grokHome := filepath.Join(home, "grok-home")
	moduHome := filepath.Join(home, "modu-home")
	writeUsageFixture(t, filepath.Join(piSessions, "session.jsonl"), `{"id":"p","timestamp":"2026-09-04T01:00:00Z","type":"message","message":{"role":"assistant","usage":{"totalTokens":11}}}`+"\n")
	writeUsageFixture(t, filepath.Join(grokHome, "sessions", "x", "updates.jsonl"), `{"timestamp":1788483600,"params":{"sessionId":"g","update":{"sessionUpdate":"turn_completed","prompt_id":"p","usage":{"totalTokens":12}}}}`+"\n")
	writeUsageFixture(t, filepath.Join(moduHome, "sessions", "x.jsonl"), `{"id":"m","timestamp":"2026-09-04T01:00:00Z","type":"message","message":{"role":"assistant","usage":{"totalTokens":13}}}`+"\n")

	pi, err := NewPiRunner("").ReadAccountUsage(context.Background(), home, []string{"HOME=" + home, "PI_CODING_AGENT_SESSION_DIR=" + piSessions})
	if err != nil || len(pi.DailyUsage) != 1 || pi.DailyUsage[0].Tokens != 11 {
		t.Fatalf("Pi configured sessions = %+v, %v", pi, err)
	}
	grok, err := NewGrokRunner("").ReadAccountUsage(context.Background(), home, []string{"HOME=" + home, "GROK_HOME=" + grokHome})
	if err != nil || len(grok.DailyUsage) != 1 || grok.DailyUsage[0].Tokens != 12 {
		t.Fatalf("Grok configured home = %+v, %v", grok, err)
	}
	modu := NewModuRunner("")
	modu.agentDir = moduHome
	moduUsage, err := modu.ReadAccountUsage(context.Background(), home, []string{"HOME=" + home})
	if err != nil || len(moduUsage.DailyUsage) != 1 || moduUsage.DailyUsage[0].Tokens != 13 {
		t.Fatalf("Modu configured home = %+v, %v", moduUsage, err)
	}
}

func writeUsageFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
