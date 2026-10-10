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

---

# Keel desktop migration — 2026-10-09

This section records the native Go client on `feat/keel-desktop`; the earlier
Wails reports above remain unchanged. Migration and visual acceptance are still
in progress.

## Findings

- **P1 — Incomplete feature surfaces.** The compact updater, full Skills
  operations and template import/export remain
  unported. Settings also lacks language and chat-font controls. Git/files/review,
  attachments, local/SSH projects and execution-location selection are now wired
  to the existing Go services; their presence does not establish full parity.
- **P2 — Typography and control density.** Native inputs/labels remain lighter
  than the source in workflow/settings views. Original CSS uses intermediate
  600/620 weights; the pinned Keel text API exposes regular/medium/bold. Match
  shaping, optical weights, row heights and guided-editor wrapping, then recheck
  full/focused comparisons. Inspector diff rows now use the source's single-line
  geometry but native font metrics and linear color mixing still differ.
- **P2 — Remaining interaction fidelity.** The Agent popover now contains the
  model, and project/worktree menus perform real operations. The full profile,
  reasoning/service-tier controls, image lightbox,
  clipboard/session actions and original titlebar actions still need migration.
  Menu density and copy need another pass against the worktree reference.

## Source truth and matching captures

Source: the existing `frontend/src/app` production components served in their
built-in, standalone demo fallback. Browser screenshots were captured through
the Chrome extension while native application access was unavailable. No real
task, settings or Skill was modified to prepare reference data.

| State | Source in `bin/keel-qa/` | Keel rendering | Logical viewport | Source pixels | Keel pixels |
| --- | --- | --- | --- | --- | --- |
| Light/new task, sidebar expanded | `reference-welcome.jpg` | `welcome.png` | 1120 × 760 | 1120 × 760 | 2240 × 1520 |
| Workflow detail | `reference-workflows.jpg` | `workflows.png` | 1040 × 760 | 1040 × 760 | 2080 × 1520 |
| Guided Loop editor | `reference-workflow-form.jpg` | `workflow-form.png` | 1040 × 760 | 1040 × 760 | 2080 × 1520 |
| DAG canvas and node inspector | `reference-dag.jpg` | `workflow-dag-default.png` | 1040 × 760 | 1040 × 760 | 2080 × 1520 |
| Appearance settings | `reference-settings.jpg` | `settings.png` | 960 × 800 | 960 × 800 | 1920 × 1600 |
| Docked Git inspector | `reference-inspector-git.jpg` | `inspector-git.png` | 1120 × 760 | 1120 × 760 | 2240 × 1520 |
| Empty file editor and tree | `reference-inspector-files.jpg` | `inspector-files.png` | 1120 × 760 | 1120 × 760 | 2240 × 1520 |
| Review, two-file demo patch | `reference-inspector-review.jpg` | `inspector-review.png` | 1120 × 760 | 1120 × 760 | 2240 × 1520 |
| Worktree menu, open | `reference-worktree-menu.jpg` | `worktree-menu.png` | 1120 × 760 | 1120 × 760 | 2240 × 1520 |

Source captures have 1× density; Gio's offscreen captures have 2× density. The
comparison boards downsample the implementation to the source's logical size
using Catmull-Rom filtering. Left is source, right is Keel, separated by a 16 px
comparison gutter. Native window frames/traffic lights are excluded from both.
The standalone source says “预览”; the Keel test fixture says “本地”. Task titles
and usage values are intentionally sample data, not evidence of account activity.
The Git branch/hash/changed path, file-tree entries and review patch match the
source demo fixtures. Worktree entries use the same branch/path examples; project
name, caption state, session list and remote availability are sample variations.
These variations are excluded from pixel-level claims. The source browser console
reported its expected missing-Wails-runtime warning and no errors.

Full-view boards:

- [Welcome](bin/keel-qa/comparison-welcome.png)
- [Workflow detail](bin/keel-qa/comparison-workflows.png)
- [Guided editor](bin/keel-qa/comparison-workflow-form.png)
- [Settings](bin/keel-qa/comparison-settings.png)
- [Git inspector](bin/keel-qa/comparison-inspector-git.png)
- [File inspector](bin/keel-qa/comparison-inspector-files.png)
- [Review](bin/keel-qa/comparison-inspector-review.png)
- [Worktree menu](bin/keel-qa/comparison-worktree-menu.png)

Focused boards:

- [Sidebar and footer](bin/keel-qa/comparison-sidebar.png)
- [Composer, heading and project caption](bin/keel-qa/comparison-composer.png)
- [Git controls](bin/keel-qa/comparison-inspector-git-focused.png)
- [File tree and editor](bin/keel-qa/comparison-inspector-files-focused.png)
- [Diff line/header geometry](bin/keel-qa/comparison-inspector-review-focused.png)

Additional production render checks cover conversation light/dark states,
templates, Skills, usage, overflow/application menus, sidebar customization,
raw/advanced workflow editing, every ported settings section, and the minimum
main-window width with expanded/collapsed sidebar. Evidence is local and ignored
by Git. Regenerate Keel captures with the command in `cmd/keel/README.md`.

## Fidelity surfaces and iteration history

- **Fonts:** system family priority is preserved for UI text with Chinese
  fallback. Intermediate weights and input metrics remain actionable P2 work;
  the offscreen render does not prove native antialiasing parity.
- **Spacing:** 244 dp main sidebar, 240 dp workflow rail, 52 dp title strip,
  original independent window dimensions and single pane seams are retained.
  Comparison iteration 1 found the collapse button at the far right, an oversized
  footer logo and the workflow editor inset too narrow. They were corrected to
  the source positions, 22 dp canonical icon and 48 dp guided-content inset.
  Subsequent boards above show the changes together with the source.
- **Colors:** source DOM measurements were RGB 250/250/249 for sidebar,
  252/252/251 for background and 242/242/241 for composer. Keel now uses those
  values. Selected/hover rows use neutral 237/236/232; fields and controls have
  transparent perimeter/focus strokes. Four existing accent themes are reused.
