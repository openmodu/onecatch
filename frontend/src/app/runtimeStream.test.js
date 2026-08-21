import test from "node:test";
import assert from "node:assert/strict";
import { applyRunState, applyRuntimeFrame, applyRuntimeFrames } from "./runtimeStream.js";

const detail = () => ({ run: { id: "run-1" }, runtimeEvents: [] });

test("folds start and deltas into one logical runtime event", () => {
  const next = applyRuntimeFrames(detail(), [
    { runId: "run-1", stepRunId: "step-1", seq: 4, kind: "message", streamId: "m1", phase: "start", revision: 0 },
    { runId: "run-1", stepRunId: "step-1", seq: 4, kind: "message", streamId: "m1", phase: "delta", revision: 1, text: "你" },
    { runId: "run-1", stepRunId: "step-1", seq: 4, kind: "message", streamId: "m1", phase: "delta", revision: 2, text: "好" },
  ]);
  assert.equal(next.runtimeEvents.length, 1);
  assert.equal(next.runtimeEvents[0].text, "你好");
  assert.equal(next.runtimeEvents[0].streaming, true);
  assert.equal(next.runtimeEvents[0].revision, 2);
});

test("discards duplicate and out-of-order deltas", () => {
  const first = applyRuntimeFrame(detail(), { runId: "run-1", stepRunId: "step-1", seq: 1, kind: "message", streamId: "m1", phase: "delta", revision: 2, text: "完整" });
  const next = applyRuntimeFrames(first, [
    { runId: "run-1", stepRunId: "step-1", seq: 1, kind: "message", streamId: "m1", phase: "delta", revision: 1, text: "旧" },
    { runId: "run-1", stepRunId: "step-1", seq: 1, kind: "message", streamId: "m1", phase: "delta", revision: 2, text: "重复" },
  ]);
  assert.strictEqual(next, first);
  assert.equal(next.runtimeEvents[0].text, "完整");
});

test("snapshot replaces text and end clears the streaming state", () => {
  const next = applyRuntimeFrames(detail(), [
    { runId: "run-1", stepRunId: "step-1", seq: 7, kind: "message", streamId: "m1", phase: "delta", revision: 1, text: "Hel" },
    { runId: "run-1", stepRunId: "step-1", seq: 7, kind: "message", streamId: "m1", phase: "snapshot", revision: 2, text: "Hello" },
    { runId: "run-1", stepRunId: "step-1", seq: 7, kind: "message", streamId: "m1", phase: "end", revision: 3, text: "Hello!" },
  ]);
  assert.deepEqual(next.runtimeEvents[0], {
    stepRunId: "step-1", seq: 7, kind: "message", streamId: "m1", revision: 3,
    streaming: false, text: "Hello!", failed: false, at: "",
  });
});

test("ignores another run and deduplicates atomic events", () => {
  const frame = { runId: "run-1", stepRunId: "step-1", seq: 9, kind: "usage" };
  const next = applyRuntimeFrames(detail(), [frame, frame, { ...frame, runId: "run-2", seq: 10 }]);
  assert.equal(next.runtimeEvents.length, 1);
});

test("applies cumulative usage frames to a running step", () => {
  const current = {
    run: { id: "run-1" },
    stepRuns: [{ id: "step-1", status: "running", inputTokens: 0, outputTokens: 0 }],
    runtimeEvents: [],
  };
  const next = applyRuntimeFrame(current, {
    runId: "run-1", stepRunId: "step-1", seq: 3, kind: "usage",
    usage: { inputTokens: 120, cachedInputTokens: 90, outputTokens: 12, reasoningOutputTokens: 4 },
  });
  assert.deepEqual(next.stepRuns[0], {
    id: "step-1", status: "running", inputTokens: 120, cachedInputTokens: 90,
    outputTokens: 12, reasoningOutputTokens: 4,
  });

  const stale = applyRuntimeFrame(next, {
    runId: "run-1", stepRunId: "step-1", seq: 4, kind: "usage",
    usage: { inputTokens: 100, cachedInputTokens: 80, outputTokens: 10 },
  });
  assert.equal(stale.stepRuns[0].inputTokens, 120);
  assert.equal(stale.stepRuns[0].cachedInputTokens, 90);
});

