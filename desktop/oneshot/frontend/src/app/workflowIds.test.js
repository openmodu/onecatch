import test from "node:test";
import assert from "node:assert/strict";
import { nextWorkflowItemID } from "./workflowIds.js";

test("serial step IDs remain unique after deletion", () => {
  const steps = [{ id: "step_1" }, { id: "step_3" }];
  assert.equal(nextWorkflowItemID("step", steps), "step_4");
});

test("DAG node IDs remain unique after deletion", () => {
  const steps = [{ id: "node_1" }, { id: "node_4" }, { id: "custom" }];
  assert.equal(nextWorkflowItemID("node", steps), "node_5");
});

test("non-numeric IDs do not affect the generated sequence", () => {
  const steps = [{ id: "node_review" }, { id: "other_99" }];
  assert.equal(nextWorkflowItemID("node", steps), "node_1");
});
