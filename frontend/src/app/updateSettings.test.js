import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

test("software updates expose the signed check-download-restart flow", async () => {
  const source = await readFile(new URL("./appUpdate.js", import.meta.url), "utf8");
  assert.match(source, /UpdateBinding\.Check\(\)/);
  assert.match(source, /UpdateBinding\.Download\(\)/);
  assert.match(source, /UpdateBinding\.Apply\(\)/);
  assert.match(source, /wails:updater:download-progress/);
  const settings = await readFile(new URL("./SettingsPage.jsx", import.meta.url), "utf8");
  assert.match(settings, /useAppUpdate\(mode\)/);
  assert.match(settings, /automaticSupported/);
});

test("the sidebar owns the glanceable update and progress control", async () => {
  const sidebar = await readFile(new URL("./components/Sidebar.jsx", import.meta.url), "utf8");
  const control = await readFile(new URL("./components/SidebarUpdateButton.jsx", import.meta.url), "utf8");
  assert.match(sidebar, /<SidebarUpdateButton mode=\{mode\} notify=\{notify\} \/>/);
  assert.match(control, /data-update-state=\{state\}/);
  assert.match(control, /<ProgressRing ratio=\{percent \/ 100\}/);
  assert.match(control, /available \? <Download/);
  assert.match(control, /state === "ready" \? <RotateCcw/);
  assert.match(control, /role="status" aria-live="polite"/);
});

test("download progress is clamped to a complete circular reading", async () => {
  const { appUpdatePercent } = await import("./appUpdate.js");
  assert.equal(appUpdatePercent({ written: 25, total: 100 }), 25);
  assert.equal(appUpdatePercent({ written: 150, total: 100 }), 100);
  assert.equal(appUpdatePercent({ written: -3, total: 100 }), 0);
  assert.equal(appUpdatePercent({ written: 4, total: 0 }), 0);
});
