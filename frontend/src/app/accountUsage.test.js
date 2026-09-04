import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { accountRateLimitName, buildUsageHeatmap, clampUsagePercent, combineAccountUsage, recentDailyUsage, usageWindowDuration } from "./accountUsage.js";

test("normalizes provider percentages and rolling window durations", () => {
  assert.equal(clampUsagePercent(-4), 0);
  assert.equal(clampUsagePercent(47), 47);
  assert.equal(clampUsagePercent(140), 100);
  assert.deepEqual(usageWindowDuration(300), { value: 5, unit: "hours" });
  assert.deepEqual(usageWindowDuration(10_080), { value: 1, unit: "weeks" });
  assert.equal(usageWindowDuration(0), null);
});

test("uses provider names with a stable Codex fallback", () => {
  assert.equal(accountRateLimitName({ id: "codex" }), "Codex");
  assert.equal(accountRateLimitName({ id: "codex_spark", name: "GPT Spark" }), "GPT Spark");
  assert.equal(accountRateLimitName({ id: "codex_spark" }), "codex spark");
});

test("builds a stable account-wide calendar with zero and future days", () => {
  const today = new Date(2026, 8, 4);
  const chart = buildUsageHeatmap([
    { startDate: "2026-09-03", tokens: 30 },
    { startDate: "2026-09-02", tokens: 20 },
    { startDate: "2026-09-01", tokens: 10 },
    { startDate: "invalid", tokens: 999 },
  ], today);
  assert.equal(chart.weeks.length, 53);
  assert.equal(chart.weeks.every((week) => week.days.length === 7), true);
  const days = chart.weeks.flatMap((week) => week.days);
  assert.equal(days.find((day) => day.key === "2026-09-03").tokens, 30);
  assert.equal(days.find((day) => day.key === "2026-09-04").level, 0);
  assert.equal(days.find((day) => day.key === "2026-09-05").future, true);
});

test("daily detail fills missing calendar dates with zero usage", () => {
  const rows = recentDailyUsage([{ startDate: "2026-09-03", tokens: 42 }], new Date(2026, 8, 4), 3);
  assert.deepEqual(rows.map(({ key, tokens }) => [key, tokens]), [
    ["2026-09-04", 0],
    ["2026-09-03", 42],
    ["2026-09-02", 0],
  ]);
});

test("combines runtime snapshots without losing provider limits", () => {
  const combined = combineAccountUsage([
    { runtime: "codex", fetchedAt: "2026-09-03T00:00:00Z", dailyUsage: [{ startDate: "2026-09-03", tokens: 30 }], summary: { lifetimeTokens: 100 }, rateLimits: [{ id: "codex" }] },
    { runtime: "claude", fetchedAt: "2026-09-04T00:00:00Z", dailyUsage: [{ startDate: "2026-09-03", tokens: 20 }, { startDate: "2026-09-04", tokens: 10 }], summary: { lifetimeTokens: 50 }, rateLimits: [] },
  ]);
  assert.deepEqual(combined.dailyUsage, [
    { startDate: "2026-09-03", tokens: 50 },
    { startDate: "2026-09-04", tokens: 10 },
  ]);
  assert.equal(combined.summary.lifetimeTokens, 150);
  assert.equal(combined.summary.peakDailyTokens, 50);
  assert.equal(combined.fetchedAt, "2026-09-04T00:00:00.000Z");
  assert.equal(combined.rateLimits[0].runtime, "codex");
});

test("usage page reads every supported runtime and renders every quota bucket", () => {
  const source = readFileSync(new URL("./UsagePage.jsx", import.meta.url), "utf8");
  assert.match(source, /RuntimeBinding\.GetAccountUsage\(runtime\)/);
  assert.match(source, /RuntimeBinding\.SyncAccountUsage\(runtime\)/);
  assert.match(source, /30 \* 60 \* 1000/);
  assert.match(source, /rateLimits\.map/);
  assert.match(source, /combineAccountUsage/);
  assert.match(source, /buildUsageHeatmap/);
  assert.match(source, /recentDailyUsage/);
  assert.match(source, /windowDurationMins/);
  assert.match(source, /resetsAt/);
});
