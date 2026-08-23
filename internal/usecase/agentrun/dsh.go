package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	dshBinaryDefault = "dsh"
	// dshProfile is the one-shot profile: it boots no host, HTTP server, or web
	// runtime, submits a single task, and exits.
	dshProfile = "headless"
	// dshTranscriptFile is the fixed name the JSONL persistence backend writes
	// inside each session's own directory.
	dshTranscriptFile = "session.jsonl"
	// dshPollInterval is how often the runner looks for the session log and for
	// bytes appended to it. The harness coalesces writes on a short timer, so
	// polling more often than this only spends syscalls.
	dshPollInterval = 100 * time.Millisecond
)

// dshMutationTools are the tools whose calls change a file on disk, so a
// file_change event is worth emitting for them.
var dshMutationTools = map[string]struct{}{
	"write": {}, "edit": {}, "str_replace_editor": {},
}

// DshRunner drives DeepSeek Harness through its one-shot headless profile.
//
// Unlike every other harness OneCatch drives, dsh publishes no machine-readable
// stream: the headless profile prints only the final assistant message. What it
// does have is a plugin architecture whose `--patch` overlay can reconfigure any
// row of the composed profile by id. This runner uses that documented seam to
// pin the harness's own durable session log to a per-run directory in plain,
// unpacked JSONL, then reads that log as it is written. The events are the same
// ones dsh's own interfaces render, not a reconstruction.
type DshRunner struct {
	binary string
	// sessionRoot is where OneCatch keeps the harness's session logs. Empty
	// falls back to a directory under the user's home.
	sessionRoot string
	now         nowFunc
}

// NewDshRunner builds a runner driving the given dsh binary and keeping session
// logs under sessionRoot. Empty values fall back to "dsh" resolved from PATH
// and to a directory under the user's home.
func NewDshRunner(binary, sessionRoot string) *DshRunner {
	if binary == "" {
		binary = dshBinaryDefault
	}
	return &DshRunner{binary: binary, sessionRoot: strings.TrimSpace(sessionRoot), now: time.Now}
}

func (r *DshRunner) Runtime() Runtime { return RuntimeDsh }

func (r *DshRunner) Available() bool {
	_, err := exec.LookPath(r.binary)
	return err == nil
}

func (r *DshRunner) Run(ctx context.Context, req Request, sink Sink) (Result, error) {
	if sink == nil {
		sink = func(Event) {}
	}
	if req.Remote != nil {
		return Result{}, fmt.Errorf("DeepSeek Harness does not support remote FS runs")
	}
	if req.ResumeSessionID != "" {
		// The headless profile creates one fresh agent per invocation and
		// exposes no resume flag. Starting over silently would drop the prior
		// conversation while the caller believed it was continuing.
		return Result{}, fmt.Errorf("DeepSeek Harness cannot resume a session: its headless profile starts a fresh conversation each run")
	}

	root, err := r.resolveSessionRoot()
	if err != nil {
		return Result{}, err
	}
	patchPath, cleanup, err := writeDshPatch(root, req)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()

	// Everything already present belongs to earlier runs; the session this run
	// creates is whatever appears that is not in here.
	existing, err := dshTranscripts(root)
	if err != nil {
		return Result{}, err
	}

	name, args := dshCommand(r.binary, patchPath, req.Prompt)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = req.Workspace
	cmd.Env = dshEnvironment(req.Environment, req.Sandbox)
	cmd.Stdin = nil
	if req.InterruptGrace > 0 {
		cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
		cmd.WaitDelay = req.InterruptGrace
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("DeepSeek Harness stdout: %w", err)
	}
	var stderr lineCapture
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start DeepSeek Harness: %w", err)
	}

	// stdout carries the harness's final message. Drain it concurrently so a
	// full pipe can never block the process we are waiting on.
	var printed strings.Builder
	printedDone := make(chan struct{})
	go func() {
		defer close(printedDone)
		data, _ := io.ReadAll(stdout)
		printed.Write(data)
	}()

	reader := &dshLogReader{root: root, existing: existing, parser: &dshParser{}, now: r.now}
	tailDone := make(chan struct{})
	tailCtx, stopTail := context.WithCancel(ctx)
	go func() {
		defer close(tailDone)
		reader.tail(tailCtx, sink)
	}()

	waitErr := cmd.Wait()
	<-printedDone
	stopTail()
	<-tailDone
	// The harness flushes its log as it exits, so the last events can land
	// after the final poll. Drain once more before reporting.
	reader.drain(sink)

	result := reader.parser.result()
	if result.FinalMessage == "" {
		result.FinalMessage = strings.TrimSpace(printed.String())
	}
	if waitErr != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		// The headless profile exits non-zero when the turn did not complete,
		// which the log already described. Report it as a failed run rather
		// than as a broken harness only when the log agrees something ran.
		result.Succeeded = false
		if reader.parser.started {
			return result, nil
		}
		return result, fmt.Errorf("DeepSeek Harness exited: %w%s", waitErr, stderr.tail())
	}
	return result, nil
}

