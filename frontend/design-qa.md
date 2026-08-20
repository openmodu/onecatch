# Design QA — Claude model menu

## Evidence

- Source visual truth: `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-536c2297-264b-4edd-b20b-461c079ce6f3.png` (`1194 × 632`), the earlier Claude model-menu reference. The newly attached `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-c1533f2b-1906-48af-b0f9-01d3a9f7c487.png` is the reported incorrect flat-menu state.
- Browser-rendered implementation: `http://127.0.0.1:4173/`.
- Implementation screenshot: `/tmp/onecatch-claude-menu-clean.png` (`1280 × 720`, browser CSS viewport `1280 × 720`, device scale `1`).
- Full-view comparison: `/tmp/onecatch-claude-menu-clean-full-comparison.jpg`; source and implementation were normalized to `1200 px` width and stacked in one comparison input.
- Focused comparison: `/tmp/onecatch-claude-menu-clean-comparison.jpg`; the `@2x` reference crop was downsampled to `0.5x` and compared beside the `1x` implementation crop, preserving true CSS-scale proportions for typography, row height, panel width, badge, check, and submenu direction.
- State: light theme, Claude Code selected, Opus 5 inherited as default, Model menu open, More models submenu open to the right.

## Findings

No actionable P0, P1, or P2 differences remain in the supported scope.

- Fonts and typography: aliases render as versioned `Fable 5`, `Opus 5`, and `Sonnet 5`; the `13 px` system typography, compact `26 px` rows, `Default` badge, and single trailing selected check follow the reference hierarchy.
- Spacing and layout rhythm: the `240 px` main menu, compact alias rows, separator, More models row, right-opening `140 px` submenu, `10 px` radii, border, and softened shadow follow the reference's normalized proportions.
- Colors and visual tokens: the selected check uses the reference's blue accent; the Default badge and open More models row use quiet neutral fills with sufficient contrast.
- Image quality and asset fidelity: no bitmap or decorative image assets are present; library icons remain vector-sharp.
- Copy and content: alias models remain in the main list and raw `claude-*` models move into More models. Only models advertised by the installed Claude CLI are selectable.
- Expected capability differences: usage-credit metadata, numeric keyboard shortcuts, unadvertised models, and Fast mode are intentionally absent because the current Claude configuration contract does not expose them; the UI does not fabricate unsupported models or controls.

## Comparison history

- Earlier findings: `[P1]` aliases and a raw `claude-fable-5` id were presented as one flat list, with no version labels, default badge, or More models hierarchy; `[P2]` the submenu initially flipped left and overlapped the composer; `[P2]` the first structured pass still had oversized row spacing, an overly wide/tall More models submenu, heavy radius/shadow, and a duplicate leading Radix check overlapping `Opus 5`.
- Fixes made: added version-aware Claude labels, Opus default resolution, a Default badge, alias/full-model grouping, and a right-opening More models submenu; then normalized row heights, submenu width, radii, shadow, and open fill against the `@2x` reference and force-hidden the built-in leading indicator so only the trailing blue check remains.
- Post-fix evidence: `/tmp/onecatch-claude-menu-clean-full-comparison.jpg` and `/tmp/onecatch-claude-menu-clean-comparison.jpg` show the corrected proportions and clean selected row.

## Interaction and accessibility checks

- Switching the composer to Claude Code resolves the compact trigger to `Opus 5` when no explicit model is configured.
- Primary alias selection and an exact model inside More models both update the trigger and close the menu.
- The selected model exposes radio-menu checked state; More models exposes submenu expanded state.
- Browser console had no application errors before this component refinement; the hot-reloaded rendered state and all tested interaction paths completed without a runtime failure.

## Verification

- Pure runtime-option tests: passed (`3/3`).
- Focused composer regression test: passed (`1/1`).
- Development production-shaped build: passed.
- Browser interaction and DOM-state verification: passed.
- Residual test gap: unavailable Claude capabilities from the external reference cannot be verified because the installed CLI does not advertise them.

final result: passed
