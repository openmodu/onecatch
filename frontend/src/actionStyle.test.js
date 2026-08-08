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
  assert.match(sidebar, /className="workspace-row-actions[^"]*\bhidden\b[^"]*"/, "project actions must stay hidden until the row is engaged");
  assert.match(sidebar, /className="workspace-row-actions[^"]*group-hover:flex[^"]*"/);
  assert.match(sidebar, /className="workspace-row-actions[^"]*group-focus-within:flex[^"]*"/, "keyboard users must be able to reach the project actions");
  assert.match(sidebar, /className="workspace-context-menu[^"]*"[^>]*role="menu"/);
  assert.match(sidebar, /className="workspace-menu-trigger"[^>]*aria-haspopup="menu"/);
  assert.match(sidebar, /className="workspace-new-task"[^>]*sidebar\.newTaskInProject/);
  assert.match(sidebar, /className="add-workspace"[^>]*>\{t\("sidebar\.addProject"\)\}/);
  assert.match(sidebar, /<Folder(?:Open)?\b/);
  assert.doesNotMatch(sidebar, /<StatusPill\b|project-task-time|project-task-heading/);
  assert.doesNotMatch(sidebar, /<Info\b/, "selected tasks must use a marker instead of an info icon");
  assert.match(sidebar, /className="project-task-marker[^"]*"[^>]*>\{selected \? "\*" : ""\}/);
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
  assert.match(sidebar, /className="secondary-navigation-menu[^"]*"[^>]*role="menu"/);
  assert.match(sidebar, /className={`secondary-navigation-trigger[^>]*aria-haspopup="menu"[^>]*aria-expanded=/);
  assert.match(sidebar, /role="menuitem"[^>]*onClick=\{\(\) => goToSecondaryView\("workflows"\)\}/);
  assert.match(sidebar, /role="menuitem"[^>]*onClick=\{\(\) => goToSecondaryView\("settings"\)\}/);
  assert.match(sidebar, /className="primary-nav[^"]*\brelative\b/, "the popover anchors to the nav");
  assert.match(sidebar, /secondary-navigation-menu[^"]*absolute[^"]*bottom-\[calc\(100%\+6px\)\]/, "the menu must open upward, clear of the trigger");
  assert.doesNotMatch(sidebar, /<(?:List|GitBranch|Settings)\b/, "footer navigation must stay text-first");
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
  assert.doesNotMatch(settings, /from "lucide-react"/, "appearance settings must not import decorative icons");
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
