# Design QA: Harness settings navigation

- Source visual truth: `/var/folders/b1/0fd1b6hs7lz0fm_mh346lybm0000gn/T/codex-clipboard-df6f2391-8f14-471b-bddc-4b1745df8187.png`
- Runtime implementation: `/tmp/onecatch-runtime-settings.png`
- Harness implementation: `/tmp/onecatch-harness-no-tabs.png`
- Browser: Codex in-app browser
- URL: `http://localhost:4173/`
- Viewport: 1280 × 720 CSS px
- Source pixels: 1960 × 1656
- Implementation pixels: 1280 × 720 for both captures
- Device pixel ratio reported by the page: 2; browser captures were normalized to CSS-pixel dimensions
- State: Chinese, light theme; Runtime page with Codex selected and Harness page showing all Agent cards

## Full-view comparison evidence

The annotated source identifies two intended information-architecture changes rather than a pixel-for-pixel final screen: add a Harness item in the settings sidebar between Runtime and Terminal, and move per-harness Agent configuration out of Runtime. The implementation captures show both changes at the same desktop settings state. Runtime now contains environment and launch controls; Harness owns model, reasoning, service-tier, and provider defaults.

## Focused region comparison evidence

The sidebar and the first content card were inspected at readable scale because they contain the marked source regions. Harness occupies the annotated empty sidebar slot, uses the existing selected-navigation treatment, and presents Codex, Claude Code, and Modu Code as vertically stacked cards without nested tab navigation. No additional focused asset comparison was needed because this screen has no image, logo, illustration, or custom icon assets.

## Fidelity surfaces

- Fonts and typography: Existing OneCatch font stacks, weights, sizes, and hierarchy are preserved. The new sidebar label and descriptions match adjacent navigation entries.
- Spacing and layout rhythm: Sidebar item height, padding, selected radius, section heading rhythm, and stacked cards reuse existing settings components and tokens.
- Colors and visual tokens: No new colors or gradients were introduced. Selected and neutral states use the existing theme tokens.
- Image quality and assets: Not applicable; neither target region contains image assets.
- Copy and content: Runtime copy now describes executables and environment variables. Harness copy describes Agent model defaults and accurately names each harness.

## Interaction verification

- Opened Settings from the application menu.
- Selected Harness from the new sidebar entry.
- Confirmed Codex, Claude Code, and Modu Code cards are visible together with no tablist inside Harness.
- Edited the Modu default model and confirmed the unsaved-settings bar appeared.
- Discarded the edit and confirmed the field returned to its saved value.
- Checked browser console errors: none.

## Findings

No actionable P0, P1, or P2 differences remain. The source screenshot is an annotated placement reference, so viewport crop and content density differences outside the two marked regions are expected rather than design drift.

## Comparison history

The first rendered comparison passed. No P0/P1/P2 visual fixes were required after capture.

## Follow-up polish

No P3 follow-up is required for the requested structural change.

final result: passed
