# Design QA: Harness settings navigation

- Source visual truth: `/var/folders/b1/0fd1b6hs7lz0fm_mh346lybm0000gn/T/codex-clipboard-f12e759c-ed12-4f32-a3bb-16b2323c4cfe.png`
- Browser: Codex in-app browser
- URL: `http://localhost:4173/`
- Viewport: 1280 × 720 CSS px
- Source pixels: 1960 × 1656
- Implementation pixels: 1280 × 720 for both captures
- Device pixel ratio reported by the page: 2; browser captures were normalized to CSS-pixel dimensions
- State: Chinese, light theme; Appearance page and Harness page showing all Agent cards

## Full-view comparison evidence

The annotated source identifies the obsolete local runtime block and clarifies that process-level integration belongs to each Harness. The implementation removes that block from the first settings page, renames the page to Appearance, and moves executable, environment, SDK/CLI, model, reasoning, service-tier, and provider controls into the corresponding Harness cards.

## Focused region comparison evidence

The sidebar and the Harness list were inspected at readable scale because they contain the marked source regions. Harness occupies the annotated sidebar slot and presents Codex, Claude Code, and Modu Code as a lightweight divided list without nested cards or tab navigation. Each row keeps identity and health visible while its configuration can be expanded independently.

## Fidelity surfaces

- Fonts and typography: Existing OneCatch font stacks, weights, sizes, and hierarchy are preserved. The new sidebar label and descriptions match adjacent navigation entries.
- Spacing and layout rhythm: Tight title/status rows and more generous expanded content create clear rhythm. Dividers and whitespace replace the previous nested-card frames.
- Colors and visual tokens: No new colors or gradients were introduced. Selected and neutral states use the existing theme tokens.
- Image quality and assets: Not applicable; neither target region contains image assets.
- Copy and content: Appearance describes only language, display, and theme. Harness describes integration, runtime environment, and Agent defaults together.

## Interaction verification

- Opened Settings from the application menu.
- Confirmed Appearance contains no local runtime or executable controls.
- Selected Harness from the new sidebar entry.
- Confirmed Codex, Claude Code, and Modu Code cards are visible together with no tablist inside Harness.
- Confirmed the initial state has one expanded Harness and two collapsed Harnesses.
- Expanded Modu Code, verified its SDK controls, then collapsed Codex independently.
- Confirmed the Modu Code card exposes Native Go SDK, shared/isolated configuration, model, provider, and configuration testing.
- Edited the Modu default model and confirmed the unsaved-settings bar appeared.
- Discarded the edit and confirmed the field returned to its saved value.
- Checked browser console errors: none.

## Findings

No actionable P0, P1, or P2 differences remain. The source screenshot is an annotated placement reference, so viewport crop and content density differences outside the two marked regions are expected rather than design drift.

## Comparison history

The first rendered comparison led to the runtime-control relocation. The revised structure passed the focused browser check.

## Follow-up polish

No P3 follow-up is required for the requested structural change.

final result: passed
