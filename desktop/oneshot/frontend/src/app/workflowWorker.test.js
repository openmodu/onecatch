import test from "node:test";
import assert from "node:assert/strict";
import { assignWorkflowWorker, isRemoteWorker } from "./workflowWorker.js";

test("selecting a remote worker preserves workspace-write for synchronized changes", () => {
  assert.deepEqual(
    assignWorkflowWorker({ id: "review", sandbox: "workspace-write" }, "mac-mini"),
    { id: "review", workerId: "mac-mini", sandbox: "workspace-write" },
  );
  assert.deepEqual(
    assignWorkflowWorker({ id: "review", sandbox: "full" }, "mac-mini"),
    { id: "review", workerId: "mac-mini", sandbox: "workspace-write" },
  );
});

test("returning to local preserves the explicit sandbox", () => {
  assert.deepEqual(
    assignWorkflowWorker({ id: "review", workerId: "mac-mini", sandbox: "read-only" }, "local"),
    { id: "review", workerId: "local", sandbox: "read-only" },
  );
  assert.equal(isRemoteWorker("local"), false);
  assert.equal(isRemoteWorker("mac-mini"), true);
});
