// Package seam decides what a coding agent's shell invocation actually means.
//
// A harness that lets us replace its shell hands us every shell invocation it
// makes, not only the model's. Claude Code's CLAUDE_CODE_SHELL_PREFIX carries
// three different kinds of traffic:
//
//   - the model's Bash tool call, wrapped in an envelope
//   - Claude Code's own use of a shell: SessionStart/Stop hooks, plugin
//     launches ("bun run --cwd ~/.claude/plugins/...")
//   - anything a plugin injected into the envelope on the way past
//
// Only the first belongs on the remote target. The second references local
// paths and local interpreters and must run locally, or every hook in the
// session fails. Telling them apart is this package's entire job, and it is a
// safety property rather than a nicety: a model command misrouted to the local
// machine executes on the operator's disk while the agent reports it acted on
// the target.
package seam

import (
	"path/filepath"
	"strings"
)

// Kind classifies one shell invocation.
type Kind int

const (
	// KindInternal is Claude Code's own use of a shell — a hook, a plugin
	// launch. It carries no envelope, references local paths, and must run on
	// the local machine.
	KindInternal Kind = iota

	// KindModel is the model's Bash tool call, recovered from its envelope.
	// It runs on the target.
	KindModel

	// KindUnknown is an invocation that carries some envelope machinery but
	// not a shape this package recognises.
	//
	// It is routed to the target, like KindModel, and the choice is
	// deliberate. The two ways of being wrong are not symmetric: an internal
	// command sent to the target fails loudly and recoverably, while a model
	// command run locally executes on the operator's own disk with the agent
	// reporting success against the target. Callers should log it — a steady
	// stream of KindUnknown means the harness changed shape and the
	// conformance suite needs a look.
	KindUnknown
)

func (k Kind) String() string {
	switch k {
	case KindInternal:
		return "internal"
	case KindModel:
		return "model"
	default:
		return "unknown"
	}
}

// Envelope is a decomposed shell invocation.
type Envelope struct {
	// Kind says where this invocation belongs.
	Kind Kind

	// Command is what to run. For KindModel it is the model's own command,
	// unwrapped from the envelope with none of the harness's bookkeeping. For
	// the other kinds it is the invocation verbatim.
	Command string

	// CwdFile is a path on the *local* machine where the harness expects the
	// post-command working directory to appear. It is how `cd` persists
	// between tool calls: the harness writes nothing there itself once the
	// redirect is stripped, so a caller that does not reproduce this file
	// silently breaks `cd`.
	//
	// The name is random per invocation, so it must be read from every
	// envelope and never cached.
	CwdFile string

	// Preamble is what was removed from in front of the command: the shell
	// snapshot source, plugin-injected exports, the shopt/unalias fixups.
	// Kept for diagnostics, never forwarded.
	Preamble string

	// LocalPaths are absolute paths belonging to this machine that appeared in
	// the preamble. They are the disclosure surface: forwarding the preamble
	// verbatim would hand the target the operator's username and directory
	// layout for no benefit. Reported rather than acted on, because the caller
	// decides how loud to be about it.
	LocalPaths []string
}

// evalSuffix closes the model's command inside a Claude Code envelope.
//
// The whole envelope reduces to `... && eval '<command>' < /dev/null [&& pwd
// -P >| <file>]`, and anchoring on this suffix is what makes the parse robust
// against everything in front of it. Enumerating the preamble segments — the
// snapshot source, the shopt fixup, the unalias fixup — was the obvious
// approach and it breaks the first time a plugin injects an `export` into the
// chain, which is a thing plugins do.
const evalSuffix = "' < /dev/null"

// ParseClaudeCode decomposes one CLAUDE_CODE_SHELL_PREFIX invocation.
//
// The observed envelope (Claude Code 2.1.239) is a single argv element, with
// no `-c` in front of it, and it may span several lines:
//
//	source <LOCAL_SNAPSHOT>.sh 2>/dev/null || true && export PLUGIN_VAR='...'
//	export CLAUDE_PLUGIN_DATA='/Users/<you>/.claude/plugins/data/...'
//	: && { shopt -u extglob || setopt ...; } >/dev/null 2>&1 || true
//	  && { \builtin unalias -- '...'; ... } >/dev/null 2>&1 || true
//	  && eval '<THE MODEL'S COMMAND>' < /dev/null
//	  && pwd -P >| /tmp/claude-<rand>-cwd
//
// The `export` lines and the bare `:` continuation come from a plugin's
// SessionStart hook, not from Claude Code, so their presence and shape are not
// something a parser can rely on. Everything before the `eval` is dropped.
func ParseClaudeCode(raw string) Envelope {
	env := Envelope{Command: raw}

	body, cwdFile := splitCwdRedirect(strings.TrimRight(raw, " \t\n"))
	env.CwdFile = cwdFile

	start, cmd, ok := splitEval(body)
	if !ok {
		// No eval wrapper. Either Claude Code's own shell use, or a shape this
		// parser does not know.
		if cwdFile != "" || hasSnapshotSource(raw) {
			env.Kind = KindUnknown
			return env
		}
		env.Kind = KindInternal
		return env
	}

	env.Kind = KindModel
	env.Command = cmd
	env.Preamble = strings.TrimSpace(body[:start])
	env.LocalPaths = absolutePathsIn(env.Preamble)
	return env
}