- **Assets:** the actual OneCatch PNG, original runtime assets and Lucide 1.30.0
  path data are embedded, with license/source attribution. An early SVG parser
  failure hid the Codex mark; compact arc-flag normalization fixed it and native
  raster tests confirm that every runtime SVG produces visible pixels. Workflow
  direction/check marks were subsequently switched to the canonical Lucide data.
- **Copy:** primary headings and guided-editor instructions reuse existing copy.
  Raw runtime IDs and settings/window labels still need the full original
  localization pass; partial features are documented, not presented as complete.
- **Settings layout:** the first row-layout revision clipped labels and created
  oversized blank cards. Switching appearance rows to two explicit grid tracks
  fixed the regression. The recaptured settings board shows both controls and
  text; a native layout test checks every appearance control at 960 × 800 and
  the 860 × 600 minimum window size.
- **Inspector iteration 1:** ported the 380 dp dock, tabs and backend operations.
  Focused comparisons exposed an extra standalone model chip, excessive toolbar
  density, a clipped send circle and wrapped diff text. Moving the model into
  the Agent popover, reducing selector padding and explicitly sizing the send
  circle corrected the composer. Diff headers/rows now follow the original
  36/28/21 dp geometry with one shared horizontal canvas and semantic tints.
- **Inspector iteration 2:** the new wide diff layout initially gave its scroll
  child zero height and collapsed file-name columns. Explicit full-height scroll
  sizing and a constrained tree row fixed both. The recaptured
  `inspector-review-wide.png` shows both files and their tree labels. A native
  layout test now checks the code region at 1120 and 860 dp window widths.
- **Form/attachment iteration:** review of `project-dialog.png` exposed internal
  values as field labels and an invisible off switch; the attachment card lost
  its name. Empty select labels with explicit initial choices, filled deferred
  checkboxes/settings switches, and intrinsic-width file chips corrected these.
  The recaptured project and attachment views show visible choices/file names;
  actual Gio pointer/Space tests confirm toggling once and disabled behavior.
- **Inspector iteration 3:** matched the 44 dp inspector toolbar, neutral tab
  strip and grouped Git repository/branch sections. The refreshed full/focused
  boards above record the resulting density; font/profile/menu differences
  remain P2 work.

## Interaction verification and limits

- Actual Gio input-router tests exercise sidebar drag/keyboard resizing and
  persistence, workflow selection, back/forward, edit navigation and retention of
  unsaved/invalid drafts. These are native UI layouts, not browser substitutes.
- Backend integration exercises task/run streaming, completion, continuation and
  reopening the store. Settings integration checks section persistence, invalid
  numeric input and stale-revision rejection. All-source usage tests cover
  same-day sums and streaks spanning different providers.
- Workbench integration uses a real temporary Git repository and worktrees,
  verifies task paths and selected execution context, retains file drafts and
  rejects stale file hashes/path escapes. Diff parsing covers staged/worktree
  merging, binary/untracked files, quoted UTF-8 names and content beginning with
  `---`/`+++`. Attachment integration covers bounded file selection, pasted PNG
  staging, cleanup that preserves original/persisted files and resume failures
  that do not duplicate an already-persisted instruction. Native input tests
  check inspector controls at normal/minimum sizes and maximize/restore/collapse.
- Full repository tests, the Keel race check, vet and whitespace checks passed.
  Offscreen render checks passed. The macOS app builds and its ad hoc signature
  verifies; the current Keel entry point has 550 dependency packages and zero
  Wails/WebView packages. Linked macOS frameworks include AppKit/Metal and no
  WebKit. This check used Go 1.27.1 on darwin/arm64; the module minimum remains
  Go 1.26.1. Local package/signature checks do not establish distribution signing
  or notarization.
- The Mac remains locked for native UI automation. Browser reference captures
  and offscreen input tests do not verify AppKit chrome, OS dialogs, native menu
  presentation, installation, or actual update delivery. No unlock workaround
  was attempted.

## Native terminal iteration

- Ported TerminalDock into the main conversation column, preserving 286 dp
  default height, the measured 36 dp toolbar, 190–560 dp resizing, square panes
  and single dividers. Hidden tabs retain their PTY sessions. Closing the original
  leaf of a split promotes its sibling without deleting the surviving tab group.
- Keel uses the existing transport-neutral local/SSH PTY service and a pinned
  Go VT emulator. The entry point does not import the Wails terminal binding.
  Output arriving before session creation resolves is buffered with a bound;
  process creation is serialized with shutdown to prevent orphan shells.
- Native Gio input tests verify one Space emission, Chinese composition without
  preedit leakage, Option text, Ctrl-C, arrows and bracketed paste. A real POSIX
  PTY checks Unicode input, resizing (`stty size`) and a nonzero exit code.
  Additional tests cover split pointer routing, alternate screens, split UTF-8,
  wide-cell selection/search, indexed-versus-RGB theme updates, web-link mapping
  and tab/event lifecycle. Actual AppKit IME presentation remains unverified.
- Captured the production React terminal in browser preview mode at 1120 × 760.
  It has no PTY in this mode, so the fidelity comparison uses the empty dock:
  [full board](bin/keel-qa/comparison-terminal-empty.png) and
  [focused board](bin/keel-qa/comparison-terminal-empty-focused.png). Source left,
  Keel right; both normalized to 1 pixel per dp. The source dock runs from y=474
  to 760, with a 36 px toolbar and zero outer radius. Running, split, search,
  narrow, maximized and dark Keel states are additional production render checks
  (`terminal*.png`), not a claim of browser PTY parity.
- The first comparison exposed a misplaced new-tab button and a toolbar one
  pixel short; both are corrected. Source/native font weights remain a P2 gap.
  Remote SSH connectivity and physical macOS window/IME behavior remain pending
  interactive verification; browser preview/offscreen evidence cannot prove them.

## Remaining implementation checklist

1. Complete remaining task actions and IME-safe Enter-to-send behavior.
2. Complete Skills operations and template import/export.
3. Complete appearance/localization and a Keel-specific compact update path.
4. Complete Agent/profile/menu interaction fidelity and fix remaining font
   differences; recapture normalized
   full/focused comparisons.
5. Verify actual native windows, menus and dialogs after the Mac is unlocked.

No finished-design handoff is claimed while the P1/P2 findings remain.

## Native DAG iteration

