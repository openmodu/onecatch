import assert from "node:assert/strict";
import test from "node:test";
import { newestTaskRun, normalizeTrayNavigation, TRAY_ACTION_EVENT } from "./trayNavigation.js";

test("tray navigation accepts new and complete open actions", () => {
  assert.equal(TRAY_ACTION_EVENT, "onecatch:tray-navigate");
  assert.deepEqual(normalizeTrayNavigation({ action: "new", ignored: true }), { action: "new" });
  assert.deepEqual(normalizeTrayNavigation({ action: "open", workspaceId: " ws ", taskId: " task ", runId: " run " }), {
    action: "open",
    workspaceId: "ws",
    taskId: "task",
    runId: "run",
  });
});

test("tray navigation rejects incomplete or unknown actions", () => {
  assert.equal(normalizeTrayNavigation(null), null);
  assert.equal(normalizeTrayNavigation({ action: "quit" }), null);
  assert.equal(normalizeTrayNavigation({ action: "open", workspaceId: "ws" }), null);
});

test("tray navigation selects the most recently active run", () => {
  const run = newestTaskRun([
    { id: "older", updatedAt: "2026-08-30T08:00:00Z" },
    { id: "latest", updatedAt: "2026-08-30T10:00:00Z" },
    { id: "middle", updatedAt: "2026-08-30T09:00:00Z" },
  ]);
  assert.equal(run.id, "latest");
});

test("tray navigation handles a task without runs", () => {
  assert.equal(newestTaskRun([]), null);
});
