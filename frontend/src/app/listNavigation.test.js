import test from "node:test";
import assert from "node:assert/strict";
import { mergeRunItems, preserveByFingerprint, preserveEqualValue, runDetailFingerprint, runItemFingerprint, workspaceResults, workspaceSections } from "./listNavigation.js";

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

test("a run detail is compared by fingerprint, never by serialising the transcript", () => {
  const transcript = Array.from({ length: 400 }, (_, index) => ({ stepRunId: "step-1", seq: index + 1, text: `entry ${index}` }));
  const detail = {
    run: { id: "run-1", revision: 7, status: "running" },
    stepRuns: [{ id: "step-1", status: "running" }],
    runtimeEvents: transcript,
    runtimeEventsTotal: 400,
    instructions: [],
    events: [],
    active: true,
  };
  // An identical reload keeps the reference, so memoised children skip a render.
  assert.equal(preserveByFingerprint(detail, structuredClone(detail), runDetailFingerprint), detail);

  // Everything the transcript can do while a run is live has to be noticed:
  // a new entry, and a growing final entry that keeps its position.
  const appended = { ...detail, runtimeEvents: [...transcript, { stepRunId: "step-1", seq: 401, text: "new" }], runtimeEventsTotal: 401 };
  assert.notEqual(preserveByFingerprint(detail, appended, runDetailFingerprint), detail);

  const grown = structuredClone(detail);
  grown.runtimeEvents[399] = { ...grown.runtimeEvents[399], text: "entry 399 and more", revision: 2 };
  assert.notEqual(preserveByFingerprint(detail, grown, runDetailFingerprint), detail);

  const finished = { ...detail, active: false, run: { ...detail.run, revision: 8, status: "completed" } };
  assert.notEqual(preserveByFingerprint(detail, finished, runDetailFingerprint), detail);

  // Loading earlier history replaces the window with the whole transcript.
  const expanded = { ...detail, runtimeEvents: transcript, runtimeEventsTotal: 900 };
  assert.notEqual(preserveByFingerprint({ ...detail, runtimeEventsTotal: 900 }, { ...expanded, runtimeEventsTotal: 400 }, runDetailFingerprint), expanded);
});

test("run rows are compared by the fields a row renders", () => {
  const row = { id: "run-1", revision: 3, status: "running", updatedAt: "2026-08-01T00:00:00Z", task: { title: "Ship it", status: "running", updatedAt: "2026-08-01T00:00:00Z" } };
  assert.equal(preserveByFingerprint(row, structuredClone(row), runItemFingerprint), row);
  assert.notEqual(preserveByFingerprint(row, { ...row, status: "completed" }, runItemFingerprint), row);
  assert.notEqual(preserveByFingerprint(row, { ...row, task: { ...row.task, title: "Renamed" } }, runItemFingerprint), row);
  // A run that only re-reported the same revision must not churn the row.
  assert.equal(preserveByFingerprint(row, { ...row, extraFieldTheRowDoesNotRender: 1 }, runItemFingerprint), row);
});
