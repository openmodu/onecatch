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

test("every action tone has a visible resting background", async () => {
  const css = await readFile(path.join(sourceRoot, "ui", "tokens.css"), "utf8");
  for (const selector of [".ui-action", ".ui-action--primary", ".ui-action--cyan", ".ui-action--danger", ".ui-action--muted", ".ui-toggle-row__state"]) {
    const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const rule = css.match(new RegExp(`${escaped}\\s*\\{([^}]*)\\}`));
    assert.ok(rule, `${selector} rule is missing`);
    assert.match(rule[1], /background\s*:/, `${selector} must have a visible resting background`);
  }
});

test("workspace actions use a compact project menu and tasks stay visually light", async () => {
  const sidebar = await readFile(path.join(sourceRoot, "app", "components", "Sidebar.jsx"), "utf8");
  const css = await readFile(path.join(sourceRoot, "mirage.css"), "utf8");
  assert.match(sidebar, /className="workspace-row-actions"/);
  assert.match(sidebar, /className="workspace-context-menu"[^>]*role="menu"/);
  assert.match(sidebar, /className="workspace-menu-trigger"[^>]*aria-haspopup="menu"/);
  assert.match(sidebar, /className="workspace-new-task"[^>]*sidebar\.newTaskInProject/);
  assert.match(sidebar, /className="add-workspace"[^>]*>\{t\("sidebar\.addProject"\)\}/);
  assert.match(sidebar, /<Folder(?:Open|Simple)\b/);
  assert.doesNotMatch(sidebar, /<StatusPill\b|project-task-time|project-task-heading/);
  assert.doesNotMatch(sidebar, /<Info\b/, "selected tasks must use a TUI marker instead of an info icon");
  assert.match(sidebar, /className="project-task-marker"[^>]*>\{selected \? "\*" : ""\}/);
  assert.match(sidebar, /aria-current=\{selected \? "page" : undefined\}/);
  assert.match(css, /\.workspace-row-actions\s*\{[^}]*display:\s*none[^}]*\}/s);
  assert.match(css, /\.workspace-row\.active \.workspace-row-actions[^}]*\{\s*display:\s*flex;/s);
  assert.match(css, /\.workspace-context-menu\s*\{[^}]*position:\s*fixed[^}]*width:\s*176px[^}]*\}/s);
  assert.match(css, /\.project-task-item\s*\{[^}]*background:\s*transparent[^}]*\}/s);
  assert.match(css, /\.workspace-row\.active\s*\{[^}]*background:\s*transparent[^}]*box-shadow:\s*none[^}]*\}/s, "active project containers must not tint the whole task list");
  assert.match(css, /\.workspace-row\.active > \.workspace-item\s*\{[^}]*background:/s, "active project background must stay on the project row");
  assert.match(css, /\.project-task-item\.selected\s*\{[^}]*box-shadow:\s*none[^}]*\}/s, "selected tasks must not use a leading accent rail");
  assert.match(css, /\.project-task-panel\s*\{[^}]*margin:\s*1px 4px 7px 18px[^}]*\}/s, "nested tasks must keep a compact TUI indent");
});

test("sidebar reserves its footer for navigation and exposes a resize separator", async () => {
  const sidebar = await readFile(path.join(sourceRoot, "app", "components", "Sidebar.jsx"), "utf8");
  const css = await readFile(path.join(sourceRoot, "mirage.css"), "utf8");
  assert.doesNotMatch(sidebar, /runtime-panel|runtime-row/, "runtime status belongs on the settings page, not in the sidebar");
  assert.match(sidebar, /className="sidebar-resizer" role="separator"/);
  assert.match(css, /\.primary-nav\s*\{[^}]*margin-top:\s*auto[^}]*\}/s);
  assert.match(css, /\.app-shell\s*\{[^}]*grid-template-columns:\s*auto minmax\(0, 1fr\)[^}]*\}/s);
  assert.doesNotMatch(css, /\.app-shell\s*\{[^}]*grid-template-columns:\s*\d+px/s, "responsive rules must not override the user-selected width");
});

