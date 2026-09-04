package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

func (r *ModuRunner) ReadAccountUsage(ctx context.Context, _ string, environment []string) (AccountUsage, error) {
	return readModuLocalAccountUsage(ctx, r.agentDir, environment, r.now())
}

func readModuLocalAccountUsage(ctx context.Context, agentDir string, environment []string, now time.Time) (AccountUsage, error) {
	home := usageHome(environment)
	agentDir = expandUsagePath(agentDir, home)
	if agentDir == "" {
		agentDir = filepath.Join(home, ".modu")
	}
	collector := newLocalUsageCollector()
	err := scanUsageJSONL(ctx, filepath.Join(agentDir, "sessions"), time.Time{}, "", func(_ string, line string) {
		var entry localAssistantUsageEntry
		if json.Unmarshal([]byte(line), &entry) != nil || entry.Type != "message" || entry.Message.Role != "assistant" {
			return
		}
		at, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if err != nil {
			return
		}
		tokens := localUsageTokens(entry.Message.Usage.Total, entry.Message.Usage.Input, entry.Message.Usage.Output, entry.Message.Usage.CacheRead, entry.Message.Usage.CacheWrite)
		collector.add("modu:"+entry.ID+":"+entry.Timestamp+":"+entry.Message.Model, at, tokens)
	})
	if err != nil {
		return AccountUsage{}, fmt.Errorf("read Modu local usage: %w", err)
	}
	return localAccountUsage(RuntimeModu, "local-sessions", now, collector.days), nil
}
