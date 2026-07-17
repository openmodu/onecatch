# Global Search And Project Actions Design QA

- Source visual truth: `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-30a8f606-bcf8-478b-81af-f29f06e4975e.png`
- Implementation screenshot: `/Users/ityike/Code/go/src/github.com/openmodu/oneshot/design-qa-global-search-project-actions.png`
- Full-view comparison: `/Users/ityike/Code/go/src/github.com/openmodu/oneshot/design-qa-global-search-comparison.png`
- Focused search comparison: `/Users/ityike/Code/go/src/github.com/openmodu/oneshot/design-qa-global-search-focus.png`
- Viewport: 1280 × 720, device pixel ratio 2
- State: global search open, empty query, first global task selected, active project actions visible

## Findings

- No actionable P0, P1, or P2 differences remain.
- The search keeps the reference hierarchy but removes the previous boxed table treatment: one compact search row, light group labels, borderless result rows, one group divider, and plain shortcut text.
- The project row now exposes two distinct compact actions. The overflow action is left and visually quiet; the new-task action is right and uses the primary fill.
- The section action says `添加项目`, so it no longer depends on a plus icon to explain its purpose.

## Fidelity Surfaces

- Fonts and typography: all new controls use the existing monospace token stack. Search, labels, task titles, workspace metadata, and shortcuts retain the product's compact weights and single-line truncation.
- Spacing and layout rhythm: the palette is reduced to 620px, removes the boxed icon column and per-row rules, and uses 8px body padding with 2px row gaps. Project actions are 24 × 24px and fit inside the existing 34px row.
- Colors and visual tokens: the palette and actions use existing canvas, panel, line, accent, muted-text, and selected-background tokens. No foreign system-gray surface was added.
- Image and icon quality: all icons come from the existing Phosphor dependency at 14–16px. There are no raster placeholders, emoji, text-glyph icons, handcrafted SVGs, or CSS-drawn icons.
- Copy and content: `添加项目`, project-specific new-task labels, global task titles, workspace names, commands, and shortcuts are localized in both Chinese and English.

## Data And Interaction Verification

- Global search now calls the desktop backend `SearchTasks` service instead of reusing the selected workspace's task/run state.
- The response joins each task with its workspace and optional latest run, so selecting a result can switch projects and open the correct run.
- Backend filtering covers task title, task ID, prompt, workspace name, and workspace path; hidden workspaces are excluded.
- Search typing, keyboard selection, Escape close, project overflow, and project-specific new-task opening were exercised in the in-app browser.
- The browser console contains no errors.

## Comparison History

1. P1: global search only contained tasks/runs loaded for the selected workspace.
   - Fix: added a backend-owned cross-workspace `SearchTasks` query returning `{ task, workspace, latestRun }` items and separated palette query state from run-history filters.
   - Post-fix evidence: backend tests create two workspaces and verify title/workspace matching and latest-run hydration; the browser shows results sourced through the new path.
2. P2: the palette used boxed icon cells, row grids, outlined shortcut boxes, and a heavy frame.
   - Fix: removed vertical icon rules and row boxes, reduced width, softened the backdrop, and changed shortcuts to plain text while preserving a square TUI frame.
   - Post-fix evidence: focused comparison shows the revised hierarchy and lower visual noise.
3. P2: project overflow was a large 28px boxed control and task creation was duplicated inside its menu.
   - Fix: reduced project controls to 24px, moved overflow left, added a dedicated primary plus action on the right, and removed duplicate task creation from the menu.

## Implementation Checklist

- [x] Search all visible workspaces through a backend query.
- [x] Keep global search independent from current run filters.
- [x] Simplify the search palette styling.
- [x] Rename the section action to `添加项目`.
- [x] Add compact per-project new-task and overflow actions.
- [x] Verify frontend tests/build, full Go tests, browser interactions, and browser errors.

## Follow-up Polish

- P3: if users routinely need more than nine visible matches, add a non-numbered continuation section while keeping `⌘1–9` reserved for the first nine results.

final result: passed
