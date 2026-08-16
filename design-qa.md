# Design QA

## Evidence

- Source visual truth: `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-c7baca24-e239-423d-8280-dee1c50a945b.png`
- Source pixels: 2558 × 1598 (`@2x` desktop capture, normalized to 1279 × 799 CSS pixels and padded by one pixel to 1280 × 800).
- Browser-rendered implementation: `/Users/ityike/.codex/visualizations/2026/08/12/019ff672-80eb-72c1-8979-8c04946edc3e/harness-toolbar-final.png`
- Implementation viewport and pixels: 1280 × 800 CSS px, 1280 × 800 captured pixels.
- Full-view comparison: `/Users/ityike/.codex/visualizations/2026/08/12/019ff672-80eb-72c1-8979-8c04946edc3e/harness-toolbar-comparison.png`
- Focused composer comparison: `/Users/ityike/.codex/visualizations/2026/08/12/019ff672-80eb-72c1-8979-8c04946edc3e/harness-toolbar-focused-comparison.png`
- State: light theme, new-task composer, Harness menu open, Codex selected, workflow and model controls visible.

## Findings

No actionable P0, P1, or P2 differences remain in the requested Harness/workflow scope.

- Fonts and typography: the toolbar keeps the existing Oneshot UI family and optical weights. Workflow, Harness, model summary, menu choices, and submit copy remain legible at the normalized reference viewport; long workflow names truncate without colliding with adjacent controls.
- Spacing and layout rhythm: the composer retains the reference’s single bottom control row. Harness now occupies the intentionally empty slot after Workflow, while the model profile stays right-aligned and the split submit action remains fully visible.
- Colors and visual tokens: the implementation uses the existing warm neutral background, muted pill, popover, border, and foreground tokens. The Harness popover has the same quiet surface, radius, and elevation language as the other app menus.
- Image quality and asset fidelity: the target contains no custom imagery. Existing Lucide chevrons and Radix menu indicators are retained; no placeholder or handcrafted icon asset was introduced.
- Copy and content: the external Harness menu exposes `Codex`, `Claude Code`, and `modu_code`. Workflow remains a separate control with `单 Agent 完成`, `实现与审查 Loop`, and `并行审查 DAG`. The model popup no longer repeats Harness.

## Full-view Comparison

The normalized side-by-side view confirms that the task composer keeps the same hierarchy as the source: prompt field above, compact controls below, model profile near the submit action, and a menu opening upward without obscuring the primary action. The implementation also preserves the currently open right inspector; that shell difference is intentional existing product state and does not alter the requested composer interaction.

## Focused-region Comparison

The focused side-by-side crop makes the requested change readable at 1:1 CSS density. In the source, Harness is nested inside the model popup and the toolbar contains an empty slot. In the implementation, Harness is a dedicated pill in that slot, its three choices appear in a solid popover, Workflow remains immediately to its left, and the model summary remains immediately to its right. The control sizes, radii, baseline alignment, and upward menu anchoring are consistent with the source.

## Comparison History

1. Initial browser capture: `/Users/ityike/.codex/visualizations/2026/08/12/019ff672-80eb-72c1-8979-8c04946edc3e/harness-toolbar-initial-overflow.png`
   - P2 finding: with the inspector open at a 1280 × 720 viewport, the combined workflow/Harness/model/submit row was wider than the conversation column and clipped the right side of the persistent submit control.
   - Fix: reduced the minimum workflow/Harness widths and gave the new-task runtime trigger a 190 px responsive minimum while preserving the full label through ellipsis.
2. Interaction pass found a P2 polish issue in the first TUI Select implementation: the item-aligned Harness popup visually blended into the white composer surface.
   - Fix: replaced the Harness selector surface with the app’s existing Radix dropdown-menu primitive, with a 190 px solid popover, radio selection, disabled-runtime metadata, and keyboard focus behavior.
3. Post-fix browser capture: `/Users/ityike/.codex/visualizations/2026/08/12/019ff672-80eb-72c1-8979-8c04946edc3e/harness-toolbar-final.png`
   - Post-fix evidence: both `/Users/ityike/.codex/visualizations/2026/08/12/019ff672-80eb-72c1-8979-8c04946edc3e/harness-toolbar-comparison.png` and `/Users/ityike/.codex/visualizations/2026/08/12/019ff672-80eb-72c1-8979-8c04946edc3e/harness-toolbar-focused-comparison.png` show the full submit control and a clearly separated Harness popover. No P0/P1/P2 issue remains.

## Interaction and Accessibility Checks

- Harness opens as a named menu button and exposes three `menuitemradio` choices.
- Selecting Claude Code leaves the chosen workflow unchanged and changes the runtime summary to the detected Claude model and supported reasoning value.
- Codex exposes model, reasoning, and speed; Claude Code exposes model and reasoning; `modu_code` omits unsupported reasoning/speed controls.
- The runtime-profile menu contains only model-capability rows and no duplicate Harness row.
- Workflow opens independently and exposes the single-Agent, loop, and DAG choices.
- Unavailable runtime binaries remain visible but disabled and labelled `未检测到`.
- Browser console check found no application errors; only the expected Wails browser-preview warning is present.

## Verification

- Production Vite build: passed.
- Targeted frontend tests for Harness metadata, switching, layout structure, and i18n: 5 passed.
- Relevant Go packages: `go test ./internal/domain/tasks ./internal/service/desktop` passed.
- Full static frontend suite: 29 passed and 6 pre-existing sidebar/command-palette assertions failed; none intersects this feature.

## Implementation Checklist

- [x] Move Harness out of the model popup and into the composer toolbar.
- [x] Expose Codex, Claude Code, and `modu_code` through one extensible catalog.
- [x] Keep Workflow as an independent selector with single-Agent and multi-step options.
- [x] Clear incompatible model/effort/speed choices when switching Harness.
- [x] Inspect Codex and Claude model capabilities and hide unsupported controls.
- [x] Accept all three Harness IDs in task-domain validation.
- [x] Preserve a separate read-only Harness pill during an active run.
- [x] Verify browser interactions, 1280 × 720 fit, 1280 × 800 visual parity, frontend build, and backend validation.

final result: passed
