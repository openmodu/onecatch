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

---

# Sidebar design QA

Scope: Recreate the local Claude desktop Code sidebar in the existing OneCatch desktop UI.

## Evidence and comparison

- Reference: `/tmp/onecatch-sidebar-qa/claude-reference.png`, native capture, 3840 × 1994 pixels (2× density).
- Implementation: `/tmp/onecatch-sidebar-qa/onecatch-preview.png`, local browser preview.
- Side-by-side focused comparison: `/tmp/onecatch-sidebar-qa/sidebar-comparison.png`. Claude is normalized to 1× density before comparing.
- Captures use the expanded sidebar, light mode, and the new-task screen. Localized labels, project/session content, native traffic lights, and the main task pane differ between products and are outside the fidelity target. The preview uses a narrow user-selected sidebar; the default has been widened to 244px, and persisted widths remain respected.

## Iterations

1. P2: Project toolbar inherited outlined button surfaces. Removed borders and shadows; retained transparent surfaces in light and dark modes.
2. P2: Old category badges crowded session titles. Removed sidebar-only category badges while retaining Agent and worktree identity.
3. P2: Focused comparison showed navigation rows 6px taller than Claude. Reduced navigation row height from 32px to 26px to align the project heading and vertical rhythm. The recaptured side-by-side comparison confirms that the navigation and project toolbar now align with the reference rhythm.

## Required fidelity surfaces

- Typography: Native system font stack, regular 14px primary navigation, regular 13px project/session labels. No former 11px bold navigation headings.
- Spacing/layout: Compact navigation, grouped project toolbar, flat project headings, 28px session rows, a fixed footer, independent project expansion and scroll containment. Native inset sidebar shape is intentionally preserved for the existing macOS window material.
- Colors/tokens: Neutral `#f9f9f7` sidebar, `#ededea` selected/hover surface, muted group labels; existing dark theme remains supported.
- Assets: Existing Lucide library icons and real Agent assets. No new raster assets or recreated Claude branding. App/environment identity replaces an account profile, because OneCatch has no account profile in this surface.
- Copy/content: Chinese/English translations preserve OneCatch semantics (New, Templates, Skills, More, Projects). Session text truncates, has full-title tooltips, and remains clickable.

## Interaction checks

- Status filter selects queued tasks and restores all sessions through the existing list filter.
- Global search opens, accepts a query, and dismisses with Escape.
- More opens Usage and Workflows entries; footer opens settings, language, color, mode, and workflows.
- Light/dark selections and keyboard submenu navigation work.
- Sidebar expands/collapses and supports keyboard width adjustment. Existing remote-workspace checks, task action menus, and worktree metadata are preserved.
- All 481 frontend tests pass. Frontend production build and desktop development build pass.

## Final review

The revised focused comparison was opened with both products side by side. No remaining P0/P1/P2 issues in the scoped navigation surface. The native inset frame, localized product labels, different session content, Agent/worktree icons, and narrow persisted preview width are intentional adaptations. Footer layout and menu states were also inspected during the interactive light/dark checks. The browser pane capture clips the lower part of the virtual desktop; the focused image is evidence for the upper navigation, not a full-window pixel match.

P3: Claude's own icon set and native mode/history toolbar differ from OneCatch; no inactive imitation controls or Claude account branding are introduced.

final result: passed

---

# Sidebar divider and customization iteration

This iteration supersedes the earlier intentional inset-frame adaptation for expanded sidebars. The user's latest screenshots request Claude's right-opening overflow interaction and a single vertical column divider.

## Evidence

- Source: user attachment `codex-clipboard-7fecac4f-8a00-447a-9648-7e5f7ff7f507.png`, showing the active Routines row and its right-aligned popup. The local Claude Edit sidebar dialog was also inspected without changing its preferences.
- Implementation: `/Users/ityike/.codex/visualizations/2026/09/30/01a0efa5-1c86-7732-8afa-8cebfca8f315/onecatch-sidebar-menu.png` and `onecatch-sidebar-edit.png`.
- Focused comparison: `sidebar-divider-comparison.png` in the same directory. The reference is normalized from 2× to 1×; both products show an active overflow page with its menu open.