- Compared the production React DAG editor at 1040 × 760 with the Go view:
  [full board](bin/keel-qa/comparison-workflow-dag.png) and
  [focused board](bin/keel-qa/comparison-workflow-dag-focused.png). Source left,
  Keel right, normalized to one pixel per dp. Preview data matches the browser
  demo; this does not change backend built-ins or existing saved coordinates.
- Matched the 240 dp rail including its divider, 174 dp editor header, 204 × 94
  dp nodes, 10 dp ports, 320 dp inspector, 18 dp grid and cubic dependencies.
  The first comparison exposed the wrong inspector width, node text scale and
  port tint; these now follow measured source values. The source keeps the edge
  delete mark hidden until hover; the native mark also appears on keyboard focus.
- Gio input-router tests verify continuous node dragging, both port directions,
  duplicate connection prevention, overlapping port/edge hit priority, clicking
  the actual Bézier path, removal, creation, automatic layout and keyboard moves.
  Horizontal/vertical scroll exposes an initially offscreen node and still lets
  the user select it. Positions and dependencies survive closing and reopening
  the real temporary store. Remote Worker selection removes an incompatible
  full-access sandbox as in the source.
- Native text-editor events verify ID reference repair, collision/empty-field
  feedback and disabled saving of a mismatched invalid ID. Stable transition
  slots retain keyboard focus while renaming signals; duplicate names preserve
  both original transitions and empty signals remain invalid drafts.
- Source browser clicks independently confirmed the two-port connection state,
  dependency count and dependency-layer automatic-layout coordinates. The source
  draft was not saved or deleted. Default, connecting, laid-out, 860 × 600 and
  dark native states were rendered from production views.
- Deliberate responsive repair: the source's 520 px minimum canvas pushes its
  footer below the 760 px window. The native canvas occupies available height,
  scrolls its full graph and keeps validation visible at 1040 × 760 and 860 × 600.
  Only interior pane/header dividers retain strokes. Neutral focus fills now
  also suppress Keel's default keyboard stroke on existing frameless controls.
- Source/native font weights and native scrollbar appearance remain P2 fidelity
  gaps. Physical macOS window presentation remains unverified while locked.


## Native Worker iteration

- Captured the production React auxiliary settings window at 960 × 800 in its
  built-in demo fallback. Unlock, add environment, sandbox/ordinary selection,
  edit and TLS expansion were inspected without saving a configuration or
  sending a pairing request. Browser preview has no real remote connection;
  its connected badges are demo data. The browser reported its expected Wails
  preview warning and no application errors.
- Compared normalized one-pixel-per-dp captures: [settings board](bin/keel-qa/comparison-worker-settings.png),
  [sandbox board](bin/keel-qa/comparison-worker-sandbox-modal.png),
  [pairing board](bin/keel-qa/comparison-worker-pair-modal.png) and
  [TLS board](bin/keel-qa/comparison-worker-edit-tls.png).
  Source left, native right. Corresponding focused boards preserve the whole
  form. Native previews use production views at 2×; all comparison boards are
  normalized before review. Additional 860 × 600 and dark captures are in
  `worker-edit-tls-narrow.png` and `worker-edit-dark.png`.
- Implemented the original Ctrl+Alt+S reveal without changing the persisted
  feature flag, a deferred enable switch with a save/discard bar, configured
  environments, 15-second health checks, pairing, edit and confirmed removal.
  Settings writes retain revision conflict checks. The 576 dp dialog has
  36 dp filled fields, optional TLS/deployment groups and a fixed action footer.
  Its body scrolls at the minimum viewport; outer strokes remain absent.
- The first comparison exposed an overly pale scrim, lighter input text,
  centered disclosure rows and an oversized TLS form. These now use the source
  overlay luminance, medium fields, left-aligned disclosures and grouped TLS
  spacing. The native renderer blends in linear light, so the scrim alpha is
  adjusted to match the source's displayed CSS half-black overlay.
- The Agent menu offers enabled remote targets, restricts sandbox targets to
  Codex and clears local model/worktree choices when moving to a Worker. It
  hides unsupported attachment controls, keeps the selected target when its
  configuration becomes unavailable, and disables sending until a valid target
  is selected. Existing tasks retain their Worker; the shared service performs
  the final validation and persists it in the same task store.
- A temporary TLS Worker verifies one-time pairing, stored certificate pin,
  authenticated health after editing, rejection of a wrong pin and removal.
  A real task-store check verifies the native Send input persists the Worker
  and uses the project directory. Credentials stay in the existing registry;
  public Info has no token, and URL query credentials are redacted in errors.
  Health generations discard a stale success after configuration changes.
- Native Gio input tests cover reveal, typed sandbox URL, sync switch,
  TLS expansion, cancellation, minimum-viewport footer visibility and actual
  Agent-popover clicks from local to remote and back to a local Agent/workflow.
  Execution
  tests cover disabled/missing Workers, sandbox Agent restrictions, local
  attachments, Remote FS, full access, task immutability and switching back to
  a local Agent or workflow. These use temporary data and test-only endpoints.
  A live cloud sandbox connection and deployed remote task are not claimed.
- Deliberate adaptation: connection failures have readable inline detail rather
  than relying solely on the source's hover title. Deployment commands can be
  copied explicitly. Remaining font/row metrics,
  full Agent profile and active-step Worker inspector defaults remain migration
  work. Physical macOS windows/menus were not rechecked in this iteration;
  earlier native automation observed a locked desktop.
- Verification: full-repository Go tests, Keel race tests and vet pass; the
  executable and local App bundle rebuild successfully. Signature verification
  passes. The 550-package Keel dependency tree contains zero Wails/WebView
  imports, and the bundle links AppKit/Metal without WebKit. Signing is ad hoc
  for this Mac; distribution signing and notarization are not established.

final result: blocked

## Native phone connection iteration

- Ported the original Mobile connection settings section between Skills and
  Terminal. The host switch applies immediately, and this independent store
  has no Settings Save/Reset or settings-revision badge. The view preserves
  the original hosting/pairing text, address list, copy actions, ten-minute
  countdown, replacement code and shortened certificate fingerprint. Surfaces
  use neutral fills without outer frames; only authored separators remain.
