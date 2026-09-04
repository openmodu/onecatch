export const demoAccountUsage = {
  runtime: "codex",
  scope: "account",
  source: "app-server",
  fetchedAt: new Date().toISOString(),
  dailyUsage: demoDailyUsage(),
  summary: {
    lifetimeTokens: 9_009_894_771,
    peakDailyTokens: 356_078_815,
    longestRunningTurnSec: 68_956,
    currentStreakDays: 40,
    longestStreakDays: 85,
  },
  rateLimits: [
    {
      id: "codex",
      planType: "pro",
      primary: { usedPercent: 42, windowDurationMins: 300, resetsAt: Math.floor(Date.now() / 1000) + 7_200 },
      secondary: { usedPercent: 18, windowDurationMins: 10_080, resetsAt: Math.floor(Date.now() / 1000) + 345_600 },
      credits: { hasCredits: false, unlimited: false, balance: "0" },
    },
    {
      id: "codex_spark",
      name: "GPT-5.3-Codex-Spark",
      planType: "pro",
      primary: { usedPercent: 7, windowDurationMins: 300, resetsAt: Math.floor(Date.now() / 1000) + 14_400 },
      secondary: { usedPercent: 3, windowDurationMins: 10_080, resetsAt: Math.floor(Date.now() / 1000) + 518_400 },
    },
  ],
  resetCredits: { availableCount: 1, credits: [] },
};

const demoRuntimeScale = { codex: 1, claude: 0.58, pi: 0.17, grok: 0.31, modu: 0.24 };

export function demoUsageForRuntime(runtime) {
  const scale = demoRuntimeScale[runtime] || 0.1;
  const dailyUsage = demoAccountUsage.dailyUsage.map((bucket, index) => ({
    ...bucket,
    tokens: Math.round(bucket.tokens * scale * (0.72 + ((index + runtime.length) % 7) * 0.08)),
  }));
  const lifetimeTokens = dailyUsage.reduce((total, bucket) => total + bucket.tokens, 0);
  const peakDailyTokens = Math.max(0, ...dailyUsage.map((bucket) => bucket.tokens));
  return {
    ...demoAccountUsage,
    runtime,
    scope: runtime === "codex" ? "account" : "device",
    source: runtime === "codex" ? "app-server" : "local-sessions",
    dailyUsage,
    summary: { ...demoAccountUsage.summary, lifetimeTokens, peakDailyTokens },
    rateLimits: runtime === "codex" ? demoAccountUsage.rateLimits : [],
    resetCredits: runtime === "codex" ? demoAccountUsage.resetCredits : null,
  };
}

export function clampUsagePercent(value) {
  const number = Number(value);
  if (!Number.isFinite(number)) return 0;
  return Math.min(100, Math.max(0, number));
}

export function usageWindowDuration(minutes) {
  const value = Number(minutes);
  if (!Number.isFinite(value) || value <= 0) return null;
  if (value % 10_080 === 0) return { value: value / 10_080, unit: "weeks" };
  if (value % 1_440 === 0) return { value: value / 1_440, unit: "days" };
  if (value % 60 === 0) return { value: value / 60, unit: "hours" };
  return { value, unit: "minutes" };
}

export function accountRateLimitName(limit = {}) {
  if (String(limit.name || "").trim()) return limit.name;
  if (limit.id === "codex") return "Codex";
  return String(limit.id || "Codex").replaceAll("_", " ");
}

function localDate(value) {
  const match = String(value || "").match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!match) return null;
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  return date.getFullYear() === Number(match[1]) && date.getMonth() === Number(match[2]) - 1 && date.getDate() === Number(match[3]) ? date : null;
}

