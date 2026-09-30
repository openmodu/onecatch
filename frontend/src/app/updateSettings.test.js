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
  assert.match(settings, /openAppUpdateWindow\(mode\)/);
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
  assert.match(control, /await openAppUpdateWindow\(mode\)/, "the footer opens the dedicated window");
  assert.doesNotMatch(control, /await (?:download|apply)\(/, "the reminder cannot download or restart the application itself");
  assert.doesNotMatch(control, /disabled=/, "progress controls stay clickable so the update window can be reopened");
  assert.match(control, /available \? "text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground active:scale-95"/, "the download action should match the footer's quiet icon controls");
  assert.doesNotMatch(control, /available && <i/, "the download action must not rely on a tiny notification dot");
  assert.match(control, /state === "ready" \? <RotateCcw/);
  assert.match(control, /state === "ready" \? <RotateCcw size=\{14\}/, "the restart glyph stays visually quieter than the shared hit target");
  assert.match(control, /ready \? <span className="grid size-6 place-items-center rounded-\[6px\] bg-primary shadow-xs">/, "the ready background stays compact inside the shared footer hit target");
  assert.match(control, /sidebar-update-trigger[^`]*size-9[^`]*bg-transparent/, "ready and download actions share the footer's transparent outer surface");
  assert.match(control, /role="status" aria-live="polite"/);
  assert.match(control, /sidebar-update-control[^\"]*relative grid size-9/, "the updater must occupy its own footer grid cell instead of overlaying the menu");
  assert.match(control, /sidebar-update-control[^\"]*size-9 shrink-0/, "the update hit target must not shrink when the sidebar is narrow");
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
  assert.match(settings, /openAppUpdateWindow\(mode\)/);
  assert.match(settings, /OneCatch \{status\?\.currentVersion \|\| "—"\} · \{appUpdateStateLabel\(status, t\)\}/);
  assert.doesNotMatch(settings, /t\("settings\.appUpdateDescription"\)/, "the compact update row must not repeat an introductory description");
  assert.doesNotMatch(settings, /t\("settings\.updateSecurityNote"\)/, "the compact update row must not keep a permanent security paragraph");
  const window = await readFile(new URL("./AppUpdateWindow.jsx", import.meta.url), "utf8");
  assert.match(window, /role="progressbar"/, "download progress belongs in the independent window");
  assert.doesNotMatch(settings, /run\((?:download|apply)/, "settings delegates download and restart to the dedicated window");
});

test("download progress is clamped to a complete circular reading", async () => {
  const { appUpdatePercent } = await import("./appUpdate.js");
  assert.equal(appUpdatePercent({ written: 25, total: 100 }), 25);
  assert.equal(appUpdatePercent({ written: 150, total: 100 }), 100);
  assert.equal(appUpdatePercent({ written: -3, total: 100 }), 0);
  assert.equal(appUpdatePercent({ written: 4, total: 0 }), 0);
});


test("opening the update window checks only when no release operation needs preserving", async () => {
  const { shouldCheckAppUpdateOnOpen } = await import("./appUpdate.js");
  const configured = { verificationEnabled: true, automaticSupported: true };
  for (const state of ["idle", "up-to-date", "error"]) {
    assert.equal(shouldCheckAppUpdateOnOpen({ ...configured, state }), true, state);
  }
  for (const state of ["available", "downloading", "verifying", "installing", "ready", "checking", "unconfigured"]) {
    assert.equal(shouldCheckAppUpdateOnOpen({ ...configured, state, availableVersion: "1.2.3" }), false, state);
  }
  assert.equal(shouldCheckAppUpdateOnOpen({ ...configured, state: "error", availableVersion: "1.2.3" }), false);
  assert.equal(shouldCheckAppUpdateOnOpen(null), false);
});

test("update actions respect busy states, failed downloads, and install support", async () => {
  const { appUpdateAction } = await import("./appUpdate.js");
  const release = { verificationEnabled: true, automaticSupported: true, availableVersion: "1.2.3" };
  assert.equal(appUpdateAction({ ...release, state: "available" }), "download");
  assert.equal(appUpdateAction({ ...release, state: "error" }), "download");
  assert.equal(appUpdateAction({ ...release, state: "ready" }), "apply");
  assert.equal(appUpdateAction({ ...release, state: "ready", automaticSupported: false }), null);
  assert.equal(appUpdateAction({ ...release, state: "available" }, true), null);
  assert.equal(appUpdateAction({ ...release, state: "available", verificationEnabled: false }), null);
  for (const state of ["checking", "downloading", "verifying", "installing", "unconfigured"]) {
    assert.equal(appUpdateAction({ ...release, state }), null, state);
  }
});

test("desktop update entry points use a dedicated window and reconcile on reopen", async () => {
  const main = await readFile(new URL("../main.jsx", import.meta.url), "utf8");
  const desktop = await readFile(new URL("../../../internal/app/desktop/desktop.go", import.meta.url), "utf8");
  const window = await readFile(new URL("./AppUpdateWindow.jsx", import.meta.url), "utf8");
  assert.match(main, /if \(windowKind === "updates"\)\s*\{\s*return import\("\.\/app\/AppUpdateWindow\.jsx"\)/);
  const menuActions = [...desktop.matchAll(/Add\("检查更新…"\)\.OnClick\(func\(\*application\.Context\) \{([^}]+)\}/g)];
  assert.equal(menuActions.length, 2);
  for (const [, action] of menuActions) {
    assert.match(action, /auxiliaryWindows\.OpenUpdates\(\)/);
    assert.doesNotMatch(action, /OpenSettings|updateService\.Check/);
  }
  assert.match(window, /Events\.On\(auxiliaryWindowShownEvent/);
  assert.match(window, /event\?\.data\?\.name === "updates"\) \{ closed\.current = false; void checkOnOpen\(\); \}/);
});

test("update dialogs resize to compact progress and result layouts", async () => {
  const { appUpdateWindowSize } = await import("./appUpdate.js");
  for (const state of ["checking", "downloading", "verifying", "installing"]) {
    assert.deepEqual(appUpdateWindowSize(state), { width: 400, height: 146 });
  }
  assert.deepEqual(appUpdateWindowSize("up-to-date"), { width: 260, height: 220 });
  assert.deepEqual(appUpdateWindowSize("available"), { width: 320, height: 260 });
  assert.deepEqual(appUpdateWindowSize("available", true), { width: 380, height: 340 });
  assert.deepEqual(appUpdateWindowSize("checking", true), { width: 400, height: 146 }, "transfers collapse the optional release notes");
});
