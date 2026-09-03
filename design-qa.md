# Design QA — Sidebar update visibility

## Comparison target

- Source visual truth: `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-d11c283d-3532-4559-b380-35fdbc3b925b.png`.
- Browser-rendered implementation, no-update state: `/tmp/oneshot-update-idle-hidden.png`.
- Browser-rendered implementation, update-available state: `/tmp/oneshot-update-available.png`.
- Focused comparison input: `/tmp/oneshot-update-comparison.png` (source, no-update state, available state from left to right).
- Browser URL: `http://127.0.0.1:9245/`.
- Browser viewport / CSS size: `1280 × 720` at device scale factor `1`.
- Source pixels: `652 × 434`; the source is a partial `@2x` desktop capture.
- Implementation pixels: `1280 × 720` for both rendered states.
- Density normalization: the source was downsampled to `326 × 217` before its footer crop was compared with the implementation's `1x` footer crops.
- Compared states: light theme; current/no-update state and version `1.2.3` available.

## Full-view comparison

The no-update implementation removes the former refresh control without changing the rest of the workbench. The menu action expands into the released footer space. When an update is available, the existing 36 px filled download action returns in the same right-hand footer slot.

## Focused comparison

The combined comparison shows the requested transition directly: the annotated permanent refresh action is absent in the no-update state, while the available state presents a high-contrast download icon and the existing update callout. No additional page, route, or decorative asset was introduced.

## Required fidelity surfaces

- Fonts and typography: the existing OneCatch font stack, menu label, tooltip hierarchy, and update callout copy are unchanged.
- Spacing and layout rhythm: the update button retains its existing `36 × 36` footprint and footer alignment when visible; the hidden state leaves no blank grid cell.
- Colors and visual tokens: the available action keeps the existing info foreground/background tokens (`rgb(240, 248, 249)` on `rgb(33, 110, 120)`).
- Image quality and asset fidelity: no raster asset is needed. The download icon continues to come from the product's existing Lucide icon library and remains sharp.
- Copy and content: the available state exposes `发现 OneCatch 1.2.3，点击下载`; idle, checking, up-to-date, and background-check failure states add no sidebar copy or control.

## Interaction and accessibility checks

- Confirmed the browser-rendered no-update state contains zero `.sidebar-update-trigger` elements.
- Confirmed the browser-rendered available state contains an enabled 36 px download action with `data-update-state="available"`, `data-codex-download="true"`, and the localized accessible label.
- Confirmed known-update download failures remain visible and retryable; background-check failures without a known version remain hidden.
- Confirmed the settings page and native menus still provide manual update checks.
- Browser console errors: none.
- Focused updater tests: `5/5` passed.
- Development build: passed.

## Findings

No actionable P0, P1, or P2 visual, interaction, responsive, or accessibility differences remain for this scope.

## Comparison history

### Iteration 1

- Earlier finding: `[P1]` the sidebar permanently exposed a refresh/check action even when no update was available.
- Fix: gate the sidebar control to a known update lifecycle; keep idle, checking, up-to-date, unconfigured, and versionless error states hidden.
- Post-fix evidence: `/tmp/oneshot-update-comparison.png` shows the former refresh state, the corrected empty footer state, and the available download state together.

## Follow-up polish

No P3 refinement is required for this scope.

final result: passed

# Design QA — Markdown code blocks (2026-09-03)

## Comparison target and evidence

- Source visual truth: `/var/folders/qs/3_jyc8zx6c92f97cpx6tkq380000gn/T/codex-clipboard-a74fb9f1-f971-4fd9-82d6-fb582aca4443.png` (3010 × 1100 pixels), opened and inspected.
- Target state: a light-theme YAML block with a language label on the left, wrap/copy icon actions on the right, and syntax-colored code below.
- Implementation: `frontend/src/app/components/MarkdownContent.jsx` and its application-owned styles.
- Implementation screenshot: unavailable. Local browser access was denied in the preceding iteration; renewed permission was requested and has not been received.
- Viewport, CSS dimensions, and source density: not established. No density-normalized visual comparison was performed.
- Full-view and focused-region comparison evidence: unavailable until browser capture is authorized.

## Required fidelity surfaces

- Fonts and typography: retains the application's UI and monospace font stacks; rendered size and line-height comparison pending.
- Spacing and layout rhythm: implements a two-sided toolbar, inset code content, and rounded card; screenshot comparison pending.
- Colors and visual tokens: uses existing light/dark theme tokens, with YAML keys in the warning color and scalar/string values in the success color; visual contrast verification pending.
- Image quality and asset fidelity: no raster assets are needed. CodeXml, WrapText, Copy, Check, and TriangleAlert use the existing Lucide icon library. Reference annotations and watermarks are not UI assets.
- Copy and content: uses the reference's language-label structure and icon-only controls. Copy's tooltip remains “复制” / “Copy”; line-wrap and status labels are localized.

## Implementation and non-visual checks

- Added per-block wrapping, semantic pressed state, and copy feedback without placing toolbar text inside the copied code.
- Reused Prism with common fence-language aliases; unknown languages fall back to escaped plain text.
- Preserved inline code, streaming fences, source whitespace, and the existing link/image safety policy.
- Real React/Streamdown server-rendering tests cover YAML toolbar/token output, inline code, unfinished streaming fences, unknown languages, and indented blocks.
- Pure highlighting tests cover aliases, source preservation, and untrusted HTML/language names.
- Browser interactions and console checks: not performed; server-rendering tests do not substitute for browser verification.

## Findings and comparison history

1. Reference inspected and implementation updated; no browser-rendered comparison has been made.
2. Visual fidelity, keyboard/pointer interaction, mobile-width layout, and dark-theme appearance remain unverified. This is a verification blocker, not a claimed visual finding.

## Remaining checklist

1. Obtain permission for local browser preview.
2. Capture the matching reference state and compare normalized full/focused views together.
3. Test wrap/copy, keyboard focus, light/dark themes, narrow widths, and console errors.
4. Fix any P0/P1/P2 findings and repeat the visual comparison.

final result: blocked
