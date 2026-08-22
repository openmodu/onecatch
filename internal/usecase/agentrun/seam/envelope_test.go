package seam

import (
	"strings"
	"testing"
)

// The fixtures below are verbatim captures from Claude Code 2.1.239, taken by
// pointing CLAUDE_CODE_SHELL_PREFIX at a recorder. They are the reason this
// test can run offline: the conformance suite re-captures them against the
// installed harness, and this one asserts the parser still handles what was
// captured before. A failure here means the parser regressed; a failure in the
// conformance suite means the harness changed.

// modelSimple is a Bash tool call with no plugins in the session.
const modelSimple = `source /Users/op/.claude/shell-snapshots/snapshot-zsh-1787362812008-dw1bsv.sh 2>/dev/null || true && { shopt -u extglob || setopt NO_EXTENDED_GLOB NO_BARE_GLOB_QUAL; } >/dev/null 2>&1 || true && { \builtin unalias -- 'unsetenv'; \builtin unset -f -- 'unsetenv'; } >/dev/null 2>&1 || true && eval 'echo REACH_PROBE_MARKER_1' < /dev/null && pwd -P >| /tmp/claude-2d13-cwd`

// modelWithPluginInjection is the same call in a session with a plugin whose
// SessionStart hook exports variables into the envelope. Note that it spans
// three lines and that the injected block is terminated by a bare `:` so the
// && chain stays valid. Neither is documented anywhere; both were observed.
const modelWithPluginInjection = `source /Users/op/.claude/shell-snapshots/snapshot-zsh-1787363270459-a9m6wp.sh 2>/dev/null || true && export CODEX_COMPANION_SESSION_ID='f8475cda-a512-45e6-a8f3-8fb4dd3adbeb'
export CLAUDE_PLUGIN_DATA='/Users/op/.claude/plugins/data/codex-openai-codex'
: && { shopt -u extglob || setopt NO_EXTENDED_GLOB NO_BARE_GLOB_QUAL; } >/dev/null 2>&1 || true && { \builtin unalias -- 'unsetenv'; \builtin unset -f -- 'unsetenv'; } >/dev/null 2>&1 || true && eval 'ls -la "/srv/app"' < /dev/null && pwd -P >| /tmp/claude-958f-cwd`

func TestParseModelCall(t *testing.T) {
	got := ParseClaudeCode(modelSimple)
	if got.Kind != KindModel {
		t.Fatalf("Kind = %v, want KindModel", got.Kind)
	}
	if want := "echo REACH_PROBE_MARKER_1"; got.Command != want {
		t.Errorf("Command = %q, want %q", got.Command, want)
	}
	if want := "/tmp/claude-2d13-cwd"; got.CwdFile != want {
		t.Errorf("CwdFile = %q, want %q", got.CwdFile, want)
	}
	// Nothing from the preamble may survive into the forwarded command: the
	// snapshot path alone discloses the operator's username to the target.
	for _, leak := range []string{"shell-snapshots", "shopt", "unalias", "/Users/op"} {
		if strings.Contains(got.Command, leak) {
			t.Errorf("forwarded command still contains %q: %s", leak, got.Command)
		}
	}
}

// The plugin-injected case is the one a preamble-stripping parser gets wrong.
// Removing known segments one at a time leaves the injected exports behind,
// and they carry the operator's home directory to the target.
func TestParsePluginInjectedEnvelope(t *testing.T) {
	got := ParseClaudeCode(modelWithPluginInjection)
	if got.Kind != KindModel {
		t.Fatalf("Kind = %v, want KindModel", got.Kind)
	}
	if want := `ls -la "/srv/app"`; got.Command != want {
		t.Errorf("Command = %q, want %q", got.Command, want)
	}
	if want := "/tmp/claude-958f-cwd"; got.CwdFile != want {
		t.Errorf("CwdFile = %q, want %q", got.CwdFile, want)
	}
	if strings.Contains(got.Command, "CLAUDE_PLUGIN_DATA") ||
		strings.Contains(got.Command, "CODEX_COMPANION_SESSION_ID") {
		t.Errorf("injected exports leaked into the forwarded command: %s", got.Command)
	}
	// The disclosure is reported so a caller can decide what to do about it.
	var sawHome bool
	for _, p := range got.LocalPaths {
		if strings.HasPrefix(p, "/Users/op/") {
			sawHome = true
		}
	}
	if !sawHome {
		t.Errorf("LocalPaths did not report the operator's home paths: %v", got.LocalPaths)
	}
}

