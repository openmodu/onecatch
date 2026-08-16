# Design QA

## Evidence

- Source visual truth: `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-824f8b1d-08b7-40cd-9220-dee5e86d317a.png`
- Browser-rendered implementation: `/Users/ityike/.codex/visualizations/2026/08/12/019ff672-80eb-72c1-8979-8c04946edc3e/conversation-tool-expanded.png`
- Side-by-side comparison: `/Users/ityike/.codex/visualizations/2026/08/12/019ff672-80eb-72c1-8979-8c04946edc3e/conversation-interleaving-comparison.png`
- Browser state: light theme, 1280 × 720 viewport, paused two-Agent demo conversation, second tool group and command result expanded.

## Findings

No actionable P0, P1, or P2 differences remain in the requested conversation scope.

- Message order: assistant prose, tool-call groups, later prose, and subsequent tool groups remain in provider event order. Tools are no longer hoisted above all prose in a round.
- Tool-call treatment: adjacent tool calls share one quiet disclosure surface. Each tool keeps its own icon, title, status, command, result, copy action, and nested expansion state.
- Conversation rhythm: prose stays on the transcript surface while tool details use a thin bordered inset, matching the supplied Claude Code pattern without introducing unrelated cards or separators.
- Completed-run interaction: the composer remains editable after a successful round, changes to continuation copy, accepts attachments, and exposes a single primary continuation action.
- Session continuity: continuing a completed serial run reuses the current Agent session. A completed DAG preserves upstream results and reopens only terminal nodes for the follow-up turn.

## Visual Comparison

The side-by-side comparison confirms the requested hierarchy in both reference and implementation: prose appears before a tool group, the tool group can expand to command/result detail, and prose resumes after it. The implementation retains Oneshot's existing sidebar, inspector, warm neutral tokens, and compact typography while adopting the reference's alternating transcript structure.

## Interaction and Accessibility Checks

- Both process disclosures and individual tool rows are keyboard-focusable native `details`/`summary` controls.
- Tool groups report a localized tool count and runtime through their accessible label.
- Expanding a tool exposes command and result without changing the surrounding message order.
- Completed conversations expose an enabled textbox and a localized `继续对话` action only after new text or an attachment is present.
- Follow-up instructions are persisted as `run.resumed` events and appear as user turns in the transcript.
- The browser-rendered paused conversation still supports its existing resume and terminate actions.

## Verification

- Relevant Go packages: `go test ./internal/domain/workflows ./internal/usecase/workflows ./internal/service/desktop` passed.
- Conversation, i18n, and focused layout tests: 21 passed.
- Production Vite build: passed.
- Full static frontend suite: 41 passed and 6 pre-existing sidebar/command-palette assertions failed; none intersects conversation ordering or completed-run continuation.
- Browser QA: message/tool interleaving, group expansion, command-result expansion, persistent composer, and existing paused-run actions verified.

## Implementation Checklist

- [x] Preserve text/tool-call/text ordering within every Agent round.
- [x] Group only adjacent tool calls.
- [x] Keep tool rows expandable with command and result details.
- [x] Keep completed conversations editable.
- [x] Continue serial runs through the existing Agent session.
- [x] Continue DAG runs from terminal nodes while preserving upstream context.
- [x] Cover ordering and completed continuation with frontend and backend regression tests.
- [x] Compare the reference and browser-rendered implementation visually.

final result: passed
