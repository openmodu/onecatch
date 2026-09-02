import test from "node:test";
import assert from "node:assert/strict";
import { buildRunConversation, groupRoundItems, readableAgentMessage, readableToolTitle, reasoningSummary, streamingOutcomeContent } from "./runConversation.js";

test("renders outcome JSON as readable content", () => {
  assert.equal(readableAgentMessage('{"signal":"need_human","content":"请授权浏览器后继续"}'), "请授权浏览器后继续");
  assert.equal(readableAgentMessage('```json\n{"signal":"need_human","content":"请授权浏览器后继续"}\n```'), "请授权浏览器后继续");
  assert.equal(readableAgentMessage('```\n{"signal":"approved","content":"已完成"}\n```'), "已完成");
  assert.equal(readableAgentMessage("```json\nnot json\n```"), "```json\nnot json\n```");
  assert.equal(readableAgentMessage("普通回复"), "普通回复");
});

test("renders a terminal outcome after provider prose as readable content", () => {
  assert.equal(
    readableAgentMessage('所有测试通过，工作区干净。\n\n{"signal":"completed","content":"提交成功"}'),
    "提交成功",
  );
});

test("projects content from an incomplete streamed outcome envelope", () => {
  assert.equal(streamingOutcomeContent('{"signal":"changes_requested","cont'), "");
  assert.equal(streamingOutcomeContent('{"signal":"changes_requested","content":"正在**修'), "正在**修");
  assert.equal(streamingOutcomeContent('```json\n{"signal":"approved","content":"第一行\\n第二行\\u4e2d'), "第一行\n第二行中");
  assert.equal(streamingOutcomeContent("普通 **Markdown**"), null);
  assert.equal(readableAgentMessage('{"signal":"approved","content":"流式内容'), "{\"signal\":\"approved\",\"content\":\"流式内容");
  assert.equal(readableAgentMessage('{"signal":"approved","content":"流式内容', true), "流式内容");
});

test("strips the launcher shell from a tool title", () => {
  assert.equal(readableToolTitle(`/etc/profiles/per-user/me/bin/zsh -lc 'npm run check && npm test'`), "运行 npm run check && npm test");
  assert.equal(readableToolTitle("go test ./..."), "go test ./...");
  assert.equal(readableToolTitle(`/bin/zsh -lc "sed -n '1,240p' '/tmp/runConversation.js'"`), "读取 runConversation.js");
  assert.equal(readableToolTitle("git diff --check"), "检查 git diff --check");
});

test("builds user messages and two ordered agent rounds", () => {
  const detail = {
    task: { prompt: "生成并验证视频", createdAt: "2026-07-11T10:00:00Z" },
    run: { startedAt: "2026-07-11T10:00:01Z" },
    workflow: { steps: [{ id: "execute", name: "执行", runtime: "codex" }] },
    events: [{ seq: 4, type: "run.resumed", payload: '{"instruction":"权限已开启，请继续"}', at: "2026-07-11T10:05:00Z" }],
    stepRuns: [
      { id: "step-1", stepId: "execute", attempt: 1, status: "succeeded", startedAt: "2026-07-11T10:00:02Z" },
      { id: "step-2", stepId: "execute", attempt: 2, status: "succeeded", startedAt: "2026-07-11T10:05:01Z" },
    ],
    runtimeEvents: [
      { stepRunId: "step-1", seq: 1, kind: "tool_use", text: "npm run render", at: "2026-07-11T10:00:03Z" },
      { stepRunId: "step-1", seq: 2, kind: "message", text: '{"signal":"need_human","content":"需要浏览器权限"}', at: "2026-07-11T10:00:04Z" },
      { stepRunId: "step-2", seq: 1, kind: "message", text: "继续渲染", at: "2026-07-11T10:05:02Z" },
      { stepRunId: "step-2", seq: 2, kind: "result", text: "继续渲染", at: "2026-07-11T10:05:03Z" },
    ],
  };
  const timeline = buildRunConversation(detail);
  assert.deepEqual(timeline.map((item) => item.type), ["user", "round", "user", "round"]);
  assert.deepEqual(timeline[1].items.map((item) => item.type), ["tool", "message"]);
  assert.equal(timeline[1].items[0].text, "npm run render");
  assert.equal(timeline[1].items[1].text, "需要浏览器权限");
  assert.equal(timeline[3].items.filter((item) => item.type === "message").length, 1);
});

