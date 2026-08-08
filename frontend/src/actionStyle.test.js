import assert from "node:assert/strict";
import { readdir, readFile } from "node:fs/promises";
import test from "node:test";
import { fileURLToPath } from "node:url";
import path from "node:path";

const sourceRoot = path.dirname(fileURLToPath(import.meta.url));

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
  assert.match(sidebar, /<DropdownMenuTrigger asChild>[\s\S]{0,240}?className="workspace-menu-trigger"/, "the ellipsis button must be the menu trigger");
  assert.match(sidebar, /<DropdownMenuItem onSelect=\{\(\) => onTogglePinned\(workspace\)\}/);
  assert.match(sidebar, /<DropdownMenuItem variant="destructive" onSelect=\{\(\) => onRemoveWorkspace\(workspace\)\}/, "remove must read as destructive");
  assert.match(sidebar, /className="workspace-new-task"[^>]*sidebar\.newTaskInProject/);
  assert.match(sidebar, /className="add-workspace"[^>]*>\{t\("sidebar\.addProject"\)\}/);
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
  assert.match(app, /className="app-shell grid min-h-0 grid-cols-\[auto_minmax\(0,1fr\)\]"/, "the rail keeps its user-selected width; only the main column flexes");
  assert.doesNotMatch(app, /className="app-shell[^"]*grid-cols-\[\d+px/, "the shell must not pin the rail to a fixed width");
});

test("workflow and settings share one expandable footer menu", async () => {
  const sidebar = await readFile(path.join(sourceRoot, "app", "components", "Sidebar.jsx"), "utf8");
  assert.match(sidebar, /<DropdownMenuTrigger asChild>[\s\S]{0,200}?className={`secondary-navigation-trigger/);
  assert.match(sidebar, /<DropdownMenuItem[^>]*onSelect=\{\(\) => goToSecondaryView\("workflows"\)\}/);
  assert.match(sidebar, /<DropdownMenuItem[^>]*onSelect=\{\(\) => goToSecondaryView\("settings"\)\}/);
  // The footer sits at the bottom of the rail, so the menu has to open upward
  // and span the rail rather than hugging the trigger's own width.
  assert.match(sidebar, /<DropdownMenuContent side="top"[^>]*w-\[var\(--radix-dropdown-menu-trigger-width\)\]/s, "the menu must open upward at rail width");
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
  assert.match(settings, /role="radio" aria-checked=\{appearance\.accent === accent\}/);
  // The accent options carry their name; the swatch only supplements it, so the
  // control never degrades into unlabelled colour dots.
  assert.match(settings, /\{t\(`settings\.themeColor\.\$\{accent\}`\)\}/);
  assert.match(settings, /ACCENT_SWATCH\[accent\]/);
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

test("tool rows spend their width on the command, not on empty columns", async () => {
  const css = await readFile(path.join(sourceRoot, "index.css"), "utf8");
  // `time` is empty on most rows (same-second events de-duplicate to a blank
  // label) and `state` is one short word, so fixed tracks for them reserved
  // ~90px per row while the command beside them was cut off.
  assert.match(
    css,
    /\.conversation-tool-summary\s*\{[^}]*grid-template-columns:[^;]*minmax\(0,\s*1fr\)\s+auto\s+auto/s,
    "the state and time columns must size to their content",
  );
  assert.doesNotMatch(
    css,
    /\.conversation-tool-summary\s*\{[^}]*grid-template-columns:[^;]*--ui-event-state-width/s,
    "the state column must not go back to a fixed track",
  );
  // Commands have no spaces to break on, so a single ellipsised line hid most
  // of them with no way to reveal the rest.
  assert.match(css, /\.conversation-tool-summary strong\s*\{[^}]*white-space:\s*normal/s);
  assert.match(css, /\.conversation-tool-summary strong\s*\{[^}]*line-clamp:\s*2/s);
  assert.match(css, /\.conversation-tool-summary strong\s*\{[^}]*overflow-wrap:\s*anywhere/s);
  // The message timestamp has the same empty-label problem.
  assert.match(css, /\.conversation-message-meta time:not\(:empty\)\s*\{[^}]*min-width:\s*var\(--ui-event-time-width\)/s);
});

test("select options keep long labels and metadata from overlapping", async () => {
  const primitives = await readFile(path.join(sourceRoot, "ui", "primitives.jsx"), "utf8");
  // The option row is now a shadcn SelectItem, so this asserts the behaviour
  // rather than the old .ui-select__* class names: both halves truncate, the
  // metadata is width-capped so it cannot crowd out the label, and the full
  // text stays reachable through title.
  assert.match(primitives, /className="truncate" title=\{option\.label\}/);
  assert.match(primitives, /max-w-\[42%\][^"]*truncate[^>]*title=\{option\.meta\}/);
});

test("select tolerates the empty-string option value Radix rejects", async () => {
  const primitives = await readFile(path.join(sourceRoot, "ui", "primitives.jsx"), "utf8");
  // Several call sites use "" to mean "inherit the global default"; Radix
  // throws on an empty SelectItem value, so it has to be mapped both ways.
  assert.match(primitives, /const toRadix = .*EMPTY_VALUE/);
  assert.match(primitives, /const fromRadix = .*EMPTY_VALUE \? "" :/);
  assert.match(primitives, /onValueChange=\{\(next\) => onChange\(fromRadix\(next\)\)\}/);
});

test("sidebar orders global search, pinned projects, projects, and nested tasks", async () => {
  const app = await readFile(path.join(sourceRoot, "app", "App.jsx"), "utf8");
  const sidebar = await readFile(path.join(sourceRoot, "app", "components", "Sidebar.jsx"), "utf8");
  const palette = await readFile(path.join(sourceRoot, "app", "components", "CommandPalette.jsx"), "utf8");
  const workbench = await readFile(path.join(sourceRoot, "app", "components", "TaskWorkbench.jsx"), "utf8");
  const css = await readFile(path.join(sourceRoot, "mirage.css"), "utf8");
  assert.ok(sidebar.indexOf('className={`sidebar-search-trigger') < sidebar.indexOf('id="pinned-project-heading"'), "global search must precede pinned projects");
  assert.ok(sidebar.indexOf('id="pinned-project-heading"') < sidebar.indexOf('id="project-heading"'), "pinned projects must precede projects");
  assert.match(sidebar, /id="project-heading"[^>]*>[\s\S]*?className="add-workspace"/);
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
  assert.match(sidebar, /<Search size=\{16\}/);
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
