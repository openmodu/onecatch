import test from "node:test";
import assert from "node:assert/strict";
import { loopTemplate, dagTemplate } from "./templates.js";
import { simpleReviewWorkflow, setReviewBehavior } from "./simpleWorkflow.js";
test("guided setup recognizes complete review loop without changing it", () => {
  const input = structuredClone(loopTemplate);
  assert.equal(simpleReviewWorkflow(input).rounds, 10);
  assert.deepEqual(input, loopTemplate);
  input.steps.reverse();
  assert.equal(simpleReviewWorkflow(input).implement.id, "implement");
});
test("guided edits preserve role, runtime, permissions and unrelated policy", () => {
  const updated = setReviewBehavior(loopTemplate, true, 3);
  assert.equal(updated.policy.maxTransitions, 6);
  assert.equal(updated.policy.stepTimeoutSeconds, loopTemplate.policy.stepTimeoutSeconds);
  assert.deepEqual(updated.steps, loopTemplate.steps);
  const paused = setReviewBehavior(updated, false, 3);
  assert.equal(paused.steps[1].transitions.changes_requested, "$pause");
  assert.equal(simpleReviewWorkflow(paused).retry, false);
  assert.equal(setReviewBehavior(paused, true, 3).steps[1].transitions.changes_requested, "implement");
  assert.equal(loopTemplate.steps[1].transitions.changes_requested, "implement");
});
test("custom graphs and odd step limits remain in advanced editor", () => {
  assert.equal(simpleReviewWorkflow(dagTemplate), null);
  for (const mutate of [
    (workflow) => { workflow.steps[1].transitions.extra = "$fail"; },
    (workflow) => { workflow.policy.maxTransitions = 5; },
    (workflow) => { workflow.steps[1].dependsOn = ["implement"]; },
    (workflow) => { workflow.steps[0].transitions.ready_for_review = "$done"; },
  ]) {
    const input = structuredClone(loopTemplate); mutate(input);
    assert.equal(simpleReviewWorkflow(input), null);
    assert.equal(setReviewBehavior(input, true, 3), input);
  }
  assert.equal(setReviewBehavior(loopTemplate, true, 0), loopTemplate);
});
