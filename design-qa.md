# Design QA — Review

## Evidence

- Source visual truth: `/var/folders/b1/0fd1b6hs7lz0fm_mh346lybm0000gn/T/codex-clipboard-ebd3f00c-4b7c-4eb9-96f8-758684d11d8f.png`.
- Browser-rendered implementation: `http://127.0.0.1:4173/`.
- Implementation capture: `/tmp/onecatch-review-implementation.png`.
- State: light theme, existing task conversation, file-change card selected, Review inspector expanded.

## Findings

No actionable P0, P1, or P2 differences remain in the requested scope.

- Entry point: each grouped file-change card exposes a dedicated Review action.
- Panel behavior: selecting Review expands the existing inspector, makes Review the active tab, and preserves the conversation alongside it.
- Information hierarchy: the toolbar presents title, file count, aggregate additions/deletions, refresh, and close controls.
- Diff layout: files render continuously with path headers, per-file statistics, scope labels, hunk headers, old/new line numbers, and semantic addition/deletion colors.
- Navigation: the right column groups files by directory and scrolls the selected file into view.
- Repository state: staged, worktree, and untracked changes are merged into one review model without losing their source scope.
- Responsive behavior: at narrow inspector widths the file navigation collapses and the diff remains horizontally scrollable instead of being clipped.
- Visual match: the implementation follows the reference's split conversation/review composition, dense code-review typography, quiet chrome, and compact file navigator while retaining OneCatch's existing design tokens and inspector controls.

## Interaction and Accessibility Checks

- Review entry, refresh, close, inspector tab, and file navigation are semantic buttons with accessible names.
- The Review region and changed-files navigation expose explicit landmarks.
- The selected inspector tab and selected file both have visible active states.
- Demo and production data paths were both exercised: demo data verifies the UI, while production bindings load Git status, staged diff, worktree diff, and untracked file content.

## Verification

- Browser interaction: opened an existing task, selected the file-change card's Review action, and confirmed the Review tab became active.
- Browser DOM: confirmed two files, aggregate `+4 −2`, per-file statistics, hunks, line numbers, and directory-grouped navigation.
- Reference/implementation comparison: inspected together at the same desktop state; no blocking layout or hierarchy mismatch found.
- Parser and integration tests: passed.
- Production frontend build: passed.

## Implementation Checklist

- [x] Add a Review action to conversation file-change cards.
- [x] Add a Review tab to the existing inspector.
- [x] Parse staged and worktree unified diffs.
- [x] Include untracked file previews.
- [x] Render continuous per-file diffs with line numbers and change stats.
- [x] Add directory-grouped file navigation.
- [x] Support refresh, close, selection, and responsive layout.
- [x] Verify the complete interaction in a rendered browser.

final result: passed