// resolveSessionRoot returns the directory holding OneCatch's dsh session logs,
// creating it if needed.
func (r *DshRunner) resolveSessionRoot() (string, error) {
	root := r.sessionRoot
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate DeepSeek Harness session root: %w", err)
		}
		root = filepath.Join(home, ".onecatch", "harnesses", "dsh", "sessions")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create DeepSeek Harness session root: %w", err)
	}
	return root, nil
}

// dshCommand builds the launch command.
//
// The published `dsh` executable is a Node script, and the launcher mounts a
// hot-reload plugin that refuses to start without Node's --expose-internals —
// a flag NODE_OPTIONS rejects, so it can only be passed on the command line.
// Resolving the script and running it under `node` explicitly is what makes the
// headless profile start at all; a non-script binary is launched directly, so a
// release that drops the requirement needs no change here.
func dshCommand(binary, patchPath, prompt string) (string, []string) {
	// Two separators, not one: the launcher parses its own flags and forwards
	// the rest verbatim, consuming the first `--` on the way. The second is
	// what reaches the headless app's parser and keeps a prompt that begins
	// with a dash from being read there as an unknown option.
	args := []string{"--profile", dshProfile, "--patch", patchPath, "--", "--", prompt}
	script := binary
	if resolved, err := exec.LookPath(binary); err == nil {
		if target, err := filepath.EvalSymlinks(resolved); err == nil {
			script = target
		} else {
			script = resolved
		}
	}
	if strings.HasSuffix(script, ".js") || strings.HasSuffix(script, ".mjs") {
		return "node", append([]string{"--expose-internals", script}, args...)
	}
	return binary, args
}

// dshEnvironment applies the sandbox through the harness's own deployment
// override, which every one of its enforcing backends reads.
func dshEnvironment(environment []string, sandbox Sandbox) []string {
	mode := "workspace-write"
	switch sandbox {
	case SandboxReadOnly:
		mode = "read-only"
	case SandboxFull:
		mode = "danger-full-access"
	}
	if environment == nil {
		environment = os.Environ()
	}
	return setEnvironmentValue(environment, "DSH_PERMISSION_MODE", mode)
}

// writeDshPatch materializes the overlay that reconfigures the composed profile
// for this run. It targets rows by id, which is the harness's documented way to
// override a bundle's configuration.
func writeDshPatch(root string, req Request) (string, func(), error) {
	var patch strings.Builder
	patch.WriteString("# Generated by OneCatch for one headless run. Safe to delete.\n")
	patch.WriteString("- id: session-persistence-jsonl\n  config:\n")
	patch.WriteString("    root: " + yamlQuote(root) + "\n")
	// The defaults are zstd frames and packed delta rows, neither of which can
	// be read incrementally as plain text while the run is still going.
	patch.WriteString("    compression: none\n")
	patch.WriteString("    packChunks: false\n")
	if req.Model != "" {
		provider := req.Provider
		if provider == "" {
			provider = "deepseek-official"
		}
		// A patch replaces the whole config of the row it targets, so both
		// fields must be written even when only the model was requested.
		patch.WriteString("- id: agent-default-model\n  config:\n")
		patch.WriteString("    provider: " + yamlQuote(provider) + "\n")
		patch.WriteString("    model: " + yamlQuote(req.Model) + "\n")
	}

	dir, err := os.MkdirTemp("", "onecatch-dsh-")
	if err != nil {
		return "", func() {}, fmt.Errorf("create DeepSeek Harness patch: %w", err)
	}
	path := filepath.Join(dir, "onecatch.patch.yml")
	if err := os.WriteFile(path, []byte(patch.String()), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", func() {}, fmt.Errorf("write DeepSeek Harness patch: %w", err)
	}
	return path, func() { _ = os.RemoveAll(dir) }, nil
}

