//go:build !onecatch_worker

package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	codingagent "github.com/openmodu/modu/pkg/coding_agent"
	codingtools "github.com/openmodu/modu/pkg/coding_agent/tools"
	"github.com/openmodu/modu/pkg/provider"
	"github.com/openmodu/modu/pkg/types"
)

// ModuSDKRunner embeds Modu's CodingSession directly in OneCatch. It avoids
// process startup and JSON/NDJSON transport while preserving the same
// normalized Event contract used by the other harness adapters.
type ModuSDKRunner struct {
	now        nowFunc
	configPath string
	agentDir   string
}

type ModuSDKOptions struct {
	ConfigPath string
	AgentDir   string
}

func NewModuSDKRunner(options ...ModuSDKOptions) *ModuSDKRunner {
	runner := &ModuSDKRunner{now: time.Now}
	if len(options) > 0 {
		runner.configPath = strings.TrimSpace(options[0].ConfigPath)
		runner.agentDir = strings.TrimSpace(options[0].AgentDir)
	}
	return runner
}

func (r *ModuSDKRunner) Runtime() Runtime { return RuntimeModu }

func (r *ModuSDKRunner) ModuIntegration() string { return "sdk" }

// The SDK is linked into the application, but it is runnable only after Modu
// can resolve a model and provider from its configured source.
func (r *ModuSDKRunner) Available() bool {
	model, getAPIKey, _, _, err := r.resolveConfiguration()
	return err == nil && model != nil && getAPIKey != nil
}

func (r *ModuSDKRunner) Run(ctx context.Context, req Request, sink Sink) (result Result, err error) {
	if sink == nil {
		sink = func(Event) {}
	}
	model, getAPIKey, thinkingLevel, scopedModels, err := r.resolveConfiguration()
	if err != nil {
		return Result{}, fmt.Errorf("resolve modu native SDK configuration: %w", err)
	}
	toolSet := codingtools.ToolSetCoding
	if req.Sandbox == SandboxReadOnly {
		toolSet = codingtools.ToolSetReadOnly
	}
	var toolProvider types.ToolManager = newModuSDKToolProvider(toolSet)
	prompt := req.Prompt
	if req.Remote != nil {
		req, err = prepareRemoteRequest(req)
		if err != nil {
			return Result{}, err
		}
		remoteProvider, providerErr := newRemoteModuToolProvider(ctx, *req.Remote, req.Sandbox == SandboxReadOnly)
		if providerErr != nil {
			return Result{}, providerErr
		}
		defer remoteProvider.ShutdownTools()
		toolProvider = remoteProvider
		prompt = remoteModuGuidance + "\n\n" + req.Prompt
	}
	session, err := codingagent.NewCodingSession(codingagent.CodingSessionOptions{
		Cwd:             req.Workspace,
		AgentDir:        r.agentDir,
		Model:           model,
		ThinkingLevel:   thinkingLevel,
		GetAPIKey:       getAPIKey,
		ScopedModels:    scopedModels,
		ModelConfigPath: r.modelConfigPath(),
		ResumeSessionID: strings.TrimSpace(req.ResumeSessionID),
		ToolProvider:    toolProvider,
	})
	if err != nil {
		return Result{}, fmt.Errorf("create modu coding session: %w", err)
	}
	defer session.Close("onecatch_run_complete")

	if err := selectModuSDKModel(session, req.Provider, req.Model); err != nil {
		return Result{}, err
	}

	adapter := newModuSDKEventAdapter(r.now, sink)
	// The window belongs to whichever model selection settled on, not to the
	// one the config resolved to, so it is read after the swap. The SDK
	// already carries it on types.Model; nothing needed adding there.
	if model := session.GetModel(); model != nil {
		adapter.context.Window = model.ContextWindow
	}
	unsubscribe := session.Subscribe(adapter.handle)
	defer unsubscribe()

	// OneCatch has already authorized writable workflow steps. Read-only steps
	// receive a read-only tool catalog above, so an approval callback is not
	// needed in either mode.
	sink(Event{Kind: KindStarted, Text: session.GetSessionID(), At: adapter.timestamp()})
	if err := session.Prompt(ctx, prompt); err != nil {
		if ctx.Err() != nil {
			session.Abort()
			return adapter.result(session.GetSessionID(), false), ctx.Err()
		}
		adapter.fail(err)
		return adapter.result(session.GetSessionID(), false), fmt.Errorf("run modu native SDK: %w", err)
	}
	session.WaitForIdle()
	session.WaitForPendingWork()
	result = adapter.result(session.GetSessionID(), true)
	if result.FinalMessage == "" {
		result.FinalMessage = session.GetLastAssistantText()
	}
	sink(Event{Kind: KindUsage, Usage: &result.Usage, Context: &result.Context, At: adapter.timestamp()})
	sink(Event{Kind: KindResult, Text: result.FinalMessage, At: adapter.timestamp()})
	return result, nil
}

