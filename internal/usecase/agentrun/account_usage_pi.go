package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

func (r *PiRunner) ReadAccountUsage(ctx context.Context, _ string, environment []string) (AccountUsage, error) {
	home := usageHome(environment)
	root := expandUsagePath(usageEnvironmentValue(environment, "PI_CODING_AGENT_SESSION_DIR"), home)
	if root == "" {
		agentDir := expandUsagePath(usageEnvironmentValue(environment, "PI_CODING_AGENT_DIR"), home)
		if agentDir == "" {
			agentDir = filepath.Join(home, ".pi", "agent")
		}
		root = filepath.Join(agentDir, "sessions")
	}
	collector := newLocalUsageCollector()
	err := scanUsageJSONL(ctx, root, time.Time{}, "", func(_ string, line string) {
		var entry localAssistantUsageEntry
		if json.Unmarshal([]byte(line), &entry) != nil || entry.Type != "message" || entry.Message.Role != "assistant" {
			return
		}
		at, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if err != nil {
			return
		}
		tokens := localUsageTokens(entry.Message.Usage.Total, entry.Message.Usage.Input, entry.Message.Usage.Output, entry.Message.Usage.CacheRead, entry.Message.Usage.CacheWrite)
		collector.add("pi:"+entry.ID+":"+entry.Timestamp+":"+entry.Message.Model, at, tokens)
	})
	if err != nil {
		return AccountUsage{}, fmt.Errorf("read Pi local usage: %w", err)
	}
	return localAccountUsage(RuntimePi, "local-sessions", r.now(), collector.days), nil
}

type localAssistantUsageEntry struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Message   struct {
		Role  string `json:"role"`
		Model string `json:"model"`
		Usage struct {
			Input      int64 `json:"input"`
			Output     int64 `json:"output"`
			CacheRead  int64 `json:"cacheRead"`
			CacheWrite int64 `json:"cacheWrite"`
			Total      int64 `json:"totalTokens"`
		} `json:"usage"`
	} `json:"message"`
}