test("falls back to task update time for the initial user message", () => {
  const [message] = buildRunConversation({
    task: { prompt: "继续处理", updatedAt: "2026-07-11T10:00:05Z" },
    run: {}, workflow: { steps: [] }, events: [], stepRuns: [], runtimeEvents: [],
  });
  assert.equal(message.at, "2026-07-11T10:00:05Z");
});

test("shows applied queued instructions as user turns", () => {
  const timeline = buildRunConversation({
    task: { prompt: "先检查问题", createdAt: "2026-07-11T10:00:00Z" },
    run: {}, workflow: { steps: [] }, events: [], stepRuns: [], runtimeEvents: [],
    instructions: [
      { id: "pending", status: "pending", content: "还没执行", createdAt: "2026-07-11T10:01:00Z" },
      { id: "applied", status: "applied", content: "优先修复测试", appliedAt: "2026-07-11T10:02:00Z" },
    ],
  });
  assert.deepEqual(timeline.map((item) => item.text), ["先检查问题", "优先修复测试"]);
});

test("keeps tool calls separate and attaches an adjacent tool result", () => {
  const [round] = buildRunConversation({
    task: {}, run: {}, events: [],
    workflow: { steps: [{ id: "execute", name: "执行", runtime: "codex" }] },
    stepRuns: [{ id: "step-1", stepId: "execute", status: "succeeded" }],
    runtimeEvents: [
      { stepRunId: "step-1", seq: 1, kind: "message", text: "开始检查" },
      { stepRunId: "step-1", seq: 2, kind: "tool_use", text: "rg -n pause internal" },
      { stepRunId: "step-1", seq: 3, kind: "tool_result", text: "3 matches" },
      { stepRunId: "step-1", seq: 4, kind: "tool_use", text: "npm test" },
      { stepRunId: "step-1", seq: 5, kind: "message", text: "检查完成" },
    ],
  });
  assert.deepEqual(round.items.map((item) => item.type), ["message", "tool", "tool", "message"]);
  assert.equal(round.items[1].details[0].text, "3 matches");
  assert.equal(round.items[2].details.length, 0);
});

test("keeps tool start and finish timestamps for elapsed-time display", () => {
  const [round] = buildRunConversation({
    task: {}, run: {}, events: [],
    workflow: { steps: [{ id: "execute", name: "执行", runtime: "codex" }] },
    stepRuns: [{ id: "step-1", stepId: "execute", status: "succeeded" }],
    runtimeEvents: [
      { stepRunId: "step-1", seq: 1, kind: "tool_use", streamId: "call-1", text: "npm test", at: "2026-07-11T10:00:03.250Z" },
      { stepRunId: "step-1", seq: 2, kind: "tool_result", streamId: "call-1", text: "ok", at: "2026-07-11T10:00:05.750Z" },
    ],
  });
  assert.equal(round.items[0].at, "2026-07-11T10:00:03.250Z");
  assert.equal(round.items[0].finishedAt, "2026-07-11T10:00:05.750Z");
});

test("groups only adjacent process rows and preserves text-tool-text order", () => {
  const blocks = groupRoundItems([
    { type: "message", id: "intro", text: "先检查" },
    { type: "tool", id: "search", kind: "tool_use", text: "rg issue" },
    { type: "tool", id: "test", kind: "tool_use", text: "npm test" },
    { type: "message", id: "result", text: "检查完成" },
    { type: "tool", id: "status", kind: "tool_use", text: "git status" },
  ]);
  assert.deepEqual(blocks.map((block) => block.type), ["message", "process", "message", "process"]);
  assert.deepEqual(blocks[1].items.map((item) => item.id), ["search", "test"]);
  assert.deepEqual(blocks[3].items.map((item) => item.id), ["status"]);
});