func (r *ModuSDKRunner) resolveConfiguration() (*types.Model, func(string) (string, error), types.ThinkingLevel, []string, error) {
	if r.configPath == "" {
		model, getAPIKey := provider.Resolve()
		if model == nil || getAPIKey == nil {
			return nil, nil, "", nil, fmt.Errorf("configure ~/.modu/config.toml or a supported provider API key")
		}
		return model, getAPIKey, provider.ResolveThinkingLevel(), provider.ConfiguredModelIDs(), nil
	}
	model, getAPIKey, err := provider.ResolveConfigFile(r.configPath)
	if err != nil {
		return nil, nil, "", nil, err
	}
	cfg, _, err := provider.LoadConfigFileAt(r.configPath)
	if err != nil {
		return nil, nil, "", nil, err
	}
	return model, getAPIKey, moduSDKThinkingLevel(cfg.Reasoning.Level), moduSDKScopedModels(cfg), nil
}

func (r *ModuSDKRunner) modelConfigPath() string {
	if r.configPath != "" {
		return r.configPath
	}
	return provider.ConfigPath()
}

func moduSDKThinkingLevel(level string) types.ThinkingLevel {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "low":
		return types.ThinkingLevelLow
	case "medium":
		return types.ThinkingLevelMedium
	case "high":
		return types.ThinkingLevelHigh
	default:
		return types.ThinkingLevelOff
	}
}

func moduSDKScopedModels(cfg provider.Config) []string {
	if len(cfg.ScopedModels) == 0 {
		models := make([]string, 0, len(cfg.Models))
		for _, model := range cfg.Models {
			if model.Model != "" {
				models = append(models, model.Model)
			}
		}
		return models
	}
	models := make([]string, 0, len(cfg.ScopedModels))
	for _, target := range cfg.ScopedModels {
		for _, model := range cfg.Models {
			if provider.ModelMatchesTarget(model, target) {
				models = append(models, model.Model)
				break
			}
		}
	}
	return models
}

type moduSDKToolProvider struct {
	base     codingtools.DefaultProvider
	readOnly bool
}

func newModuSDKToolProvider(set codingtools.ToolSet) *moduSDKToolProvider {
	return &moduSDKToolProvider{base: codingtools.NewProvider(set), readOnly: set == codingtools.ToolSetReadOnly}
}

func (p *moduSDKToolProvider) Tools(ctx types.ToolContext) []types.Tool {
	return p.filter(p.base.Tools(ctx))
}

func (p *moduSDKToolProvider) Rebind(tool types.Tool, ctx types.ToolContext) (types.Tool, bool) {
	rebound, ok := p.base.Rebind(tool, ctx)
	if !ok || len(p.filter([]types.Tool{rebound})) == 0 {
		return nil, false
	}
	return rebound, true
}

func (p *moduSDKToolProvider) ShutdownTools() { p.base.ShutdownTools() }

func (p *moduSDKToolProvider) filter(input []types.Tool) []types.Tool {
	if !p.readOnly {
		return input
	}
	output := make([]types.Tool, 0, len(input))
	for _, tool := range input {
		switch tool.Name() {
		case "read", "grep", "find", "ls", "read_tool_result", "context_remaining", "trajectory":
			output = append(output, tool)
		}
	}
	return output
}

func selectModuSDKModel(session *codingagent.CodingSession, providerID, modelName string) error {
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	if providerID == "auto" {
		providerID = ""
	}
	modelName = strings.TrimSpace(modelName)
	if modelName != "" {
		if err := session.SetModelByName(modelName); err != nil {
			return fmt.Errorf("select modu model: %w", err)
		}
		if providerID != "" && !strings.EqualFold(session.GetModel().ProviderID, providerID) {
			return fmt.Errorf("modu model %q belongs to provider %q, not %q", modelName, session.GetModel().ProviderID, providerID)
		}
		return nil
	}
	if providerID == "" || strings.EqualFold(session.GetModel().ProviderID, providerID) {
		return nil
	}
	for _, candidate := range session.GetAvailableModels() {
		if strings.EqualFold(candidate.ProviderID, providerID) {
			session.SetModel(candidate)
			return nil
		}
	}
	return fmt.Errorf("modu provider %q has no configured model", providerID)
}

type moduSDKEventAdapter struct {
	now             nowFunc
	sink            Sink
	usage           Usage
	context         ContextUsage
	final           string
	failed          bool
	messageSequence int
	messageStreamID string
	reasonStreamID  string
}

func newModuSDKEventAdapter(now nowFunc, sink Sink) *moduSDKEventAdapter {
	if now == nil {
		now = time.Now
	}
	return &moduSDKEventAdapter{now: now, sink: sink}
}

func (a *moduSDKEventAdapter) timestamp() time.Time { return a.now() }

func (a *moduSDKEventAdapter) emit(kind EventKind, streamID string, phase StreamPhase, text string, raw string, failed bool) {
	a.sink(Event{Kind: kind, StreamID: streamID, Phase: phase, Text: text, Raw: raw, Failed: failed, At: a.timestamp()})
}