- Compared React at 960 × 800 with the normalized native render:
  [off state](bin/keel-qa/comparison-mobile-off.png),
  [pairing full board](bin/keel-qa/comparison-mobile-pair.png) and
  [pairing focused board](bin/keel-qa/comparison-mobile-pair-focused.png).
  Source is left, native right. Pairing source uses a temporary public demo
  fixture, restored immediately after capture; no user's host was enabled.
  The source demo deliberately disables its switch, while the native fixture
  shows the enabled production action. Countdown differences are elapsed
  fixture time. Minimum-window and dark-theme native captures were inspected.
- Corrected navigation rows to the source's 10 dp vertical padding and 2 dp
  label/hint gap. Copy actions use 28 dp filled controls, and pairing glyphs
  match the source tracking and weight more closely. Fine font metrics and
  several vertical positions remain a P2 fidelity difference. An apparent
  light selection in the first dark preview was Keel's 120 ms background
  transition; a fresh dark root verifies the settled palette.
- The Go UI calls the existing HTTPS hosting service through a backend lock
  shared with shutdown. It serializes requests, rejects a cancelled pending
  start, ignores late results after closing settings, and prevents late calls
  from reopening a closed client. Host start/stop failures reread listener
  state, so a failed preference write cannot falsely change the switch.
  A failed refresh retains the previous state and shows an inline retry.
- Launch restores the existing explicit opt-in. A new store remains closed;
  shutdown stops the listener while preserving the opt-in for next launch.
  A temporary store and ephemeral port validate real TLS pairing, replacing
  an older code, single-use enforcement, retained certificate pin on restart,
  persisted stop, released port on shutdown and rejected post-close requests.
  No actual phone, mDNS discovery or OS local-network permission was tested.
- Gio input routing verifies pointer and keyboard activation of the immediate
  switch without optimistic state changes, scrolling to the pairing controls
  at 860 × 600 and removing an expired code's copy action. Clipboard writes
  use Keel's native clipboard command; physical pasteboard delivery remains
  part of native interactive verification.
- Verification: repository Go tests, Keel race checks, vet and offscreen
  renders pass. The executable and local App bundle rebuild and its ad hoc
  signature verifies. The Keel dependency tree still has no Wails/WebView,
  and its macOS executable links AppKit/Metal without WebKit. Full migration
  and physical native window/dialog verification remain pending; the earlier
  locked-desktop observation was not rechecked in this iteration.

final result: blocked

## Native inspector defaults and pending messages iteration

- Git source follows the latest step attempt, falling back to the run's current
  step. A registered, enabled Worker is selected for read-only steps; writable
  steps use the local result. Remote FS and task-bound Worktrees suppress the
  Worker selector. Manual choices survive token events and periodic refreshes;
  context, active Worker or registry changes recompute the default. Requests
  capture their version, context and source, preventing late remote responses
  from replacing the current pane or local project caption.
- Remote Git lists do not offer local branch operations or open a local review
  for a remote file. Source controls use a neutral fill and no outer stroke.
  The original read-only copy, 36 dp selector, branch weight and file-state
  colors were compared with the React implementation.
- `reference-inspector-worker.jpg` and `inspector-worker.png` are 1120 × 760
  logical captures. `comparison-inspector-worker.png` and its `-focused.png`
  counterpart normalize both sides to one pixel per dp. The focused 380 dp pane
  has the same snapshot and Worker label. The full native fixture intentionally
  has no conversation history or populated task list; the full board therefore
  does not establish conversation/sidebar fidelity. A temporary demo-only
  Worker option exposed the source's existing selector and was fully restored.
- Pending instructions appear above the composer with the source's 52 dp rows,
  20 dp side inset, neutral card fill and top-only corners. Remove calls the
  existing service; steering promotes the same durable instruction and attached
  files. Both serialize while a request is pending. Applied/removed messages
  are excluded, and paused tasks disable steering. Completion callbacks guard
  the selected run and leave the current prompt and attachment draft intact.
- Cmd/Ctrl+Shift+Enter inserts a correction into an active turn; failed inserts
  remain retryable. Cmd/Ctrl+Enter retains queue/resume behavior. A paused task
  can resume with no new text. Empty running composers show Stop; a draft shows
  Send. Finished unsupported states expose no send action. Unmodified Enter
  remains a newline until IME-safe submission can be verified.
- `reference-pending-instructions.jpg` captures the original at 1120 × 760.
  `pending-instructions.png` is an isolated 860 × 320 native composer; their
  matching 788 × 105 message strips form
  `comparison-pending-instructions-focused.png` (source left, native right).
  Narrow 420 dp and dark captures check native layout separately. The temporary
  public demo instructions were restored; no user run or Worker was modified.
- A temporary TLS Worker verifies the selected Git endpoint, bearer token,
  retained pin, local-caption isolation and writable-step fallback. Gio pointer
  routing verifies the source menu and rejects remote branch/review controls.
  A real temporary execution store verifies native message removal, steering
  identity/attachments, priority ordering, failed-insert retention and native
  empty-message resume. The existing backend combines pending instructions in
  the resumed prompt; this migration preserves that behavior.
- Verification: repository tests, Keel race checks, vet and whitespace checks
  pass. The executable and macOS App bundle rebuild, and the ad hoc signature
  verifies. There are still 550 Go dependency packages and zero Wails/WebView;
  native linkage uses AppKit/Metal without WebKit. Physical native windows,
  global shortcuts and system pasteboard delivery remain unverified. Full
  migration and overall visual acceptance remain incomplete.

final result: blocked

## Native Skills management iteration

- Ported the searchable Skills list, compact Markdown/frontmatter preview,
  create and delete confirmations, and the original right-hand files/debug
  inspector. The normal inspector is 438 dp wide; all panes use neutral fills
  with one-pixel separators and no outer focus or selection strokes. Icons
  come from the application's existing Lucide package.