test("scopes failure to the tool that failed, not the whole failed step", () => {
  const [round] = buildRunConversation({
    task: {}, run: {}, events: [],
    workflow: { steps: [{ id: "execute", name: "执行", runtime: "claude" }] },
    stepRuns: [{ id: "step-1", stepId: "execute", status: "failed" }],
    runtimeEvents: [
      { stepRunId: "step-1", seq: 1, kind: "tool_use", text: "git status --short" },
      { stepRunId: "step-1", seq: 2, kind: "tool_result", text: "M App.jsx" },
      { stepRunId: "step-1", seq: 3, kind: "tool_use", text: "Read missing.js" },
      { stepRunId: "step-1", seq: 4, kind: "tool_result", text: "File does not exist.", failed: true },
      { stepRunId: "step-1", seq: 5, kind: "tool_use", text: "npm test" },
    ],
  });
  const [ok, bad, cutOff] = round.items;
  assert.equal(ok.failed, false, "a tool that returned a result did not fail");
  assert.equal(ok.settled, true);
  assert.equal(bad.failed, true, "the tool whose result carried is_error failed");
  assert.equal(cutOff.failed, false, "a tool the runtime never answered is unfinished, not failed");
  assert.equal(cutOff.settled, false);
});

test("a result with no output still settles its tool and carries its failure", () => {
  const [round] = buildRunConversation({
    task: {}, run: {}, events: [],
    workflow: { steps: [{ id: "execute", name: "执行", runtime: "codex" }] },
    stepRuns: [{ id: "step-1", stepId: "execute", status: "failed" }],
    runtimeEvents: [
      { stepRunId: "step-1", seq: 1, kind: "tool_use", text: "mkdir build" },
      { stepRunId: "step-1", seq: 2, kind: "tool_result", text: "" },
      { stepRunId: "step-1", seq: 3, kind: "tool_use", text: "false" },
      { stepRunId: "step-1", seq: 4, kind: "tool_result", text: "", failed: true },
    ],
  });
  const [silentOk, silentFail] = round.items;
  // Empty output must not drop the result: the command still finished.
  assert.equal(silentOk.settled, true);
  assert.equal(silentOk.failed, false);
  assert.equal(silentOk.details.length, 0, "no empty RESULT block is added");
  // A non-zero exit with no output must keep its failure signal.
  assert.equal(silentFail.settled, true);
  assert.equal(silentFail.failed, true);
});

test("keeps a streaming tool result unsettled until its end frame", () => {
  const [round] = buildRunConversation({
    task: {}, run: {}, events: [],
    workflow: { steps: [{ id: "execute", name: "执行", runtime: "codex" }] },
    stepRuns: [{ id: "step-1", stepId: "execute", status: "running" }],
    runtimeEvents: [
      { stepRunId: "step-1", seq: 1, kind: "tool_use", text: "go test ./..." },
      { stepRunId: "step-1", seq: 2, kind: "tool_result", streamId: "output-1", revision: 3, streaming: true, text: "ok package/a" },
    ],
  });
  assert.equal(round.items[0].settled, false);
  assert.equal(round.items[0].details[0].text, "ok package/a");
});

