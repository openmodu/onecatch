import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { collapsePanelAtCompact, COMPACT_LAYOUT_MAX_WIDTH, COMPACT_LAYOUT_QUERY } from "./responsiveLayout.js";

test("the compact layout covers the desktop window's minimum width", () => {
  const desktopApp = readFileSync(new URL("../../../internal/app/desktop/desktop.go", import.meta.url), "utf8");
  const minimumWidth = Number(desktopApp.match(/MinWidth:\s*(\d+)/)?.[1]);

  assert.ok(Number.isFinite(minimumWidth));
  assert.equal(minimumWidth, 860);
  assert.match(desktopApp, /WindowRuntimeReady[\s\S]*?SetMinSize\(860, 720\)/);
  assert.ok(minimumWidth <= COMPACT_LAYOUT_MAX_WIDTH);
  assert.equal(COMPACT_LAYOUT_QUERY, "(max-width: 1100px)");
});

test("entering the compact layout closes a panel without changing it at wider widths", () => {
  assert.equal(collapsePanelAtCompact(false, true), true);
  assert.equal(collapsePanelAtCompact(true, true), true);
  assert.equal(collapsePanelAtCompact(false, false), false);
  assert.equal(collapsePanelAtCompact(true, false), true);
});
