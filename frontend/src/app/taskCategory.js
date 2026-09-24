export const TASK_CATEGORIES = ["feat", "fix", "refactor", "perf", "docs", "test", "chore", "research", "other"];
const explicitType = /^(?:\s*(?:codex|onecatch)\/)?\s*(feat(?:ure)?|fix|bugfix|hotfix|refactor|perf|docs?|test|chore|research)(?:\([^)]*\))?!?(?=[:/\s-])/i;
const aliases = { feature: "feat", bugfix: "fix", hotfix: "fix", doc: "docs" };
const intentRules = [
  ["fix", /修复|修正|排错|解决.{0,12}(?:报错|崩溃|异常|故障)|\b(?:fix|debug|resolve\s+(?:a\s+)?(?:bug|error|crash))\b/i],
  ["refactor", /重构|代码整理|\brefactor\b/i],
  ["perf", /性能|提速|降低.{0,8}(?:延迟|内存|耗时)|\b(?:performance|latency|speed\s+up|optimi[sz]e)\b/i],
  ["test", /(?:添加|增加|编写|补充|补齐|完善).{0,8}测试|测试覆盖|\b(?:add|write|expand)\b.{0,20}\btests?\b/i],
  ["docs", /文档|说明书|\b(?:documentation|readme|docs)\b/i],
  ["research", /调研|研究|分析|解释|评估|审查|\b(?:research|investigate|explain|analy[sz]e|review)\b/i],
  ["chore", /依赖|构建|发布|升级|清理|\b(?:dependencies|release|upgrade|cleanup|ci\/cd)\b/i],
  ["feat", /新增|添加|增加|实现|支持|开发|接入|接通|补齐|改进|优化|\b(?:add|implement|introduce|support|build\s+a|create)\b/i],
];

// Auto classification is deliberately local and conservative. It never starts
// an extra agent or writes back over the user's manual choice.
export function taskCategory(task) {
  if (TASK_CATEGORIES.includes(task?.category)) return task.category;
  const title = String(task?.title || "");
  const request = String(task?.prompt || "").trim().split(/\r?\n/, 1)[0].slice(0, 500);
  for (const text of [title, request]) {
    const match = text.match(explicitType);
    if (match) return aliases[match[1].toLowerCase()] || match[1].toLowerCase();
  }
  for (const text of [title, request]) {
    for (const [category, rule] of intentRules) if (rule.test(text)) return category;
  }
  const branchType = String(task?.worktree?.branch || "").match(explicitType);
  return branchType ? aliases[branchType[1].toLowerCase()] || branchType[1].toLowerCase() : "other";
}
