import test from "node:test";
import assert from "node:assert/strict";
import { errorMessage, formatDateTime, formatMessageDateTime, formatTime, formatToolTime, taskTitleFromPrompt } from "./format.js";

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

function atClock(days = 0, years = 0) {
  const now = new Date();
  return new Date(now.getFullYear() - years, now.getMonth(), now.getDate() - days, 14, 2, 3);
}

test("a timestamp carries its date as soon as it is no longer today's", () => {
  // Asserted against each other rather than against a literal, so the test
  // stays true in whichever locale the suite happens to resolve.
  const today = formatTime(atClock(0));
  const earlier = formatTime(atClock(3));
  const lastYear = formatTime(atClock(3, 1));

  assert.match(today, /\d/);
  assert.ok(earlier.endsWith(today), `${earlier} should end with the same clock reading as ${today}`);
  assert.ok(earlier.length > today.length, "an older row must say which day it was");
  assert.ok(earlier.includes(String(atClock(3).getDate())), "the day of the month belongs in the reading");
});

test("the year appears only once it stops being this one", () => {
  const thisYear = String(new Date().getFullYear());
  assert.ok(!formatTime(atClock(3)).includes(thisYear), "this year's rows do not spend width repeating it");
  assert.ok(formatTime(atClock(3, 1)).includes(String(atClock(3, 1).getFullYear())));
});

test("the unabbreviated stamp always answers with a full date", () => {
  const full = formatDateTime(atClock(0));
  assert.ok(full.includes(String(new Date().getFullYear())));
  assert.ok(full.endsWith(formatTime(atClock(0))));
});

test("message hover timestamps always carry a compact calendar date", () => {
  const today = atClock(0);
  const lastYear = atClock(3, 1);
  const day = (date) => `${String(date.getMonth() + 1).padStart(2, "0")}/${String(date.getDate()).padStart(2, "0")}`;
  assert.equal(formatMessageDateTime(today), `${day(today)} 14:02`);
  assert.equal(formatMessageDateTime(lastYear), `${lastYear.getFullYear()}/${day(lastYear)} 14:02`);
});

test("tool timestamps stay compact while their title carries the full date", () => {
  assert.equal(formatToolTime(atClock(3)), "14:02");
});

test("a missing or unparseable timestamp reads as absent, not as the epoch", () => {
  assert.equal(formatTime(""), "—");
  assert.equal(formatTime("not a date"), "—");
  assert.equal(formatDateTime(null), "—");
  assert.equal(formatDateTime("not a date"), "—");
  assert.equal(formatMessageDateTime("not a date"), "—");
  assert.equal(formatToolTime("not a date"), "—");
});
