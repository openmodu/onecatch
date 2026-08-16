import test from "node:test";
import assert from "node:assert/strict";
import {
  activeWorkerID,
  buildInspectorContext,
  inspectorContextSignature,
  queuedTaskPosition,
  sortQueuedTasks,
} from "./inspectorContext.js";

const runDetail = {
  run: { id: "run_1", status: "running", revision: 4, currentStepId: "build" },
  task: { id: "task_1", title: "Ship it" },
  workflow: { id: "single_agent", steps: [{ id: "build", sandbox: "read-only", workerId: "worker-a" }] },
  stepRuns: [{ stepId: "build", attempt: 1, status: "running", inputTokens: 10, outputTokens: 2 }],
  events: [{ seq: 7, type: "run.started" }],
  instructions: [{ id: "i1", status: "pending" }],
  active: true,
  runtimeEvents: [{ stepRunId: "sr1", seq: 1, text: "a".repeat(4096) }],
};

test("follows a read-only remote step, but never a writable one", () => {
  assert.equal(activeWorkerID(runDetail), "worker-a");
  assert.equal(activeWorkerID({ ...runDetail, workflow: { steps: [{ id: "build", sandbox: "workspace-write", workerId: "worker-a" }] } }), "");
  assert.equal(activeWorkerID({ ...runDetail, workflow: { steps: [{ id: "build", sandbox: "read-only", workerId: "local" }] } }), "");
  assert.equal(activeWorkerID(null), "");
});

test("orders the queue by enqueue time and reports a 1-based position", () => {
  const tasks = [
    { id: "b", status: "queued", queue: { enqueuedAt: "2026-08-16T10:00:02Z" } },
    { id: "done", status: "completed", createdAt: "2026-08-16T09:00:00Z" },
    { id: "a", status: "queued", queue: { enqueuedAt: "2026-08-16T10:00:01Z" } },
  ];
  assert.deepEqual(sortQueuedTasks(tasks).map((task) => task.id), ["a", "b"]);
  assert.equal(queuedTaskPosition(tasks, "b"), 2);
  assert.equal(queuedTaskPosition(tasks, "done"), 0);
  assert.equal(queuedTaskPosition(tasks, ""), 0);
});

test("published context carries what the panel renders and drops the transcript", () => {
  const context = buildInspectorContext({ mode: "wails", workspaceID: "ws_1", runDetail, tasks: [] });
  assert.equal(context.mode, "wails");
  assert.equal(context.workspaceID, "ws_1");
  assert.equal(context.runWorkerID, "worker-a");
  assert.equal(context.detail.run.id, "run_1");
  assert.deepEqual(context.detail.events, runDetail.events);
  assert.equal(context.detail.active, true);
  assert.equal("runtimeEvents" in context.detail, false);
});

test("a draft publishes no run at all", () => {
  const context = buildInspectorContext({ mode: "wails", workspaceID: "ws_1", runDetail, tasks: [], draft: true });
  assert.equal(context.draft, true);
  assert.equal(context.detail, null);
  assert.equal(context.runWorkerID, "");
  assert.equal(context.queuePosition, 0);
});

test("a selected queued task travels with its position", () => {
  const tasks = [
    { id: "a", status: "queued", queue: { enqueuedAt: "2026-08-16T10:00:01Z" } },
    { id: "b", status: "queued", queue: { enqueuedAt: "2026-08-16T10:00:02Z" } },
  ];
  const context = buildInspectorContext({ mode: "wails", workspaceID: "ws_1", runDetail: null, tasks, selectedQueuedTaskID: "b" });
  assert.equal(context.queuedTask.id, "b");
  assert.equal(context.queuePosition, 2);
});

test("the signature ignores transcript growth but catches real state changes", () => {
  const base = buildInspectorContext({ mode: "wails", workspaceID: "ws_1", runDetail, tasks: [] });
  const streamed = buildInspectorContext({
    mode: "wails",
    workspaceID: "ws_1",
    runDetail: { ...runDetail, runtimeEvents: [...runDetail.runtimeEvents, { stepRunId: "sr1", seq: 2, text: "more" }] },
    tasks: [],
  });
  assert.equal(inspectorContextSignature(base), inspectorContextSignature(streamed));

  const paused = buildInspectorContext({
    mode: "wails",
    workspaceID: "ws_1",
    runDetail: { ...runDetail, run: { ...runDetail.run, status: "paused", revision: 5 } },
    tasks: [],
  });
  assert.notEqual(inspectorContextSignature(base), inspectorContextSignature(paused));

  const tokens = buildInspectorContext({
    mode: "wails",
    workspaceID: "ws_1",
    runDetail: { ...runDetail, stepRuns: [{ ...runDetail.stepRuns[0], outputTokens: 99 }] },
    tasks: [],
  });
  assert.notEqual(inspectorContextSignature(base), inspectorContextSignature(tokens));

  const logged = buildInspectorContext({
    mode: "wails",
    workspaceID: "ws_1",
    runDetail: { ...runDetail, events: [...runDetail.events, { seq: 8, type: "step.finished" }] },
    tasks: [],
  });
  assert.notEqual(inspectorContextSignature(base), inspectorContextSignature(logged));
});
