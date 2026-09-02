import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

test("software updates expose the signed check-download-restart flow", async () => {
  const source = await readFile(new URL("./appUpdate.js", import.meta.url), "utf8");
  assert.match(source, /UpdateBinding\.Check\(\)/);
  assert.match(source, /UpdateBinding\.Download\(\)/);
  assert.match(source, /UpdateBinding\.Apply\(\)/);
  assert.match(source, /wails:updater:download-progress/);
  assert.match(source, /const \[status, setStatus\] = useState\(null\)/);
  assert.doesNotMatch(source, /demo(?:Available)?Status|pause\(|0\.1\.5/, "release validation must not be contaminated by simulated updater states");
  const settings = await readFile(new URL("./SettingsPage.jsx", import.meta.url), "utf8");
  assert.match(settings, /useAppUpdate\(mode\)/);
  assert.match(settings, /automaticSupported/);
});

test("the sidebar owns the glanceable update and progress control", async () => {
  const sidebar = await readFile(new URL("./components/Sidebar.jsx", import.meta.url), "utf8");
  const control = await readFile(new URL("./components/SidebarUpdateButton.jsx", import.meta.url), "utf8");
  assert.match(sidebar, /<SidebarUpdateButton mode=\{mode\} notify=\{notify\} \/>/);
  assert.match(control, /data-update-state=\{state\}/);
  assert.match(control, /if \(!visible\) return null;/, "the sidebar control stays hidden until an update needs attention");
  assert.doesNotMatch(control, /RefreshCw/, "the sidebar must not expose a permanent manual-check button");
  assert.match(control, /<ProgressRing ratio=\{percent \/ 100\}/);
  assert.match(control, /available \? <Download/);
  assert.match(control, /data-codex-download=\{available \|\| undefined\}/, "an available release must switch the footer control to the Codex-style download action");
  assert.match(control, /available \? "text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground active:scale-95"/, "the download action should match the footer's quiet icon controls");
  assert.doesNotMatch(control, /available && <i/, "the download action must not rely on a tiny notification dot");
  assert.match(control, /state === "ready" \? <RotateCcw/);
  assert.match(control, /state === "ready" \? <RotateCcw size=\{14\}/, "the restart glyph stays visually quieter than the shared hit target");
  assert.match(control, /ready \? <span className="grid size-6 place-items-center rounded-\[6px\] bg-primary shadow-xs">/, "the ready background stays compact inside the shared footer hit target");
  assert.match(control, /sidebar-update-trigger[^`]*size-9[^`]*bg-transparent/, "ready and download actions share the footer's transparent outer surface");
  assert.match(control, /role="status" aria-live="polite"/);
  assert.match(control, /sidebar-update-control[^\"]*relative grid size-9/, "the updater must occupy its own footer grid cell instead of overlaying the menu");
  assert.match(control, /sidebar-update-trigger[^`]*size-9[^`]*rounded-lg/, "the updater needs its own button surface");
  assert.doesNotMatch(control, /sidebar-update-control[^\"]*absolute/, "the updater must never cover the menu trigger");
});

test("the sidebar update control appears only for a known update lifecycle", async () => {
  const { shouldShowSidebarUpdate } = await import("./appUpdate.js");
  assert.equal(shouldShowSidebarUpdate(null), false);
  assert.equal(shouldShowSidebarUpdate({ state: "unconfigured" }), false);
  assert.equal(shouldShowSidebarUpdate({ state: "checking" }), false);
  assert.equal(shouldShowSidebarUpdate({ state: "up-to-date" }), false);
  assert.equal(shouldShowSidebarUpdate({ state: "error" }), false);
  assert.equal(shouldShowSidebarUpdate({ state: "available", availableVersion: "1.2.3" }), true);
  assert.equal(shouldShowSidebarUpdate({ state: "downloading", availableVersion: "1.2.3" }), true);
  assert.equal(shouldShowSidebarUpdate({ state: "ready", availableVersion: "1.2.3" }), true);
  assert.equal(shouldShowSidebarUpdate({ state: "error", availableVersion: "1.2.3" }), true);
});

test("the settings page keeps software updates to one compact status row", async () => {
  const settings = await readFile(new URL("./SettingsPage.jsx", import.meta.url), "utf8");
  assert.match(settings, /app-update-settings rounded-md[^\"]*bg-muted\/25[^\"]*px-2\.5 py-2/);
  assert.match(settings, /state === "ready" \? <SettingsButton compact className="rounded-md"/, "the restart action must use the same compact control scale as the row");
  assert.match(settings, /OneCatch \{status\?\.currentVersion \|\| "—"\} · \{stateLabel\}/);
  assert.doesNotMatch(settings, /t\("settings\.appUpdateDescription"\)/, "the compact update row must not repeat an introductory description");
  assert.doesNotMatch(settings, /t\("settings\.updateSecurityNote"\)/, "the compact update row must not keep a permanent security paragraph");
  assert.match(settings, /role="progressbar"/, "download progress remains available only when it is useful");
});

test("download progress is clamped to a complete circular reading", async () => {
  const { appUpdatePercent } = await import("./appUpdate.js");
  assert.equal(appUpdatePercent({ written: 25, total: 100 }), 25);
  assert.equal(appUpdatePercent({ written: 150, total: 100 }), 100);
  assert.equal(appUpdatePercent({ written: -3, total: 100 }), 0);
  assert.equal(appUpdatePercent({ written: 4, total: 0 }), 0);
});
