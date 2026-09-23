// Only expose the guided editor when every branch can be represented faithfully.
export function simpleReviewWorkflow(definition) {
  if (!definition || definition.mode === "dag" || definition.steps?.length !== 2) return null;
  const implement = definition.steps.find((step) => step.id === definition.entryStepId);
  const review = definition.steps.find((step) => step.id !== definition.entryStepId);
  if (!implement || !review || definition.steps.some((step) => step.dependsOn?.length)) return null;
  const same = (actual, expected) => Object.keys(actual || {}).length === Object.keys(expected).length && Object.entries(expected).every(([key, value]) => actual[key] === value);
  if (!same(implement.transitions, { ready_for_review: review.id, need_human: "$pause" })) return null;
  if (![implement.id, "$pause"].includes(review.transitions?.changes_requested)) return null;
  if (!same(review.transitions, { changes_requested: review.transitions.changes_requested, approved: "$done", need_human: "$pause" })) return null;
  const limit = definition.policy?.maxTransitions || 20;
  if (!Number.isInteger(limit) || limit < 2 || limit % 2) return null;
  return { implement, review, rounds: limit / 2, retry: review.transitions.changes_requested !== "$pause" };
}

export function setReviewBehavior(definition, retry, rounds) {
  const simple = simpleReviewWorkflow(definition);
  if (!simple || !Number.isInteger(rounds) || rounds < 1 || rounds > 50) return definition;
  return { ...definition, policy: { ...definition.policy, maxTransitions: rounds * 2 }, steps: definition.steps.map((step) => step.id === simple.review.id ? { ...step, transitions: { ...step.transitions, changes_requested: retry ? simple.implement.id : "$pause" } } : step) };
}