test("keeps correlated atomic tool results instead of treating their id as a stream", () => {
  const current = {
    run: { id: "run-1" },
    runtimeEvents: [
      { stepRunId: "step-1", seq: 5, kind: "tool_use", streamId: "call-1", revision: 0, streaming: false, text: "git status" },
    ],
  };
  const result = { runId: "run-1", stepRunId: "step-1", seq: 6, kind: "tool_result", streamId: "call-1", text: "clean" };
  const next = applyRuntimeFrames(current, [result, result]);

  assert.equal(next.runtimeEvents.length, 2);
  assert.equal(next.runtimeEvents[0].kind, "tool_use");
  assert.equal(next.runtimeEvents[1].kind, "tool_result");
  assert.equal(next.runtimeEvents[1].streamId, "call-1");
  assert.equal(next.runtimeEvents[1].streaming, false);
});

test("coalesces a large delta batch without growing event rows", () => {
  const frames = Array.from({ length: 1_000 }, (_, index) => ({
    runId: "run-1", stepRunId: "step-1", seq: 1, kind: "message", streamId: "m1",
    phase: "delta", revision: index + 1, text: "x",
  }));
  const next = applyRuntimeFrames(detail(), frames);
  assert.equal(next.runtimeEvents.length, 1);
  assert.equal(next.runtimeEvents[0].text.length, 1_000);
  assert.equal(next.runtimeEvents[0].revision, 1_000);
});

test("applyRunState merges bounded state and preserves the transcript", () => {
  const current = {
    run: { id: "run-1", status: "running", revision: 3 },
    task: { id: "task-1" },
    stepRuns: [{ id: "s1", status: "running" }],
    runtimeEvents: [{ streamId: "m1", text: "keep me" }],
    instructions: [],
    active: true,
  };
  const next = applyRunState(current, {
    runId: "run-1",
    run: { id: "run-1", status: "completed", revision: 4 },
    stepRuns: [{ id: "s1", status: "succeeded" }],
    instructions: [{ id: "i1", status: "pending" }],
    active: false,
  });
  assert.equal(next.run.status, "completed");
  assert.equal(next.stepRuns[0].status, "succeeded");
  assert.equal(next.instructions.length, 1);
  assert.equal(next.active, false);
  assert.deepEqual(next.runtimeEvents, current.runtimeEvents, "transcript is untouched");
  assert.equal(next.task.id, "task-1", "unrelated fields survive");
});

test("applyRunState does not erase newer live usage with a stale bounded snapshot", () => {
  const current = {
    run: { id: "run-1", status: "running", revision: 3 },
    stepRuns: [{ id: "s1", status: "running", inputTokens: 120, cachedInputTokens: 90, outputTokens: 12 }],
    runtimeEvents: [], instructions: [], active: true,
  };
  const next = applyRunState(current, {
    runId: "run-1", run: { id: "run-1", status: "running", revision: 3 },
    stepRuns: [{ id: "s1", status: "running", inputTokens: 0, cachedInputTokens: 0, outputTokens: 0 }],
    active: true,
  });
  assert.equal(next.stepRuns[0].inputTokens, 120);
  assert.equal(next.stepRuns[0].cachedInputTokens, 90);
  assert.equal(next.stepRuns[0].outputTokens, 12);
});

test("applyRunState ignores a view for a different run", () => {
  const current = { run: { id: "run-1", revision: 1 } };
  assert.strictEqual(applyRunState(current, { runId: "run-2", run: { id: "run-2" } }), current);
});

test("applyRunState never moves the run revision backwards", () => {
  const current = { run: { id: "run-1", status: "completed", revision: 5 }, stepRuns: [], instructions: [] };
  const stale = applyRunState(current, { runId: "run-1", run: { id: "run-1", status: "running", revision: 4 }, active: true });
  assert.strictEqual(stale, current, "a stale push is dropped");
});
