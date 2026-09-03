import assert from "node:assert/strict";
import { readdir, readFile } from "node:fs/promises";
import test from "node:test";
import { fileURLToPath } from "node:url";
import path from "node:path";

const sourceRoot = path.dirname(fileURLToPath(import.meta.url));

function harnessSettingsSection(source) {
  const start = source.indexOf("function HarnessSettings(");
  const end = source.indexOf("function ExecutionSettings(", start);
  assert.ok(start >= 0 && end > start, "locate the bounded Harness section before applying its chrome exceptions");
  return source.slice(start, end);
}

async function jsxSources(directory = sourceRoot) {
  const entries = await readdir(directory, { withFileTypes: true });
  const sources = [];
  for (const entry of entries) {
    const filename = path.join(directory, entry.name);
    if (entry.isDirectory()) sources.push(...await jsxSources(filename));
    else if (entry.name.endsWith(".jsx")) sources.push([filename, await readFile(filename, "utf8")]);
  }
  return sources;
}

test("action buttons use the shared unbracketed component", async () => {
  const sources = await jsxSources();
  for (const [filename, source] of sources) {
    assert.doesNotMatch(source, /\bbracket=/, `${filename} still configures bracket rendering`);
    assert.doesNotMatch(source, /<button\b[^>]*>\s*\[\s/s, `${filename} still renders a manually bracketed button`);
    assert.doesNotMatch(source, /className=["'][^"']*\b(?:primary-button|secondary-button|text-button|danger-button|danger-outline)\b/, `${filename} still uses a legacy button class`);
  }
});

test("every action tone maps to a real button variant", async () => {
  const primitives = await readFile(path.join(sourceRoot, "ui", "primitives.jsx"), "utf8");
  const button = await readFile(path.join(sourceRoot, "components", "ui", "button.jsx"), "utf8");
  // The old .ui-action-- classes are gone; Action now delegates to shadcn, so
  // the invariant worth protecting is that every tone it can be handed resolves
  // to a variant the Button actually defines.
  const variants = [...primitives.matchAll(/^\s{2}(\w+): "(\w+)",$/gm)]
    .filter(([, key]) => ["primary", "accent", "muted", "danger", "cyan"].includes(key))
    .map(([, , variant]) => variant);
  assert.equal(variants.length, 5, "ACTION_VARIANT must cover every tone the app passes");
  for (const variant of new Set(variants)) {
    assert.match(button, new RegExp(`\\b${variant}:\\s*$|\\b${variant}:\\s*"`, "m"), `button.jsx defines no "${variant}" variant`);
  }
  // The two tinted tones must still restate a colour, or they collapse into
  // plain outline buttons and stop reading as destructive/informational.
  assert.match(primitives, /danger: "border-destructive\/40 text-destructive/);
  assert.match(primitives, /cyan: "border-info\/40 text-info/);
});

test("workspace actions use a compact project menu and tasks stay visually light", async () => {
  const sidebar = await readFile(path.join(sourceRoot, "app", "components", "Sidebar.jsx"), "utf8");
  // Geometry now lives in Tailwind classes on the markup rather than in
  // mirage.css, so these assert the same intent against the class lists.
  assert.match(sidebar, /className="workspace-row-actions[^"]*"/);
  assert.match(sidebar, /className="workspace-row-actions[^"]*\bopacity-0\b/, "project actions must stay out of sight until the row is engaged");
  assert.match(sidebar, /className="workspace-row-actions[^"]*group-hover:opacity-100/);
  assert.match(sidebar, /className="workspace-row-actions[^"]*group-focus-within:opacity-100/, "keyboard users must be able to reach the project actions");
  // Not `hidden`: display:none drops the buttons out of the tab order (so
  // group-focus-within can never fire) and un-focuses the trigger the instant
  // the menu closes, leaving Radix nowhere to restore focus to.
  assert.doesNotMatch(sidebar, /className="workspace-row-actions[^"]*\bhidden\b/, "project actions must not be hidden with display:none");
  // The menu is a shadcn DropdownMenu now, so role="menu"/aria-haspopup are
  // emitted by Radix at runtime and cannot be asserted in the source. What the
  // source still has to prove is that the row keeps its actions revealed while
  // the menu is up — Radix stamps data-state on the trigger.
  assert.match(sidebar, /className="workspace-row-actions[^"]*has-\[\[data-state=open\]\]:opacity-100/, "the row must stay revealed while its menu is open");
  assert.match(sidebar, /<DropdownMenuTrigger asChild>\s*<Action\b[^>]*className="workspace-menu-trigger(?:\s[^"]*)?"[^>]*><Ellipsis\b/, "the ellipsis button must be the menu trigger");
  // Pinning moved from projects to tasks; task rows own the pin/unpin action.
  assert.doesNotMatch(sidebar, /\bonTogglePinned\b/);
  assert.match(sidebar, /onClick=\{\(\) => onToggleTaskPinned\(task\)\}/);
  assert.match(sidebar, /<DropdownMenuItem variant="destructive" onSelect=\{\(\) => onRemoveWorkspace\(workspace\)\}/, "remove must read as destructive");
  assert.match(sidebar, /className="workspace-new-task(?:\s[^"]*)?"[^>]*aria-label=\{t\("sidebar\.newTaskInProject"/);
  assert.match(sidebar, /className="add-workspace(?:\s[^"]*)?"[^>]*aria-label=\{t\("sidebar\.addProject"\)\}[^>]*onClick=\{onAddWorkspace\}/);
  assert.doesNotMatch(sidebar, /<StatusPill\b|project-task-time|project-task-heading/);
  assert.match(sidebar, /aria-current=\{selected \? "page" : undefined\}/);
  // A selected task is tinted, never railed with a leading accent bar.
  assert.match(sidebar, /project-task-item[^`]*bg-transparent/, "task rows must not carry their own surface");
  assert.doesNotMatch(sidebar, /project-task-item[^`]*shadow-\[inset/, "selected tasks must not use a leading accent rail");
});

test("sidebar reserves its footer for navigation and exposes a resize separator", async () => {
  const sidebar = await readFile(path.join(sourceRoot, "app", "components", "Sidebar.jsx"), "utf8");
  const app = await readFile(path.join(sourceRoot, "app", "App.jsx"), "utf8");
  assert.doesNotMatch(sidebar, /runtime-panel|runtime-row/, "runtime status belongs on the settings page, not in the sidebar");
  assert.match(sidebar, /className="sidebar-resizer[^"]*"[^>]*role="separator"/);
  assert.match(sidebar, /className="primary-nav[^"]*\bmt-auto\b/, "navigation must sit at the bottom of the rail");
  assert.match(app, /className=\{`app-shell\b[^`]*\bgrid-cols-\[auto_minmax\(0,1fr\)\]/, "the rail keeps its user-selected width; only the main column flexes");
  assert.doesNotMatch(app, /className=\{`app-shell[^`]*grid-cols-\[\d+px/, "the shell must not pin the rail to a fixed width");
});