// yamlQuote renders a single-quoted YAML scalar, where the only escape is a
// doubled quote. It keeps a path containing spaces, colons, or a leading dash
// from being reinterpreted as YAML structure.
func yamlQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// dshTranscripts lists every session log currently under root.
func dshTranscripts(root string) (map[string]struct{}, error) {
	found := make(map[string]struct{})
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			// A directory that vanished mid-walk is not this run's session.
			return nil //nolint:nilerr // tolerate concurrent cleanup
		}
		if !entry.IsDir() && entry.Name() == dshTranscriptFile {
			found[path] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan DeepSeek Harness sessions: %w", err)
	}
	return found, nil
}

// dshLogReader follows this run's session log as the harness appends to it.
type dshLogReader struct {
	root     string
	existing map[string]struct{}
	parser   *dshParser
	now      nowFunc

	mu     sync.Mutex
	path   string
	offset int64
	// pending holds a trailing partial line: the harness appends in batches,
	// so a poll can land mid-line and must not parse the fragment.
	pending string
}

// tail polls for the run's log and forwards each complete line until ctx ends.
func (r *dshLogReader) tail(ctx context.Context, sink Sink) {
	ticker := time.NewTicker(dshPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.drain(sink)
		}
	}
}

// drain reads whatever has been appended since the last call.
func (r *dshLogReader) drain(sink Sink) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.path == "" {
		r.path = r.discover()
		if r.path == "" {
			return
		}
	}
	file, err := os.Open(r.path)
	if err != nil {
		return
	}
	defer file.Close()
	if _, err := file.Seek(r.offset, io.SeekStart); err != nil {
		return
	}
	data, err := io.ReadAll(file)
	if err != nil || len(data) == 0 {
		return
	}
	r.offset += int64(len(data))
	buffer := r.pending + string(data)
	lines := strings.Split(buffer, "\n")
	// The final element is either empty (the batch ended on a newline) or a
	// partial line still being written; either way it waits for the next read.
	r.pending = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		r.parser.parse(line, r.now(), sink)
	}
}

// discover returns the log this run created: the one transcript under root that
// was not there when the run started. Sorting makes the pick deterministic if a
// concurrent run ever raced one in.
func (r *dshLogReader) discover() string {
	current, err := dshTranscripts(r.root)
	if err != nil {
		return ""
	}
	fresh := make([]string, 0, 1)
	for path := range current {
		if _, ok := r.existing[path]; !ok {
			fresh = append(fresh, path)
		}
	}
	if len(fresh) == 0 {
		return ""
	}
	sort.Strings(fresh)
	return fresh[0]
}

// dshEnvelope is one line of the harness's durable session log: the immutable
// header first, then one record per session event.
type dshEnvelope struct {
	Type string          `json:"type"`
	Seq  int             `json:"seq"`
	Data json.RawMessage `json:"data,omitempty"`
	// Header-only fields.
	ID  string `json:"id,omitempty"`
	Cwd string `json:"cwd,omitempty"`
}

// dshContentBlock is one model-facing block. Only the variants OneCatch renders
// carry fields here; the rest survive in the event's Raw line.
type dshContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type dshParser struct {
	sessionID    string
	finalMessage string
	usage        Usage
	usageSeen    bool
	succeeded    bool
	completed    bool
	started      bool
}