- Compared original React captures with production Go renders at 1120 × 760:
  [library](bin/keel-qa/comparison-skills.png),
  [create dialog](bin/keel-qa/comparison-skill-create-focused.png),
  [files](bin/keel-qa/comparison-skill-files-focused.png),
  [debug](bin/keel-qa/comparison-skill-debug-focused.png) and
  [sync](bin/keel-qa/comparison-skill-sync-focused.png).
  [Skills settings](bin/keel-qa/comparison-skill-settings.png) and
  [add target](bin/keel-qa/comparison-skill-target-add.png) use 960 × 800.
  Source is left, native right; captures are normalized to one pixel per dp.
  Fixtures contain only public sample Skills. The full native fixture has an
  empty project list and does not establish sidebar/task-history parity.
- Inspected dark and 860 × 560 native file views. A narrow-column layout
  initially hid the Markdown preview; adaptive header stacking and smaller
  card insets restore it. Remaining P2 differences include font metrics,
  English word wrapping in narrow Markdown, compact heading inline formatting,
  modal spacing/radii and sync-row rhythm. The smallest file pane truncates
  its filename; the tree can collapse and the inspector can maximize.
- Resource reads/writes use the existing rooted Skills manager. Per-file drafts
  remain separate from saved snapshots; a completion cannot replace text typed
  during save. Dirty closes require confirmation. Selection, tree and document
  generations reject late replies, and shutdown detaches the debug emitter.
- Receiving-app selection and single/library sync call the existing rsync
  service. Empty selections retain the backend's meaning of all Skills; the UI
  rejects removing the last explicit selection instead of displaying a false
  none state. Settings adds custom targets, commits path edits on blur/Enter,
  and removes only target configuration while preserving files.
- Debug uses the existing read-only Modu SDK service, with streamed events,
  stop, history and final output. Indexed frames replace prior stream versions;
  unrelated run frames are ignored. Cmd/Ctrl+Enter starts debug only when its
  prompt has focus and cannot submit the conversation behind the Skills page.
- Temporary directories validate real create/delete, root traversal rejection,
  file writes, retained newer drafts, target configuration and rsync copies.
  Gio pointer routing exercises the native delete confirmation. Controlled
  debug services verify request identity, stop and partial-output retention.
  No user's Skills, sync target, debug history or model account was mutated.
- Repository tests, Keel race checks, vet and offscreen renders pass. The
  macOS bundle is rebuilt and its ad hoc signature verifies for local use.
  Its 550-package Go dependency tree contains no Wails/WebView imports, and
  the executable links AppKit/Metal without WebKit. Physical native
  window/dialog interaction and the remaining full-client migration are still
  pending; these renders do not establish final visual acceptance.

final result: blocked


## Native conversation module and Select alignment iteration

### Sources and scope

- Conversation reference: `codex-clipboard-1f7006a1-29bc-4eba-b46c-e4f6c53e5073.png`.
  Implement the conversation module first, following the user's request to
  reuse Keel examples and progress one module at a time.
- Focus/alignment defect: `codex-clipboard-638994cc-1532-4f66-960a-5cab017013d5.png`.
  The supplied project dialog shows a brown, text-height outline on the
  permission selector. The annotated rectangle is evidence, not product UI.
- Reuse Keel `examples/chat` (Message, MessageScroller, Markdown, CopyButton),
  `examples/components/select.go` and `form.go`. Icon geometry comes from the
  installed Lucide package; no replacement application logo is generated.

### Changes and cause

- Conversation messages use right-aligned user bubbles, unlabelled assistant
  replies, grouped/collapsible tool activity, timestamps and copy actions.
  Parallel tool results pair by step and call ID, including silent failed
  results. Explicit disclosure state survives stream refreshes. The copy
  getter reads the latest document, retaining original Unicode content.
- Emoji use Markdown's documented InlineObject extension and native text
  widgets. The reference warning emoji still needs color-font refinement;
  complex heading formatting and long-message disclosure remain pending.
- Retain CopyButton's native ID so its enabled-owner feedback timer can finish.
  Message wrappers use ID-based hover observation rather than an empty Hover
  style handler, which made their input clip exclude the overflowing actions.
  The action row keeps visibility while the pointer moves from the message to
  the copy button.
- Keel v0.1.8 Select stripped the outer appearance but retained the default
  2dp keyboard focus stroke on its inner trigger. Padding belonged to the outer
  field, so the inner hit area covered only the text line. Fix the shared
  component: transfer padding/control height to the trigger, center the label
  and caret, use a highlight fill for plain keyboard focus and retain the single
  frame for styled controls. Normal height and Size scaling are preserved.
- Apply the same component fix in the local Keel checkout and carry a portable
  patch under `build/keel/go/select.patch`. The build launcher uses a private
  cached copy and temporary modfile, preserving the module cache and project
  dependency files. Direct Go commands receive a CLI flag so gopls and project
  subprocesses do not inherit the build replacement. Keel watch launches restore
  the user's GOFLAGS at application startup. Development packaging uses -debug
  to preserve the inherited replacement with Keel v0.1.8.

### Captures and validation

- `bin/keel-qa/project-select-comparison.png`: annotated source, unpatched Keel
  and patched Keel, with equal 440x379dp dialog crops at 2x scale. Source origin
  is (239,153); native crop origin is (246,169). The entire captured viewport is
  686x548dp. This compares the same open dialog and programmatic keyboard focus;
  the background task content is fixture data.
- `project-select-focus.png`, `project-select-focus-narrow.png` and
  `project-select-focus-dark.png`: actual production dialog, rendered over
  several Gio input-router frames to establish keyboard focus. The permission
  row fills its 32dp height without a stroke. Label/caret alignment and the
  reference input/dialog geometry are preserved. Dark focus uses the dark fill.
- `conversation-reference.png`, `conversation-reference-narrow.png`,
  `conversation-tools-open.png` and `conversation-reference-dark.png` remain
  iterative conversation evidence, not acceptance of the entire migrated UI.
- Passed patched full Go tests, race checks for the Keel application package,
  and full Go vet. Keel-side `ui/kit` and `ui/el` suites passed. Shared-control
  regressions cover framed/plain variants at 32/36/48dp, padding-corner clicks,
  keyboard opening and focus restoration. The production project dialog test
  covers padding clicks, direction-key permission changes, Space reopening and
  Esc without closing its parent dialog.
