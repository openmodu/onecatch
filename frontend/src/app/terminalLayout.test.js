import assert from "node:assert/strict";
import test from "node:test";
import { clampSplitRatio, layoutGeometry, paneIDs, paneNode, removePane, splitPane, updateSplitRatio } from "./terminalLayout.js";

test("supports recursively nested terminal splits", () => {
  let layout = splitPane(paneNode("a"), "a", "b", "vertical", "s1");
  layout = splitPane(layout, "b", "c", "horizontal", "s2");
  assert.deepEqual(paneIDs(layout), ["a", "b", "c"]);
  assert.equal(layout.second.direction, "horizontal");
});

test("closing a pane collapses its empty branch", () => {
  let layout = splitPane(paneNode("a"), "a", "b", "vertical", "s1");
  layout = splitPane(layout, "b", "c", "horizontal", "s2");
  layout = removePane(layout, "b");
  assert.deepEqual(paneIDs(layout), ["a", "c"]);
  assert.equal(layout.second.paneID, "c");
});

test("split ratios are clamped and update only their target", () => {
  const layout = splitPane(paneNode("a"), "a", "b", "vertical", "s1");
  assert.equal(updateSplitRatio(layout, "s1", 0.01).ratio, 0.12);
  assert.equal(updateSplitRatio(layout, "s1", 0.7).ratio, 0.7);
});

test("nested layouts produce stable flat pane geometry", () => {
  let layout = splitPane(paneNode("a"), "a", "b", "vertical", "s1");
  layout = splitPane(layout, "b", "c", "horizontal", "s2");
  const geometry = layoutGeometry(layout);
  assert.deepEqual(geometry.panes.map((item) => item.paneID), ["a", "b", "c"]);
  assert.deepEqual(geometry.panes[0].rect, { x: 0, y: 0, width: 0.5, height: 1 });
  assert.deepEqual(geometry.panes[2].rect, { x: 0.5, y: 0.5, width: 0.5, height: 0.5 });
});

test("split dragging preserves a usable pixel size", () => {
  assert.equal(clampSplitRatio(0.02, 1000, 180), 0.18);
  assert.equal(clampSplitRatio(0.98, 400, 96), 0.76);
  assert.equal(clampSplitRatio(0.1, 100, 96), 0.45);
});
