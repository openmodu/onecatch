import assert from "node:assert/strict";
import test from "node:test";
import { newestTaskRun, TRAY_ACTION_EVENT } from "./trayNavigation.js";

test("tray navigation selects the most recently active run", () => {
  const run = newestTaskRun([
    { id: "older", updatedAt: "2026-08-30T08:00:00Z" },
    { id: "latest", updatedAt: "2026-08-30T10:00:00Z" },
    { id: "middle", updatedAt: "2026-08-30T09:00:00Z" },
  ]);
  assert.equal(run.id, "latest");
  assert.equal(TRAY_ACTION_EVENT, "onecatch:tray-action");
});

test("tray navigation handles a task without runs", () => {
  assert.equal(newestTaskRun([]), null);
});
