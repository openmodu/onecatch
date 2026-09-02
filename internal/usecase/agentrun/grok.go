package agentrun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const grokBinaryDefault = "grok"

// GrokRunner drives xAI's Grok Build through its Agent Client Protocol server
// (`grok agent stdio`).
//
// Grok also offers `-p --output-format streaming-json`, but that stream is a
// flattened, vendor-named projection of the very same ACP session updates. The
// protocol surface is the one xAI's own SDKs target, and it is the only one
// that carries blocking permission requests — so it is what OneCatch drives,
// and the shared [runACPSession] client does the talking.
type GrokRunner struct {
	binary string
	now    nowFunc
}

// NewGrokRunner builds a runner driving the given grok binary. An empty binary
// falls back to "grok" resolved from PATH.
func NewGrokRunner(binary string) *GrokRunner {
	if binary == "" {
		binary = grokBinaryDefault
	}
	return &GrokRunner{binary: binary, now: time.Now}
}

func (r *GrokRunner) Runtime() Runtime { return RuntimeGrok }

func (r *GrokRunner) Available() bool {
	_, err := exec.LookPath(r.binary)
	return err == nil
}

// ListSkills reads Grok Build's resolved catalog. inspect applies Grok's own
// compatibility, disabled-state, plugin, and collision rules, so the returned
// invocation names are exactly the slash commands a later run can accept.
func (r *GrokRunner) ListSkills(ctx context.Context, cwd string, environment []string) ([]Skill, error) {
	cmd := exec.CommandContext(ctx, r.binary, "inspect", "--json")
	if strings.TrimSpace(cwd) != "" {
		cmd.Dir = cwd
	}
	cmd.Env = environment
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list Grok skills: %w%s", err, stderrSuffix(stderr.String()))
	}
	var inspection struct {
		Skills []struct {
			Name                string `json:"name"`
			Description         string `json:"description"`
			UserInvocable       bool   `json:"userInvocable"`
			CompatibilityStatus string `json:"compatibilityStatus"`
			InvocableAs         string `json:"invocableAs"`
			Source              struct {
				Type       string `json:"type"`
				Path       string `json:"path"`
				PluginName string `json:"plugin_name"`
			} `json:"source"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(output, &inspection); err != nil {
		return nil, fmt.Errorf("decode Grok skills: %w", err)
	}
	items := make([]Skill, 0, len(inspection.Skills))
	for _, item := range inspection.Skills {
		if !item.UserInvocable || item.CompatibilityStatus != "" && item.CompatibilityStatus != "enabled" {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSpace(item.InvocableAs), "/")
		if name == "" {
			name = item.Name
		}
		scope := item.Source.Type
		if scope == "plugin" && item.Source.PluginName != "" {
			scope += "/" + item.Source.PluginName
		}
		items = append(items, Skill{Name: name, DisplayName: item.Name, Description: item.Description, Path: item.Source.Path, Scope: scope})
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	return items, nil
}

// SupportsInteractivePermissions covers read-only and workspace-write runs.
//
// ACP carries permission requests in every mode, so unlike Claude Code there is
// no launch-shape reason to restrict approvals to read-only: the sandbox is the
// backstop and the card is the first line. The full sandbox is excluded because
// the user has already granted that run blanket access by choosing it, and it
// is reserved for unattended automation that nobody is watching to answer.
func (r *GrokRunner) SupportsInteractivePermissions(sandbox Sandbox) bool {
	return sandbox == SandboxReadOnly || sandbox == SandboxWorkspaceWrite || sandbox == ""
}

func (r *GrokRunner) Run(ctx context.Context, req Request, sink Sink) (Result, error) {
	req.Prompt = adaptSkillMentions(req.Prompt, "/")
	return runACPSession(ctx, acpLaunch{
		runtime:     RuntimeGrok,
		displayName: "Grok Build",
		binary:      r.binary,
		command:     grokCommand,
		// Grok states the window in the handshake it already sends, so this
		// costs no extra round trip and no model quota.
		contextWindow: grokContextWindow,
	}, req, sink, r.now)
}

// grokCommand builds the `grok agent … stdio` invocation.
//
// Model and effort belong to the `agent` command, not to its `stdio`
// subcommand, so they are placed before `stdio`; passing them after is rejected
// as an unexpected argument. The sandbox is not on this path at all — it is a
// flag on the root command only — so it travels in GROK_SANDBOX, which Grok
// documents as its equivalent and which still refuses to start when the profile
// is missing.
func grokCommand(req Request) (acpCommand, error) {
	sandbox, err := grokSandbox(req.Sandbox)
	if err != nil {
		return acpCommand{}, err
	}
	args := []string{"agent"}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.ReasoningEffort != "" {
		args = append(args, "--reasoning-effort", req.ReasoningEffort)
	}
	return acpCommand{
		args:        append(args, "stdio"),
		environment: []string{"GROK_SANDBOX=" + sandbox},
	}, nil
}

// grokSandbox maps OneCatch's permission levels onto Grok's sandbox profiles.
//
// Grok refuses to start when a named profile is missing rather than running
// unprotected, so an unknown name fails the run instead of weakening it.
func grokSandbox(sandbox Sandbox) (string, error) {
	switch sandbox {
	case SandboxReadOnly:
		return "read-only", nil
	case SandboxWorkspaceWrite, "":
		return "workspace", nil
	case SandboxFull:
		return "none", nil
	default:
		return "", fmt.Errorf("grok: unsupported sandbox %q", sandbox)
	}
}

// InspectConfiguration discovers the models and reasoning-effort levels the
// installed Grok Build offers.
//
// Grok reports its catalog in the ACP initialize result, so this performs the
// protocol handshake and stops there: no session is opened, no prompt is sent,
// and no model quota or credentials are consumed.
func (r *GrokRunner) InspectConfiguration(ctx context.Context, cwd string, environment []string) (HarnessConfiguration, error) {
	// Built through grokCommand so the probe and a real run cannot drift into
	// two different invocations; a handshake runs no tools, so the sandbox it
	// carries only has to be a profile Grok will accept.
	command, err := grokCommand(Request{Sandbox: SandboxReadOnly})
	if err != nil {
		return HarnessConfiguration{}, err
	}
	cmd := exec.CommandContext(ctx, r.binary, command.args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = environment
	for _, entry := range command.environment {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		cmd.Env = setEnvironmentValue(cmd.Env, key, value)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return HarnessConfiguration{}, fmt.Errorf("Grok Build stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return HarnessConfiguration{}, fmt.Errorf("Grok Build stdout: %w", err)
	}
	var stderr lineCapture
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return HarnessConfiguration{}, fmt.Errorf("start Grok Build: %w", err)
	}
	defer stopACPServer(cmd, stdin)

	if err := json.NewEncoder(stdin).Encode(map[string]any{
		"jsonrpc": "2.0", "id": acpIDInitialize, "method": "initialize",
		"params": map[string]any{
			"protocolVersion":    acpProtocolVersion,
			"clientInfo":         map[string]string{"name": "onecatch", "title": "OneCatch", "version": "0.1.0"},
			"clientCapabilities": map[string]any{"fs": map[string]bool{"readTextFile": false, "writeTextFile": false}, "terminal": false},
		},
	}); err != nil {
		return HarnessConfiguration{}, fmt.Errorf("initialize Grok Build: %w", err)
	}

	scanner := newJSONLineScanner(stdout)
	for scanner.Scan() {
		var envelope acpEnvelope
		if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
			continue
		}
		if acpResponseID(envelope.ID) != acpIDInitialize {
			continue
		}
		if len(envelope.Error) > 0 {
			return HarnessConfiguration{}, fmt.Errorf("initialize Grok Build: %s", acpErrorText(envelope.Error))
		}
		configuration := parseGrokConfiguration(envelope.Result)
		if len(configuration.Models) == 0 {
			return HarnessConfiguration{}, fmt.Errorf("Grok Build did not advertise any models")
		}
		return configuration, nil
	}
	return HarnessConfiguration{}, fmt.Errorf("Grok Build did not answer the initialize handshake%s", stderr.tail())
}

// grokContextWindow resolves the window for the model a run will use out of the
// same handshake the catalog comes from. An empty model means the run accepted
// Grok's current selection, which the handshake names.
func grokContextWindow(initializeResult json.RawMessage, model string) int {
	configuration := parseGrokConfiguration(initializeResult)
	if strings.TrimSpace(model) == "" {
		model = configuration.Model
	}
	for _, entry := range configuration.Models {
		if strings.EqualFold(entry.Model, model) {
			return entry.ContextWindow
		}
	}
	return 0
}

// parseGrokConfiguration reads Grok's model catalog out of its handshake.
// Effort levels are declared per model — 4.6 offers xhigh where 4.5 does not —
// so they are kept on each model rather than flattened into one list that would
// offer a level the selected model rejects.
func parseGrokConfiguration(raw json.RawMessage) HarnessConfiguration {
	var response struct {
		Meta struct {
			ModelState struct {
				CurrentModelID  string `json:"currentModelId"`
				AvailableModels []struct {
					ModelID     string `json:"modelId"`
					Name        string `json:"name"`
					Description string `json:"description"`
					Meta        struct {
						ReasoningEfforts []struct {
							Value   string `json:"value"`
							Default bool   `json:"default"`
						} `json:"reasoningEfforts"`
						// Grok is the only harness that states the window in
						// its handshake, so no catalog probe is needed at all.
						TotalContextTokens int `json:"totalContextTokens"`
					} `json:"_meta"`
				} `json:"availableModels"`
			} `json:"modelState"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return HarnessConfiguration{}
	}
	state := response.Meta.ModelState
	configuration := HarnessConfiguration{Model: state.CurrentModelID, Models: make([]HarnessModel, 0, len(state.AvailableModels))}
	for _, model := range state.AvailableModels {
		if strings.TrimSpace(model.ModelID) == "" {
			continue
		}
		entry := HarnessModel{Model: model.ModelID, DisplayName: model.Name, Description: model.Description, ContextWindow: model.Meta.TotalContextTokens}
		if entry.DisplayName == "" {
			entry.DisplayName = model.ModelID
		}
		for _, effort := range model.Meta.ReasoningEfforts {
			if effort.Value == "" {
				continue
			}
			entry.Efforts = append(entry.Efforts, effort.Value)
			if effort.Default {
				entry.DefaultEffort = effort.Value
			}
		}
		configuration.Models = append(configuration.Models, entry)
	}
	return configuration
}
