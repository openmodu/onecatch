package agentrun

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CodexToolHistory contains authoritative completed items from Codex's local
// rollout. Reading it never resumes a thread or executes a tool.
type CodexToolHistory struct {
	ID         string
	Command    []string
	Text       string
	Output     string
	At         time.Time
	FinishedAt time.Time
}

func ReadCodexToolHistory(session string) []CodexToolHistory {
	if session == "" || strings.ContainsAny(session, "/\\*?[]") {
		return nil
	}
	root := os.Getenv("CODEX_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		root = filepath.Join(home, ".codex")
	}
	var result []CodexToolHistory
	for _, folder := range []string{"sessions", "archived_sessions"} {
		pattern := filepath.Join(root, folder, "*", "*", "*", "*"+session+".jsonl")
		if folder == "archived_sessions" {
			pattern = filepath.Join(root, folder, "*"+session+".jsonl")
		}
		files, _ := filepath.Glob(pattern)
		for _, name := range files {
			file, err := os.Open(name)
			if err != nil {
				continue
			}
			result = append(result, readCodexToolHistory(file)...)
			file.Close()
		}
	}
	return result
}

func readCodexToolHistory(reader io.Reader) []CodexToolHistory {
	scan := bufio.NewScanner(reader)
	scan.Buffer(make([]byte, 4096), 16<<20)
	var result []CodexToolHistory
	for scan.Scan() {
		var row struct {
			Type      string    `json:"type"`
			Timestamp time.Time `json:"timestamp"`
			Payload   struct {
				Type string `json:"type"`
				Item struct {
					ID       string          `json:"id"`
					Type     string          `json:"type"`
					Command  json.RawMessage `json:"command"`
					Duration struct {
						Secs  int64 `json:"secs"`
						Nanos int64 `json:"nanos"`
					} `json:"duration"`
					AggregatedOutput string          `json:"aggregated_output"`
					Output           string          `json:"aggregatedOutput"`
					Tool             string          `json:"tool"`
					Server           string          `json:"server"`
					Arguments        json.RawMessage `json:"arguments"`
					Result           json.RawMessage `json:"result"`
				} `json:"item"`
			} `json:"payload"`
		}
		if json.Unmarshal(scan.Bytes(), &row) != nil || row.Type != "event_msg" || row.Payload.Type != "item_completed" {
			continue
		}
		item := row.Payload.Item
		if item.ID == "" {
			continue
		}
		record := CodexToolHistory{ID: codexToolID(item.ID), At: row.Timestamp, FinishedAt: row.Timestamp}
		switch item.Type {
		case "CommandExecution", "commandExecution":
			record.At = record.At.Add(-time.Duration(item.Duration.Secs)*time.Second - time.Duration(item.Duration.Nanos))
			if json.Unmarshal(item.Command, &record.Command) != nil {
				_ = json.Unmarshal(item.Command, &record.Text)
			}
			record.Output = item.AggregatedOutput
			if record.Output == "" {
				record.Output = item.Output
			}
		case "McpToolCall", "mcpToolCall", "DynamicToolCall", "dynamicToolCall":
			record.Text = codexAppToolText(codexAppItem{Tool: item.Tool, Server: item.Server, Arguments: item.Arguments})
			record.Output = codexToolResultText(item.Result)
		default:
			continue
		}
		result = append(result, record)
	}
	return result
}