func (a *moduSDKEventAdapter) handle(event types.Event) {
	rawBytes, _ := json.Marshal(event)
	raw := string(rawBytes)
	switch event.Type {
	case types.EventTypeMessageStart:
		a.messageSequence++
		a.messageStreamID = fmt.Sprintf("modu-sdk-message-%d", a.messageSequence)
		a.reasonStreamID = fmt.Sprintf("modu-sdk-thinking-%d", a.messageSequence)
	case types.EventTypeMessageUpdate:
		if event.StreamEvent == nil {
			return
		}
		switch event.StreamEvent.Type {
		case "text_start":
			a.emit(KindMessage, a.messageStreamID, StreamStart, "", raw, false)
		case "text_delta":
			a.emit(KindMessage, a.messageStreamID, StreamDelta, event.StreamEvent.Delta, raw, false)
		case "thinking_start":
			a.emit(KindReasoning, a.reasonStreamID, StreamStart, "", raw, false)
		case "thinking_delta":
			a.emit(KindReasoning, a.reasonStreamID, StreamDelta, event.StreamEvent.Delta, raw, false)
		}
	case types.EventTypeMessageEnd:
		message, ok := assistantMessage(event.Message)
		if !ok {
			return
		}
		text, thinking := moduSDKMessageContent(message)
		if strings.TrimSpace(thinking) != "" {
			a.emit(KindReasoning, a.reasonStreamID, StreamEnd, thinking, raw, false)
		}
		if strings.TrimSpace(text) != "" {
			a.final = text
			a.emit(KindMessage, a.messageStreamID, StreamEnd, text, raw, false)
		}
		// Modu reports Input as fresh input only. OneCatch's normalized usage
		// contract includes cache reads and writes in InputTokens, with the
		// detailed cache fields retained as subsets of that total.
		a.usage.InputTokens += message.Usage.Input + message.Usage.CacheRead + message.Usage.CacheWrite
		a.usage.CachedInputTokens += message.Usage.CacheRead
		a.usage.CacheCreationInputTokens += message.Usage.CacheWrite
		a.usage.OutputTokens += message.Usage.Output
		// Cost accumulates across the step; occupancy does not. This one
		// message's prompt is what the window holds right now, and it falls
		// when the harness compacts, so it replaces rather than adds.
		if prompt := message.Usage.Input + message.Usage.CacheRead + message.Usage.CacheWrite; prompt > 0 {
			a.context.Tokens = prompt
			usage, context := a.usage, a.context
			a.sink(Event{Kind: KindUsage, Usage: &usage, Context: &context, At: a.timestamp()})
		}
		if strings.TrimSpace(message.ErrorMessage) != "" {
			a.failed = true
			a.emit(KindError, "", "", message.ErrorMessage, raw, true)
		}
	case types.EventTypeToolExecutionStart:
		args, _ := json.Marshal(event.Args)
		a.emit(KindToolUse, event.ToolCallID, "", moduToolText(event.ToolName, args), raw, false)
	case types.EventTypeToolExecutionUpdate:
		if text := moduSDKToolResultText(event.Partial); text != "" {
			a.emit(KindToolResult, event.ToolCallID, StreamDelta, text, raw, event.IsError)
		}
	case types.EventTypeToolExecutionEnd:
		a.emit(KindToolResult, event.ToolCallID, StreamEnd, moduSDKToolResultText(event.Result), raw, event.IsError)
	case types.EventTypeInterrupt:
		a.failed = true
		a.emit(KindError, "", "", "Modu run interrupted", raw, true)
	}
}

func assistantMessage(value types.AgentMessage) (types.AssistantMessage, bool) {
	switch message := value.(type) {
	case types.AssistantMessage:
		return message, true
	case *types.AssistantMessage:
		if message != nil {
			return *message, true
		}
	}
	return types.AssistantMessage{}, false
}

func moduSDKMessageContent(message types.AssistantMessage) (string, string) {
	var text, thinking strings.Builder
	for _, block := range message.Content {
		switch content := block.(type) {
		case *types.TextContent:
			if content != nil {
				text.WriteString(content.Text)
			}
		case *types.ThinkingContent:
			if content != nil {
				thinking.WriteString(content.Thinking)
			}
		}
	}
	return text.String(), thinking.String()
}

func moduSDKToolResultText(value any) string {
	result, ok := value.(types.ToolResult)
	if !ok {
		if pointer, pointerOK := value.(*types.ToolResult); pointerOK && pointer != nil {
			result, ok = *pointer, true
		}
	}
	if ok {
		var text strings.Builder
		for _, block := range result.Content {
			if content, contentOK := block.(*types.TextContent); contentOK && content != nil {
				text.WriteString(content.Text)
			}
		}
		return text.String()
	}
	encoded, _ := json.Marshal(value)
	return strings.TrimSpace(string(encoded))
}

func (a *moduSDKEventAdapter) fail(err error) {
	a.failed = true
	a.emit(KindError, "", "", err.Error(), "", true)
}

func (a *moduSDKEventAdapter) result(sessionID string, completed bool) Result {
	return Result{FinalMessage: a.final, Usage: a.usage, Context: a.context, SessionID: sessionID, Succeeded: completed && !a.failed}
}
