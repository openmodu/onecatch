import assert from "node:assert/strict";
import test from "node:test";
import { REMOTE_FS_HEALTH_INTERVAL_MS, shouldAutoCheckRemoteFS } from "./remoteFSHealth.js";

test("healthy remote FS targets are checked every five minutes", () => {
  assert.equal(REMOTE_FS_HEALTH_INTERVAL_MS, 300_000);
  assert.equal(shouldAutoCheckRemoteFS(undefined), true);
  assert.equal(shouldAutoCheckRemoteFS({ healthy: true }), true);
  assert.equal(shouldAutoCheckRemoteFS({ healthy: true, checking: true }), false);
});

test("unhealthy remote FS targets wait for a manual retry", () => {
  assert.equal(shouldAutoCheckRemoteFS({ healthy: false }), false);
});
