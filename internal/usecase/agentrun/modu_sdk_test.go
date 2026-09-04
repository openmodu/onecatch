//go:build !onecatch_worker

package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	codingagent "github.com/openmodu/modu/pkg/coding_agent"
	"github.com/openmodu/modu/pkg/types"
)

type namedModuSDKTool struct{ name string }

func (t namedModuSDKTool) Name() string        { return t.name }
func (t namedModuSDKTool) Label() string       { return t.name }
func (t namedModuSDKTool) Description() string { return t.name }
func (t namedModuSDKTool) Parameters() any     { return nil }
func (t namedModuSDKTool) Execute(context.Context, string, map[string]any, types.ToolUpdateCallback) (types.ToolResult, error) {
	return types.ToolResult{}, nil
}

func TestModuSDKEventAdapterNormalizesStreamsAndUsage(t *testing.T) {
	now := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	var events []Event
	adapter := newModuSDKEventAdapter(func() time.Time { return now }, func(event Event) {
		events = append(events, event)
	})
	adapter.handle(types.Event{Type: types.EventTypeMessageStart})
	adapter.handle(types.Event{Type: types.EventTypeMessageUpdate, StreamEvent: &types.StreamEvent{Type: "text_start"}})
	adapter.handle(types.Event{Type: types.EventTypeMessageUpdate, StreamEvent: &types.StreamEvent{Type: "text_delta", Delta: "done"}})
	message := types.AssistantMessage{Role: types.RoleAssistant, Content: []types.ContentBlock{&types.ThinkingContent{Type: "thinking", Thinking: "reason"}, &types.TextContent{Type: "text", Text: "done"}}}
	message.Usage.Input = 11
	message.Usage.Output = 7
	message.Usage.CacheRead = 3
	message.Usage.CacheWrite = 2
	adapter.handle(types.Event{Type: types.EventTypeMessageEnd, Message: message})

	result := adapter.result("session-1", true)
	if !result.Succeeded || result.FinalMessage != "done" || result.SessionID != "session-1" {
		t.Fatalf("result = %+v", result)
	}
	if result.Usage.InputTokens != 16 || result.Usage.OutputTokens != 7 || result.Usage.CachedInputTokens != 3 || result.Usage.CacheCreationInputTokens != 2 {
		t.Fatalf("usage = %+v", result.Usage)
	}
	// The message-end now also publishes usage, so a long run shows occupancy
	// while it is still running instead of only once it finishes.
	if len(events) != 5 || events[0].Phase != StreamStart || events[1].Phase != StreamDelta || events[2].Kind != KindReasoning || events[3].Phase != StreamEnd || events[4].Kind != KindUsage {
		t.Fatalf("events = %+v", events)
	}
	// Cost accumulates across the step; occupancy is this one prompt and is
	// replaced, not added, so it can fall when the harness compacts.
	if events[4].Context == nil || events[4].Context.Tokens != 16 {
		t.Fatalf("live occupancy = %+v", events[4].Context)
	}
}

func TestEngineSelectsConfiguredModuAdapter(t *testing.T) {
	if _, ok := NewEngine(Config{}).Runner(RuntimeModu).(*ModuSDKRunner); !ok {
		t.Fatal("default Modu adapter is not the native SDK")
	}
	cli, ok := NewEngine(Config{ModuIntegration: "cli"}).Runner(RuntimeModu).(*ModuRunner)
	if !ok {
		t.Fatal("CLI Modu adapter was not selected")
	}
	if cli.remoteRunner == nil {
		t.Fatal("CLI Modu adapter has no native SDK fallback for Remote FS")
	}
}

func TestModuSDKExposesHumanInputAndPreservesQuestionShape(t *testing.T) {
	runner := NewModuSDKRunner()
	if !runner.SupportsInteractiveUserInput() || !NewEngineWithRunners(runner).SupportsInteractiveUserInput(RuntimeModu) {
		t.Fatal("native Modu SDK must advertise interactive user input")
	}
	request := moduUserInputRequest(codingagent.AskRequest{Questions: []codingagent.AskQuestion{{
		ID: "branch", Header: "Branch base", Question: "Where should the branch start?",
		Options: []codingagent.AskOption{{Label: "main", Description: "Start clean"}, {Label: "current"}},
	}}})
	if request.ID == "" || len(request.Questions) != 1 || request.Questions[0].ID != "branch" || request.Questions[0].Options[0].Description != "Start clean" {
		t.Fatalf("request = %+v", request)
	}
}

func TestModuSDKRunnerUsesExplicitConfiguration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte(`version = 2
active = "onecatch"
scopedModels = ["onecatch"]

[reasoning]
level = "medium"

[providers.onecatch]
baseUrl = "https://onecatch.example/v1"
apiKeyEnv = "ONECATCH_MODU_TEST_KEY"

[[models]]
name = "onecatch"
provider = "onecatch"
model = "onecatch-model"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ONECATCH_MODU_TEST_KEY", "secret")
	runner := NewModuSDKRunner(ModuSDKOptions{ConfigPath: configPath, AgentDir: filepath.Dir(configPath)})
	model, getAPIKey, thinking, scoped, err := runner.resolveConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	if model.ProviderID != "onecatch" || model.ID != "onecatch-model" {
		t.Fatalf("model = %+v", model)
	}
	key, err := getAPIKey("onecatch")
	if err != nil || key != "secret" {
		t.Fatalf("key = %q, %v", key, err)
	}
	if thinking != types.ThinkingLevelMedium || len(scoped) != 1 || scoped[0] != "onecatch-model" {
		t.Fatalf("metadata = (%q, %#v)", thinking, scoped)
	}
}

func TestModuSDKReadOnlyProviderFiltersMutatingAndExtensionTools(t *testing.T) {
	provider := newModuSDKToolProvider("read-only")
	filtered := provider.filter([]types.Tool{
		namedModuSDKTool{name: "read"},
		namedModuSDKTool{name: "write"},
		namedModuSDKTool{name: "bash"},
		namedModuSDKTool{name: "workflow"},
		namedModuSDKTool{name: "context_remaining"},
	})
	if len(filtered) != 2 || filtered[0].Name() != "read" || filtered[1].Name() != "context_remaining" {
		t.Fatalf("filtered tools = %#v", filtered)
	}
}
