import test from "node:test";
import assert from "node:assert/strict";
import { buildLockSignal, completionEdge, LOCK_PHASE } from "./lockSignal.js";

test("working phase when a run is running", () => {
  const signal = buildLockSignal([], [{ id: "r1", status: "running", task: { title: "build" } }]);
  assert.equal(signal.phase, LOCK_PHASE.working);
  assert.equal(signal.running, 1);
  assert.equal(signal.active, 1);
  assert.equal(signal.items[0].title, "build");
});

test("queued tasks count as active work", () => {
  const signal = buildLockSignal([{ id: "t1", status: "queued", title: "later" }], []);
  assert.equal(signal.phase, LOCK_PHASE.working);
  assert.equal(signal.queued, 1);
  assert.equal(signal.active, 1);
});

test("waiting phase takes priority over running", () => {
  const signal = buildLockSignal([], [
    { id: "r1", status: "running", task: { title: "a" } },
    { id: "r2", status: "paused", task: { title: "b" } },
  ]);
  assert.equal(signal.phase, LOCK_PHASE.waiting);
  assert.equal(signal.paused, 1);
  // Waiting runs are listed after the in-flight ones.
  assert.deepEqual(signal.items.map((item) => item.status), ["running", "paused"]);
});

test("finished work does not keep the standby busy", () => {
  const signal = buildLockSignal([{ id: "t1", status: "completed" }], [
    { id: "r1", status: "completed" },
    { id: "r2", status: "failed" },
    { id: "r3", status: "cancelled" },
  ]);
  assert.equal(signal.phase, LOCK_PHASE.done);
  assert.equal(signal.active, 0);
  assert.equal(signal.items.length, 0);
});

test("completionEdge fires done when the last active run finishes", () => {
  const before = buildLockSignal([], [{ id: "r1", status: "running" }]);
  const after = buildLockSignal([], [{ id: "r1", status: "completed" }]);
  assert.equal(completionEdge(before, after), LOCK_PHASE.done);
});

test("completionEdge does not fire done when work remains", () => {
  const before = buildLockSignal([], [{ id: "r1", status: "running" }, { id: "r2", status: "running" }]);
  const after = buildLockSignal([], [{ id: "r1", status: "completed" }, { id: "r2", status: "running" }]);
  assert.equal(completionEdge(before, after), "");
});

test("completionEdge does not fire done when a run finished into paused", () => {
  const before = buildLockSignal([], [{ id: "r1", status: "running" }]);
  const after = buildLockSignal([], [{ id: "r1", status: "paused" }]);
  // active fell to 0 but one now needs attention — this is a waiting edge, not done.
  assert.equal(completionEdge(before, after), LOCK_PHASE.waiting);
});

test("completionEdge fires waiting when a run newly pauses mid-flight", () => {
  const before = buildLockSignal([], [{ id: "r1", status: "running" }, { id: "r2", status: "running" }]);
  const after = buildLockSignal([], [{ id: "r1", status: "paused" }, { id: "r2", status: "running" }]);
  assert.equal(completionEdge(before, after), LOCK_PHASE.waiting);
});

test("completionEdge is silent with no prior signal", () => {
  const after = buildLockSignal([], []);
  assert.equal(completionEdge(null, after), "");
});
