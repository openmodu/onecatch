import assert from "node:assert/strict";
import test from "node:test";

import { applyMobileRunFrame, foldMobileEvents, groupMobileConversations, mergeMobileRun, mobileEventSummary, mobileRunTitle, projectActivity, sortMobileRuns, unwrapShellCommand } from "./mobileRuns.js";

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

test("projectActivity summarises a project's sessions for the list row", () => {
  const activity = projectActivity([
    { startedAt: "2026-09-06T01:00:00Z", status: "succeeded" },
    { startedAt: "2026-09-06T04:30:00Z", status: "running" },
    { startedAt: "2026-09-05T22:00:00Z", status: "failed" },
  ]);
  assert.deepEqual(activity, { count: 3, latestAt: "2026-09-06T04:30:00Z", running: true });
  assert.deepEqual(projectActivity([]), { count: 0, latestAt: "", running: false });
  assert.deepEqual(projectActivity(), { count: 0, latestAt: "", running: false });
});

test("a tool call reads as the command, not the shell that carried it", () => {
  assert.equal(
    unwrapShellCommand(`/etc/profiles/per-user/ityike/bin/zsh -lc "pwd; rg --files -g 'README*'"`),
    "pwd; rg --files -g 'README*'",
  );
  assert.equal(unwrapShellCommand("/bin/bash -lc 'go test ./...'"), "go test ./...");
  // Anything that is not a shell wrapper is left exactly as it came.
  assert.equal(unwrapShellCommand("git status --porcelain"), "git status --porcelain");
  assert.equal(unwrapShellCommand(""), "");
});

test("a tool row leads with the program and carries its output", () => {
  const summary = mobileEventSummary({ kind: "tool_use", text: `/bin/zsh -lc "ls -la"`, result: "total 12" }, "调用");
  assert.equal(summary.label, "ls", "the row names what ran, not the shell that carried it");
  assert.equal(summary.detail, "-la");
  assert.equal(summary.result, "total 12");
  assert.equal(summary.expandable, true);
  // The row is cut by width in CSS, so the summary hands over the whole line
  // and only guards against a runaway blob reaching the DOM.
  const long = mobileEventSummary({ kind: "tool_result", text: "x".repeat(900) }, "结果");
  assert.equal(long.detail.length, 400);
  assert.doesNotMatch(long.detail, /…/, "the ellipsis belongs to CSS, which knows the width");
  assert.equal(long.expandable, true);
  assert.equal(mobileEventSummary({ kind: "tool_result", text: "ok" }, "结果").expandable, false);
  assert.equal(mobileEventSummary({ kind: "reasoning", text: "" }, "思考过程").expandable, false);
});

test("a call and its result become one row, and empty thoughts none at all", () => {
  const folded = foldMobileEvents([
    { kind: "reasoning", text: "" },
    { kind: "tool_use", text: "/bin/zsh -lc 'ls -la'" },
    { kind: "tool_result", text: "total 12" },
    { kind: "tool_use", text: "/bin/zsh -lc 'cat README.md'" },
    { kind: "tool_result", text: "# Modu", failed: true },
    { kind: "message", text: "done" },
  ]);
  assert.deepEqual(folded.map((event) => event.kind), ["tool_use", "tool_use", "message"]);
  assert.equal(folded[0].result, "total 12");
  assert.equal(folded[1].failed, true, "a failed result marks the call it came from");
});
