import assert from "node:assert/strict";
import test from "node:test";

import { applyMobileRunFrame, foldMobileEvents, groupMobileConversations, mergeMobileRun, mobileRunTitle, sortMobileRuns } from "./mobileRuns.js";

test("mobile task titles use the first compact prompt line", () => {
  assert.equal(mobileRunTitle("  Review the worker API\nthen add tests  "), "Review the worker API");
  assert.equal(mobileRunTitle(""), "未命名任务");
  assert.equal(mobileRunTitle("123456789", 6), "12345…");
});

test("mobile run history is newest first and replaces updated runs", () => {
  const older = { id: "old", startedAt: "2026-08-10T08:00:00Z", status: "running" };
  const newer = { id: "new", startedAt: "2026-08-10T09:00:00Z", status: "running" };
  assert.deepEqual(sortMobileRuns([older, newer]).map((item) => item.id), ["new", "old"]);
  assert.deepEqual(mergeMobileRun([older, newer], { ...older, status: "succeeded" }).map((item) => `${item.id}:${item.status}`), ["new:running", "old:succeeded"]);
});

test("mobile run frames append events and settle the run", () => {
  const run = { id: "run-1", status: "running", events: [] };
  const withEvent = applyMobileRunFrame(run, { runId: "run-1", event: { kind: "message", text: "hello" } });
  assert.equal(withEvent.events[0].text, "hello");
  const settled = applyMobileRunFrame(withEvent, { runId: "run-1", status: "succeeded", result: { finalMessage: "done" } });
  assert.equal(settled.status, "succeeded");
  assert.equal(settled.result.finalMessage, "done");
});

test("mobile conversations group follow-up runs into one workspace session", () => {
  const conversations = groupMobileConversations([
    { id: "turn-2", conversationId: "chat-1", workspaceId: "onecatch", prompt: "follow up", status: "succeeded", startedAt: "2026-08-10T09:10:00Z" },
    { id: "turn-1", conversationId: "chat-1", workspaceId: "onecatch", prompt: "review iOS", status: "succeeded", startedAt: "2026-08-10T09:00:00Z" },
    { id: "turn-3", conversationId: "chat-2", workspaceId: "api", prompt: "check API", status: "running", startedAt: "2026-08-10T10:00:00Z" },
  ]);
  assert.deepEqual(conversations.map((item) => `${item.workspaceId}:${item.title}`), ["api:check API", "onecatch:review iOS"]);
  assert.deepEqual(conversations[1].runs.map((item) => item.id), ["turn-1", "turn-2"]);
});

test("mobile transcript folds streamed deltas into one assistant message", () => {
  const events = foldMobileEvents([
    { kind: "message", streamId: "answer", phase: "start", revision: 1, text: "" },
    { kind: "message", streamId: "answer", phase: "delta", revision: 2, text: "hello " },
    { kind: "message", streamId: "answer", phase: "delta", revision: 3, text: "world" },
  ]);
  assert.equal(events.length, 1);
  assert.equal(events[0].text, "hello world");
  assert.equal(events[0].streaming, true);
});
