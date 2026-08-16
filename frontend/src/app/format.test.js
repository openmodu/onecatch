import test from "node:test";
import assert from "node:assert/strict";
import { errorMessage, taskTitleFromPrompt } from "./format.js";

test("worker protocol errors become actionable UI copy", () => {
  const message = errorMessage("worker_workspace_revision_missing: requested revision is unavailable");
  assert.doesNotMatch(message, /^worker_workspace_revision_missing:/);
  assert.match(message, /B|remote/i);
});

test("unknown errors preserve their original detail", () => {
  assert.equal(errorMessage(new Error("custom failure detail")), "custom failure detail");
});

test("task titles are derived from the first meaningful prompt line", () => {
  assert.equal(taskTitleFromPrompt("\n  ## 增加语法高亮\n并支持多个文件"), "增加语法高亮");
  assert.equal(taskTitleFromPrompt("  - Refine the Codex-style composer  "), "Refine the Codex-style composer");
  assert.equal(taskTitleFromPrompt("", "New task"), "New task");
  assert.equal(Array.from(taskTitleFromPrompt("这是一个需要被自动截断成任务标题的很长描述，它包含了超过四十八个字符，并且后面还有更多不需要展示的验收细节。请继续处理。" )).length, 48);
});