func (p *dshParser) parse(line string, at time.Time, sink Sink) {
	var envelope dshEnvelope
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		sink(Event{Kind: KindError, Text: fmt.Sprintf("parse DeepSeek Harness event: %v", err), Raw: line, At: at})
		return
	}
	switch envelope.Type {
	case "session":
		p.sessionID = envelope.ID
		p.started = true
		sink(Event{Kind: KindStarted, Text: envelope.ID, Raw: line, At: at})
	case "assistant/chunk":
		p.handleChunk(envelope.Data, line, at, sink)
	case "assistant/message":
		p.handleMessage(envelope.Data, line, at, sink)
	case "tool/call":
		p.handleToolCall(envelope.Data, line, at, sink)
	case "tool/result":
		p.handleToolResult(envelope.Data, line, at, sink)
	case "turn/end":
		p.handleTurnEnd(envelope.Data, line, at, sink)
	}
}

func (p *dshParser) handleChunk(raw json.RawMessage, line string, at time.Time, sink Sink) {
	var data struct {
		Turn  int `json:"turn"`
		Step  int `json:"step"`
		Chunk struct {
			Type  string `json:"type"`
			Index int    `json:"index"`
			Text  string `json:"text"`
			Usage *struct {
				InputTokens      int `json:"inputTokens"`
				OutputTokens     int `json:"outputTokens"`
				CacheReadTokens  int `json:"cacheReadTokens"`
				CacheWriteTokens int `json:"cacheWriteTokens"`
				ReasoningTokens  int `json:"reasoningTokens"`
			} `json:"usage,omitempty"`
			Reason *struct {
				Kind    string `json:"kind"`
				Failure *struct {
					Message string `json:"message"`
				} `json:"failure,omitempty"`
			} `json:"reason,omitempty"`
		} `json:"chunk"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}
	streamID := "dsh-" + strconv.Itoa(data.Turn) + "-" + strconv.Itoa(data.Step) + "-" + strconv.Itoa(data.Chunk.Index)
	switch data.Chunk.Type {
	case "text-delta":
		sink(Event{Kind: KindMessage, Text: data.Chunk.Text, StreamID: streamID, Phase: StreamDelta, Raw: line, At: at})
	case "reasoning-delta":
		sink(Event{Kind: KindReasoning, Text: data.Chunk.Text, StreamID: streamID, Phase: StreamDelta, Raw: line, At: at})
	case "usage":
		if data.Chunk.Usage == nil {
			return
		}
		usage := Usage{
			InputTokens:              data.Chunk.Usage.InputTokens + data.Chunk.Usage.CacheReadTokens + data.Chunk.Usage.CacheWriteTokens,
			CachedInputTokens:        data.Chunk.Usage.CacheReadTokens,
			CacheCreationInputTokens: data.Chunk.Usage.CacheWriteTokens,
			OutputTokens:             data.Chunk.Usage.OutputTokens,
			ReasoningOutputTokens:    data.Chunk.Usage.ReasoningTokens,
		}
		p.usage = usage
		p.usageSeen = true
		sink(Event{Kind: KindUsage, Usage: &usage, Raw: line, At: at})
	case "finish":
		if data.Chunk.Reason != nil && data.Chunk.Reason.Kind == "error" && data.Chunk.Reason.Failure != nil {
			sink(Event{Kind: KindError, Text: data.Chunk.Reason.Failure.Message, Raw: line, At: at})
		}
	}
}

// handleMessage records the assembled assistant message for one step. Its text
// replaces any earlier candidate, so the last step's prose is what the run
// reports, and its usage travels with it rather than in a separate record.
func (p *dshParser) handleMessage(raw json.RawMessage, line string, at time.Time, sink Sink) {
	var data struct {
		Message struct {
			Content []dshContentBlock `json:"content"`
		} `json:"message"`
		Usage *struct {
			InputTokens      int `json:"inputTokens"`
			OutputTokens     int `json:"outputTokens"`
			CacheReadTokens  int `json:"cacheReadTokens"`
			CacheWriteTokens int `json:"cacheWriteTokens"`
			ReasoningTokens  int `json:"reasoningTokens"`
		} `json:"usage,omitempty"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}
	var text strings.Builder
	for _, block := range data.Message.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	if text.Len() > 0 {
		p.finalMessage = text.String()
	}
	if data.Usage == nil {
		return
	}
	usage := Usage{
		InputTokens:              data.Usage.InputTokens + data.Usage.CacheReadTokens + data.Usage.CacheWriteTokens,
		CachedInputTokens:        data.Usage.CacheReadTokens,
		CacheCreationInputTokens: data.Usage.CacheWriteTokens,
		OutputTokens:             data.Usage.OutputTokens,
		ReasoningOutputTokens:    data.Usage.ReasoningTokens,
	}
	p.usage = usage
	p.usageSeen = true
	sink(Event{Kind: KindUsage, Usage: &usage, Raw: line, At: at})
}

