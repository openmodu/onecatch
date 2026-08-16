# Design QA

## Evidence

- Source visual truth: `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-d6fc701d-aae5-4544-8eeb-13d7ae69ae27.png`
- Source pixels: 1776 × 774.
- Browser-rendered implementation: `/Users/ityike/.codex/visualizations/2026/08/12/019ff672-80eb-72c1-8979-8c04946edc3e/runtime-profile-menu-final.png`
- Implementation viewport: 1280 × 720 CSS px, browser device pixel ratio 2; the Browser capture is CSS-normalized to 1280 × 720 pixels.
- Normalized focused comparison: `/Users/ityike/.codex/visualizations/2026/08/12/019ff672-80eb-72c1-8979-8c04946edc3e/runtime-profile-comparison-normalized.png`
- Density normalization: the source focus crop was scaled by `1280 / 1776 = 0.7207` before comparison with the implementation crop.
- State: light theme, preview mode, new-task composer, runtime menu open, Harness `Codex`, model `GPT-5.6-Terra`, reasoning `超高`, speed `Fast`.

## Findings

No actionable P0, P1, or P2 differences remain in the requested runtime-profile scope.

- Fonts and typography: the final menu uses 14 px/13 px label and value type, close to the viewport-normalized Codex reference. Labels retain the product UI font and semibold hierarchy; values stay muted and truncate safely.
- Spacing and layout rhythm: the final 320 px menu width, 46 px rows, 16 px corner treatment, compact shadow, and 250 px summary pill track the normalized reference proportions. The extra Harness row is intentional and requested.
- Colors and tokens: the implementation keeps Oneshot's existing background, card, border, muted, foreground, and shadow tokens while preserving Codex's quiet neutral hierarchy.
- Image quality and asset fidelity: this control uses existing Lucide chevrons/loading indicators and native menu primitives. The reference contains no custom image, logo, or illustration asset that needs reproduction.
- Copy and content: the menu exposes Harness, 模型, 推理强度, and 速度. The summary pill shows the effective model, effort, and speed without opening the menu.

## Full-view Comparison

The source is a focused Codex composer crop rather than a complete application frame, so whole-shell parity cannot be judged from it without inventing missing visual context. The implementation full capture confirms the menu is anchored above the new-task composer, remains inside the shared chat column, does not obscure the send action, and preserves the existing sidebar and titlebar hierarchy.

## Focused-region Comparison

The normalized side-by-side comparison covers the visible source truth: summary pill, popup width, label/value alignment, chevrons, row density, radius, shadow, and composer anchoring. A focused region is necessary because these details are too small to judge reliably in the full 1280 px application capture.

Intentional differences:

- The new Harness row appears above Model because the user requested Harness selection first; this pass exposes Codex as the first supported Harness.
- The source's `高级` row is omitted because Oneshot directly exposes the requested model, effort, and speed controls and has no additional advanced runtime fields in this task flow.
- The menu uses Oneshot's warm neutral token rather than copying Codex's exact gray surface.

## Comparison History

1. Initial browser capture: `/Users/ityike/.codex/visualizations/2026/08/12/019ff672-80eb-72c1-8979-8c04946edc3e/runtime-profile-menu.png`
   - P2 finding: the menu typography and summary trigger were visibly denser and narrower than the viewport-normalized Codex reference.
   - Fix: increased the menu from 304 px to 320 px, rows from 43 px to 46 px, label/value type to 14 px/13 px, and summary trigger to a 250 px minimum width.
2. Post-fix browser capture: `/Users/ityike/.codex/visualizations/2026/08/12/019ff672-80eb-72c1-8979-8c04946edc3e/runtime-profile-menu-final.png`
   - Post-fix evidence: `/Users/ityike/.codex/visualizations/2026/08/12/019ff672-80eb-72c1-8979-8c04946edc3e/runtime-profile-comparison-normalized.png`
   - Result: the normalized menu and trigger proportions now match the reference closely; no P0/P1/P2 issue remains.

## Interaction and Accessibility Checks

- Opening the runtime-profile trigger exposes four named rows and preserves focus in the menu.
- Harness opens a radio submenu with Codex selected.
- Model selection switches from GPT-5.6-Sol to GPT-5.6-Terra and updates the summary.
- Reasoning selection switches to `超高` and updates the summary.
- Speed selection switches to `Fast` and updates the summary.
- All main rows, submenus, and radio choices are keyboard-accessible Radix menu primitives with accessible roles and names.
- The running composer shows its resolved Harness/model/effort/speed profile read-only, so an active run cannot be silently reconfigured.

## Verification

- Production Vite build: passed.
- Targeted frontend tests: 10 passed.
- Relevant Go packages: `go test ./internal/domain/tasks ./internal/service/desktop` passed.
- Run snapshot test verifies task-level Codex overrides are frozen into the run while a Claude workflow step keeps its own configured model.
- The full frontend suite still has 7 pre-existing sidebar/list assertion failures. The untracked `internal/usecase/blackboard/` work also prevents an unrelated `go test ./...`; neither failure touches this feature.

## Implementation Checklist

- [x] Select Harness first, with Codex available in this pass.
- [x] Read the actual Codex model catalog and detected defaults.
- [x] Select model, reasoning effort, and speed through nested menus.
- [x] Show the effective profile in the composer summary.
- [x] Persist task runtime choices and apply them only to matching Codex workflow steps.
- [x] Show the frozen runtime profile in the active-run composer.
- [x] Regenerate Wails bindings.
- [x] Verify browser interactions, focused visual fidelity, frontend build, and backend resolution.

final result: passed
