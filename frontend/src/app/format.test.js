import test from "node:test";
import assert from "node:assert/strict";
import { errorMessage } from "./format.js";

test("worker protocol errors become actionable UI copy", () => {
  const message = errorMessage("worker_workspace_revision_missing: requested revision is unavailable");
  assert.doesNotMatch(message, /^worker_workspace_revision_missing:/);
  assert.match(message, /B|remote/i);
});

test("unknown errors preserve their original detail", () => {
  assert.equal(errorMessage(new Error("custom failure detail")), "custom failure detail");
});
