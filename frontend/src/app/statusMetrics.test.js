import test from "node:test";
import assert from "node:assert/strict";
import { currentWorkflowStepID, effectiveWorkflowStepStatus, latestStepRunsByStep, summarizeAgentDuration } from "./statusMetrics.js";

test("latestStepRunsByStep uses attempt and time rather than response order", () => {
  const latest = latestStepRunsByStep([
    { id: "newer-list-item", stepId: "code", attempt: 1, startedAt: "2026-08-15T10:00:00Z" },
    { id: "latest-attempt", stepId: "code", attempt: 2, startedAt: "2026-08-15T09:00:00Z" },
    { id: "review", stepId: "review", attempt: 1 },
  ]);
  assert.equal(latest.get("code").id, "latest-attempt");
  assert.equal(latest.get("review").id, "review");
});

test("terminal attempt corrects a stale running workflow node", () => {
  assert.equal(effectiveWorkflowStepStatus(
    { status: "paused", currentStepId: "code", pauseReason: "interrupted" },
    "code",
    { status: "running" },
    { status: "interrupted" },
  ), "interrupted");
});

test("DAG current step follows the active node rather than the serial cursor", () => {
  assert.equal(currentWorkflowStepID({
    status: "paused",
    currentStepId: "entry",
    nodes: {
      entry: { status: "completed", finishedAt: "2026-08-15T09:00:00Z" },
      review: { status: "paused", finishedAt: "2026-08-15T10:00:00Z" },
    },
  }, { mode: "dag" }, []), "review");
});

test("finished serial runs use the latest persisted attempt as the displayed step", () => {
  assert.equal(currentWorkflowStepID(
    { status: "paused", currentStepId: "implement" },
    { mode: "serial" },
    [
      { stepId: "implement", startedAt: "2026-08-15T09:00:00Z" },
      { stepId: "review", startedAt: "2026-08-15T10:00:00Z" },
    ],
  ), "review");
});

test("running serial runs trust the state-machine cursor during transitions", () => {
  assert.equal(currentWorkflowStepID(
    { status: "running", currentStepId: "review" },
    { mode: "serial" },
    [{ stepId: "implement", startedAt: "2026-08-15T10:00:00Z" }],
  ), "review");
});

test("an interrupted DAG node is reported as interrupted even if its attempt was saved as failed", () => {
  assert.equal(effectiveWorkflowStepStatus(
    { status: "paused", currentStepId: "review", pauseReason: "interrupted" },
    "review",
    { status: "paused" },
    { status: "failed" },
  ), "interrupted");
});

test("workflow-signal DAG pause retains the paused node state", () => {
  assert.equal(effectiveWorkflowStepStatus(
    { status: "paused", currentStepId: "review", pauseReason: "workflow_signal" },
    "review",
    { status: "paused" },
    { status: "succeeded" },
  ), "paused");
});

test("paused run corrects a stale current node when no terminal attempt exists", () => {
  assert.equal(effectiveWorkflowStepStatus(
    { status: "paused", currentStepId: "code", pauseReason: "workspace_locked" },
    "code",
    { status: "running" },
    null,
  ), "paused");
});

test("completed historical nodes keep their node status", () => {
  assert.equal(effectiveWorkflowStepStatus(
    { status: "running", currentStepId: "review" },
    "code",
    { status: "completed" },
    { status: "succeeded" },
  ), "completed");
});

test("agent duration includes an unfinished active attempt without double counting persisted time", () => {
  const now = new Date("2026-08-15T10:00:10Z").getTime();
  assert.equal(summarizeAgentDuration([
    { status: "succeeded", durationMs: 4000, startedAt: "2026-08-15T09:00:00Z" },
    { status: "running", startedAt: "2026-08-15T10:00:04Z" },
    { status: "running", durationMs: 2500, startedAt: "2026-08-15T10:00:00Z" },
  ], now), 12500);
});