func (p *dshParser) handleToolCall(raw json.RawMessage, line string, at time.Time, sink Sink) {
	var data struct {
		CallID    string `json:"callId"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}
	// The harness records the model's raw JSON argument string unparsed, so it
	// has to be decoded here before any field can be read from it.
	var input map[string]any
	_ = json.Unmarshal([]byte(data.Arguments), &input)
	sink(Event{
		Kind: KindToolUse, Text: dshToolText(data.Name, input),
		StreamID: "dsh-tool-" + data.CallID, Raw: line, At: at,
	})
	if _, ok := dshMutationTools[data.Name]; !ok {
		return
	}
	if path := toolInputPath(input); path != "" {
		sink(Event{Kind: KindFileChange, Text: path, Raw: line, At: at})
	}
}

func (p *dshParser) handleToolResult(raw json.RawMessage, line string, at time.Time, sink Sink) {
	var data struct {
		Message struct {
			Content []struct {
				Type       string `json:"type"`
				ToolCallID string `json:"toolCallId"`
				IsError    bool   `json:"isError"`
			} `json:"content"`
		} `json:"message"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}
	callID, failed := "", data.Error != nil
	for _, block := range data.Message.Content {
		if block.Type != "tool-result" {
			continue
		}
		callID = block.ToolCallID
		failed = failed || block.IsError
	}
	sink(Event{
		Kind: KindToolResult, Text: callID, Failed: failed,
		StreamID: "dsh-tool-" + callID, Raw: line, At: at,
	})
}

// handleTurnEnd settles the run. The headless profile submits exactly one task,
// so the first turn to end is the run's outcome.
func (p *dshParser) handleTurnEnd(raw json.RawMessage, line string, at time.Time, sink Sink) {
	if p.completed {
		return
	}
	var data struct {
		Reason struct {
			Kind  string `json:"kind"`
			Error *struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			} `json:"error,omitempty"`
		} `json:"reason"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}
	p.completed = true
	p.succeeded = data.Reason.Kind == "completed"
	if !p.succeeded {
		text := fmt.Sprintf("DeepSeek Harness turn ended: %s", data.Reason.Kind)
		if data.Reason.Error != nil && data.Reason.Error.Message != "" {
			text = data.Reason.Error.Message
		}
		sink(Event{Kind: KindError, Text: text, Raw: line, At: at})
	}
	event := Event{Kind: KindResult, Text: p.finalMessage, Failed: !p.succeeded, Raw: line, At: at}
	if p.usageSeen {
		usage := p.usage
		event.Usage = &usage
	}
	sink(event)
}

func (p *dshParser) result() Result {
	return Result{
		FinalMessage: strings.TrimSpace(p.finalMessage),
		Usage:        p.usage,
		SessionID:    p.sessionID,
		Succeeded:    p.started && p.succeeded,
	}
}

// dshToolText describes a tool call for the run log.
func dshToolText(name string, input map[string]any) string {
	if input == nil {
		return name
	}
	if command, ok := input["command"].(string); ok && strings.TrimSpace(command) != "" {
		return command
	}
	if path := toolInputPath(input); path != "" {
		return name + " " + path
	}
	return name
}
