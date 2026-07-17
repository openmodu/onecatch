import test from "node:test";
import assert from "node:assert/strict";
import { mergeRunItems, preserveEqualValue, workspaceResults, workspaceSections } from "./listNavigation.js";

test("compact workspaces keep pinned, recent and the current selection", () => {
  const items = Array.from({ length: 10 }, (_, index) => ({ id: `ws-${index}`, name: `Workspace ${index}`, path: `/tmp/${index}`, pinned: index === 3, lastOpenedAt: new Date(2026, 0, index + 1).toISOString() }));
  const compact = workspaceResults(items, { selectedID: "ws-0", limit: 4 });
  assert.equal(compact.length, 4);
  assert.equal(compact[0].id, "ws-3");
  assert.equal(compact.some((item) => item.id === "ws-0"), true);
  assert.deepEqual(workspaceResults(items, { query: "/tmp/8" }).map((item) => item.id), ["ws-8"]);
  assert.equal(workspaceResults(items, { expanded: true }).length, 10);
});

test("sidebar separates pinned projects from the compact project list", () => {
  const items = Array.from({ length: 10 }, (_, index) => ({ id: `ws-${index}`, name: `Workspace ${index}`, path: `/tmp/${index}`, pinned: index < 2, lastOpenedAt: new Date(2026, 0, 10 - index).toISOString() }));
  const compact = workspaceSections(items, { selectedID: "ws-9", limit: 3 });
  assert.deepEqual(compact.pinned.map((item) => item.id), ["ws-0", "ws-1"]);
  assert.equal(compact.projects.length, 3);
  assert.equal(compact.projects.some((item) => item.id === "ws-9"), true);
  assert.equal(workspaceSections(items, { expanded: true }).projects.length, 8);
});

test("run page merge removes duplicate run IDs", () => {
  assert.deepEqual(mergeRunItems([{ id: "a" }, { id: "b" }], [{ id: "b" }, { id: "c" }]).map((item) => item.id), ["a", "b", "c"]);
  assert.deepEqual(mergeRunItems([{ id: "a" }], [{ id: "b" }], true).map((item) => item.id), ["b"]);
});

test("background refresh preserves state references when data did not change", () => {
  const current = [{ id: "a", status: "running", task: { title: "Keep rendering stable" } }];
  assert.equal(preserveEqualValue(current, structuredClone(current)), current);
  assert.equal(mergeRunItems(current, structuredClone(current), true), current);
  assert.notEqual(preserveEqualValue(current, [{ ...current[0], status: "completed" }]), current);
});