- The final app executable was rebuilt with the patched dependency, installed
  into the existing Keel-produced bundle and signed ad hoc. Strict/deep signature
  verification passed; `go version -m` identifies the private patched Keel copy.
  A subsequent normal packager run encountered a Go proxy EOF/deprecation lookup
  failure, so the verified existing bundle resources were retained.
- Native copy routing verifies the latest document, Unicode preservation and
  feedback expiry. Parallel tool pairing and disclosure retention also pass.
- AppKit window chrome/native sheets and paid Agent calls are not inferred from
  these sample-data captures. The scoped Select defect is resolved. Remaining
  conversation refinements and composer/profile/updater modules prevent final
  migration acceptance.

final result: blocked

## Agent picker and terminal alignment iteration — 2026-10-09

Scope: the latest two user screenshots report centered picker entries and a
terminal close button sitting above its title. These are application layout
choices, rather than defects in Keel's general-purpose centered Button.

- **P2, resolved:** local Agent, remote Worker and workflow entries now share a
  full-width list presentation: 8dp leading inset, fixed 16dp icon slot and 8dp
  icon-to-label gap. Short labels no longer move toward the center. Model text
  uses the same leading inset as section headings. Selected, disabled and hover
  states remain handled by Keel; keyboard focus uses the existing neutral fill.
- **P2, resolved:** terminal tabs explicitly center their children within the
  36dp header. Title controls use the header height and close controls retain
  their 26dp hit area. The 150dp maximum tab width reserves 26dp for closing,
  preventing a long title from pushing its close button outside the tab.
- `bin/keel-align-before/alignment-{menu,terminal}.png` and the matching files
  in `bin/keel-qa` use identical 1120x760dp viewports and fixture content.
  Narrow captures use 860x600dp; light and dark production layouts were inspected.
  `bin/keel-qa/alignment-comparison.png` compares identical 300x507dp picker
  crops from the 2x native captures. The terminal crop below is resized equally
  on both sides to show short/long title and close-button placement together.
  These captures exercise actual production views over Gio input-router frames.
- Native pointer tests verify blank-row selection and dismissal, unavailable
  Agent rejection, workflow selection, terminal tab focus and closing a tab
  without losing its siblings. Geometry checks verify shared icon/text columns
  and terminal centerlines at 1x and 2x, including a visible long tab in a narrow
  strip. Screenshots use isolated data; no Agent calls or home settings writes.
- Full patched Go tests, application race tests and full Go vet passed. The
  executable and existing Keel app bundle were rebuilt; strict/deep ad hoc
  signature verification is recorded with the build output.

Scoped alignment result: passed. Final migration acceptance remains pending
the conversation/composer/profile/updater work recorded above.

## Native titlebar double-click deadlock — 2026-10-09

- **P1, resolved:** reproduced the user's hang through a real AppKit double-click
  in an isolated data-directory client. Computer Use timed out after the second
  mouse-down. A process sample and SIGQUIT dump identify the native main thread
  in `keel_titlebar_double_click -> core.Update -> loop.Post -> InvalidateAll ->
  Gio Invalidate -> FlushEvents -> nextEvent`, re-entering Gio's invalidation
  mutex while `Invalidate` still holds it. The renderer waits for further events.
- Keel's native callback now posts from a Go goroutine; the UI action still runs
  under the frame lock and retains the closed-window check and macOS preference.
  The local Keel fix and regression live in `ui/window/titlebar_darwin.go` and
  `titlebar_darwin_test.go`. OneCatch carries `build/keel/go/titlebar.patch`; its
  build launcher fingerprints both patches and validates both modified sources.
- The regression blocks the registered invalidator, verifies that the native
  callback already returned, then drains the frame and verifies maximize.
- Actual native validation: the rebuilt signed bundle zoomed from 1120x760dp to
  the screen's larger window size, restored on a second double-click, then opened
  the More menu successfully. This is native AppKit evidence, beyond headless
  input checks. QA used `/tmp/onecatch-keel-titlebar-qa`, with no project changes
  or Agent calls. Original user app and other running apps were preserved.
- The hot-reload launcher reproduced the `sandbox_extension_issue_file_to_process`
  warning while the client continued running. It is separate from the diagnosed
  double-click deadlock; this iteration does not claim to remove that warning.
  The duplicate `-lobjc` linker warning also remains.
- Full patched project tests, application race checks and full Go vet passed.
  The signed macOS bundle and standalone binary were rebuilt against both fixes.
  Restart `task dev:keel` to pick up the new immutable dependency copy; an already
  running watcher retains its previous module replacement.

Scoped double-click result: passed. Final migration acceptance remains pending.

Keel's full `ui/window` suite and the native callback race regression also
passed. The isolated validation process was stopped after verification.

## Missing user message bubbles — 2026-10-09

- **P1, resolved:** the native transcript read task prompts and queued
  instructions, but omitted text-only continuations in `run.resumed` events.
  The original React timeline reads all three sources. Keel now restores these
  legacy messages by event sequence and retains them across live state snapshots.
  Identical text sent twice remains two turns; repeated detail reads stay idempotent.
- New text-only continuations use the existing durable instruction path before
  resume dispatch, matching attachment submissions. The user entry is available
  even before the asynchronous resume event exists. Empty retries do not enqueue
  another message; failed resumes retain the persisted message for retry.
- Complete instruction snapshots remove stale or removed queued bubbles, and
  applied entries use their application time to preserve conversation ordering.
- **P2, resolved:** bubble width measurement now uses Markdown's font family,
  rather than the different control font. A short English message no longer
  wraps its final letter onto a second line. The user bubble has an accessible
  article label and keeps its existing right alignment and neutral rounded fill.
- The real service regression runs an initial turn, a legacy continuation and
  two identical new sends with a mock Agent, closes/reopens the same isolated
  store, and verifies all four user messages. Production bubble geometry at
  1x/2x verifies visible right placement and single-line height for these texts.
  `bin/keel-qa/conversation-persisted-user-bubbles.png` renders that reopened
  conversation at 1120x900dp through Gio's router. No live Agent or home data
  was used. This turn's report contained no missing-message screenshot to pair
  against; this capture is evidence for the scoped fix, not final visual parity.
- Full patched Go tests, application race tests and full Go vet passed. The
  standalone executable and macOS bundle were rebuilt and signed.

