package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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
	return runACPSession(ctx, acpLaunch{
		runtime:     RuntimeGrok,
		displayName: "Grok Build",
		binary:      r.binary,
		command:     grokCommand,
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

// GrokModelInfo is one model the installed Grok Build advertises.
type GrokModelInfo struct {
	Model       string `json:"model"`
	DisplayName string `json:"displayName"`
	Description string `json:"description,omitempty"`
}

// GrokConfiguration is the model catalog discovered from a Grok installation.
type GrokConfiguration struct {
	Models  []GrokModelInfo `json:"models"`
	Efforts []string        `json:"efforts"`
}

// InspectConfiguration discovers the models and reasoning-effort levels the
// installed Grok Build offers.
//
// Grok reports its catalog in the ACP initialize result, so this performs the
// protocol handshake and stops there: no session is opened, no prompt is sent,
// and no model quota or credentials are consumed.
func (r *GrokRunner) InspectConfiguration(ctx context.Context, cwd string, environment []string) (GrokConfiguration, error) {
	// Built through grokCommand so the probe and a real run cannot drift into
	// two different invocations; a handshake runs no tools, so the sandbox it
	// carries only has to be a profile Grok will accept.
	command, err := grokCommand(Request{Sandbox: SandboxReadOnly})
	if err != nil {
		return GrokConfiguration{}, err
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
		return GrokConfiguration{}, fmt.Errorf("Grok Build stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return GrokConfiguration{}, fmt.Errorf("Grok Build stdout: %w", err)
	}
	var stderr lineCapture
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return GrokConfiguration{}, fmt.Errorf("start Grok Build: %w", err)
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
		return GrokConfiguration{}, fmt.Errorf("initialize Grok Build: %w", err)
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
			return GrokConfiguration{}, fmt.Errorf("initialize Grok Build: %s", acpErrorText(envelope.Error))
		}
		configuration := parseGrokConfiguration(envelope.Result)
		if len(configuration.Models) == 0 {
			return GrokConfiguration{}, fmt.Errorf("Grok Build did not advertise any models")
		}
		return configuration, nil
	}
	return GrokConfiguration{}, fmt.Errorf("Grok Build did not answer the initialize handshake%s", stderr.tail())
}

func parseGrokConfiguration(raw json.RawMessage) GrokConfiguration {
	var response struct {
		Meta struct {
			ModelState struct {
				AvailableModels []struct {
					ModelID     string `json:"modelId"`
					Name        string `json:"name"`
					Description string `json:"description"`
					Meta        struct {
						ReasoningEfforts []struct {
							Value string `json:"value"`
						} `json:"reasoningEfforts"`
					} `json:"_meta"`
				} `json:"availableModels"`
			} `json:"modelState"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return GrokConfiguration{}
	}
	configuration := GrokConfiguration{Models: make([]GrokModelInfo, 0, len(response.Meta.ModelState.AvailableModels))}
	seenEffort := make(map[string]struct{})
	for _, model := range response.Meta.ModelState.AvailableModels {
		if strings.TrimSpace(model.ModelID) == "" {
			continue
		}
		displayName := model.Name
		if displayName == "" {
			displayName = model.ModelID
		}
		configuration.Models = append(configuration.Models, GrokModelInfo{Model: model.ModelID, DisplayName: displayName, Description: model.Description})
		// Effort levels are declared per model; the settings UI offers one
		// list, so collect the union in first-seen order.
		for _, effort := range model.Meta.ReasoningEfforts {
			if effort.Value == "" {
				continue
			}
			if _, ok := seenEffort[effort.Value]; ok {
				continue
			}
			seenEffort[effort.Value] = struct{}{}
			configuration.Efforts = append(configuration.Efforts, effort.Value)
		}
	}
	return configuration
}
