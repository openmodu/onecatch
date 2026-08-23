package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// dshAuthFailureLog is a verbatim capture of the session log DeepSeek Harness
// wrote for a run whose API key was rejected. It pins the header shape and the
// error-path chunk and turn records.
const dshAuthFailureLog = `{"type":"session","version":0,"id":"session-d561567e-d83f-48dd-a4bc-c033fb1d06e7","createdAt":1787468173043,"cwd":"/tmp/dsh-probe/work","delegationDepth":0}
{"type":"permission/preset","seq":0,"time":1787468173044,"data":{"preset":"workspace-write"}}
{"type":"sandbox/mode","seq":1,"time":1787468173045,"data":{"mode":"workspace-write"}}
{"type":"turn/start","seq":4,"time":1787468173046,"data":{"turn":1}}
{"type":"step/start","seq":6,"time":1787468173082,"data":{"turn":1,"step":1}}
{"type":"assistant/chunk","seq":13,"time":1787468173240,"data":{"turn":1,"step":1,"chunk":{"type":"finish","reason":{"kind":"error","failure":{"message":"Authentication Fails, Your api key: ****robe is invalid","code":"AUTH","status":401}}}}}
{"type":"step/end","seq":14,"time":1787468173241,"data":{"turn":1,"step":1}}
{"type":"turn/end","seq":15,"time":1787468173241,"data":{"turn":1,"reason":{"kind":"error","error":{"message":"Authentication Fails, Your api key: ****robe is invalid","code":"AUTH","status":401}}}}`

// dshSuccessLog is a completed run: streamed reasoning and text, a bash call, a
// file write, a failed read, and the assembled message carrying usage. The
// envelope and header come from the capture above; the payloads follow the
// harness's published SessionEventMap and content-block types.
const dshSuccessLog = `{"type":"session","version":0,"id":"session-11111111-2222-3333-4444-555555555555","createdAt":1787468300000,"cwd":"/tmp/dsh-probe/work","delegationDepth":0}
{"type":"turn/start","seq":0,"time":1787468300001,"data":{"turn":1}}
{"type":"step/start","seq":1,"time":1787468300002,"data":{"turn":1,"step":1}}
{"type":"assistant/chunk","seq":2,"time":1787468300003,"data":{"turn":1,"step":1,"chunk":{"type":"reasoning-delta","index":0,"text":"Reading the repo."}}}
{"type":"assistant/chunk","seq":3,"time":1787468300004,"data":{"turn":1,"step":1,"chunk":{"type":"text-delta","index":1,"text":"Writing "}}}
{"type":"assistant/chunk","seq":4,"time":1787468300005,"data":{"turn":1,"step":1,"chunk":{"type":"text-delta","index":1,"text":"the summary."}}}
{"type":"tool/call","seq":5,"time":1787468300006,"data":{"turn":1,"step":1,"callId":"c1","name":"bash","arguments":"{\"command\":\"ls -la\"}"}}
{"type":"tool/result","seq":6,"time":1787468300007,"data":{"turn":1,"step":1,"message":{"role":"user","content":[{"type":"tool-result","toolCallId":"c1","content":[{"type":"text","text":"README.md"}]}]}}}
{"type":"tool/call","seq":7,"time":1787468300008,"data":{"turn":1,"step":1,"callId":"c2","name":"write","arguments":"{\"path\":\"SUMMARY.md\",\"content\":\"done\"}"}}
{"type":"tool/result","seq":8,"time":1787468300009,"data":{"turn":1,"step":1,"message":{"role":"user","content":[{"type":"tool-result","toolCallId":"c2","content":[]}]}}}
{"type":"tool/call","seq":9,"time":1787468300010,"data":{"turn":1,"step":1,"callId":"c3","name":"read","arguments":"{\"path\":\"missing.txt\"}"}}
{"type":"tool/result","seq":10,"time":1787468300011,"data":{"turn":1,"step":1,"message":{"role":"user","content":[{"type":"tool-result","toolCallId":"c3","content":[{"type":"text","text":"ENOENT"}],"isError":true}]},"error":{"name":"Error","code":"ENOENT"}}}
{"type":"assistant/message","seq":11,"time":1787468300012,"data":{"turn":1,"step":1,"message":{"role":"assistant","content":[{"type":"text","text":"Writing the summary."}]},"usage":{"inputTokens":1500,"outputTokens":220,"cacheReadTokens":900,"cacheWriteTokens":60,"reasoningTokens":80}}}
{"type":"step/end","seq":12,"time":1787468300013,"data":{"turn":1,"step":1}}
{"type":"turn/end","seq":13,"time":1787468300014,"data":{"turn":1,"reason":{"kind":"completed"}}}`