Scoped user-message result: passed. Final migration acceptance remains pending.

## Standard Keel Sidebar integration — 2026-10-09

Acceptance changed during this iteration: the user prioritizes implementing
functionality with Keel's existing components and permits their standard styling.
Exact OneCatch sidebar styling is no longer a requirement for this module.

- `go.mod` now replaces Keel with the requested local checkout at
  `/Users/ityike/Code/go/src/github.com/dyike/keel`. The build launcher detects
  a local replacement and invokes Go directly instead of installing its cached
  v0.1.8 backport. Normal Go commands and `task dev:keel` share that dependency.
- The sidebar now uses `kit.Sidebar`, its nested items, `Header`, `Footer`,
  `Suffix` and `ContextMenu`, following Keel's component example. Keel owns
  row height, icons, pointer/keyboard activation, selected styling, scrolling,
  expansion and context-menu presentation. OneCatch supplies its data/actions.
  Suffixes remain visible and occupy stable slots; there is no hover-dependent
  insertion that moves the label or removes the menu under the pointer.
- Project/task selection, nested collapse, local/remote project labels, pinned
  tasks, title/prompt search, status filtering, show-all commands and existing
  project/task menus are connected. New-task buttons call the existing draft
  path. The retained component pointer preserves element identity while model
  refreshes update renamed, added or removed destinations. Hidden destinations
  close their obsolete menus.
- The experimental Keel API/style extensions made earlier in this turn were
  withdrawn. The Keel checkout is clean; this integration changes OneCatch only.
  No new component API or application-specific styling was added to Keel.
- Gio input-router tests at 1x/2x verify standard 36dp rows, full-height padding
  activation, stable suffix bounds, independent plus/menu actions, right-click,
  Escape, left/right keyboard expansion and task selection. Further checks
  exercise prompt search, show-all and model rename refresh without changing the
  component pointer. Captures `bin/keel-qa/sidebar-standard-{light,dark}.png`
  show the production component over isolated fixture data at 1120x900dp.
- Full local-dependency Go tests, application race tests and full Go vet passed.
  The standalone executable and signed macOS bundle were rebuilt using the
  local checkout. Restart an existing watcher after the replacement change.

Scoped sidebar functionality: passed. Other migration capabilities remain
pending; the earlier requirement to reproduce all sidebar styles is superseded.

## Agent icons in the standard Sidebar — 2026-10-09

- Single-Agent task rows, including pinned tasks, now use the existing embedded
  Codex, Claude Code, Modu Code, Pi, Grok, TRAE and DeepSeek marks. The selector
  and conversation controls reuse the same renderer. Multi-Agent workflows keep
  their workflow icon. Task model refresh includes the Agent ID, so a changed
  Agent refreshes its logo without changing the destination ID or title.
- This iteration adds one optional generic `SidebarItem.IconView` field to the
  local Keel checkout. It overrides the built-in icon inside the same 16dp slot;
  Keel still owns row layout, hover, selection, menus and keyboard handling.
  This supersedes the clean-checkout statement in the preceding iteration.
- Modu's transparent monochrome PNG uses the theme text color to remain visible
  in dark mode. Pi/Grok likewise follow the theme; colored marks preserve their
  original colors. No brand assets were replaced.
- Keel Sidebar tests at 1x/2x pass for custom-icon geometry, row selection,
  collapsed display and disabled behavior. OneCatch Sidebar interaction tests
  and SVG visible-pixel checks pass. Updated production light/dark captures
  `bin/keel-qa/sidebar-standard-{light,dark}.png` include all seven Agent marks.
  All OneCatch Keel application tests pass. The standalone executable and signed
  macOS bundle were rebuilt with the local dependency.

Scoped Agent-icon result: passed. Overall migration acceptance remains pending.

## Project disclosure after its label — 2026-10-09

- The local Keel Sidebar renderer now places a branch's disclosure arrow
  immediately after its label, such as `modu ›` / `modu ⌄`. The label and arrow
  share the flexible text region; the arrow keeps its 14dp size while long labels
  can truncate. Independent plus/menu suffixes stay in their trailing slots.
- Keel Sidebar tests and OneCatch project pointer/keyboard, suffix, search and
  model-refresh tests pass. Updated light/dark production captures show the
  arrow after `modu`. Both executables were rebuilt and the bundle re-signed.

Scoped disclosure-position result: passed. Migration acceptance remains pending.

## Header and section title alignment — 2026-10-09

- The sidebar remains the production `kit.Sidebar`; project management is
  application content in its Header slot, while projects use its Section title.
  The Header toolbar now matches the Section's 10dp left inset, 28dp height and
  `theme.TextSm` typography. This iteration changes only OneCatch's slot content.
- Production light/dark captures were regenerated and the left alignment was
  inspected. Existing Sidebar interaction tests pass. Both executables were
  rebuilt and the macOS bundle re-signed.

Scoped title-alignment result: passed. Migration acceptance remains pending.

## Sidebar search and project creation placement — 2026-10-09

- Search is now a labeled standard Keel ghost-button row below New. Its input
  opens immediately beneath that row and retains the existing query state.
  Closing search clears the query as before.
- Project management keeps its filter control; its adjacent plus/search icons
  were removed. A separate `+ 新建项目` row calls the existing project dialog.
  Project task-creation suffixes keep their existing actions.
- Existing Sidebar interaction checks pass. Updated light/dark captures show
  the revised production Header; the executables were rebuilt and the macOS
  bundle re-signed. This iteration changes OneCatch only.

Scoped control-placement result: passed. Migration acceptance remains pending.

## Standard Keel Settings window — 2026-10-10

- The settings window now uses `kit.Settings`, `SettingPage`, `SettingGroup`
  and `SettingItem` for navigation, search, groups and ordinary form rows.
  Skills, mobile pairing and Worker management use documented custom-content
  slots and keep their existing application actions. Keel itself is unchanged.
- Application-owned drafts, revision checks and persistence are preserved.
  Save/reload actions remain in a fixed footer so search cannot hide them.
  Worker unlock shortcuts also work when the standard sidebar has focus.
