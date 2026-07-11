import test from "node:test";
import assert from "node:assert/strict";
import { buildRunConversation, readableAgentMessage } from "./runConversation.js";

test("renders outcome JSON as readable content", () => {
  assert.equal(readableAgentMessage('{"signal":"need_human","content":"请授权浏览器后继续"}'), "请授权浏览器后继续");
  assert.equal(readableAgentMessage("普通回复"), "普通回复");
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
  assert.equal(timeline[1].items[0].text, "需要浏览器权限");
  assert.equal(timeline[1].items[1].type, "activity");
  assert.equal(timeline[3].items.filter((item) => item.type === "message").length, 1);
});

test("falls back to StepRun content when no message event exists", () => {
  const [round] = buildRunConversation({
    task: {}, run: {}, events: [], runtimeEvents: [],
    workflow: { steps: [{ id: "review", name: "审查", runtime: "claude" }] },
    stepRuns: [{ id: "step-review", stepId: "review", attempt: 1, status: "succeeded", content: "审查完成" }],
  });
  assert.equal(round.items[0].text, "审查完成");
});