// replayDshLog feeds a whole log through the parser the way the reader does.
func replayDshLog(log string) ([]Event, Result) {
	parser := &dshParser{}
	now := fixedNow()
	var events []Event
	for _, line := range strings.Split(log, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parser.parse(line, now(), func(event Event) { events = append(events, event) })
	}
	return events, parser.result()
}

func TestDshParserNormalizesCompletedRun(t *testing.T) {
	events, result := replayDshLog(dshSuccessLog)
	if !result.Succeeded {
		t.Fatal("a turn that ended completed must report success")
	}
	if result.SessionID != "session-11111111-2222-3333-4444-555555555555" {
		t.Fatalf("session id = %q", result.SessionID)
	}
	if result.FinalMessage != "Writing the summary." {
		t.Fatalf("final message = %q", result.FinalMessage)
	}
	// The harness reports cache reads and writes separately from fresh input,
	// so the normalized input total folds all three together.
	want := Usage{InputTokens: 2460, CachedInputTokens: 900, CacheCreationInputTokens: 60, OutputTokens: 220, ReasoningOutputTokens: 80}
	if result.Usage != want {
		t.Fatalf("usage = %+v, want %+v", result.Usage, want)
	}

	var message, reasoning, toolText strings.Builder
	toolUses, failedResults := 0, 0
	var fileChanges []string
	for _, event := range events {
		switch event.Kind {
		case KindMessage:
			message.WriteString(event.Text)
		case KindReasoning:
			reasoning.WriteString(event.Text)
		case KindToolUse:
			toolUses++
			toolText.WriteString(event.Text + "|")
		case KindToolResult:
			if event.Failed {
				failedResults++
			}
		case KindFileChange:
			fileChanges = append(fileChanges, event.Text)
		}
	}
	if message.String() != "Writing the summary." {
		t.Fatalf("streamed message = %q", message.String())
	}
	if reasoning.String() != "Reading the repo." {
		t.Fatalf("streamed reasoning = %q", reasoning.String())
	}
	if toolUses != 3 {
		t.Fatalf("tool uses = %d, want 3", toolUses)
	}
	// The harness records the model's argument JSON as an unparsed string, so a
	// shell command only reads well once the adapter decodes it.
	if !strings.Contains(toolText.String(), "ls -la") {
		t.Fatalf("shell command not decoded from raw arguments: %q", toolText.String())
	}
	if failedResults != 1 {
		t.Fatalf("failed tool results = %d, want 1", failedResults)
	}
	if len(fileChanges) != 1 || fileChanges[0] != "SUMMARY.md" {
		t.Fatalf("file changes = %v, want [SUMMARY.md]", fileChanges)
	}
}

func TestDshParserReportsTurnFailure(t *testing.T) {
	events, result := replayDshLog(dshAuthFailureLog)
	if result.Succeeded {
		t.Fatal("a turn that ended in error must not report success")
	}
	if result.SessionID != "session-d561567e-d83f-48dd-a4bc-c033fb1d06e7" {
		t.Fatalf("session id = %q", result.SessionID)
	}
	errors := 0
	for _, event := range events {
		if event.Kind == KindError && strings.Contains(event.Text, "Authentication Fails") {
			errors++
		}
	}
	// Once from the stream's finish chunk, once from the turn's end reason.
	if errors != 2 {
		t.Fatalf("authentication failure surfaced %d times, want 2", errors)
	}
}

