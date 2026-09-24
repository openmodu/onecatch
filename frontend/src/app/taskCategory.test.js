import test from "node:test";
import assert from "node:assert/strict";
import { taskCategory } from "./taskCategory.js";

test("explicit types and task intent classify old and new sessions", () => {
 const examples = [
  ["feat(auth): add login", "feat"], ["fix: 修复登录", "fix"], ["修复侧栏滚动问题", "fix"],
  ["新增 worktree 支持", "feat"], ["重构任务调度", "refactor"], ["降低首屏延迟", "perf"],
  ["补齐单元测试", "test"], ["更新 README 文档", "docs"], ["升级依赖版本", "chore"],
  ["分析 worktree 生命周期", "research"], ["看看这个", "other"],
 ];
 for (const [title, expected] of examples) assert.equal(taskCategory({ title }), expected, title);
 assert.equal(taskCategory({ title: "登录问题", prompt: "修复登录接口的超时" }), "fix");
 assert.equal(taskCategory({ title: "登录问题", worktree: { branch: "hotfix/login" } }), "fix");
 assert.equal(taskCategory({ title: "Review", worktree: { branch: "feat/login" } }), "research");
 assert.equal(taskCategory({ title: "Bugfixes documentation", prompt: "Write docs" }), "docs");
});

test("manual type is durable and automatic mode can be restored", () => {
 assert.equal(taskCategory({ title: "修复崩溃", category: "feat" }), "feat");
 assert.equal(taskCategory({ title: "修复崩溃", category: "other" }), "other");
 assert.equal(taskCategory({ title: "修复崩溃", category: "" }), "fix");
 assert.equal(taskCategory({ title: "修复崩溃", category: "unexpected" }), "fix");
 assert.equal(taskCategory(undefined), "other");
});