// splitCwdRedirect removes a trailing `&& pwd -P >| <file>` and returns the
// file it named.
//
// Forwarded verbatim this redirect would be written on the *target* while the
// harness reads it *locally*, and `cd` would stop persisting between tool
// calls with no error anywhere — the harness would simply start every command
// in the original directory again. Both `>|` and `>` are accepted; the
// noclobber-defeating form is what has been observed.
func splitCwdRedirect(s string) (body, cwdFile string) {
	const marker = "&& pwd -P >"
	idx := strings.LastIndex(s, marker)
	if idx < 0 {
		return s, ""
	}
	rest := strings.TrimSpace(s[idx+len(marker):])
	rest = strings.TrimPrefix(rest, "|")
	rest = strings.TrimSpace(rest)
	if rest == "" || strings.ContainsAny(rest, " \t\n") {
		// Not the shape we know; leave the string alone rather than guessing.
		return s, ""
	}
	return strings.TrimRight(s[:idx], " \t\n"), rest
}

// splitEval recovers the model's command from `eval '<escaped>' < /dev/null`.
//
// The command is a POSIX single-quoted word, so a literal quote inside it
// arrives as one of the escape idioms unquoteSingle folds back. The closing
// quote is
// found from the right: a command that itself contains "' < /dev/null"
// produces an earlier false candidate, and only the last one is the real
// terminator.
func splitEval(s string) (start int, command string, ok bool) {
	s = strings.TrimRight(s, " \t\n")
	if !strings.HasSuffix(s, evalSuffix) {
		return 0, "", false
	}
	end := len(s) - len(evalSuffix) // index of the closing quote

	// The first `eval '` is the harness's own: the preamble in front of it
	// contains no eval, while an escaped one inside the command necessarily
	// comes after it.
	open := strings.Index(s, "&& eval '")
	if open < 0 {
		open = strings.Index(s, "eval '")
		if open < 0 || open > end {
			return 0, "", false
		}
		start = open
		open += len("eval '")
	} else {
		start = open
		open += len("&& eval '")
	}
	if open > end {
		return 0, "", false
	}
	return start, unquoteSingle(s[open:end]), true
}

// unquoteSingle folds a shell's single-quote escape idioms back into literal
// quotes.
//
// There are two of them, and which one appears is the harness's choice rather
// than something a parser can assume. Claude Code 2.1.239 uses the
// double-quote sandwich
//
//	'"'"'
//
// while the backslash form
//
//	'\''
//
// is what most hand-written quoting produces. Both close the single-quoted
// run, contribute one literal quote, and reopen it; both have to be folded, or
// the recovered command differs from what the model asked for by exactly the
// characters most likely to matter — the ones inside a string literal.
//
// The sandwich is folded first: it contains no backslash, so the two patterns
// cannot overlap and the order only decides which is tried first.
//
// Everything else inside a POSIX single-quoted run is literal, including
// backslashes, so nothing but these two sequences is interpreted.
func unquoteSingle(s string) string {
	s = strings.ReplaceAll(s, `'"'"'`, `'`)
	return strings.ReplaceAll(s, `'\''`, `'`)
}

// hasSnapshotSource reports whether the invocation begins by sourcing the
// harness's local shell snapshot, which is the other marker of a model call.
func hasSnapshotSource(raw string) bool {
	trimmed := strings.TrimLeft(raw, " \t\n")
	if !strings.HasPrefix(trimmed, "source ") && !strings.HasPrefix(trimmed, ". ") {
		return false
	}
	return strings.Contains(trimmed, "2>/dev/null")
}

// absolutePathsIn reports the absolute local paths a preamble mentions.
//
// This is a report, not a rewrite. Scanning an arbitrary command for the
// operator's home directory and editing it out trades a disclosure for a
// mangled command, and a mangled command is the worse failure: it fails
// somewhere the agent cannot attribute. What the caller does with this is a
// policy decision — log it, refuse an untrusted target, or ignore it.
func absolutePathsIn(preamble string) []string {
	var out []string
	seen := map[string]bool{}
	for _, field := range strings.FieldsFunc(preamble, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\'' || r == '"' || r == ';'
	}) {
		if !strings.HasPrefix(field, "/") || len(field) < 2 {
			continue
		}
		if !filepath.IsAbs(field) || seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}
