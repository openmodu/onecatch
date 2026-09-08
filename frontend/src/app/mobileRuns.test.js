import assert from "node:assert/strict";
import test from "node:test";
import { mobileRunDetailRecord, mobileTranscriptNotice, needsMobileRunDetail } from "./mobileRuns.js";

test("completed summaries keep requesting a body until detail loading succeeds", () => {
  const summary = { status: "succeeded", finishedAt: "2026-09-08T10:00:00Z" };
  assert.equal(needsMobileRunDetail(summary, undefined), true);
  assert.equal(needsMobileRunDetail(summary, undefined), true, "a failed attempt does not count as loaded");
  const loaded = mobileRunDetailRecord({ ...summary, events: [{ kind: "message" }] });
  assert.equal(needsMobileRunDetail(summary, loaded), false);
  assert.equal(needsMobileRunDetail(summary, { status: "running" }), true);
  assert.equal(needsMobileRunDetail({ ...summary, finishedAt: "2026-09-08T10:01:00Z" }, loaded), true);
  assert.equal(needsMobileRunDetail({ status: "running" }, { status: "running" }), true);
});

test("a detail that came back without a body is asked for again", () => {
  const summary = { status: "succeeded", finishedAt: "2026-09-08T10:00:00Z" };
  // A finished turn whose transcript is missing renders as its prompt and its
  // final message alone, which reads as a two-message conversation.
  let record = mobileRunDetailRecord({ ...summary, events: [] });
  assert.equal(needsMobileRunDetail(summary, record), true);
  record = mobileRunDetailRecord({ ...summary, events: [] }, record);
  record = mobileRunDetailRecord({ ...summary, events: [] }, record);
  assert.equal(needsMobileRunDetail(summary, record), false, "a run that really is empty settles");
  const loaded = mobileRunDetailRecord({ ...summary, events: [{ kind: "message" }] }, record);
  assert.equal(needsMobileRunDetail(summary, loaded), false);
  assert.equal(loaded.attempts, 0, "a body that arrives clears the empty-body count");
});

test("a turn without its body says so instead of showing only head and tail", () => {
  const run = { id: "run_1", status: "succeeded", finishedAt: "2026-09-08T10:00:00Z", events: [] };
  assert.match(mobileTranscriptNotice(run, undefined), /加载/);
  assert.match(mobileTranscriptNotice(run, undefined, true), /失败/);
  assert.equal(mobileTranscriptNotice({ ...run, events: [{ kind: "message" }] }, undefined), "");
  assert.equal(mobileTranscriptNotice({ ...run, status: "running" }, undefined), "", "a running turn already has its own indicator");
  const settled = mobileRunDetailRecord(run, mobileRunDetailRecord(run, mobileRunDetailRecord(run)));
  assert.equal(mobileTranscriptNotice(run, settled), "", "an empty run stops promising more");
});

import { applyMobileRunFrame, applyMobileRunFrames, conversationUsage, describeToolArguments, foldMobileEvents, groupMobileConversations, groupMobileTranscriptEvents, mergeMobileRun, mergeMobileRunSummaries, mobileEventSummary, mobileRunTitle, projectActivity, sortMobileRuns, unwrapShellCommand } from "./mobileRuns.js";

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

test("loaded history replaces changed middle messages even when its ending is unchanged", () => {
  const original = { id: "history", status: "succeeded", events: [
    { kind: "message", text: "first" },
    { kind: "message", text: "" },
    { kind: "message", text: "last" },
  ], result: { finalMessage: "last" } };
  const full = { ...original, events: original.events.map((event, index) => index === 1 ? { ...event, text: "middle reply" } : event) };
  const merged = mergeMobileRun([original], full);
  assert.equal(merged[0].events[1].text, "middle reply");
  assert.equal(mergeMobileRun(merged, structuredClone(full)), merged, "identical history keeps its rendering identity");
});

test("mobile run frames append events and settle the run", () => {
  const run = { id: "run-1", status: "running", events: [] };
  const withEvent = applyMobileRunFrame(run, { runId: "run-1", event: { kind: "message", text: "hello" } });
  assert.equal(withEvent.events[0].text, "hello");
  const settled = applyMobileRunFrame(withEvent, { runId: "run-1", status: "succeeded", result: { finalMessage: "done" } });
  assert.equal(settled.status, "succeeded");
  assert.equal(settled.result.finalMessage, "done");
});

