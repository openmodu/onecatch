import test from "node:test";
import assert from "node:assert/strict";
import { buildRunRows, mergeRunItems, virtualRunWindow, workspaceResults } from "./listNavigation.js";

test("compact workspaces keep pinned, recent and the current selection", () => {
  const items = Array.from({ length: 10 }, (_, index) => ({ id: `ws-${index}`, name: `Workspace ${index}`, path: `/tmp/${index}`, pinned: index === 3, lastOpenedAt: new Date(2026, 0, index + 1).toISOString() }));
  const compact = workspaceResults(items, { selectedID: "ws-0", limit: 4 });
  assert.equal(compact.length, 4);
  assert.equal(compact[0].id, "ws-3");
  assert.equal(compact.some((item) => item.id === "ws-0"), true);
  assert.deepEqual(workspaceResults(items, { query: "/tmp/8" }).map((item) => item.id), ["ws-8"]);
  assert.equal(workspaceResults(items, { expanded: true }).length, 10);
});

test("run page merge removes duplicate run IDs", () => {
  assert.deepEqual(mergeRunItems([{ id: "a" }, { id: "b" }], [{ id: "b" }, { id: "c" }]).map((item) => item.id), ["a", "b", "c"]);
  assert.deepEqual(mergeRunItems([{ id: "a" }], [{ id: "b" }], true).map((item) => item.id), ["b"]);
});

test("run rows group dates and virtualize the visible window", () => {
  const now = new Date("2026-07-12T12:00:00Z");
  const rows = buildRunRows([
    { id: "today", updatedAt: "2026-07-12T10:00:00Z" },
    { id: "yesterday", updatedAt: "2026-07-11T10:00:00Z" },
    { id: "old", updatedAt: "2026-06-01T10:00:00Z" },
  ], now);
  assert.deepEqual(rows.filter((row) => row.type === "group").map((row) => row.label), ["今天", "昨天", "更早"]);
  const window = virtualRunWindow(rows, 0, 70, 0);
  assert.equal(window.visible.some((row) => row.key === "today"), true);
  assert.equal(window.visible.some((row) => row.key === "old"), false);
  assert.equal(window.totalHeight, 3 * 30 + 3 * 64);
});

test("virtual run window keeps thousand-item histories bounded", () => {
  const items = Array.from({ length: 1000 }, (_, index) => ({ id: `run-${index}`, updatedAt: "2026-07-12T10:00:00Z" }));
  const rows = buildRunRows(items, new Date("2026-07-12T12:00:00Z"));
  const window = virtualRunWindow(rows, 24_000, 600);
  assert.equal(window.totalHeight, 30 + 1000 * 64);
  assert.equal(window.visible.length < 20, true);
});