test("settles every call in a parallel batch by id, not just the last started", () => {
  // A parallel tool batch emits all its starts before any of its ends, and the
  // ends arrive out of start order. Pairing by "most recent tool_use" would
  // settle only the last call; pairing by streamId settles each on its own.
  const [round] = buildRunConversation({
    task: {}, run: {}, events: [],
    workflow: { steps: [{ id: "execute", name: "执行", runtime: "modu" }] },
    stepRuns: [{ id: "step-1", stepId: "execute", status: "succeeded" }],
    runtimeEvents: [
      { stepRunId: "step-1", seq: 1, kind: "tool_use", streamId: "call-a", text: "read git-commit/SKILL.md" },
      { stepRunId: "step-1", seq: 2, kind: "tool_use", streamId: "call-b", text: "read git-push/SKILL.md" },
      { stepRunId: "step-1", seq: 3, kind: "tool_use", streamId: "call-c", text: "read git-branch/SKILL.md" },
      { stepRunId: "step-1", seq: 4, kind: "tool_result", streamId: "call-c", text: "branch skill" },
      { stepRunId: "step-1", seq: 5, kind: "tool_result", streamId: "call-a", text: "commit skill" },
      { stepRunId: "step-1", seq: 6, kind: "tool_result", streamId: "call-b", text: "push skill" },
    ],
  });
  const [a, b, c] = round.items;
  assert.equal(a.settled, true, "the first-started call settles from its own result");
  assert.equal(b.settled, true, "the middle call settles from its own result");
  assert.equal(c.settled, true);
  assert.equal(a.details[0].text, "commit skill", "each result lands on its own call");
  assert.equal(b.details[0].text, "push skill");
  assert.equal(c.details[0].text, "branch skill");
});

test("falls back to StepRun content when no message event exists", () => {
  const [round] = buildRunConversation({
    task: {}, run: {}, events: [], runtimeEvents: [],
    workflow: { steps: [{ id: "review", name: "审查", runtime: "claude" }] },
    stepRuns: [{ id: "step-review", stepId: "review", attempt: 1, status: "succeeded", content: "审查完成" }],
  });
  assert.equal(round.items[0].text, "审查完成");
});

test("keeps an interactive permission card and applies its decision", () => {
  const detail = {
    task: { prompt: "research", createdAt: "2026-07-11T10:00:00Z" },
    run: { id: "run-1" },
    workflow: { steps: [{ id: "review", name: "Review", runtime: "claude" }] },
    stepRuns: [{ id: "step-1", stepId: "review", status: "running", attempt: 1, startedAt: "2026-07-11T10:00:01Z" }],
    runtimeEvents: [
      { stepRunId: "step-1", seq: 1, kind: "permission_request", permission: { id: "p1", toolName: "WebFetch", input: { url: "https://v3.wails.io" } }, at: "2026-07-11T10:00:02Z" },
      { stepRunId: "step-1", seq: 2, kind: "permission_resolved", permission: { id: "p1", toolName: "WebFetch" }, permissionDecision: "allow", at: "2026-07-11T10:00:03Z" },
    ],
  };
  const conversation = buildRunConversation(detail);
  const permission = conversation.find((item) => item.type === "round").items[0];
  assert.equal(permission.type, "permission");
  assert.equal(permission.request.toolName, "WebFetch");
  assert.equal(permission.decision, "allow");
});

test("keeps a thought as prose instead of reading it as a command", () => {
  const [round] = buildRunConversation({
    task: {}, run: {}, events: [],
    workflow: { steps: [{ id: "execute", name: "执行", runtime: "pi" }] },
    stepRuns: [{ id: "step-1", stepId: "execute", status: "succeeded" }],
    runtimeEvents: [
      { stepRunId: "step-1", seq: 1, kind: "reasoning", text: "find the config file first,\nthen read it." },
      { stepRunId: "step-1", seq: 2, kind: "tool_use", text: "rg -n config internal" },
    ],
  });
  // readableToolTitle would turn the thought into a `find` invocation.
  assert.equal(round.items[0].title, "find the config file first, then read it.");
  assert.equal(round.items[0].text, "find the config file first,\nthen read it.");
  assert.equal(round.items[1].title, "搜索 rg -n config internal");
});

test("trims a long thought preview and drops its markdown lead-in", () => {
  assert.equal(reasoningSummary("## **Checking** the plan"), "Checking the plan");
  assert.equal(reasoningSummary("  "), "");
  const long = reasoningSummary("x".repeat(400));
  assert.equal(long.length, 160);
  assert.ok(long.endsWith("…"));
});