test("mobile run frames render a burst in one immutable list update", () => {
  const idle = { id: "idle", events: [] };
  const active = { id: "active", status: "running", events: [] };
  const next = applyMobileRunFrames([idle, active], [
    { runId: "active", event: { kind: "message", text: "one" } },
    { runId: "active", event: { kind: "message", text: "two" } },
  ]);
  assert.equal(next[0], idle);
  assert.deepEqual(next[1].events.map((event) => event.text), ["one", "two"]);
  assert.equal(applyMobileRunFrames(next, []), next);
});

test("summary polling preserves loaded transcripts and skips unchanged state", () => {
  const loaded = {
    id: "run-1", conversationId: "chat-1", status: "succeeded", startedAt: "2026-09-08T01:00:00Z",
    events: [{ kind: "message", streamId: "answer", revision: 2, text: "loaded reply" }],
    result: { sessionId: "session-1", finalMessage: "loaded reply", succeeded: true },
  };
  const summary = { ...loaded, events: undefined };
  const current = [loaded];
  const unchanged = mergeMobileRunSummaries(current, [summary]);
  assert.equal(unchanged, current);
  assert.equal(unchanged[0], loaded);
  assert.equal(unchanged[0].events[0].text, "loaded reply");

  const changed = mergeMobileRunSummaries(unchanged, [{ ...summary, status: "running" }]);
  assert.notEqual(changed, unchanged);
  assert.equal(changed[0].events, loaded.events);
  assert.equal(changed[0].status, "running");
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

test("a call and its result become one row with elapsed-time boundaries, and empty thoughts none at all", () => {
  const folded = foldMobileEvents([
    { kind: "reasoning", text: "" },
    { kind: "tool_use", text: "/bin/zsh -lc 'ls -la'", at: "2026-09-07T02:00:00.250Z" },
    { kind: "tool_result", text: "total 12", at: "2026-09-07T02:00:02.750Z" },
    { kind: "tool_use", text: "/bin/zsh -lc 'cat README.md'" },
    { kind: "tool_result", text: "# Modu", failed: true },
    { kind: "message", text: "done" },
  ]);
  assert.deepEqual(folded.map((event) => event.kind), ["tool_use", "tool_use", "message"]);
  assert.equal(folded[0].result, "total 12");
  assert.equal(folded[0].at, "2026-09-07T02:00:00.250Z");
  assert.equal(folded[0].finishedAt, "2026-09-07T02:00:02.750Z");
  assert.equal(folded[1].failed, true, "a failed result marks the call it came from");
});

// Codex hands over a shell command, Modu hands over the tool's JSON arguments.
// The row has to read for both without dumping a payload into it.
test("a tool row reads the same whichever harness produced it", () => {
  assert.equal(
    describeToolArguments(`{"command":"curl -s \\"https://wttr.in/Shanghai?format=3\\" --max-time 20"}`),
    `curl -s "https://wttr.in/Shanghai?format=3" --max-time 20`,
  );
  assert.equal(
    describeToolArguments(`{"path":"/Users/ityike/.modu/skills","pattern":"weather|wttr"}`),
    "weather|wttr · ~/.modu/skills",
    "a pattern alone does not say where it was looked for",
  );
  // A shape with no line worth writing keeps the row to its tool name.
  assert.equal(describeToolArguments(`{"questions":[{"header":"城市确认"}]}`), "");
  assert.equal(describeToolArguments("ls -la"), "ls -la");
  assert.equal(describeToolArguments("{not json"), "{not json");
});

// The agent narrates between calling a tool and getting its answer, which left
// the result stranded in a row of its own.
test("a result rejoins its call even when prose came between them", () => {
  const folded = foldMobileEvents([
    { kind: "tool_use", streamId: "call-1", text: `grep {"pattern":"wttr"}` },
    { kind: "message", text: "I don't have a weather skill, so I'll query a service." },
    { kind: "tool_result", streamId: "call-1", text: "no matches", failed: true },
  ]);
  assert.deepEqual(folded.map((event) => event.kind), ["tool_use", "message"]);
  assert.equal(folded[0].result, "no matches");
  assert.equal(folded[0].failed, true);
});

test("adjacent tools share a disclosure without moving prose", () => {
  const blocks = groupMobileTranscriptEvents([
    { kind: "message", text: "先检查" },
    { kind: "tool_use", streamId: "one", text: "rg issue" },
    { kind: "tool_use", streamId: "two", text: "npm test" },
    { kind: "reasoning", text: "测试通过后再检查状态" },
    { kind: "tool_use", streamId: "three", text: "git status" },
  ]);
  assert.deepEqual(blocks.map((block) => block.type), ["event", "tools", "event", "tools"]);
  assert.deepEqual(blocks[1].events.map((event) => event.streamId), ["one", "two"]);
  assert.equal(blocks[2].event.kind, "reasoning");
});

test("Grok session replay is reduced to each turn's new prose and tools", () => {
  const folded = foldMobileEvents([
    { kind: "message", streamId: "step_one:grok-message", phase: "end", text: "Hello." },
    { kind: "user_message", text: "weather" },
    { kind: "tool_use", streamId: "step_two:grok-tool-search-1", text: "Web search: weather" },
    { kind: "tool_result", streamId: "step_two:grok-tool-search-1", text: "sunny" },
    { kind: "message", streamId: "step_two:grok-message", phase: "end", text: "Hello.Weather answer." },
    { kind: "user_message", text: "why" },
    { kind: "tool_use", streamId: "step_three:grok-tool-search-1", text: "Web search: weather" },
    { kind: "message", streamId: "step_three:grok-message", phase: "end", text: "Hello.Weather answer.Why answer." },
  ]);
  assert.deepEqual(folded.map((event) => [event.kind, event.text]), [
    ["message", "Hello."],
    ["user_message", "weather"],
    ["tool_use", "Web search: weather"],
    ["message", "Weather answer."],
    ["user_message", "why"],
    ["message", "Why answer."],
  ]);
  assert.equal(folded[2].result, "sunny");
});

// Modu ends a run with a `result` event carrying the same prose as the final
// reply, so the transcript printed the answer twice.
test("the terminal result event does not repeat the answer", () => {
  const folded = foldMobileEvents([
    { kind: "message", text: "I'll query a public weather service." },
    { kind: "result", text: "I'll query a public weather service." },
  ]);
  assert.deepEqual(folded.map((event) => event.kind), ["message"]);
});

// The numbers ride on the usage events' structured fields, which is why the
// transcript's old "usage" row — built from event text — opened onto nothing.
test("conversationUsage adds up a conversation's tokens and its context", () => {
  const usage = conversationUsage([
    { result: { usage: { inputTokens: 18976, cachedInputTokens: 18816, outputTokens: 37 } },
      events: [{ kind: "usage", context: { window: 258400, tokens: 18976 } }] },
    { result: { usage: { inputTokens: 94346, cachedInputTokens: 83200, outputTokens: 826 } },
      events: [{ kind: "usage", context: { window: 258400, tokens: 29421 } }] },
  ]);
  assert.equal(usage.total, 18976 + 37 + 94346 + 826);
  assert.equal(usage.cached, 18816 + 83200);
  assert.deepEqual(usage.context, { window: 258400, tokens: 29421 }, "the newest reading wins");
});

test("a run still in flight counts from its latest reading", () => {
  const usage = conversationUsage([
    { events: [
      { kind: "usage", usage: { inputTokens: 100, outputTokens: 5 } },
      { kind: "usage", usage: { inputTokens: 900, outputTokens: 40 } },
    ] },
  ]);
  assert.equal(usage.total, 940, "a run with no result yet still reports what it has spent");
  assert.deepEqual(conversationUsage([]), { input: 0, output: 0, cached: 0, total: 0, context: null });
});

test("shared history keeps more than one page and desktop titles", () => {
  const runs = Array.from({ length: 125 }, (_, index) => ({ id: `run-${index}`, conversationId: `task-${index}`, startedAt: new Date(index * 1000).toISOString(), prompt: "original", title: `Renamed ${index}`, shared: true }));
  const merged = mergeMobileRun(runs, { ...runs[124], status: "succeeded" });
  assert.equal(merged.length, 125);
  assert.equal(groupMobileConversations(merged)[0].title, "Renamed 124");
});

test("shared snapshots keep tool calls paired across repeated step stream IDs", () => {
  const events = foldMobileEvents([
    { kind: "tool_use", streamId: "step1:call", text: "bash pwd" },
    { kind: "tool_result", streamId: "step1:call", phase: "end", text: "/workspace" },
    { kind: "user_message", text: "continue" },
    { kind: "tool_use", streamId: "step2:call", text: "bash ls" },
    { kind: "tool_result", streamId: "step2:call", phase: "end", text: "README.md" },
  ]);
  assert.deepEqual(events.map(({ kind, text, result }) => [kind, text, result]), [
    ["tool_use", "bash pwd", "/workspace"], ["user_message", "continue", undefined], ["tool_use", "bash ls", "README.md"],
  ]);
});

test("a permission answered on either device no longer asks again", () => {
  const events = foldMobileEvents([
    { kind: "permission_request", permission: { id: "permission-1" } },
    { kind: "permission_request", permission: { id: "permission-2" } },
    { kind: "permission_resolved", permission: { id: "permission-1" }, permissionDecision: "allow" },
  ]);
  assert.deepEqual(events.map((event) => event.permission.id), ["permission-2"]);
});