## Changes and visual review

- Expanded main, settings and workflow rails use a flush column with one theme-aware right divider. Native material defaults to an unrounded full-height region and no enclosing native stroke. The temporary collapsed-sidebar hover preview retains its floating shape.
- The overflow popup opens to the right, aligned to the navigation row, with viewport collision handling. Its open trigger has the reference's blue focus outline; the active overflow page supplies the trigger label.
- Edit sidebar offers four labeled checkboxes. Selected entries appear in the primary navigation; the rest stay accessible in the overflow. Preferences update immediately and persist locally.
- Opened the reference, latest menu capture, dialog capture and normalized comparison. Row alignment, blue outline, divider, menu shape and separator match the scoped reference. Extra OneCatch entries and localized labels account for the taller popup. Existing theme colors and Agent icons remain deliberate product adaptations.

## Verification

- Moved Usage into the primary navigation and Templates into overflow, reloaded the page and confirmed both choices persisted, then restored defaults through the dialog.
- Selected Usage through overflow and confirmed its label replaces More. Escape dismisses the menu and returns focus to Usage. Done dismisses the dialog and returns focus to the overflow trigger.
- Computed expanded-rail styles confirm `clip-path: none` and a `1px` themed right divider. Browser console showed no application errors in the verified flow.
- 483 frontend tests pass, including preference persistence and malformed-storage recovery. Frontend production build, desktop binary build and `go test ./internal/app/desktop` pass. Native runtime appearance after restarting the built binary remains unobserved; visual comparison uses the browser rendering of the shared frontend.

No remaining P0/P1/P2 findings in the verified frontend scope. Native traffic lights and history/mode controls are outside this iteration's target.

final result: passed

---

# Footer identity and sidebar editor refinement

The latest user screenshots identify the generic footer glyph, redundant Local label, and heavy editor styling as the revision targets.

## Final implementation and evidence

- Footer now uses the same `internal/app/desktop/assets/appicon.png` asset as the native application. It shows OneCatch and the menu chevron; Local/Preview is removed from this footer.
- Editor is a compact 360 × 314px dialog with 18px heading, regular 14px item labels, 36px rows, shared Radix checkboxes and a neutral completion button. Scoped tokens preserve theme support without inheriting the user's accent color for the checks and completion button.
- Opened and reviewed the user's original editor screenshot alongside the new rendered editor. Fixed the inherited bold green-gray labels, platform-dependent round checkbox appearance, uneven spacing and colored completion button.
- Rendered evidence in `/Users/ityike/.codex/visualizations/2026/09/30/01a0efa5-1c86-7732-8afa-8cebfca8f315/`: `sidebar-edit-polished.png`, `sidebar-edit-final.png`, `sidebar-footer-polished.png`, `sidebar-footer-final.png` and `sidebar-update-min-width.png`.

## Interaction and update verification

- Keyboard Space toggles the new checkbox and preserves the existing immediate-save behavior. Done dismisses the editor. Actual footer image loads successfully, and its accessible text is OneCatch without the environment label. No console errors in the main preview flow.
- Used a temporary isolated Vite fixture with the production Sidebar and SidebarUpdateButton components. Only the updater hook was substituted; no native update download or application was performed. At the minimum 180px sidebar width, the update target remains exactly 36 × 36px, separated from the menu by 4px with no overlap. The app name truncates as needed.
- Verified the available-download action transitions to ready in the fixture, and the downloading state displays a 75% progress ring with a disabled button. Real updater bindings, status events and lifecycle logic are unchanged. The update control now explicitly prevents flex shrinking.
- Removed the fixture files and stopped its server after capturing evidence. No simulated updater state is shipped in the application.
- All 483 frontend tests and the production frontend build pass. Desktop binary rebuilt successfully after final layout refinement. Native appearance is not claimed as visually verified; screenshots use the shared frontend browser rendering.