- Production input-router checks at 1x/2x pass for page selection, search,
  Agent changes and draft retention. Existing appearance, Worker, mobile and
  Skills checks pass, as do the complete Keel application suite, race checks
  and Go vet. Light/dark captures of six ordinary settings pages are saved as
  `bin/keel-qa/settings-component-{page}-{light,dark}.png`; Agent dark and
  security light captures were visually inspected. Both executables rebuilt.

Scoped Settings-component result: passed. Overall migration remains pending.

## Sidebar header simplification — 2026-10-10

- Removed the `+ 新建` header row; tasks start from each project's `+` suffix.
- Search is an always-visible input with a leading search icon. ⌘K focuses it
  (expanding a collapsed sidebar), Esc and the trailing clear button empty it.
- `+ 新建项目` moved into the 项目 section heading as a trailing `+`. Keel's
  Sidebar gained `SectionAction(title, action, items...)`; a section with an
  action keeps its heading while empty (hidden during a filter query), so a
  first project can still be added. The action sits flush with the filter icon.
- Keel kit and OneCatch keel tests pass; light/dark sidebar captures inspected.

## Icon and label vertical alignment — 2026-10-10

- Cause: Keel's optical text shift was measured once per font from a `国Ag`
  sample. With `SF Pro Text` and a PingFang fallback that sample's line box
  mixes both faces, so pure-Latin labels (own, shorter SF line box) sat 2.5dp
  low and pure-Chinese labels (own PingFang box) 1.25dp high beside icons.
- Fix (local Keel `ui/el/paint.go`): the shift is measured per script class —
  Latin (`Ag`, cap height), East Asian (`国`), mixed (`国Ag`) — and rounded
  instead of truncated. Inputs keep the mixed measurement so typing never
  moves the line.
- Measured on the 2× sidebar capture, ink-center offset text vs icon: Latin
  +5px → 0, Chinese −2.5/−3px → −0.5/−1px; composer chips and footer within
  ±2px. Keel `./...` and OneCatch keel tests pass.

## Sidebar section heading weight — 2026-10-10

- Keel's Sidebar section headings were always regular weight (the components
  gallery matches). Keel now offers `SectionTitleWeight(font.Weight)` (default
  unchanged) and `el` gained a general `Weight(font.Weight)` text setter.
- OneCatch sets `font.SemiBold` for 项目 and for its mirrored 项目管理 Header
  heading through one shared constant. Keel and OneCatch keel tests pass.

## Remote project marker as a Keel Tag — 2026-10-10

- Remote FS projects show `H5000M` followed by a neutral `kit.Tag("远端")`
  (20dp) instead of the ` · 远端` label suffix. Keel's `SidebarItem` gained a
  display-only `Tag` slot right after the label (before the disclosure arrow),
  kept at regular weight on selected rows and hidden when collapsed.
- The sidebar model fingerprint now includes the remote flag, since the label
  alone no longer changes. Keel and OneCatch keel tests pass.

## Sidebar search state, filter placement, More menu, remote tone — 2026-10-10

- The status filter now sits right of the search box on the same row; the
  项目管理 heading stands alone above the navigation links.
- The search box shows focus: Surface fill, 1dp Border and a Text-colored
  search icon while `task-search` is focused (1dp transparent border at rest,
  so focus does not move layout).
- 更多 opens as a dropdown below its row (`el.Bottom, el.Start`) inside the
  sidebar instead of overlapping the page. The separator appears only when
  hidden pages precede it, and 编辑侧栏… has the square-pen icon like the other
  entries. Keel still focuses the first item on open for keyboard use.
- The 远端 tag uses `kit.ToneInfo`, distinct from the green 本地 caption pill.
- Captures of the open menu and focused search were inspected; OneCatch keel
  tests pass.

## Focus ring after closing a menu with the mouse — 2026-10-10

- Cause: a closing overlay returned focus to its trigger (the 更多 row) as
  programmatic focus, which Keel always ringed, so a mouse open/pick left a
  2dp Primary outline on the row.
- Fix (local Keel `ui/el`): the root tracks input modality — pointer presses,
  outside-press dismissal and drags clear it; key presses, Escape and
  shortcuts set it. Restored focus shows its ring only after keyboard input,
  like `:focus-visible`; Escape/Tab users still see where focus returned.
- New Keel test `TestRestoredFocusRingFollowsInputModality`; Keel `./...` and
  OneCatch keel tests pass.

## CoreText text rendering and PingFang — 2026-10-10

- Cause of "fonts look worse than Wails": (1) go-text never scanned macOS's
  `/System/Library/AssetsV2` font assets, so `PingFang SC` never resolved and
  Chinese used Hiragino Sans GB W3 (no Medium/Semibold); (2) Gio fills vector
  outlines blended in linear light, without CoreText's stem darkening and
  gamma, so all text carried ~62–70% of WKWebView's ink.
- Fix (local Keel and its Gio/go-text copies): fontscan scans the AssetsV2
  font directories; Gio's shaper exposes `GlyphFile` and a per-shaper
  `SetRasterHook`; labels, editors and selectables pass their solid color.
  Keel's theme rasterizes glyph masks with CoreText (sRGB coverage converted
  to the linear alpha that reproduces it; dark-on-light and light-on-dark
  variants; quarter-pixel phases), caches them in tinted 1024² pages with
  per-frame copy-on-write, and draws labels, inputs, rich ranges, markdown,
  the code editor, shimmer and the terminal through it. Emoji, in-memory
  fonts and >160px text keep vector outlines. `KEEL_TEXT=vector` compares.
- OneCatch `go.mod` now replaces `gioui.org` and go-text with Keel's
  `third_party` copies, as Keel documents for apps.
- Measured at 14sp/2x against a WKWebView capture of the same line: ink
  Latin 0.70 → 1.00, Chinese 0.62 → 1.06; dark mode matches visually. Icon
  and label ink centers stay within ±1px. Steady-state frame cost for ~2,000
  glyphs 0.43 ms vs 0.42 ms vector. Remaining difference: WebKit's system
  font tightens Latin tracking slightly.
- Keel (including both third_party modules), race checks on theme/el/markdown
  and OneCatch tests pass; Keel `ui/...` still builds against upstream Gio.