func TestDshCommandPassesDashPromptsThrough(t *testing.T) {
	// A script binary has to be launched under node explicitly: the harness's
	// loader mounts a hot-reload plugin that will not start without
	// --expose-internals, which NODE_OPTIONS refuses to carry.
	script := filepath.Join(t.TempDir(), "bin.js")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env node\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	name, args := dshCommand(script, "/tmp/patch.yml", "--not-a-flag")
	// The launched path is the symlink-resolved one, which on macOS differs
	// from the temp path by its /private prefix.
	if name != "node" || args[0] != "--expose-internals" || !strings.HasSuffix(args[1], "bin.js") {
		t.Fatalf("script launch = %s %v", name, args)
	}
	// Two separators: the launcher consumes the first while forwarding the
	// rest, so only the second reaches the app's own parser.
	if args[len(args)-3] != "--" || args[len(args)-2] != "--" || args[len(args)-1] != "--not-a-flag" {
		t.Fatalf("prompt not protected by two separators: %v", args)
	}

	binary := filepath.Join(t.TempDir(), "dsh")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	if name, _ := dshCommand(binary, "/tmp/patch.yml", "hi"); name != binary {
		t.Fatalf("a non-script binary must launch directly, got %q", name)
	}
}

func TestDshEnvironmentMapsSandbox(t *testing.T) {
	for _, testCase := range []struct {
		sandbox Sandbox
		want    string
	}{
		{SandboxReadOnly, "DSH_PERMISSION_MODE=read-only"},
		{SandboxWorkspaceWrite, "DSH_PERMISSION_MODE=workspace-write"},
		{"", "DSH_PERMISSION_MODE=workspace-write"},
		{SandboxFull, "DSH_PERMISSION_MODE=danger-full-access"},
	} {
		environment := dshEnvironment([]string{"HOME=/home/x"}, testCase.sandbox)
		if !containsArgs(environment, testCase.want) {
			t.Fatalf("sandbox %q produced %v, want %q", testCase.sandbox, environment, testCase.want)
		}
	}
	// An inherited value must not survive and silently widen the sandbox.
	environment := dshEnvironment([]string{"DSH_PERMISSION_MODE=danger-full-access"}, SandboxReadOnly)
	if containsArgs(environment, "DSH_PERMISSION_MODE=danger-full-access") {
		t.Fatalf("inherited permission mode was not overridden: %v", environment)
	}
}

func TestWriteDshPatchPinsPlainTextLog(t *testing.T) {
	root := t.TempDir()
	path, cleanup, err := writeDshPatch(root, Request{Model: "deepseek-v4", Provider: "deepseek-official"})
	if err != nil {
		t.Fatalf("write patch: %v", err)
	}
	defer cleanup()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patch: %v", err)
	}
	patch := string(data)
	// Both defaults have to be off: zstd frames and packed delta rows cannot be
	// read incrementally as plain text while the run is still going.
	for _, want := range []string{"id: session-persistence-jsonl", "compression: none", "packChunks: false", "'" + root + "'", "id: agent-default-model", "model: 'deepseek-v4'"} {
		if !strings.Contains(patch, want) {
			t.Fatalf("patch missing %q:\n%s", want, patch)
		}
	}

	// A patch replaces the whole config of the row it targets, so the provider
	// must be written even when only a model was asked for.
	modelOnly, cleanupModelOnly, err := writeDshPatch(root, Request{Model: "deepseek-v4"})
	if err != nil {
		t.Fatalf("write patch: %v", err)
	}
	defer cleanupModelOnly()
	data, _ = os.ReadFile(modelOnly)
	if !strings.Contains(string(data), "provider: 'deepseek-official'") {
		t.Fatalf("model-only patch dropped the provider:\n%s", data)
	}

	// With no model requested the harness's own default row is left alone.
	plain, cleanupPlain, err := writeDshPatch(root, Request{})
	if err != nil {
		t.Fatalf("write patch: %v", err)
	}
	defer cleanupPlain()
	data, _ = os.ReadFile(plain)
	if strings.Contains(string(data), "agent-default-model") {
		t.Fatalf("an unrequested model must not override the profile:\n%s", data)
	}
}

func TestYamlQuoteEscapesApostrophes(t *testing.T) {
	if got := yamlQuote("/Users/o'brien/it's here"); got != `'/Users/o''brien/it''s here'` {
		t.Fatalf("yamlQuote = %s", got)
	}
}