No remaining P0/P1/P2 findings in the scoped footer/editor revision.

final result: passed

---

# Sidebar theme color reconciliation

This iteration supersedes the previous editor's forced black-and-white color overrides. The user requested a coherent palette within the existing application.

- Removed editor-local primary color overrides and hard-coded black checkbox/button surfaces. Shared Checkbox and Button components now inherit the application's selected accent and foreground tokens.
- Replaced the navigation's hard-coded blue focus outline with `--sidebar-ring`. Editor background, text and border explicitly use shared popover tokens; this also corrects the default dialog background utility overriding the intended surface.
- Verified Ocean in light and dark modes through the actual appearance menus. Light checkbox, completion button and focus all resolve to `#1f6475`; dark resolves to `#79d7e7`. Surfaces resolve to `#fcfcfa` and `#272727`, respectively. Restored the browser preview's original Forest/Light preferences after checking.
- Opened the supplied editor screenshot and the new screenshot in `sidebar-color-comparison.png`. The original mixed blue checks/teal button are replaced by one accent. Kept the refined compact layout and regular typography. Reviewed the dark capture as well.
- Evidence in the existing visualization directory: `sidebar-color-ocean.png`, `sidebar-color-dark.png`, `sidebar-color-comparison.png` and `sidebar-colors-final.png`.
- 483 frontend tests and the final production build pass. Browser color changes also log the existing unavailable Wails event-broadcast calls in preview mode; colors and selections update correctly. Native event broadcasting and native runtime appearance are outside the browser visual comparison.

No remaining P0/P1/P2 visual findings in the scoped color revision.

final result: passed

---

# Whole-window palette alignment to Claude

The latest screenshot places OneCatch over Claude and reveals that the previous revisions left the original cream canvas behind the revised sidebar. This iteration addresses the full light-mode shell rather than another editor-only accent change.

## Grounding and final palette

- Sampled large flat regions from `codex-clipboard-5d994544-6aa0-4387-a0a5-fa951424f0ff.png`. Claude's main canvas is RGB 252/252/251 (`#fcfcfb`), sidebar 250/250/249 (`#fafaf9`), selected row 237/236/232 (`#edece8`) and sidebar text 82/81/78 (`#52514e`). OneCatch's old canvas was 245/245/240 (`#f5f5f0`).
- Applied those surface and sidebar colors, white cards/popovers, neutral input borders and coordinated subtle separators. Shared content's muted text uses a slightly stronger gray for readability; sidebar group labels retain the sampled hierarchy.
- Synchronized the startup HTML, CSS fallback, macOS native backing canvas, main/auxiliary window initial backgrounds and Windows auxiliary captions. System terminal fallback follows the same tokens. Expanded native sidebars are opaque so wallpaper/material tint cannot shift the chosen gray; floating hover previews retain their existing material tint.
- Input-shell focus borders use the neutral input token. Existing configurable accent colors and semantic status colors remain independent of the shell surfaces. Dark palette values are retained.

## Evidence and verification

- Captured the full expanded light-mode application as `onecatch-shell-neutral.png` and opened it alongside the supplied source in `shell-palette-comparison.png`, in the existing visualization directory. This comparison targets surfaces and hierarchy; source panes show different pages and content.
- Rendered values confirm main canvas 252/252/251, sidebar 250/250/249, selected row 237/236/232, separator 228/228/225, composer white and focused composer border 212/212/208. The former yellow canvas mismatch is resolved across the visible window.
- 483 frontend tests, final production frontend build and desktop package tests pass. Final desktop binary is rebuilt with synchronized native colors. Native runtime appearance after restarting remains unobserved; visual evidence is the shared frontend preview.

No remaining P0/P1/P2 findings in the scoped light-mode shell palette.

final result: passed

---

# Workflow navigation and edit action

