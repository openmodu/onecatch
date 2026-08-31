# Design QA — Sidebar update control

## Comparison target

- Source visual truth: `/var/folders/b1/0fd1b6hs7lz0fm_mh346lybm0000gn/T/codex-clipboard-78a1fe0f-7298-414f-bd6b-e86c783f4f77.png`
- Rendered implementation, available state: `/Users/bytedance/Code/go/src/github.com/openmodu/onecatch/artifacts/design-qa/update-button-light-available.png`
- Rendered implementation, downloading state: `/Users/bytedance/Code/go/src/github.com/openmodu/onecatch/artifacts/design-qa/update-button-downloading.png`
- Rendered implementation, ready state: `/Users/bytedance/Code/go/src/github.com/openmodu/onecatch/artifacts/design-qa/update-button-ready.png`
- Full comparison evidence: `/Users/bytedance/Code/go/src/github.com/openmodu/onecatch/artifacts/design-qa/comparison-full-light.png`
- Focused footer comparison evidence: `/Users/bytedance/Code/go/src/github.com/openmodu/onecatch/artifacts/design-qa/comparison-footer-light.png`
- Browser viewport / implementation CSS size: `1182 × 704` at device scale factor `1`.
- Source pixels: `1182 × 676`. The source is an annotated partial desktop capture rather than the same full app state.
- Implementation pixels: `1182 × 704`.
- Density normalization: the source footer crop (`424 × 168`) was downsampled to `212 × 84` to match the implementation sidebar footer crop (`212 × 84`). The full comparison pads the source to `1182 × 704` without scaling.
- Compared state: light theme, new version available, sidebar expanded. The source specifies the target slot only; it does not prescribe the update icon or prompt styling.

## Full-view comparison

The implementation preserves the existing OneCatch layout and adds no new page or route. The update control remains confined to the bottom navigation row, so the task list, composer, and main content proportions are unchanged. The prompt opens upward inside the sidebar and does not cover the main workbench.

## Focused footer comparison

The update control center aligns with the annotated empty slot to the right of “菜单”. The menu icon and label retain their original baseline and left padding. The 28 px update button stays inside the rounded sidebar inset, while the available-state dot and callout remain readable in both light and dark themes.

## Required fidelity surfaces

- Fonts and typography: existing application font, weights, sizes, and line heights are reused. The prompt uses the same small UI hierarchy as current menus and tooltips; no wrapping or truncation issue was observed.
- Spacing and layout rhythm: the footer remains 52 px high. The update button is 28 px, inset 16 px from the right, and vertically centered at 12 px from the row top. Its center matches the annotated target after density normalization.
- Colors and visual tokens: the control uses existing sidebar, primary, muted, border, popover, and destructive tokens. Light and dark themes were both rendered and inspected; contrast and state emphasis remain consistent with the product.
- Image quality and asset fidelity: no raster asset was required. Icons come from the product's existing Lucide icon library. The progress indicator is a vector data gauge shared with the existing context gauge and remains sharp at the compact size.
- Copy and content: available, download progress, verification, error, and restart labels are localized in Chinese and English. The visible prompt says a new version was found and that the right-side button downloads and verifies it.

## Interaction and accessibility checks

- Checked for updates from the idle button.
- Confirmed the available-version callout, notification dot, and download action.
- Confirmed a determinate circular download indicator with a live numeric percentage (`84%` capture).
- Confirmed the verifying transition and ready-to-restart action.
- Confirmed the demo restart returns to the idle/check state.
- Confirmed every state has an accessible button label, busy state, tooltip, and polite live announcement for the available-version prompt.
- Checked browser console: no application errors. The only warning is Wails' expected notice that native bindings are unavailable in a standalone browser preview.

## Findings

No actionable P0, P1, or P2 visual or interaction findings remain.

## Comparison history

### Iteration 1

- Earlier finding: `[P1]` the standalone preview entered the checking spinner but did not commit the asynchronous available state under React Strict Mode.
- Fix: reset the mounted lifecycle guard in effect setup so Strict Mode's setup/cleanup/setup development cycle leaves the live instance writable.
- Post-fix evidence: `update-button-light-available.png` shows the available prompt; `update-button-downloading.png` shows the determinate circular progress; `update-button-ready.png` shows the restart state.

### Iteration 2

- Recompared the normalized source and implementation footer crops in `comparison-footer-light.png`.
- Result: no P0/P1/P2 differences. The control occupies the requested slot, preserves the menu row alignment, and remains inside the sidebar radius.

## Follow-up polish

No P3 refinement is required for this scope.

final result: passed