// Claude Code's own shell use carries no envelope. Routing these to a remote
// target breaks every hook in the session: they reference local paths and
// local interpreters that do not exist there.
func TestParseInternalInvocations(t *testing.T) {
	for _, raw := range []string{
		`node "${CLAUDE_PLUGIN_ROOT}/scripts/session-lifecycle-hook.mjs" SessionStart`,
		`node "${CLAUDE_PLUGIN_ROOT}/scripts/stop-review-gate-hook.mjs"`,
		`bun run --cwd /Users/op/.claude/plugins/cache/claude-plugins-official/telegram/0.0.7 --shell=bun --silent start`,
	} {
		got := ParseClaudeCode(raw)
		if got.Kind != KindInternal {
			t.Errorf("Kind = %v for %q, want KindInternal", got.Kind, raw)
		}
		if got.Command != raw {
			t.Errorf("Command = %q, want it forwarded verbatim (%q)", got.Command, raw)
		}
		if got.CwdFile != "" {
			t.Errorf("CwdFile = %q, want empty", got.CwdFile)
		}
	}
}

func TestParseQuotingAndEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"plain", `echo hello`},
		{"double quotes", `grep -n "foo bar" /srv/app`},
		// Both escape idioms. The sandwich is what Claude Code 2.1.239 emits,
		// confirmed by the conformance suite; the backslash form is what most
		// other quoting produces and costs nothing to accept.
		{"single quote, sandwich idiom", `echo "it'"'"'s fine"`},
		{"single quote, backslash idiom", `echo it'\''s fine`},
		{"chained", `cd /srv/app && go build ./... && echo done`},
		// A command that itself ends in the envelope's own terminator: the
		// closing quote has to be found from the right, or the parse cuts
		// short and forwards a truncated command.
		{"contains the eval terminator", `cat foo' < /dev/null bar`},
		// A command containing the word eval, which a left-anchored search for
		// the opening quote would otherwise latch onto.
		{"contains eval", `eval 'inner'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := buildEnvelope(tt.command, "/tmp/claude-abcd-cwd")
			got := ParseClaudeCode(raw)
			if got.Kind != KindModel {
				t.Fatalf("Kind = %v, want KindModel", got.Kind)
			}
			want := unquoteSingle(tt.command)
			if got.Command != want {
				t.Errorf("Command = %q, want %q", got.Command, want)
			}
			if got.CwdFile != "/tmp/claude-abcd-cwd" {
				t.Errorf("CwdFile = %q", got.CwdFile)
			}
		})
	}
}

// An envelope with no cwd redirect still parses; the caller then has no local
// file to reproduce and `cd` does not persist, which is the harness's choice
// rather than an error.
func TestParseWithoutCwdRedirect(t *testing.T) {
	raw := `source /Users/op/.claude/shell-snapshots/s.sh 2>/dev/null || true && eval 'echo hi' < /dev/null`
	got := ParseClaudeCode(raw)
	if got.Kind != KindModel {
		t.Fatalf("Kind = %v, want KindModel", got.Kind)
	}
	if got.Command != "echo hi" {
		t.Errorf("Command = %q", got.Command)
	}
	if got.CwdFile != "" {
		t.Errorf("CwdFile = %q, want empty", got.CwdFile)
	}
}

// Envelope machinery with no recognisable command must not be classified as
// internal: running it locally is the failure this package exists to prevent.
func TestParseUnknownShapeRoutesToTarget(t *testing.T) {
	raw := `source /Users/op/.claude/shell-snapshots/s.sh 2>/dev/null || true && something-new 'echo hi'`
	got := ParseClaudeCode(raw)
	if got.Kind != KindUnknown {
		t.Fatalf("Kind = %v, want KindUnknown", got.Kind)
	}
	if got.Command != raw {
		t.Errorf("an unrecognised envelope must be forwarded whole, got %q", got.Command)
	}
}

// buildEnvelope reproduces the harness's wrapper around an already-escaped
// command, so the quoting cases exercise the same shape the harness emits.
func buildEnvelope(escapedCommand, cwdFile string) string {
	var b strings.Builder
	b.WriteString(`source /Users/op/.claude/shell-snapshots/snapshot-zsh-1.sh 2>/dev/null || true`)
	b.WriteString(` && { shopt -u extglob || setopt NO_EXTENDED_GLOB NO_BARE_GLOB_QUAL; } >/dev/null 2>&1 || true`)
	b.WriteString(` && { \builtin unalias -- 'unsetenv'; } >/dev/null 2>&1 || true`)
	b.WriteString(` && eval '`)
	b.WriteString(escapedCommand)
	b.WriteString(`' < /dev/null`)
	if cwdFile != "" {
		b.WriteString(` && pwd -P >| `)
		b.WriteString(cwdFile)
	}
	return b.String()
}
