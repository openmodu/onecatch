package agentrun

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type traeLocalTokens struct {
	Input     int64 `json:"input_tokens"`
	Output    int64 `json:"output_tokens"`
	Read      int64 `json:"cached_input_tokens"`
	Write     int64 `json:"cache_creation_input_tokens"`
	Reasoning int64 `json:"reasoning_output_tokens"`
}
type traeUsageSample struct {
	at     time.Time
	tokens traeLocalTokens
}

// TRAE's total_token_usage is cumulative across a session. Only positive
// increments count; last_token_usage can also contain context estimates and
// must not be summed as if every notification were a new model request.
func readTraeLocalUsage(ctx context.Context, environment []string, now time.Time) (AccountUsage, error) {
	if err := ctx.Err(); err != nil {
		return AccountUsage{}, err
	}
	root := filepath.Join(usageHome(environment), ".trae", "cli", "sessions")
	sessions := map[string][]traeUsageSample{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == root && os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), maxLocalUsageLineBytes)
		session := filepath.Base(path)
		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				return err
			}
			var record struct {
				Type      string    `json:"type"`
				Timestamp time.Time `json:"timestamp"`
				Payload   struct {
					ID   string `json:"id"`
					Type string `json:"type"`
					Info *struct {
						Total *traeLocalTokens `json:"total_token_usage"`
					} `json:"info"`
				} `json:"payload"`
			}
			if json.Unmarshal(scanner.Bytes(), &record) != nil {
				continue
			}
			if record.Type == "session_meta" && record.Payload.ID != "" {
				session = record.Payload.ID
			}
			if record.Type != "event_msg" || record.Payload.Type != "token_count" || record.Payload.Info == nil || record.Payload.Info.Total == nil || record.Timestamp.IsZero() {
				continue
			}
			sessions[session] = append(sessions[session], traeUsageSample{record.Timestamp, *record.Payload.Info.Total})
		}
		return scanner.Err()
	})
	if err != nil {
		return AccountUsage{}, fmt.Errorf("read TRAE local token history: %w", err)
	}
	days := map[string]int64{}
	details := map[string]*AccountTokenBreakdown{}
	// Forked/copied histories preserve timestamps and counters. Do not charge
	// the same historical sample again when another file includes it.
	seen := map[traeUsageSample]bool{}
	for _, samples := range sessions {
		sort.SliceStable(samples, func(i, j int) bool { return samples[i].at.Before(samples[j].at) })
		var previous traeLocalTokens
		for _, sample := range samples {
			current := sample.tokens
			delta := AccountTokenBreakdown{
				Input: max(0, current.Input-previous.Input), Output: max(0, current.Output-previous.Output),
				CacheRead: max(0, current.Read-previous.Read), CacheWrite: max(0, current.Write-previous.Write), Reasoning: max(0, current.Reasoning-previous.Reasoning),
			}
			previous = traeLocalTokens{max(previous.Input, current.Input), max(previous.Output, current.Output), max(previous.Read, current.Read), max(previous.Write, current.Write), max(previous.Reasoning, current.Reasoning)}
			if seen[sample] {
				continue
			}
			seen[sample] = true
			if delta.Input+delta.Output == 0 {
				continue
			}
			day := sample.at.In(time.Local).Format("2006-01-02")
			days[day] += delta.Input + delta.Output
			if details[day] == nil {
				details[day] = &AccountTokenBreakdown{}
			}
			d := details[day]
			d.Input += delta.Input
			d.Output += delta.Output
			d.CacheRead += delta.CacheRead
			d.CacheWrite += delta.CacheWrite
			d.Reasoning += delta.Reasoning
		}
	}
	usage := localAccountUsage(RuntimeTrae, "local-sessions", now, days)
	usage.DailyUsageScope = AccountUsageScopeDevice
	for i := range usage.DailyUsage {
		usage.DailyUsage[i].Breakdown = details[usage.DailyUsage[i].StartDate]
	}
	return usage, nil
}