- Reproduced the macOS workflow history bug in the shared frontend: after selecting the DAG, Back was enabled, but the full-width titlebar overlay was the pointer hit target instead of the button. Making only the sidebar-column span transparent did not exempt the overlay itself.
- The macOS overlay now passes pointer input through to the rail. Its content-column child explicitly receives input and inherits the Wails drag region. Windows/Linux caption controls retain their existing routing.
- Replaced Pencil with a consistent 1.75-stroke SquarePen in both edit actions. The detail action is a compact, regular-weight ghost button with neutral text and hover colors.
- Verified actual browser clicks: Loop to DAG, Back to Loop, Forward to DAG, edit, change the draft name, Back to detail, Forward to the editor. The unsaved draft name was preserved. Also opened the list's edit menu and inspected both icons. No workflow was saved or deleted during verification.
- Updated the titlebar regression checks to assert the overlay passes input through while its content child retains drag/zoom. The test runs the installed Wails drag handler and confirms content dragging and one double-click zoom dispatch.
- All 483 frontend tests, frontend production build, desktop package tests and desktop binary build pass. Native drag/zoom is verified at the runtime-handler boundary, not by resizing the user's running native window.
- Final visual evidence: `/Users/ityike/.codex/visualizations/2026/09/30/01a0efa5-1c86-7732-8afa-8cebfca8f315/workflow-navigation-edit-final.png`.

No remaining P0/P1/P2 findings in the scoped workflow navigation and edit-action revision.

final result: passed

---

# Independent software update window

- Added a native `updates` window, separate from Settings, with a 520 × 460 utility layout, application icon, current/new versions, release notes, download progress, error/retry and restart action. It shares appearance/language preferences and hides the native sidebar material. The fixed utility window omits maximise controls.
- Native macOS/Windows Check for Updates menus, the footer reminder, a new footer-menu check entry and the compact Settings status row all open this role through the generated `WindowBinding.OpenUpdates` binding. The footer reminder no longer downloads or restarts the app directly, and remains clickable while progress is active.
- On first open and retained-window reopen, the panel reconciles native status and checks when idle, current or after a failed check. Known releases, active operations and verified downloads are preserved. All windows continue subscribing to the same native updater lifecycle; a new download clears the previous progress sample.
- Reminder presentation: a separate 36px footer target shows a download glyph for an available release, progress during download and a compact restart glyph when ready. The first available-version notice appears above it for seven seconds. Its timer is separate from the version guard so React Strict Mode cannot cancel it permanently.
- Exercised the real frontend components in an isolated Vite fixture with only updater operations/window-host calls substituted. Verified reminder click, menu check with automatic checking, 75% progress, Later dismissal, reopening during download, download failure/retry, disabled operation during verification, ready/restart state, one install-request dispatch and the no-update state. No real update was downloaded, installed or applied. Also opened the real browser `/?window=updates` route and confirmed its honest desktop-only preview state.
- The native window callback and retained-window visibility tests include the update role. All 486 frontend tests, transport/desktop/updater Go tests, frontend production build and desktop binary build pass. Native visual appearance and a real restart/install were not exercised in the user's running application.
- Removed all fixture files and stopped its server. No simulated versions or updater state are shipped. Evidence in the existing visualization directory includes `update-notification.png`, `update-window.png`, and full available/downloading/ready fixture captures. Version numbers in these visual examples are test data.

No remaining P0/P1/P2 findings in the scoped update-window flow.

final result: passed

---

# Update window border removal

- Removed the release-notes card border, separate white card surface and inset padding. Notes now align directly with their section title on the window canvas. Removed the footer divider, changed Later from outline to ghost, and removed the reminder bubble's border.
- The prior screenshot also included an extra bordered preview wrapper. The new capture renders the production panel directly with no preview frame. Measured the root, notes container and footer at 0px border width.
- Used a temporary isolated render of `AppUpdatePanel` for visual verification, then removed both preview files. No test versions or preview code enter the production build. Final evidence: `update-window-borderless.png` in the existing visualization directory; its versions are test data.
- Eight updater regression tests, production frontend build and desktop binary build pass. This is a presentation-only revision; native update actions are unchanged.

