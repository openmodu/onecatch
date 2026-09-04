package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ReadAccountUsage combines Claude Code's locally generated stats snapshot
// with newer transcript records. The stats file avoids rescanning old project
// history, while the transcript tail keeps manual sync useful even when the
// interactive /stats screen has not been opened recently.
func (r *ClaudeRunner) ReadAccountUsage(ctx context.Context, _ string, environment []string) (AccountUsage, error) {
	home := usageHome(environment)
	root := expandUsagePath(usageEnvironmentValue(environment, "CLAUDE_CONFIG_DIR"), home)
	if root == "" {
		root = filepath.Join(home, ".claude")
	}
	collector := newLocalUsageCollector()
	cutoff, statsLoaded := readClaudeStats(filepath.Join(root, "stats-cache.json"), collector)
	err := scanUsageJSONL(ctx, filepath.Join(root, "projects"), cutoff, "", func(_ string, line string) {
		var entry struct {
			UUID      string `json:"uuid"`
			Timestamp string `json:"timestamp"`
			Type      string `json:"type"`
			Message   struct {
				ID    string `json:"id"`
				Role  string `json:"role"`
				Usage struct {
					Input         int64 `json:"input_tokens"`
					Output        int64 `json:"output_tokens"`
					CacheRead     int64 `json:"cache_read_input_tokens"`
					CacheCreation int64 `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &entry) != nil || entry.Type != "assistant" || entry.Message.Role != "assistant" {
			return
		}
		at, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if err != nil || !cutoff.IsZero() && at.Before(cutoff) {
			return
		}
		id := entry.Message.ID
		if id == "" {
			id = entry.UUID
		}
		collector.add("claude:"+id, at, entry.Message.Usage.Input+entry.Message.Usage.Output+entry.Message.Usage.CacheRead+entry.Message.Usage.CacheCreation)
	})
	if err != nil {
		return AccountUsage{}, fmt.Errorf("read Claude Code local usage: %w", err)
	}
	source := "local-sessions"
	if statsLoaded {
		source = "local-stats-and-sessions"
	}
	return localAccountUsage(RuntimeClaude, source, r.now(), collector.days), nil
}

func readClaudeStats(path string, collector *localUsageCollector) (time.Time, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, false
	}
	var stats struct {
		LastComputedDate string `json:"lastComputedDate"`
		DailyModelTokens []struct {
			Date          string           `json:"date"`
			TokensByModel map[string]int64 `json:"tokensByModel"`
		} `json:"dailyModelTokens"`
	}
	if json.Unmarshal(data, &stats) != nil {
		return time.Time{}, false
	}
	cutoff, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(stats.LastComputedDate), time.Local)
	if err != nil {
		return time.Time{}, false
	}
	for _, bucket := range stats.DailyModelTokens {
		if bucket.Date >= stats.LastComputedDate {
			continue
		}
		var tokens int64
		for _, count := range bucket.TokensByModel {
			tokens += count
		}
		collector.merge(bucket.Date, tokens)
	}
	return cutoff, true
}