test("application chrome cannot be selected while transcript content remains copyable", async () => {
  const sidebar = await readFile(path.join(sourceRoot, "app", "components", "Sidebar.jsx"), "utf8");
  const workbench = await readFile(path.join(sourceRoot, "app", "components", "TaskWorkbench.jsx"), "utf8");
  assert.match(sidebar, /className=\{`sidebar[^`]*\bselect-none\b/, "navigation labels must not behave like document text");
  assert.match(workbench, /className="workbench-empty[^"]*\bselect-none\b/, "empty transcript guidance is application chrome");
  assert.match(workbench, /className="conversation-scroll[^"]*\bselect-text\b/, "conversation content must remain explicitly selectable");
});

test("application separators stay thin and use the shared border theme", async () => {
  // Harness disclosures, workflow editors, review panes and Markdown tables
  // now have intentional separators. A blanket ban contradicts those flows;
  // protect their quiet, theme-aware treatment instead.
  const sources = await jsxSources();
  for (const [filename, source] of sources) {
    const relative = path.relative(sourceRoot, filename);
    if (relative !== path.join("ui", "primitives.jsx") && !relative.startsWith(`app${path.sep}`)) continue;
    for (const [, literal, template] of source.matchAll(/className=(?:"([^"]*)"|\{`([^`]*)`)/g)) {
      const classes = literal ?? template;
      const seams = classes.match(/\b(?:border-[tby]|divide-y)(?:-[^\s"'`}]+)?\b/g) || [];
      for (const seam of seams) {
        assert.match(seam, /^(?:border-[tby]|divide-y)(?:-[01])?$/, `${relative} adds a heavy or hard-coded separator: ${seam}`);
        if (seam.endsWith("-0")) continue;
        const color = seam.startsWith("divide-") ? /\bdivide-border(?:\/\d+)?\b/ : /\bborder-border(?:\/\d+)?\b/;
        assert.match(classes, color, `${relative} must color ${seam} through the shared border token`);
      }
    }
  }

  for (const file of ["styles.css", "mirage.css", "index.css"]) {
    const css = await readFile(path.join(sourceRoot, file), "utf8");
    for (const [, value] of css.matchAll(/border-(?:top|bottom)\s*:\s*([^;}]+)/g)) {
      assert.match(value.trim(), /^(?:0|1px solid (?:var\(--border\)|color-mix\(in oklab, var\(--border\) \d+%, transparent\)))$/, `${file} adds a heavy or unthemed separator: ${value}`);
    }
  }
});

test("workflow and settings share one contained footer menu", async () => {
  const sidebar = await readFile(path.join(sourceRoot, "app", "components", "Sidebar.jsx"), "utf8");
  assert.match(sidebar, /className="primary-nav[^\"]*grid-cols-\[minmax\(0,1fr\)_36px\][^\"]*bg-sidebar-accent\/20[^\"]*px-3[^\"]*pt-1 pb-3[^\"]*shadow-\[inset_0_1px_0_color-mix\(in_oklab,var\(--sidebar-border\)_35%,transparent\)\]/, "the footer must be full width, visually centered above the clipped rail edge, and use only one quiet top divider");
  assert.doesNotMatch(sidebar, /className="primary-nav[^\"]*(?:mx-|mb-|rounded|\sborder(?:-|\s))/, "the footer must not be inset or outlined on four sides");
  assert.match(sidebar, /<DropdownMenuTrigger asChild>[\s\S]{0,200}?className={`secondary-navigation-trigger/);
  assert.match(sidebar, /secondary-navigation-trigger-content[^`]*inline-flex h-8[^`]*max-w-full self-center[^`]*rounded-lg[^`]*group-data-\[state=open\]:bg-sidebar-accent/, "the visible menu selection must hug its label and stay vertically centered inside the button row");
  assert.match(sidebar, /secondary-navigation-trigger group flex h-9[^`]*self-center items-center/, "the menu button must stay vertically centered in the footer band");
  assert.match(sidebar, /secondary-navigation-trigger-content[^`]*items-center[^`]*leading-none/, "the menu icon and label must share one optical center");
  assert.doesNotMatch(sidebar, /secondary-navigation-trigger-content[^`]*group-focus-visible:ring/, "the menu must not retain an outlined focus frame after pointer interaction");
  assert.match(sidebar, /<DropdownMenuItem[^>]*onSelect=\{\(\) => goToSecondaryView\("workflows"\)\}/);
  assert.match(sidebar, /<DropdownMenuItem[^>]*onSelect=\{\(\) => goToSecondaryView\("settings"\)\}/);
  // The footer sits at the bottom of the rail, so the menu opens upward while
  // matching the full inner width of the two-button group.
  assert.match(sidebar, /<DropdownMenuContent side="top"[^>]*alignOffset=\{0\}[^>]*collisionPadding=\{12\}[^>]*w-\[calc\(var\(--radix-dropdown-menu-trigger-width\)\+40px\)\]/s, "the menu must align to the footer band");
});

test("settings and workflows are routed to singleton native windows", async () => {
  const main = await readFile(path.join(sourceRoot, "main.jsx"), "utf8");
  const app = await readFile(path.join(sourceRoot, "app", "App.jsx"), "utf8");
  assert.match(main, /if \(windowKind === "settings"\)[\s\S]*import\("\.\/app\/AuxiliaryWindow\.jsx"\)[\s\S]*module\.SettingsWindow/);
  assert.match(main, /if \(windowKind === "workflows"\)[\s\S]*module\.WorkflowsWindow/);
  assert.match(main, /return import\("\.\/app\/App\.jsx"\)/);
  assert.doesNotMatch(main, /^import App from/m, "auxiliary windows must not eagerly load the main application");
  assert.match(app, /next === "settings" \? WindowBinding\.OpenSettings\(\) : WindowBinding\.OpenWorkflows\(\)/);
});

test("settings and terminal defer non-critical startup work", async () => {
  const auxiliary = await readFile(path.join(sourceRoot, "app", "AuxiliaryWindow.jsx"), "utf8");
  const settings = await readFile(path.join(sourceRoot, "app", "SettingsPage.jsx"), "utf8");
  const workbench = await readFile(path.join(sourceRoot, "app", "components", "TaskWorkbench.jsx"), "utf8");
  assert.match(auxiliary, /loadSettingsBootstrap\(\)[\s\S]*setMode\("wails"\)/);
  assert.match(auxiliary, /scheduleIdle\(\(\) => \{[\s\S]*loadSettingsSupport\(\)/);
  assert.match(auxiliary, /Promise\.allSettled\(\[[\s\S]*ListRuntimes\(\)/);
  assert.doesNotMatch(settings, /runtimeConfigurationAutoChecked|checkRuntime\("codex"\);\s*checkRuntime\("claude"\);/);
  assert.match(workbench, /lazy\(\(\) => import\("\.\/TerminalDock\.jsx"\)\)/);
  assert.match(workbench, /terminalMounted && <Suspense/);
});

test("the settings window uses the main window's inset sidebar and draggable chrome", async () => {
  const settings = await readFile(path.join(sourceRoot, "app", "SettingsPage.jsx"), "utf8");
  const auxiliary = await readFile(path.join(sourceRoot, "app", "AuxiliaryWindow.jsx"), "utf8");
  const nativeWindow = await readFile(path.join(sourceRoot, "..", "..", "internal", "app", "desktop", "auxiliary_window_controller.go"), "utf8");
  assert.match(settings, /grid-cols-\[216px_minmax\(0,1fr\)\]/);
  assert.match(settings, /settings-page[^\"]*bg-transparent/);
  assert.match(settings, /<ScrollArea className="sidebar settings-sidebar[^\"]*\[clip-path:inset\(8px_4px_8px_8px_round_16px\)\]/);
  assert.match(settings, /<div className="settings-titlebar drag-region [^"]*h-\[52px\][^"]*grid-cols-\[216px_minmax\(0,1fr\)\]/);
  assert.match(settings, /settings-titlebar-sidebar/);
  assert.match(settings, /const compactAuxiliaryChrome = usesCompactAuxiliaryChrome\(\)/, "settings chrome follows the desktop platform");
  assert.match(settings, /\? <strong className="auxiliary-sidebar-title[^"]*left-0[^"]*w-\[216px\][^"]*justify-start/, "Windows and Linux keep the settings identity in the sidebar title region");
  assert.match(settings, /: <strong className="pointer-events-none absolute inset-0 flex items-center justify-center/, "macOS centres the settings identity in the full window title strip");
  assert.doesNotMatch(settings, /settings\.preferences/, "the settings rail starts with navigation instead of a redundant preferences title");
  assert.match(settings, /<header className="drag-region [^"]*"/);
  assert.match(settings, /<div className="no-drag flex shrink-0 items-center gap-2\.5">/);
  assert.match(auxiliary, /nativeSidebar\.postMessage\(\{ width: document\.querySelector\("\.settings-sidebar"\)[^;]*\|\| 216 \}\);/);
  assert.doesNotMatch(auxiliary, /flush: true/);
  assert.match(auxiliary, /flex h-full min-h-0 flex-col overflow-hidden bg-transparent text-foreground/);
  assert.match(nativeWindow, /macOptions\.InvisibleTitleBarHeight = 28/);
  assert.match(nativeWindow, /CustomTheme:\s*auxiliaryWindowsTheme\(\)/, "native Windows auxiliary captions use the application canvas colours");
  assert.match(nativeWindow, /Frameless:\s*runtime\.GOOS == "windows" \|\| runtime\.GOOS == "linux"/, "desktop auxiliary windows keep their divider inside app-drawn chrome");
});

test("settings uses shadcn form controls with native harness disclosures", async () => {
  const settings = await readFile(path.join(sourceRoot, "app", "SettingsPage.jsx"), "utf8");
  const controls = await readFile(path.join(sourceRoot, "app", "components", "settings", "SettingsControls.jsx"), "utf8");
  const workerPage = await readFile(path.join(sourceRoot, "app", "components", "WorkerPage.jsx"), "utf8");
  const workerModal = await readFile(path.join(sourceRoot, "app", "components", "WorkerModal.jsx"), "utf8");
  // The confirmation dialog lives beside the controls it borrows rather than
  // inside the settings screen, so that a window needing only the dialog does
  // not load the whole screen with it.
  const confirmDialog = await readFile(path.join(sourceRoot, "app", "components", "settings", "ConfirmDialog.jsx"), "utf8");
  for (const [name, source] of [["settings", settings], ["worker page", workerPage], ["worker dialog", workerModal], ["confirm dialog", confirmDialog]]) {
    assert.doesNotMatch(source, /ui\/primitives|\bTUISelect\b|<(?:input|select|textarea)\b/, `${name} still uses the TUI layer or raw form inputs`);
    // Native buttons are valid for the custom two-part harness disclosure.
    // settingsHarnessSection.test.js checks their count, naming, panel
    // linkage, and lack of nested form controls.
    const harness = name === "settings" ? harnessSettingsSection(source) : "";
    const controls = harness ? source.replace(harness, harness.replace(/<button\b[\s\S]*?<\/button>/g, "")) : source;
    assert.doesNotMatch(controls, /<button\b/, `${name} has an unshared button outside the harness disclosures`);
  }
  for (const component of ["Button", "Card", "Input", "Label", "Select", "Switch"]) {
    assert.match(controls, new RegExp(`components/ui/${component.toLowerCase()}`), `${component} must come from shadcn`);
  }
  assert.match(confirmDialog, /components\/ui\/dialog/);
  assert.match(settings, /components\/ui\/scroll-area/);
});

test("remote worker settings remain a disabled coming-soon preview", async () => {
  const settings = await readFile(path.join(sourceRoot, "app", "SettingsPage.jsx"), "utf8");
  const i18n = await readFile(path.join(sourceRoot, "i18n.js"), "utf8");
  assert.match(settings, /function ExperimentalSettings\(\{ enabled \}\)/);
  assert.match(settings, /pointer-events-none select-none opacity-50 grayscale/);
  assert.match(settings, /<SettingsSwitchRow checked=\{enabled\} disabled/);
  assert.doesNotMatch(settings, /function ExperimentalSettings[^]*workersPanel/);
  assert.match(settings, /!\["runtime", "experimental"\]\.includes\(section\)/, "the preview section must not expose reset actions");
  assert.match(i18n, /"settings\.comingSoon": "敬请期待"/);
  assert.match(i18n, /"settings\.comingSoon": "Coming soon"/);
});

test("workflow windows use desktop selection rules and shadcn editors", async () => {
  const library = await readFile(path.join(sourceRoot, "app", "components", "workflow", "WorkflowLibrary.jsx"), "utf8");
  const loopEditor = await readFile(path.join(sourceRoot, "app", "components", "workflow", "WorkflowEditor.jsx"), "utf8");
  const dagEditor = await readFile(path.join(sourceRoot, "app", "components", "workflow", "DAGWorkflowEditor.jsx"), "utf8");
  const identity = await readFile(path.join(sourceRoot, "app", "components", "workflow", "WorkflowIdentityFields.jsx"), "utf8");

  assert.match(library, /workflow-window[^\"]*\bselect-none\b/, "workflow display pages should behave like desktop chrome");
  assert.doesNotMatch(library, /className="[^"]*\bselect-text\b/, "workflow IDs and transition chips should not behave like web document text");

  for (const [name, source] of [["loop editor", loopEditor], ["DAG editor", dagEditor], ["workflow identity", identity]]) {
    assert.doesNotMatch(source, /ui\/primitives|\bTUISelect\b|<(?:input|select|textarea)\b/, `${name} still uses the TUI layer or raw form controls`);
  }
  for (const source of [loopEditor, dagEditor]) {
    assert.match(source, /workflow-editor-surface[^\"]*\bselect-none\b/, "editor labels and cards must not be accidentally selectable");
    assert.match(source, /components\/ui\/button/);
    assert.match(source, /components\/ui\/input/);
    assert.match(source, /components\/ui\/textarea/);
    assert.match(source, /SettingsSelect/);
    assert.match(source, /Textarea className="[^"]*\bselect-text\b/, "editable prompts must remain selectable inside the non-selectable editor shell");
  }
  assert.doesNotMatch(dagEditor, /<Modal\s+wide/, "the DAG editor should not be wrapped in the legacy wide toolbar");
});

test("confirmation dialogs contain long workspace paths", async () => {
  const confirmDialog = await readFile(path.join(sourceRoot, "app", "components", "settings", "ConfirmDialog.jsx"), "utf8");
  assert.match(confirmDialog, /className="min-w-0 sm:max-w-md"/, "confirmation content must allow long details to shrink inside the dialog");
  assert.match(confirmDialog, /\[overflow-wrap:anywhere\]/, "long workspace paths must wrap instead of widening the confirmation dialog");
});

test("sidebar menus delegate dismissal to Radix instead of hand-rolled effects", async () => {
  const sidebar = await readFile(path.join(sourceRoot, "app", "components", "Sidebar.jsx"), "utf8");
  // Both menus used to carry their own outside-click, Escape and focus-restore
  // listeners plus manual fixed positioning. DropdownMenu owns all of it now;
  // this guards against that machinery creeping back in.
  assert.doesNotMatch(sidebar, /secondaryNavigationOpen|projectMenuWorkspaceID|projectMenuPosition/, "menu open-state must live in Radix, not component state");
  assert.doesNotMatch(sidebar, /firstSecondaryNavigationItem|secondaryNavigationTrigger|projectMenuTrigger/, "Radix restores focus; no manual focus refs");
  assert.doesNotMatch(sidebar, /getBoundingClientRect\(\)[\s\S]{0,200}menuWidth/, "menu placement must come from Radix, not manual measurement");
  assert.doesNotMatch(sidebar, /addEventListener\("pointerdown"/, "outside-click dismissal is Radix's job");
});

test("appearance controls stay labelled radiogroups", async () => {
  const settings = await readFile(path.join(sourceRoot, "app", "SettingsPage.jsx"), "utf8");
  assert.match(settings, /className="appearance-mode-picker[^"]*"[^>]*role="radiogroup"/);
  assert.match(settings, /className="appearance-accent-picker[^"]*"[^>]*role="radiogroup"/);
  // Each option must be an aria-checked radio, not a plain button.
  assert.match(settings, /role="radio" aria-checked=\{appearance\.theme === mode\}/);
  assert.match(settings, /role="radio"[^>]*aria-checked=\{appearance\.accent === accent\}/);
  // The accent options carry their name; the swatch only supplements it, so the
  // control never degrades into unlabelled colour dots.
  assert.match(settings, /\{t\(`settings\.themeColor\.\$\{accent\}`\)\}/);
  assert.match(settings, /ACCENT_SWATCH\[accent\]/);
});

test("appearance changes propagate to every native window", async () => {
  const main = await readFile(path.join(sourceRoot, "main.jsx"), "utf8");
  const settings = await readFile(path.join(sourceRoot, "app", "SettingsPage.jsx"), "utf8");
  assert.match(settings, /Events\.Emit\(APPEARANCE_CHANGED_EVENT, next\)/);
  assert.match(main, /Events\.On\(APPEARANCE_CHANGED_EVENT, syncAppearance\)/);
  assert.match(main, /window\.addEventListener\("storage"/);
});

test("no stylesheet has a dangling selector group", async () => {
  // A line-based edit that removes the line carrying "{ ... }" leaves the
  // selectors above it dangling, and they silently merge into the next rule.
  // That is how `white-space: nowrap` once got applied to every transcript
  // container, blowing the column past its width so content was clipped.
  for (const file of ["mirage.css", "styles.css", "index.css"]) {
    const raw = await readFile(path.join(sourceRoot, file), "utf8");
    const css = raw.replace(/\/\*[\s\S]*?\*\//g, "");
    const lines = css.split("\n");
    // Track block nesting so a wrapped property value (which also ends in a
    // comma) is not mistaken for a selector group.
    const stack = [];
    for (let i = 0; i < lines.length; i += 1) {
      const line = lines[i].trim();
      const inDeclarations = stack.length > 0 && stack[stack.length - 1] === "rule";
      if (line.endsWith(",") && !inDeclarations && /[.#[a-zA-Z]/.test(line)) {
        let j = i + 1;
        while (j < lines.length && lines[j].trim().endsWith(",")) j += 1;
        assert.ok(
          (lines[j] || "").includes("{"),
          `${file}:${i + 1} starts a selector group that never reaches a declaration block`,
        );
      }
      for (const char of line) {
        if (char === "{") stack.push(line.trim().startsWith("@") ? "at" : "rule");
        else if (char === "}") stack.pop();
      }
    }
  }
});

test("the transcript column cannot be widened past its container", async () => {
  const css = await readFile(path.join(sourceRoot, "index.css"), "utf8");
  // A bare `display: grid` gives an implicit `auto` track that sizes toward
  // max-content, so one long unbreakable command widens the whole transcript
  // and .conversation-scroll clips it with overflow-x: hidden.
  for (const selector of ["conversation-list", "conversation-round-body"]) {
    assert.match(
      css,
      new RegExp(`\\.${selector}\\s*\\{[^}]*grid-template-columns:\\s*minmax\\(0,\\s*1fr\\)`, "s"),
      `.${selector} must pin its track to minmax(0, 1fr)`,
    );
  }
  // Nothing may impose nowrap on a transcript container.
  const mirage = await readFile(path.join(sourceRoot, "mirage.css"), "utf8");
  for (const rule of mirage.matchAll(/([^{}]+)\{([^}]*)\}/g)) {
    if (!/white-space:\s*nowrap/.test(rule[2])) continue;
    assert.doesNotMatch(
      rule[1],
      /\.conversation-(?:list|section|round|round-body|agent|user)\b/,
      "transcript containers must be allowed to wrap",
    );
  }
});

test("the transcript and composer share one content edge", async () => {
  const css = await readFile(path.join(sourceRoot, "index.css"), "utf8");
  assert.match(css, /\.conversation-workspace\s*\{[^}]*--conversation-content-width:\s*920px[^}]*--conversation-inline-gutter:/s);
  assert.match(css, /\.conversation-section\s*\{[^}]*padding-inline:\s*var\(--conversation-inline-gutter\)/s);
  assert.match(css, /\.conversation-list\s*\{[^}]*max-width:\s*var\(--conversation-content-width\)[^}]*padding:\s*30px\s+0\s+44px/s);
  assert.match(css, /\.workbench-composer\s*\{[^}]*padding:\s*8px\s+var\(--conversation-inline-gutter\)\s+14px/s);
  assert.match(css, /\.workbench-composer-inner\s*\{[^}]*max-width:\s*var\(--conversation-content-width\)/s);
});

test("the task title is unified into the app titlebar", async () => {
  const app = await readFile(path.join(sourceRoot, "app", "App.jsx"), "utf8");
  const workbench = await readFile(path.join(sourceRoot, "app", "components", "TaskWorkbench.jsx"), "utf8");
  assert.match(app, /className="app-titlebar-task[^\"]*"/);
  assert.match(app, /<StatusPill status=\{selectedTaskStatus\}/);
  assert.doesNotMatch(workbench, /conversation-workspace-head|workflowNameFor|shortID\(runDetail\.run\.id\)/, "task metadata must not repeat in a second header");
});

test("new tasks are composed inside the chat workspace instead of a modal", async () => {
  const app = await readFile(path.join(sourceRoot, "app", "App.jsx"), "utf8");
  const workbench = await readFile(path.join(sourceRoot, "app", "components", "TaskWorkbench.jsx"), "utf8");
  const newTask = await readFile(path.join(sourceRoot, "app", "components", "NewTaskView.jsx"), "utf8");
  const executor = await readFile(path.join(sourceRoot, "app", "components", "TaskExecutorSelector.jsx"), "utf8");
  const css = await readFile(path.join(sourceRoot, "index.css"), "utf8");
  assert.doesNotMatch(app, /task-create-dialog|<Modal[^>]*task\.createTitle/, "task creation must not open a blocking modal");
  assert.match(app, /const \[taskModal, setTaskModal\] = useState\(true\)/, "a cold start must open the actionable composer instead of the legacy welcome card");
  assert.match(app, /newTaskOpen=\{taskCreateVisible\}/);
  assert.match(app, /const taskCreateVisible = view === "tasks" && !editor && \(taskModal \|\| !selectedTask\)/, "the composer stays open for the empty workspace and when a task is being created");
  assert.match(workbench, /newTaskOpen \? <NewTaskView/);
  assert.match(newTask, /className="new-task-composer"/);
  assert.match(newTask, /className=\{`new-task-toolbar \$\{directAgent \? "agent-mode" : "workflow-mode"\} \$\{showRuntimeProfile \? "has-runtime-profile" : "no-runtime-profile"\}`\}/);
  assert.match(css, /\.new-task-toolbar\.no-runtime-profile \.new-task-submit-group\s*\{\s*margin-left:\s*auto;/, "Agents without model controls must keep the submit action aligned to the edge");
  assert.doesNotMatch(newTask, /className="new-task-select execution"/, "execution mode belongs with the final submit action");
  assert.match(newTask, /className=\{`new-task-submit-group \$\{executionMode\}`\}/);
  assert.match(newTask, /className="new-task-add"/, "the plus button keeps attachment and execution-mode actions compact");
  assert.match(newTask, /<TaskExecutorSelector form=\{form\} workflows=\{workflows\} runtimes=\{runtimes\} runtimeSettings=\{runtimeSettingsByHarness\} remoteFS=\{remoteFS\} onChange=\{onChange\} \/>/, "Agent and workflow selection remains visible in the composer and filters by enabled harnesses");
  assert.match(newTask, /DropdownMenuRadioGroup value=\{executionMode\}/);
  assert.match(newTask, /value="immediate"[^>]*><ArrowUp/);
  assert.match(newTask, /value="queued"[^>]*><ListPlus/);
  assert.doesNotMatch(executor, /compact|new-task-add|executionMode/, "the Agent and workflow selector is not hidden inside the plus menu");
  assert.match(newTask, /executionMode === "queued" \? <ListPlus/);
  assert.doesNotMatch(newTask, /\bMic\b|new-task-voice|task\.voiceInput/, "voice input must stay hidden until it is implemented");
  assert.doesNotMatch(newTask, /new-task-submit-mode/, "the primary action is one circular button instead of a segmented control");
  assert.doesNotMatch(newTask, /task-create-title|t\("task\.name"\)|<Input\b/, "the first prompt must create the task title instead of asking for a separate name");
  assert.doesNotMatch(newTask, /onCancel|new-task-cancel/, "navigation back to history replaces a modal-style cancel action");
  assert.match(app, /taskTitleFromPrompt\(taskForm\.prompt/, "demo mode keeps a deterministic title fallback");
  assert.match(app, /TaskRunBinding\.CreateTask\(\{[^}]*title:\s*""/, "desktop task creation lets the backend show a provisional title and refine it after the first run");
  assert.match(css, /\.new-task-layout\s*\{[^}]*max-width:\s*calc\(var\(--conversation-content-width\)/s, "the creation screen must share the chat column width");
  assert.match(css, /\.new-task-layout\s*\{[^}]*justify-content:\s*center/s, "the creation composer should sit in the central working area instead of hugging the window bottom");
  assert.doesNotMatch(workbench, /!newTaskOpen && !inspectorCollapsed/, "new-task mode must not hide the inspector controls");
  assert.doesNotMatch(css, /\.task-workbench\.new-task-active\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\)\s+0/s, "new-task mode must respect the saved inspector state");
  assert.match(app, /inspectorCollapsed && <button[^>]*inspector\.expand/, "a collapsed inspector needs an expand button while creating a task");
  assert.match(app, /inspectorDetached\s*\r?\n?\s*\? <button[^>]*inspector\.dock/, "a detached inspector offers to dock back rather than to expand into a panel that lives elsewhere");
});

test("resizing the inspector preserves the minimum usable conversation width", async () => {
  const workbench = await readFile(path.join(sourceRoot, "app", "components", "TaskWorkbench.jsx"), "utf8");
  const css = await readFile(path.join(sourceRoot, "index.css"), "utf8");

  assert.match(workbench, /const MIN_CONVERSATION_WIDTH = 620;/);
  assert.match(workbench, /Math\.min\(MIN_CONVERSATION_WIDTH, Math\.floor\(workbenchWidth \/ 2\)\)/, "compact two-pane layouts must reserve visible space for both panes");
  assert.match(workbench, /workbenchWidth - reservedConversationWidth/);
  assert.match(workbench, /new ResizeObserver\(keepConversationUsable\)/);
  assert.match(css, /\.task-workbench\.inspector-open\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\)\s+minmax\(0,\s*var\(--workbench-inspector-width,\s*380px\)\)/s);
  assert.match(css, /\.workbench-inspector-resize\s*\{[^}]*right:\s*calc\(var\(--workbench-inspector-width,\s*380px\)\s*-\s*18px\)/s, "the resize target must stay out of the conversation scroll surface");
  assert.match(css, /\.workbench-inspector-resize::after\s*\{[^}]*left:\s*0;[^}]*transform:\s*none;/s, "the visible divider stays on the pane boundary while its hit target moves into the inspector");
  assert.match(css, /\.new-task-toolbar\s*\{[^}]*flex-wrap:\s*nowrap[^}]*\}[\s\S]*?\.new-task-submit-action\s*\{[^}]*min-width:\s*32px/s);
  assert.doesNotMatch(workbench, /workbenchWidth - INSPECTOR_SNAP_DISTANCE/);
});

test("every new task chooses either an Agent or a workflow plus an explicit permission level", async () => {
  const app = await readFile(path.join(sourceRoot, "app", "App.jsx"), "utf8");
  const newTask = await readFile(path.join(sourceRoot, "app", "components", "NewTaskView.jsx"), "utf8");
  const executor = await readFile(path.join(sourceRoot, "app", "components", "TaskExecutorSelector.jsx"), "utf8");
  const permission = await readFile(path.join(sourceRoot, "app", "components", "TaskPermissionSelector.jsx"), "utf8");
  const runtimeMenu = await readFile(path.join(sourceRoot, "app", "components", "RuntimeProfileMenu.jsx"), "utf8");
  const i18n = await readFile(path.join(sourceRoot, "i18n.js"), "utf8");
  const css = await readFile(path.join(sourceRoot, "index.css"), "utf8");
  const harnesses = await readFile(path.join(sourceRoot, "app", "runtimeHarnesses.js"), "utf8");
  const workflowLibrary = await readFile(path.join(sourceRoot, "app", "components", "workflow", "WorkflowLibrary.jsx"), "utf8");
  assert.match(newTask, /<TaskExecutorSelector form=\{form\} workflows=\{workflows\} runtimes=\{runtimes\}/);
  assert.match(newTask, /<TaskPermissionSelector value=\{form\.sandbox\}/);
  assert.doesNotMatch(newTask, /<HarnessSelector/, "the Agent is selected by the mutually exclusive execution-target control");
  assert.match(newTask, /showRuntimeProfile && <RuntimeProfileMenu/, "model controls only apply to a directly selected Agent with configurable runtime options");
  assert.match(app, /SettingsBinding\.InspectHarnessConfiguration\(harness, runtimeSettings\)/, "Pi and Grok task profiles use the generic harness configuration probe");
  assert.match(app, /if \(!supportsRuntimeProfile\(harness\)\)/, "task profile probing follows catalog capabilities instead of a Codex/Claude allowlist");
  assert.match(runtimeMenu, /runtime-profile-submenu-heading/, "runtime submenus should identify the active setting");
  assert.match(runtimeMenu, /runtime-profile-submenu-option/, "runtime submenu choices use full-row selection styling");
  assert.match(runtimeMenu, /runtime-profile-submenu-check/, "runtime submenu choices place the selected check at the trailing edge");
  assert.match(runtimeMenu, /profile\.harness === "claude" \? <>/, "Claude uses its own compact model menu instead of Codex controls");
  assert.match(runtimeMenu, /className="runtime-profile-submenu-heading">\{t\("task\.models"\)\}/, "Claude's menu starts with a simple model list");
  assert.match(runtimeMenu, /claudeModels\.primary\.map/, "Claude aliases stay in the primary model list");
  assert.match(runtimeMenu, /className="claude-more-models-trigger">\{t\("task\.moreModels"\)\}/, "full Claude model ids move into the More models submenu");
  assert.match(runtimeMenu, /isDefault=\{\(model\.model \|\| model\.id\) === inheritedClaudeModel\}/, "Claude's inherited model gets the Default badge");
  assert.match(executor, /runtimeHarnessOptions\(runtimes/, "the execution target lists available coding Agents");
  assert.match(executor, /selectTaskExecutionTarget\(current, target\)/, "switching target must clear the mutually exclusive selection");
  assert.match(executor, /workflow\.id !== directAgentWorkflowID/, "the internal single-Agent definition must not appear as a user-facing workflow");
  assert.match(workflowLibrary, /workflows\.filter\(\(workflow\) => workflow\.id !== directAgentWorkflowID\)/, "the internal direct-Agent definition must not be editable or deletable in the workflow library");
  assert.match(executor, /value=\{`agent:\$\{option\.value\}`\}/);
  assert.match(executor, /value=\{`workflow:\$\{workflow\.id\}`\}/);
  assert.match(executor, /directAgent\s*\? selectedHarnessEnabled \? selectedHarness\.label : t\("task\.noHarnessEnabled"\)/, "a directly selected Agent shows its runtime name, or a notice when no harness is enabled");
  assert.doesNotMatch(executor, /t\("task\.agentLabel"/, "the execution target must not spend width on a redundant Agent prefix");
  assert.doesNotMatch(executor, /t\("task\.workflowTargetLabel"/, "the execution target must not spend width on a redundant workflow prefix");
  assert.match(permission, /value: "read-only"/);
  assert.match(permission, /value: "workspace-write"/);
  assert.match(permission, /value: "full"/);
  assert.match(permission, /disabled=\{option\.value === "full" && !allowFull\}/);
  assert.match(harnesses, /id: "codex", label: "Codex"/);
  assert.match(harnesses, /id: "claude", label: "Claude Code"/);
  assert.match(harnesses, /id: "modu", label: "modu_code"/);
  assert.doesNotMatch(runtimeMenu, /label=\{t\("task\.harness"\)\}/, "Harness belongs in the composer toolbar, not the model profile menu");
  assert.match(runtimeMenu, /const summary = profile\.harness === "claude" \? displayModelLabel : \[\s*displayModelLabel,\s*capability\.supportsReasoning[\s\S]*?\.join\(" "\)/, "Codex shows model and effort while Claude stays model-only");
  assert.doesNotMatch(runtimeMenu, /join\(" · "\)/, "the compact trigger should not read like a three-field status sentence");
  assert.doesNotMatch(runtimeMenu, /task\.advanced|runtime-profile-advanced/, "the runtime menu has no Advanced row");
  assert.match(i18n, /"settings\.speed\.priority": "快速"/, "the Codex priority service tier uses the compact Chinese Fast label");
  assert.match(i18n, /"settings\.speed\.priority\.description": "1\.5 倍速度，用量更多"/, "the Codex priority service tier does not leak an English backend description into Chinese UI");
  assert.match(css, /\.runtime-profile-submenu-option\[data-state="checked"\]\s*\{\s*background:\s*color-mix\(in oklab, var\(--foreground\) 5%, var\(--popover\)\);/, "selected runtime choices use the quiet neutral fill from the reference");
  assert.match(css, /\.claude-model-option > span:first-child\s*\{\s*display:\s*none\s*!important;/, "Claude uses one clean trailing check instead of overlapping Radix's leading indicator");
  assert.match(app, /runtimeSettings=\{settings\.runtimes\?\.\[taskForm\.harness\]\}/);
  assert.match(app, /workflowId: form\.workflowId/);
  assert.match(app, /sandbox: form\.sandbox \|\| "workspace-write"/);
  assert.match(app, /CreateTask\(\{[^}]*workflowId: execution\.workflowId[^}]*sandbox: execution\.sandbox/);
  assert.match(app, /const execution = selectedTaskExecution\(taskForm\)/);
  assert.match(app, /SettingsBinding\.InspectClaudeConfiguration/);
});

test("task editing joins the existing sidebar row hover actions", async () => {
  const app = await readFile(path.join(sourceRoot, "app", "App.jsx"), "utf8");
  const sidebar = await readFile(path.join(sourceRoot, "app", "components", "Sidebar.jsx"), "utf8");
  assert.doesNotMatch(app, /app-titlebar-task-actions|deleteSelectedTask/, "the titlebar must not carry task edit or delete actions");
  assert.match(app, /onRenameTask=\{openRenameTask\}/);
  assert.match(sidebar, /className="task-row-actions[^\"]*"/);
  assert.match(sidebar, /onClick=\{\(\) => onRenameTask\(task\)\}><Pencil/);
  assert.match(sidebar, /onClick=\{\(\) => onToggleTaskPinned\(task\)\}><Pin/);
  assert.match(sidebar, /onClick=\{\(\) => onDeleteTask\(task\)\}><Trash2/);
});

test("the transcript follows Codex's user, process, and answer rhythm", async () => {
  const css = await readFile(path.join(sourceRoot, "index.css"), "utf8");
  const timeline = await readFile(path.join(sourceRoot, "app", "components", "ConversationTimeline.jsx"), "utf8");
  assert.match(css, /\.conversation-user\s*\{[^}]*justify-self:\s*end/s, "user messages must sit on the right");
  assert.match(css, /\.conversation-user\s+\.conversation-bubble\s*\{[^}]*border-radius:/s, "user messages need their own chat bubble treatment");
  assert.doesNotMatch(timeline, /\b(?:Bot|UserRound)\b|conversation-message-avatar/, "Codex-style turns do not need chat avatars");
  assert.match(timeline, /groupRoundItems\(round\.items\)/, "provider event order must drive transcript blocks");
  assert.match(timeline, /<ProcessGroup entries=\{block\.items\}/, "adjacent tool work must collapse into a local process block");
  assert.match(timeline, /className=\{`conversation-agent-message \$\{entry\.tone\}`\}/, "assistant prose must render separately from process chrome");
  assert.match(timeline, /<FileChangeGroup entries=\{block\.items\}/, "file edits need a dedicated review card in event order");
  assert.match(timeline, /<MessageActions at=\{item\.at\} content=\{item\.text\} align="end"/, "user messages expose hover actions");
  assert.match(timeline, /<MessageActions at=\{entry\.at \|\| round\.finishedAt \|\| round\.startedAt\} content=\{entry\.text\}/, "assistant messages expose hover actions");
  assert.match(timeline, /<Copy aria-hidden="true"/, "message actions must include a copy control");
  assert.match(timeline, /<TooltipContent side="top" sideOffset=\{6\}>\{copyLabel\}<\/TooltipContent>/, "copy help belongs above the action instead of covering the next timeline block");
  assert.doesNotMatch(timeline, /title=\{copyLabel\}/, "the copy action must not use a browser-native tooltip below the button");
  assert.match(css, /\.conversation-user:hover \.conversation-message-actions[^}]*opacity:\s*1/s, "hovering a user turn must reveal its metadata actions");
  assert.match(css, /\.conversation-round-body\s*\{[^}]*gap:\s*26px/s, "the compact hover rail must fit without leaving a large blank band around process blocks");
});

test("completed conversations remain available for a follow-up turn", async () => {
  const app = await readFile(path.join(sourceRoot, "app", "App.jsx"), "utf8");
  const composer = await readFile(path.join(sourceRoot, "app", "components", "Composer.jsx"), "utf8");
  const harnessSelector = await readFile(path.join(sourceRoot, "app", "components", "HarnessSelector.jsx"), "utf8");
  assert.match(composer, /\["running", "paused", "completed"\]\.includes\(runStatus\)/);
  assert.match(composer, /runStatus === "completed"[\s\S]*t\("composer\.continue"\)/);
  assert.match(app, /run\.status === "paused" \|\| run\.status === "completed"/);
  assert.match(app, /TaskRunBinding\.ResumeRunConfigured\(run\.id, \{ instruction: content, \.\.\.runtimeProfile \}\)/);
  // Direct-agent conversations lock their harness; model/effort controls
  // remain editable between turns via RuntimeProfileMenu.
  const harnessProps = composer.match(/<HarnessSelector\b([^>]*?)\/>/)?.[1] || "";
  assert.match(harnessProps, /value=\{runtimeProfile\}/);
  assert.match(harnessProps, /runtimes=\{runtimes\}/);
  assert.match(harnessProps, /\breadOnly\s/);
  assert.match(harnessProps, /\bagentLabel\b/);
  assert.doesNotMatch(harnessProps, /\bonChange=/, "follow-up turns must not switch the conversation's harness");
  assert.match(composer, /RuntimeProfileMenu[^>]*onChange=\{onRuntimeProfileChange\}[^>]*configuration=\{runtimeConfiguration\?\.data\}/);
  assert.match(composer, /<TaskPermissionSelector value=\{permission\} readOnly/);
  assert.match(composer, /supportsRuntimeProfile\(runtimeProfile\.harness\)/, "runtimes without model controls must not repeat the Agent name");
  assert.match(composer, /readOnly=\{runStatus === "running"\}/, "runtime controls stay editable between completed or paused turns");
  assert.doesNotMatch(harnessSelector, /t\("task\.agentLabel"/, "the follow-up runtime selector must not spend width on a redundant Agent prefix");
  assert.match(composer, /<span>\{workflowLabel\}<\/span>/, "the follow-up workflow selector shows only the workflow name");
});

test("message hover timestamps include the date, and the transcript reads them from one place", async () => {
  const format = await readFile(path.join(sourceRoot, "app", "format.js"), "utf8");
  const timeline = await readFile(path.join(sourceRoot, "app", "components", "ConversationTimeline.jsx"), "utf8");
  assert.match(format, /export function formatMessageDateTime/);
  assert.doesNotMatch(timeline, /toLocaleTimeString/, "the transcript must not keep a second copy of the clock format");
  assert.match(timeline, /<time dateTime=\{at \|\| undefined\} title=\{formatDateTime\(at\)\}>\{formatMessageDateTime\(at\)\}<\/time>/);
});

test("long user messages collapse behind a measured disclosure", async () => {
  const css = await readFile(path.join(sourceRoot, "index.css"), "utf8");
  const timeline = await readFile(path.join(sourceRoot, "app", "components", "ConversationTimeline.jsx"), "utf8");
  assert.match(timeline, /content\.scrollHeight > content\.clientHeight \+ 1/, "overflow must follow rendered height instead of a brittle character count");
  assert.match(timeline, /aria-expanded=\{expanded\}/, "the disclosure must expose its state to assistive technology");
  assert.match(timeline, /t\(expanded \? "timeline\.showLess" : "timeline\.showMore"\)/, "the disclosure must support both expansion and collapse");
  assert.match(css, /\.conversation-user-message-content\.is-collapsed\s*\{[^}]*max-height:[^;]+;[^}]*overflow:\s*hidden/s);
  assert.match(css, /\.conversation-user-message-content\.is-collapsed\.has-overflow::after\s*\{[^}]*linear-gradient/s, "collapsed prose needs a quiet fade into its action");
  assert.match(css, /\.conversation-user-disclosure\s*\{[^}]*width:\s*fit-content[^}]*justify-content:\s*flex-start/s, "the disclosure belongs at the bubble's lower-left edge");
});

test("tool rows spend their width on the command, not on empty columns", async () => {
  const css = await readFile(path.join(sourceRoot, "index.css"), "utf8");
  const timeline = await readFile(path.join(sourceRoot, "app", "components", "ConversationTimeline.jsx"), "utf8");
  // The command owns the flexible track; only the transient state and caret
  // reserve space beside it. Timing metadata shares the command's flexible
  // column inline instead of adding a second row or a fixed-width track.
  assert.match(
    css,
    /\.conversation-tool-summary\s*\{[^}]*grid-template-columns:\s*20px\s+minmax\(0,\s*1fr\)\s+auto\s+15px/s,
    "tool activity rows must prioritize the readable summary",
  );
  assert.doesNotMatch(
    css,
    /\.conversation-tool-summary\s*\{[^}]*grid-template-columns:[^;]*--ui-event-state-width/s,
    "the state column must not go back to a fixed track",
  );
  // The tool row reads as a lightweight activity feed: icon, single-line
  // summary, transient state, and disclosure caret.
  assert.match(css, /\.conversation-tool-summary strong\s*\{[^}]*white-space:\s*nowrap/s);
  assert.match(css, /\.conversation-tool-summary strong\s*\{[^}]*text-overflow:\s*ellipsis/s);
  assert.match(timeline, /className="conversation-tool-meta"/);
  assert.match(timeline, /dateTime=\{entry\.at \|\| undefined\}/);
  assert.match(timeline, /formatToolTime\(entry\.at\)/, "tool rows show only a compact clock while the full stamp remains in the title");
  assert.match(timeline, /t\("timeline\.toolDuration", \{ duration \}\)/);
  assert.doesNotMatch(timeline, /<time className="sr-only">/, "tool timing must be visible instead of screen-reader-only");
  assert.match(css, /\.conversation-tool\s*\{[^}]*border:\s*0[^}]*background:\s*transparent/s, "tool events must not look like stacked cards");
  assert.match(css, /\.conversation-tool-icon\s*\{[^}]*background:\s*transparent/s, "tool icons stay unboxed like Codex activity rows");
  assert.match(css, /\.conversation-message-actions\s*\{[^}]*opacity:\s*0/s, "message metadata stays quiet until hover or focus");
  assert.match(css, /\.conversation-tool-heading\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\)\s+auto[^}]*gap:\s*12px/s, "tool time stays inline with enough separation from the summary");
  assert.match(css, /\.conversation-tool-body\s*\{[^}]*padding:\s*11px\s+12px\s+13px\s+40px/s, "expanded tool content must not crowd the summary metadata");
});

test("select options keep long labels and metadata from overlapping", async () => {
  const primitives = await readFile(path.join(sourceRoot, "ui", "primitives.jsx"), "utf8");
  // The option row is now a shadcn SelectItem, so this asserts the behaviour
  // rather than the old .ui-select__* class names: both halves truncate, the
  // metadata is width-capped so it cannot crowd out the label, and the full
  // text stays reachable through title. Radix wraps the row in a shrink-to-fit
  // ItemText span, so the row has to be stretched to the item width first —
  // otherwise the percentage cap resolves against the label's own width and
  // chops short values down to "l…".
  assert.match(primitives, /className="\[&>span:last-child\]:w-full \[&>span:last-child\]:min-w-0"/);
  assert.match(primitives, /className="min-w-0 flex-1 truncate" title=\{option\.label\}/);
  assert.match(primitives, /max-w-\[50%\][^"]*truncate[^>]*title=\{option\.meta\}/);
});

test("select tolerates the empty-string option value Radix rejects", async () => {
  const primitives = await readFile(path.join(sourceRoot, "ui", "primitives.jsx"), "utf8");
  // Several call sites use "" to mean "inherit the global default"; Radix
  // throws on an empty SelectItem value, so it has to be mapped both ways.
  assert.match(primitives, /const toRadix = .*EMPTY_VALUE/);
  assert.match(primitives, /const fromRadix = .*EMPTY_VALUE \? "" :/);
  assert.match(primitives, /onValueChange=\{\(next\) => onChange\(fromRadix\(next\)\)\}/);
});

test("sidebar orders global search, pinned tasks, projects, and nested tasks", async () => {
  const app = await readFile(path.join(sourceRoot, "app", "App.jsx"), "utf8");
  const sidebar = await readFile(path.join(sourceRoot, "app", "components", "Sidebar.jsx"), "utf8");
  const palette = await readFile(path.join(sourceRoot, "app", "components", "CommandPalette.jsx"), "utf8");
  const workbench = await readFile(path.join(sourceRoot, "app", "components", "TaskWorkbench.jsx"), "utf8");
  const searchIndex = sidebar.indexOf('className={`sidebar-search-trigger');
  const pinnedIndex = sidebar.indexOf('id="pinned-task-heading"');
  const projectIndex = sidebar.indexOf('id="project-heading"');
  assert.ok(searchIndex >= 0 && pinnedIndex >= 0 && projectIndex >= 0, "search and both section headings must exist");
  assert.ok(searchIndex < pinnedIndex, "global search must precede pinned tasks");
  assert.ok(pinnedIndex < projectIndex, "pinned tasks must precede projects");
  assert.match(sidebar, /pinnedTasks\.map\(renderPinnedTask\)/);
  assert.doesNotMatch(sidebar, /pinned-project-heading/);
  const addProjectIndex = sidebar.indexOf('className="add-workspace ');
  assert.ok(addProjectIndex > searchIndex && addProjectIndex < pinnedIndex, "add project belongs beside search in the rail header");
  assert.match(sidebar, /className="project-task-panel[^"]*"/);
  assert.doesNotMatch(sidebar, /className="cwd-shell"|sidebar-search-popover/, "search must use the global palette instead of a sidebar card");
  assert.match(palette, /createPortal\(<div className="command-palette-backdrop"/);
  assert.match(palette, /role="dialog" aria-modal="true"/);
  assert.match(palette, /moveCommandPaletteIndex|commandPaletteShortcutIndex/);
  assert.match(app, /TaskRunBinding\.SearchTasks\(\{ keyword: globalSearchQuery\.trim\(\), limit: 50 \}\)/, "global search must query tasks across workspaces through the backend");
  assert.match(palette, /taskResults\.slice\(0, 9\)/);
  const appCss = await readFile(path.join(sourceRoot, "index.css"), "utf8");
  assert.match(appCss, /\.command-palette-backdrop\s*\{[^}]*position:\s*fixed[^}]*inset:\s*0[^}]*\}/s);
  assert.doesNotMatch(workbench, /task-history-pane/, "task history belongs below its project in the sidebar");
  // The rail's scroll containment moved from mirage.css onto the markup.
  assert.match(sidebar, /className="workspace-block[^"]*flex-1[^"]*flex-col/, "the project block must absorb the leftover rail height");
  assert.match(sidebar, /className="project-sections[^"]*overflow-y-auto[^"]*"/, "only the project list scrolls, not the whole rail");
});

test("command palette keeps its row anatomy and icon treatment", async () => {
  const sidebar = await readFile(path.join(sourceRoot, "app", "components", "Sidebar.jsx"), "utf8");
  const palette = await readFile(path.join(sourceRoot, "app", "components", "CommandPalette.jsx"), "utf8");
  // The palette's rules moved to @layer app in index.css; mirage.css no longer
  // owns them, so the CSS assertions follow them there.
  const css = await readFile(path.join(sourceRoot, "index.css"), "utf8");
  assert.match(sidebar, /<Search size=\{15\} strokeWidth=\{2\} aria-hidden="true"/, "the compact sidebar search icon stays smaller than the palette's input icon");
  assert.match(palette, /className="command-palette__icon"[^>]*><Icon size=\{15\} strokeWidth=/);
  assert.match(palette, /<Search size=\{16\}/);
  assert.match(palette, /setActiveIndex\(0\);\s*\}, \[normalizedQuery, open\]\);/s, "filter changes must select the first matching result");
  // Four-column rows: icon, copy, meta, shortcut — the shape the keyboard
  // navigation and the shortcut hints both depend on.
  assert.match(css, /\.command-palette__item\s*\{[^}]*grid-template-columns:\s*18px[^}]*background:\s*transparent[^}]*\}/s);
  assert.match(css, /\.command-palette__icon\s*\{[^}]*color:\s*var\(--muted-foreground\)[^}]*\}/s);
  assert.doesNotMatch(css, /\.command-palette__icon\s*\{[^}]*border-right:/s, "palette icons must not sit in boxed grid cells");
  assert.match(css, /\.command-palette kbd\s*\{[^}]*border:\s*0[^}]*background:\s*transparent[^}]*\}/s);
});