final result: passed

---

# Footer menu focus frame removal

- The latest user screenshot clarifies that the remaining frame is around the OneCatch footer menu trigger, not the software update panel. The prior border-removal iteration targeted the wrong surface.
- Removed the trigger's two-pixel accent focus ring. Its border, outline and shadow are explicitly suppressed; keyboard focus now uses the same neutral background as hover/open state.
- Verified the real main frontend by opening the menu and dismissing it with Escape. The trigger retained keyboard focus and matched `:focus-visible`, while computed border width was 0px, outline was none and all shadow layers were transparent. The background was the coordinated sidebar accent RGB 237/236/232.
- Evidence: `footer-no-focus-frame.png` and its full-window capture in the existing visualization directory. The canonical application icon, footer menu actions, divider and separate update control are preserved.
- All 34 existing action-style tests, the production frontend build and desktop binary build pass.

final result: passed

---

# Remove the extra footer update menu item

- The user clarified that software update should use the existing update entry points rather than adding a new item to the OneCatch footer dropdown. Removed that item and its unused helper import from Sidebar.
- Native Check for Updates, the existing Settings update entry and the separate available-update reminder still open the independent update window.
- Opened the real frontend footer menu and verified its entries are Settings, Language, Theme color, Appearance and Workflows. The Check for Updates menu-item count is zero. Evidence: `footer-menu-original-entries.png` in the existing visualization directory.
- All 42 existing action-style/updater tests, production frontend build and desktop binary build pass.

final result: passed


---

# Terminal edge alignment

- The reported lower-left artifact came from the terminal dock's inset margins, outer 14px radius, inner 13px viewport radius and one-pixel pane padding. These combined with the native window corner to leave a second curved edge and light rim.
- Removed dock margins and side/bottom borders; retained only the top divider. Dock, toolbar and viewport now have zero radius, and the maximized dock uses zero inset. The desktop window remains responsible for its outer shape.
- Removed pane padding and drew a single one-pixel divider on the existing split handles instead, preserving their seven-pixel interaction targets without a perimeter rim.
- Verified real TerminalDock, TerminalPane/XTerm and TerminalSplitLayout in a temporary isolated Vite fixture. Only native PTY operations and runtime events were substituted. The black terminal host reaches the workspace left/right/bottom edges; dock/viewport radii and margins measure zero. Sampled bottom pixels are black or near-black with no light gutter.
- Exercised keyboard height resizing (286 to 306px), left/right split resizing (50 to 54 percent), nested upper/lower splitting, maximization and restoration. Both divider orientations measure one pixel. Maximized bounds exactly match the workspace. Evidence: `terminal-edge-fixed.png`, `terminal-flush-bottom.png`, `terminal-flush-maximized.png` and `terminal-flush-splits.png` in the existing visualization directory.
- Removed the fixture files and stopped its server before the production build. All 16 existing terminal regression tests, frontend production build, desktop binary build and diff whitespace check pass. Native AppKit clipping has not been visually verified in the user's running application.

final result: passed (frontend verified; native visual verification pending)


---

# Compact native update dialogs

**Comparison target and evidence**
- Checking reference: `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-23f23035-8f24-4e1f-a1e7-589010521fbe.png`, 800 × 292 pixels, normalized at 2× density to 400 × 146 CSS pixels.
- Current-version reference: `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-13fd0bae-bdc5-4f34-98e5-f6a3761448c7.png`, 520 × 438 pixels, normalized at 2× density to 260 × 219 CSS pixels.
- Browser viewport: 1280 × 720 at 1× density. Actual production panel and updater hook are rendered in a fixture host measuring 400 × 146 for checking and 260 × 220 for current-version results. The one-pixel result-height difference is intentional rounding. Native window size is updated through Window.SetSize; native creation now starts at 400 × 146 with minimum 260 × 146 rather than a fixed 520 × 460.
- Full and focused evidence: `update-compact-checking-final-full.png`, `update-compact-current-final-full.png`, `update-compact-checking-final.png`, `update-compact-current-final.png`, and the two `update-compact-*-comparison-final.png` boards in `/Users/ityike/.codex/visualizations/2026/09/30/01a0efa5-1c86-7732-8afa-8cebfca8f315/`. The entire panel is small and legible at 1× in each comparison board, so separate detail crops are unnecessary.

