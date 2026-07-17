import test from "node:test";
import assert from "node:assert/strict";
import { SIDEBAR_TASK_PREVIEW_LIMIT, buildSidebarTaskEntries, visibleSidebarTaskEntries } from "./sidebarNavigation.js";

test("sidebar task preview stays compact until the project is expanded", () => {
  const entries = Array.from({ length: 7 }, (_, index) => ({ key: `run:${index}` }));
  assert.equal(SIDEBAR_TASK_PREVIEW_LIMIT, 3);
  assert.deepEqual(visibleSidebarTaskEntries(entries).map((entry) => entry.key), ["run:0", "run:1", "run:2"]);
  assert.equal(visibleSidebarTaskEntries(entries, true).length, 7);
});

test("sidebar task entries combine queued tasks and filtered runs", () => {
  const tasks = [
    { id: "queued-1", title: "Ship UI", prompt: "sidebar", status: "queued", createdAt: "2026-01-01" },
    { id: "done-1", title: "Ignore completed task record", status: "completed" },
  ];
  const runs = [
    { id: "run-1", status: "paused", task: { title: "Review UI" } },
    { id: "run-2", status: "completed", task: { title: "Build backend" } },
  ];
  assert.deepEqual(buildSidebarTaskEntries(tasks, runs).map((entry) => entry.key), ["task:queued-1", "run:run-1", "run:run-2"]);
  assert.deepEqual(buildSidebarTaskEntries(tasks, runs, { query: "Review" }).map((entry) => entry.key), ["run:run-1"]);
  assert.deepEqual(buildSidebarTaskEntries(tasks, runs, { status: "queued" }).map((entry) => entry.key), ["task:queued-1"]);
});
