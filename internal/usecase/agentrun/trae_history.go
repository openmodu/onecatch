package agentrun

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ReadTraeToolHistory recovers arguments by exact call ID from the CLI's
// durable session history. It never resumes a thread or executes a tool.
func ReadTraeToolHistory(session string) map[string]string {
	if session == "" || strings.ContainsAny(session, "/\\*?[]") {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	files, err := filepath.Glob(filepath.Join(home, ".trae", "cli", "sessions", "*", "*", "*", "*"+session+".jsonl"))
	if err != nil {
		return nil
	}
	result := map[string]string{}
	for _, name := range files {
		file, err := os.Open(name)
		if err != nil {
			continue
		}
		readTraeToolHistory(file, result)
		file.Close()
	}
	return result
}

func readTraeToolHistory(file *os.File, result map[string]string) {
	scan := bufio.NewScanner(file)
	scan.Buffer(make([]byte, 4096), 16<<20)
	for scan.Scan() {
		var row struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(scan.Bytes(), &row) != nil {
			continue
		}
		var items []json.RawMessage
		if row.Type == "history_mutation" {
			var mutation struct {
				Items []json.RawMessage `json:"items"`
			}
			if json.Unmarshal(row.Payload, &mutation) != nil {
				continue
			}
			items = mutation.Items
		} else if row.Type == "response_item" {
			items = []json.RawMessage{row.Payload}
		}
		for _, raw := range items {
			var item struct {
				Type, Name, Arguments, Input string
				CallID                       string `json:"call_id"`
			}
			if json.Unmarshal(raw, &item) != nil || item.CallID == "" || item.Name == "" {
				continue
			}
			if item.Type != "function_call" && item.Type != "custom_tool_call" {
				continue
			}
			args := item.Arguments
			if args == "" {
				args = item.Input
			}
			if args == "" {
				continue
			}
			result[traeToolID(item.CallID)] = moduToolText(item.Name, json.RawMessage(args))
		}
	}
}