**Findings and comparison history**
- [P1, fixed] The former 520 × 460 updater page was much larger than either reference. Replaced it with a horizontal checking/transfer dialog and a small result card, with optional release notes collapsed. Results with download/restart/retry actions use 320 × 260; only explicitly expanded notes use 380 × 340. macOS native caption buttons are hidden for this dialog role, leaving explicit actions and the drag surface.
- [P1, fixed] The first current-version capture still had a brown confirmation button and focus halo because the shared Button utility styles overrode the scoped blue rule. Added explicit blue/white button utility styles and replaced the halo with a subtle keyboard-focus brightness change. Recaptured and compared the revised result; the button is RGB 0/122/255 with transparent shadow layers.
- [P2, fixed] Expanded notes could leave a later ready result at 380 × 340. Notes now collapse across progress-state transitions, and expanded sizing only applies when notes actually exist. Retested download to ready: 400 × 146 becomes 320 × 260.

**Required fidelity surfaces**
- Typography: system UI font and Chinese fallback, 13px semibold status and 13px/18px body text, 12px compact title/cancel label. Status hierarchy, button alignment and body wrapping match the reference proportions. OneCatch's shorter test version intentionally occupies one line; longer versions can wrap. Current-version main content measures clientHeight = scrollHeight = 166px, with no clipping or unintended scrolling.
- Spacing/layout: checking title strip 32px, 52px icon slot, 26px body side padding, 100 × 28 cancel control. Result icon begins in a 26px top inset, status follows a 22px gap, and the full-width 28px action has 16px side/bottom spacing. No internal framed card or permanent version/release-notes sections remain. The preview host uses 16px rounding; actual outer corners and shadows are owned by AppKit rather than rendered as a second web frame.
- Colors: white/popover result surface, neutral separator and secondary control, blue progress/primary button. The progress bar uses a moving blue segment rather than the source's fading native gradient. This intentional behavior also respects reduced-motion preferences.
- Image quality/assets: canonical OneCatch PNG app icon reused without redrawing it. Its visible logo silhouette differs from the reference's ChatGPT icon, as requested; no generated or approximate logo asset is used.
- Copy/content: latest-version headline and single OK action follow the reference, with OneCatch's actual current version. Available, download, verification, ready, error and unconfigured states retain their necessary product actions. Release notes are an optional disclosure rather than permanent page content.

**Interaction verification and limits**
- Tested the production panel and updater hook using substituted native operations/events only: checking, cancellation of the owned cancellable request, a second check, current-version confirmation dismissal, available release, notes disclosure and resizing, download at 75 percent, ready/restart dispatch, install progress, failed-check retry, and Escape cancellation. Console error list is empty. Closing a download window preserves the download and uses the honest Close label; Cancel is reserved for checking.
- Wails Check now receives the runtime call context rather than context.Background, allowing its generated cancellable promise to cancel the network check. A dialog closed before initial status arrives no longer starts a fresh hidden check. Download/signature verification/install operations are unchanged.
- All 487 frontend tests, transport/desktop/updater Go tests, frontend production build and diff whitespace checks pass. The desktop binary build also passes (existing SDK/deployment-target linker warnings only). Actual downloaded update installation and AppKit presentation in the user's running application have not been exercised.
- Preview test versions and controls are bundled separately in the visualization directory. All fixture source/config files were removed from the repository before the final production build. The saved interactive preview is served at `http://127.0.0.1:9250/.update-dialog-qa.html` and explicitly labels its data as test examples.