function dateKey(date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

function addDays(value, count) {
  const date = new Date(value.getFullYear(), value.getMonth(), value.getDate());
  date.setDate(date.getDate() + count);
  return date;
}

function usageByDate(dailyUsage = []) {
  const result = new Map();
  for (const bucket of dailyUsage || []) {
    if (!localDate(bucket?.startDate)) continue;
    const tokens = Math.max(0, Number(bucket?.tokens) || 0);
    result.set(bucket.startDate, (result.get(bucket.startDate) || 0) + tokens);
  }
  return result;
}

function dailyStreaks(dailyUsage = [], today = new Date()) {
  const active = [...usageByDate(dailyUsage).entries()]
    .filter(([, tokens]) => tokens > 0)
    .map(([key]) => localDate(key))
    .filter(Boolean)
    .sort((a, b) => a - b);
  let longest = 0;
  let run = 0;
  let previous = null;
  for (const date of active) {
    run = previous && dateKey(addDays(previous, 1)) === dateKey(date) ? run + 1 : 1;
    longest = Math.max(longest, run);
    previous = date;
  }
  const current = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  const currentStreak = previous && previous >= addDays(current, -1) ? run : 0;
  return { currentStreak, longest };
}

// combineAccountUsage powers the "All" view. It merges only the snapshots the
// caller selected; the UI keeps every source's account/device scope visible so
// the aggregate is not mistaken for a single provider bill.
export function combineAccountUsage(usages = []) {
  const values = usages.filter(Boolean);
  const byDate = new Map();
  for (const usage of values) {
    for (const bucket of usage.dailyUsage || []) {
      if (!localDate(bucket?.startDate)) continue;
      byDate.set(bucket.startDate, (byDate.get(bucket.startDate) || 0) + Math.max(0, Number(bucket.tokens) || 0));
    }
  }
  const dailyUsage = [...byDate.entries()]
    .map(([startDate, tokens]) => ({ startDate, tokens }))
    .sort((a, b) => a.startDate.localeCompare(b.startDate));
  const dailyTotal = dailyUsage.reduce((total, bucket) => total + bucket.tokens, 0);
  const lifetimeTokens = values.reduce((total, usage) => total + Math.max(0, Number(usage.summary?.lifetimeTokens) || 0), 0) || dailyTotal;
  const peakDailyTokens = Math.max(0, ...dailyUsage.map((bucket) => bucket.tokens));
  const { currentStreak, longest } = dailyStreaks(dailyUsage);
  const fetchedAt = values
    .map((usage) => new Date(usage.fetchedAt || 0))
    .filter((date) => Number.isFinite(date.getTime()))
    .sort((a, b) => b - a)[0];
  return {
    runtime: "all",
    scope: "mixed",
    source: "combined",
    fetchedAt: fetchedAt?.toISOString() || "",
    dailyUsage,
    summary: {
      lifetimeTokens,
      peakDailyTokens,
      currentStreakDays: currentStreak,
      longestStreakDays: longest,
    },
    rateLimits: values.flatMap((usage) => (usage.rateLimits || []).map((limit) => ({ ...limit, runtime: usage.runtime }))),
    resetCredits: values.reduce((combined, usage) => {
      const count = Math.max(0, Number(usage.resetCredits?.availableCount) || 0);
      return count ? { availableCount: (combined?.availableCount || 0) + count, credits: [...(combined?.credits || []), ...(usage.resetCredits?.credits || [])] } : combined;
    }, null),
  };
}

function heatThresholds(values) {
  const sorted = values.filter((value) => value > 0).sort((a, b) => a - b);
  if (!sorted.length) return [0, 0, 0, 0];
  const at = (ratio) => sorted[Math.min(sorted.length - 1, Math.floor((sorted.length - 1) * ratio))];
  return [at(0.25), at(0.5), at(0.75), sorted[sorted.length - 1]];
}

function heatLevel(tokens, thresholds) {
  if (tokens <= 0) return 0;
  const index = thresholds.findIndex((threshold) => tokens <= threshold);
  return (index < 0 ? thresholds.length - 1 : index) + 1;
}

// A fixed 53-week grid keeps the chart stable as days arrive and matches the
// calendar shape people already know from contribution heatmaps. Missing days
// are real zeroes; future cells in the current week are marked separately.
export function buildUsageHeatmap(dailyUsage = [], today = new Date()) {
  const current = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  const currentWeek = addDays(current, -current.getDay());
  const start = addDays(currentWeek, -52 * 7);
  const values = usageByDate(dailyUsage);
  const visibleValues = [];
  for (let offset = 0; offset < 53 * 7; offset += 1) {
    const date = addDays(start, offset);
    if (date <= current) visibleValues.push(values.get(dateKey(date)) || 0);
  }
  const thresholds = heatThresholds(visibleValues);
  const weeks = Array.from({ length: 53 }, (_, weekIndex) => {
    const days = Array.from({ length: 7 }, (_, dayIndex) => {
      const date = addDays(start, weekIndex * 7 + dayIndex);
      const tokens = values.get(dateKey(date)) || 0;
      const future = date > current;
      return { date, key: dateKey(date), tokens, future, level: future ? 0 : heatLevel(tokens, thresholds) };
    });
    return { days, monthStart: days.find((day) => day.date.getDate() === 1)?.date || (weekIndex === 0 ? days[0].date : null) };
  });
  return { weeks, thresholds, startDate: start, endDate: current };
}

export function recentDailyUsage(dailyUsage = [], today = new Date(), count = 14) {
  const current = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  const values = usageByDate(dailyUsage);
  return Array.from({ length: Math.max(0, count) }, (_, index) => {
    const date = addDays(current, -index);
    return { date, key: dateKey(date), tokens: values.get(dateKey(date)) || 0 };
  });
}

function demoDailyUsage() {
  const today = new Date();
  return Array.from({ length: 365 }, (_, index) => {
    const date = addDays(today, index - 364);
    const weekday = date.getDay();
    const active = weekday !== 0 || index % 3 === 0;
    const wave = Math.abs(Math.sin(index * 0.43)) * 82_000_000;
    return active ? { startDate: dateKey(date), tokens: Math.round(2_000_000 + wave + (index % 11) * 1_700_000) } : null;
  }).filter(Boolean);
}
