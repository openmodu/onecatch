import test from "node:test";
import assert from "node:assert/strict";

import { normalizeTrayNavigation, trayNavigationEvent } from "./trayNavigation.js";

test("tray navigation accepts new and complete open actions", () => {
  assert.equal(trayNavigationEvent, "onecatch:tray-navigate");
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