**Implementation checklist**
- Compact creation dimensions and state-dependent resizing: complete.
- Canonical icon, blue primary control, optional notes and lifecycle actions: complete.
- Cancel check, preserve downloads, retry, keyboard dismiss and browser error check: complete.
- Visual comparisons after the two fixes: complete. No remaining actionable P0/P1/P2 findings. Native AppKit presentation remains a validation gap, not a claim made from browser evidence.

final result: passed


---

# Consistent frameless action focus

- The reported brown perimeter on More was explicitly drawn by `.sidebar-more-navigation[data-state="open"]`, while other controls inherited native/base outlines or shadcn focus rings. The earlier footer-only fix did not cover those paths.
- Removed More's open/focus outline and added a shared button-focus policy that suppresses action outlines and ring shadows. Plain actions use a neutral surface and foreground; shared buttons use a subtle brightness change; sidebar actions use the sidebar's neutral fill. Checkbox focus is excluded from the outline/ring suppression. Existing field boundaries, content-card boundaries and divider geometry remain structural styles.
- Removed the sidebar updater's explicit accent ring, decorative outlines from all sidebar popovers/submenus, and the outline/colored selection treatment from Usage source tabs. Selected Usage tabs now use neutral fill and foreground. The sidebar menu continues to have its normal shadow and internal group separator.
- Verified the real main frontend: More while open measured 0px border, outline none and shadow none, with RGB 237/236/232 fill. Its menu measured 0px border. After Escape, the trigger still matched `:focus-visible` with the same frameless state. Tab from Add project focused the search icon, whose border is 0px, outline is none and all shadow layers are transparent. Selecting Codex updated the Usage tab's aria-selected state while keeping 0px border, outline none and neutral fill.
- Captured `sidebar-more-no-frames-full.png`, `controls-no-frames-full.png` and the focused `sidebar-controls-no-frames.png` in the existing visualization directory. Font sizes, icons, layout and menu actions are preserved; the focused view demonstrates the requested neutral selected/open surfaces. No native presentation claim is inferred from these browser screenshots.
- All 58 existing action-style, updater and account/token usage tests pass, as do the frontend production build, desktop binary build and diff whitespace check.

final result: passed

---

# Remove decorative frames across desktop windows

- The broader request covers content cards, form controls, buttons, menus, dialogs and docked panels. The previous focus-only cleanup left authored card and field borders visible.
- Added the shared `surfaces` cascade layer, loaded by every desktop window and Radix portal. It disables border styles, outlines, ring shadows and inset frames across desktop surfaces; only explicitly authored single-edge dividers are restored. Local border widths remain intact so first/last-child and collapsed-pane divider exceptions still apply. The separate mobile presentation is unchanged.
- Fields and secondary actions use neutral filled surfaces, with a fill change for focus. Composer textareas remain transparent inside one filled parent. Unchecked custom checkboxes retain a visible neutral fill, selected buttons and DAG nodes retain filled selection feedback, and invalid fields retain a tinted error surface. Floating menus and dialogs retain only soft elevation shadows, with no stroke or spread ring.
- The main Usage page measured zero visible multi-edge frames, zero outlines and zero inset shadows. Its sidebar divider remains `1px solid rgb(228, 228, 225)`. Verified the overflow menu and sidebar editor, including keyboard Space toggling of the Usage checkbox and restoration of its original unchecked state.
- Verified Settings in light and dark modes, restored the original light appearance, and checked keyboard focus from the Shell field to the arguments textarea. Both fields have 0px computed borders and no shadows; the terminal theme dropdown also has a 0px border and still opens/dismisses normally. Workflow editing measured zero multi-edge frames while preserving its sidebar divider and filled fields. No workflow or terminal configuration was saved during QA.
- Captured `all-surfaces-frameless.png` and `workflow-frameless.png` in the existing visualization directory. These are browser previews of the production components; AppKit window decoration and the already-running native application were not changed by these browser checks.
- All 487 frontend tests pass. The frontend production build, desktop binary build and diff whitespace check pass; existing bundle-size and SDK/deployment-target warnings remain non-fatal.

final result: passed
