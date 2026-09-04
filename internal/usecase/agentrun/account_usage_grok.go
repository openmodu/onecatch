package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

func (r *GrokRunner) ReadAccountUsage(ctx context.Context, _ string, environment []string) (AccountUsage, error) {
	home := usageHome(environment)
	grokHome := expandUsagePath(usageEnvironmentValue(environment, "GROK_HOME"), home)
	if grokHome == "" {
		grokHome = filepath.Join(home, ".grok")
	}
	collector := newLocalUsageCollector()
	err := scanUsageJSONL(ctx, filepath.Join(grokHome, "sessions"), time.Time{}, "updates.jsonl", func(_ string, line string) {
		var entry struct {
			Timestamp int64  `json:"timestamp"`
			Method    string `json:"method"`
			Params    struct {
				SessionID string `json:"sessionId"`
				Update    struct {
					SessionUpdate string `json:"sessionUpdate"`
					PromptID      string `json:"prompt_id"`
					Usage         struct {
						Input      int64 `json:"inputTokens"`
						Output     int64 `json:"outputTokens"`
						CacheRead  int64 `json:"cachedReadTokens"`
						CacheWrite int64 `json:"cacheCreationTokens"`
						Total      int64 `json:"totalTokens"`
					} `json:"usage"`
				} `json:"update"`
			} `json:"params"`
		}
		if json.Unmarshal([]byte(line), &entry) != nil || entry.Params.Update.SessionUpdate != "turn_completed" {
			return
		}
		seconds := entry.Timestamp
		if seconds > 10_000_000_000 {
			seconds /= 1000
		}
		id := entry.Params.Update.PromptID
		if id == "" {
			id = entry.Params.SessionID + ":" + fmt.Sprint(entry.Timestamp)
		}
		tokens := localUsageTokens(entry.Params.Update.Usage.Total, entry.Params.Update.Usage.Input, entry.Params.Update.Usage.Output, entry.Params.Update.Usage.CacheRead, entry.Params.Update.Usage.CacheWrite)
		collector.add("grok:"+id, time.Unix(seconds, 0), tokens)
	})
	if err != nil {
		return AccountUsage{}, fmt.Errorf("read Grok local usage: %w", err)
	}
	return localAccountUsage(RuntimeGrok, "local-sessions", r.now(), collector.days), nil
}