test("workflow and settings share one expandable footer menu", async () => {
  const sidebar = await readFile(path.join(sourceRoot, "app", "components", "Sidebar.jsx"), "utf8");
  const css = await readFile(path.join(sourceRoot, "mirage.css"), "utf8");
  assert.match(sidebar, /className="secondary-navigation-menu"[^>]*role="menu"/);
  assert.match(sidebar, /className={`secondary-navigation-trigger[^>]*aria-haspopup="menu"[^>]*aria-expanded=/);
  assert.match(sidebar, /role="menuitem"[^>]*onClick=\{\(\) => goToSecondaryView\("workflows"\)\}/);
  assert.match(sidebar, /role="menuitem"[^>]*onClick=\{\(\) => goToSecondaryView\("settings"\)\}/);
  assert.match(css, /\.primary-nav\s*\{[^}]*position:\s*relative[^}]*\}/s);
  assert.match(css, /\.primary-nav \.secondary-navigation-trigger\s*\{[^}]*border-radius:\s*0[^}]*background:\s*transparent[^}]*\}/s, "the footer menu must remain an edge-aligned TUI row");
  assert.doesNotMatch(sidebar, /<(?:List|GitBranch|GearSix)\b/, "footer navigation must stay text-first");
  assert.match(css, /\.secondary-navigation-menu\s*\{[^}]*position:\s*absolute[^}]*bottom:\s*calc\(100% \+ 6px\)[^}]*\}/s);
});

test("appearance controls use text-first TUI geometry", async () => {
  const settings = await readFile(path.join(sourceRoot, "app", "SettingsPage.jsx"), "utf8");
  const css = await readFile(path.join(sourceRoot, "mirage.css"), "utf8");
  assert.doesNotMatch(settings, /from "@phosphor-icons\/react"/, "appearance settings must not import decorative icons");
  assert.match(settings, /className="appearance-mode-picker"[^>]*role="radiogroup"/);
  assert.match(settings, /className="appearance-accent-picker"[^>]*role="radiogroup"/);
  assert.match(css, /\.appearance-accent\s*\{[^}]*border-radius:\s*0[^}]*\}/s);
  assert.doesNotMatch(css, /\.appearance-accent\s*\{[^}]*border-radius:\s*50%/s);
});

test("select options keep long labels and metadata from overlapping", async () => {
  const primitives = await readFile(path.join(sourceRoot, "ui", "primitives.jsx"), "utf8");
  const css = await readFile(path.join(sourceRoot, "ui", "tokens.css"), "utf8");
  assert.match(primitives, /className="ui-select__label"[^>]*title=\{option\.label\}/);
  assert.match(primitives, /<small title=\{option\.meta\}>\{option\.meta\}<\/small>/);
  assert.match(css, /\.ui-select__option\s*\{[^}]*display:\s*flex[^}]*\}/s);
  assert.match(css, /\.ui-select__label\s*\{[^}]*min-width:\s*0[^}]*overflow:\s*hidden[^}]*text-overflow:\s*ellipsis[^}]*white-space:\s*nowrap[^}]*\}/s);
  assert.match(css, /\.ui-select__option small\s*\{[^}]*max-width:\s*42%[^}]*overflow:\s*hidden[^}]*text-overflow:\s*ellipsis[^}]*white-space:\s*nowrap[^}]*\}/s);
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
  assert.match(sidebar, /className="project-task-panel"/);
  assert.doesNotMatch(sidebar, /className="cwd-shell"|sidebar-search-popover/, "search must use the global palette instead of a sidebar card");
  assert.match(palette, /createPortal\(<div className="command-palette-backdrop"/);
  assert.match(palette, /role="dialog" aria-modal="true"/);
  assert.match(palette, /moveCommandPaletteIndex|commandPaletteShortcutIndex/);
  assert.match(app, /TaskRunBinding\.SearchTasks\(\{ keyword: globalSearchQuery\.trim\(\), limit: 50 \}\)/, "global search must query tasks across workspaces through the backend");
  assert.match(palette, /taskResults\.slice\(0, 9\)/);
  assert.match(css, /\.command-palette-backdrop\s*\{[^}]*position:\s*fixed[^}]*inset:\s*0[^}]*\}/s);
  assert.doesNotMatch(workbench, /task-history-pane/, "task history belongs below its project in the sidebar");
  assert.match(css, /\.workspace-block\s*\{[^}]*flex:\s*1[^}]*display:\s*flex[^}]*flex-direction:\s*column[^}]*\}/s);
  assert.match(css, /\.project-sections\s*\{[^}]*flex:\s*1[^}]*overflow-y:\s*auto[^}]*\}/s);
});

test("command palette keeps the product TUI geometry and icon treatment", async () => {
  const sidebar = await readFile(path.join(sourceRoot, "app", "components", "Sidebar.jsx"), "utf8");
  const palette = await readFile(path.join(sourceRoot, "app", "components", "CommandPalette.jsx"), "utf8");
  const css = await readFile(path.join(sourceRoot, "mirage.css"), "utf8");
  assert.match(sidebar, /<MagnifyingGlass size=\{16\} weight="regular"/);
  assert.match(palette, /className="command-palette__icon"[^>]*><Icon size=\{15\} weight=/);
  assert.match(palette, /<MagnifyingGlass size=\{16\} weight="regular"/);
  assert.match(palette, /setActiveIndex\(0\);\s*\}, \[normalizedQuery, open\]\);/s, "filter changes must select the first matching result");
  assert.match(css, /\.command-palette\s*\{[^}]*border-radius:\s*0[^}]*background:\s*var\(--acp-panel\)[^}]*\}/s);
  assert.match(css, /\.command-palette__item\s*\{[^}]*grid-template-columns:\s*18px[^}]*border-radius:\s*0[^}]*background:\s*transparent[^}]*\}/s);
  assert.match(css, /\.command-palette__icon\s*\{[^}]*color:\s*var\(--acp-text-soft\)[^}]*\}/s);
  assert.doesNotMatch(css, /\.command-palette__icon\s*\{[^}]*border-right:/s, "palette icons must not sit in boxed grid cells");
  assert.match(css, /\.command-palette kbd\s*\{[^}]*border:\s*0[^}]*background:\s*transparent[^}]*\}/s);
});