func TestDshLogReaderFollowsTheRunsOwnSession(t *testing.T) {
	root := t.TempDir()
	// A log from an earlier run: present before the run starts, so it must be
	// ignored no matter what it contains.
	stale := filepath.Join(root, "project", "session-old")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stale, dshTranscriptFile), []byte(dshAuthFailureLog), 0o644); err != nil {
		t.Fatalf("write stale log: %v", err)
	}
	existing, err := dshTranscripts(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	var events []Event
	reader := &dshLogReader{root: root, existing: existing, parser: &dshParser{}, now: fixedNow()}
	sink := func(event Event) { events = append(events, event) }

	// Nothing new yet.
	reader.drain(sink)
	if len(events) != 0 {
		t.Fatalf("a pre-existing log was read: %+v", events)
	}

	fresh := filepath.Join(root, "project", "session-new")
	if err := os.MkdirAll(fresh, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	transcript := filepath.Join(fresh, dshTranscriptFile)
	lines := strings.SplitAfter(dshSuccessLog, "\n")
	// Write the first half, then split a line mid-write to prove a partial
	// trailing line is held back rather than parsed as a broken record.
	half := strings.Join(lines[:6], "")
	if err := os.WriteFile(transcript, []byte(half+`{"type":"assistant/chunk","seq":99,"data":{"turn":1,"st`), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	reader.drain(sink)
	for _, event := range events {
		if event.Kind == KindError {
			t.Fatalf("a partial trailing line was parsed: %+v", event)
		}
	}
	if reader.parser.sessionID == "" {
		t.Fatal("the run's own session was not picked up")
	}

	// Now replace the truncated tail with the whole log and drain again.
	if err := os.WriteFile(transcript, []byte(dshSuccessLog+"\n"), 0o644); err != nil {
		t.Fatalf("rewrite log: %v", err)
	}
	reader.drain(sink)
	if !reader.parser.succeeded {
		t.Fatal("the completed turn was never observed")
	}
}

func TestDshRunnerRefusesResume(t *testing.T) {
	runner := NewDshRunner(stubBinary(t, "", "", 0), t.TempDir())
	_, err := runner.Run(context.Background(), Request{
		Prompt: "carry on", Workspace: t.TempDir(), ResumeSessionID: "session-1",
	}, nil)
	// Silently starting a fresh conversation would drop the prior context while
	// the caller believed it was continuing.
	if err == nil || !strings.Contains(err.Error(), "resume") {
		t.Fatalf("expected a resume rejection, got %v", err)
	}
}

func TestDshRunnerStreamsFromTheHarnessLog(t *testing.T) {
	root := t.TempDir()
	// Stand in for the harness: write the session log where the patch points,
	// then print the final message the way the headless profile does.
	binary := stubDshHarness(t, dshSuccessLog)
	runner := NewDshRunner(binary, root)
	runner.now = fixedNow()

	var events []Event
	result, err := runner.Run(context.Background(), Request{
		Prompt: "write a summary", Workspace: t.TempDir(), Sandbox: SandboxWorkspaceWrite,
	}, func(event Event) { events = append(events, event) })
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !result.Succeeded || result.FinalMessage != "Writing the summary." {
		t.Fatalf("result = %+v", result)
	}
	kinds := map[EventKind]int{}
	for _, event := range events {
		kinds[event.Kind]++
	}
	if kinds[KindStarted] != 1 || kinds[KindToolUse] != 3 || kinds[KindResult] != 1 {
		t.Fatalf("events = %v", kinds)
	}
}

// stubDshHarness writes a script that behaves like the headless profile: it
// reads the root out of the patch file it was handed, creates a session
// directory there, writes the log, and prints the final message.
func stubDshHarness(t *testing.T, log string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "dsh")
	script := `#!/bin/sh
patch=""
while [ $# -gt 0 ]; do
  if [ "$1" = "--patch" ]; then patch="$2"; fi
  shift
done
root=$(sed -n "s/^    root: '\(.*\)'$/\1/p" "$patch")
session="$root/project/session-stub"
mkdir -p "$session"
cat <<'ONECATCH_EOF' > "$session/` + dshTranscriptFile + `"
` + log + `
ONECATCH_EOF
printf '%s\n' 'Writing the summary.'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

func TestDshPollIntervalIsResponsive(t *testing.T) {
	// The harness coalesces log writes on a 200ms window by default; polling
	// slower than that would make the live run log visibly lag the agent.
	if dshPollInterval > 200*time.Millisecond {
		t.Fatalf("poll interval %v is slower than the harness's write batching", dshPollInterval)
	}
}
