import test from "node:test";
import assert from "node:assert/strict";
import { runPairsContain } from "./workspaceRunSelection.js";

test("checks whether a selected run belongs to the loaded workspace", () => {
  const pairs = [
    ["task-a", [{ id: "run-a" }]],
    ["task-b", [{ id: "run-b" }]],
  ];
  assert.equal(runPairsContain(pairs, "run-b"), true);
  assert.equal(runPairsContain(pairs, "run-from-another-workspace"), false);
  assert.equal(runPairsContain([], "run-a"), false);
});
