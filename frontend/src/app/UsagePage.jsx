import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Activity, AlertCircle, CalendarDays, Clock3, Flame, Gauge, RefreshCw, RotateCcw, WalletCards } from "lucide-react";
import { Button } from "@/components/ui/button";
import { RuntimeBinding } from "../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { accountRateLimitName, buildUsageHeatmap, clampUsagePercent, combineAccountUsage, demoUsageForRuntime, recentDailyUsage, usageWindowDuration } from "./accountUsage.js";
import { errorMessage, formatDateTime, formatTokens } from "./format.js";
import RuntimeHarnessIcon from "./components/RuntimeHarnessIcon.jsx";

const heatTones = ["bg-muted/70", "bg-primary/20", "bg-primary/40", "bg-primary/60", "bg-primary"];
const automaticSyncInterval = 30 * 60 * 1000;
const usageRuntimeIDs = ["codex", "claude", "pi", "grok", "modu"];
const runtimeFallbackNames = { codex: "Codex", claude: "Claude Code", pi: "Pi", grok: "Grok Build", modu: "Modu" };

function calendarDate(date, language, options = {}) {
  return date.toLocaleDateString(language === "en" ? "en-US" : "zh-CN", options);
}

function durationLabel(minutes, t) {
  const duration = usageWindowDuration(minutes);
  return duration ? t(`usage.duration.${duration.unit}`, { count: duration.value }) : t("usage.rollingWindow");
}

function progressTone(percent) {
  if (percent >= 90) return "bg-destructive";
  if (percent >= 75) return "bg-warning";
  return "bg-primary";
}

function RateWindow({ value, t }) {
  const percent = clampUsagePercent(value?.usedPercent);
  const reset = Number(value?.resetsAt) > 0 ? formatDateTime(new Date(Number(value.resetsAt) * 1000)) : t("usage.resetUnknown");
  return <section className="rounded-lg border border-border/70 bg-background/60 p-4">
    <div className="flex items-start justify-between gap-4">
      <div><span className="text-xs font-medium text-muted-foreground">{durationLabel(value?.windowDurationMins, t)}</span><strong className="mt-1 block text-2xl font-semibold tracking-tight text-foreground">{t("usage.percentUsed", { percent })}</strong></div>
      <span className="rounded-md bg-muted px-2 py-1 text-xs font-medium text-muted-foreground">{t("usage.percentRemaining", { percent: 100 - percent })}</span>
    </div>
    <div className="mt-4 h-2 overflow-hidden rounded-full bg-muted" role="progressbar" aria-label={t("usage.windowProgress", { percent })} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}>
      <i className={`block h-full rounded-full transition-[width] ${progressTone(percent)}`} style={{ width: `${percent}%` }} />
    </div>
    <p className="mt-3 mb-0 flex items-center gap-1.5 text-xs text-muted-foreground"><Clock3 size={13} aria-hidden="true" />{t("usage.resetsAt", { time: reset })}</p>
  </section>;
}

function Credits({ value, t }) {
  if (!value) return null;
  const copy = value.unlimited ? t("usage.creditsUnlimited") : value.hasCredits ? t("usage.creditsBalance", { balance: value.balance || "—" }) : t("usage.creditsNone");
  return <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground"><WalletCards size={13} aria-hidden="true" />{copy}</span>;
}

function LimitCard({ value, runtime, t }) {
  const windows = [value.primary, value.secondary].filter(Boolean);
  return <article className="rounded-xl border bg-card p-5 shadow-xs">
    <header className="flex items-start justify-between gap-4">
      <div className="flex min-w-0 items-start gap-2.5">{runtime && <span className="mt-0.5 grid size-7 shrink-0 place-items-center rounded-md bg-muted"><RuntimeHarnessIcon harness={runtime} size={15} /></span>}<div className="min-w-0"><h2 className="m-0 truncate text-base font-semibold text-foreground">{accountRateLimitName(value)}</h2><p className="mt-1 mb-0 font-mono text-[11px] text-muted-foreground">{value.id}</p></div></div>
      {value.planType && <span className="shrink-0 rounded-md border bg-muted/50 px-2 py-1 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">{value.planType}</span>}
    </header>
    {windows.length > 0 ? <div className="mt-4 grid gap-3 md:grid-cols-2">{windows.map((window, index) => <RateWindow value={window} t={t} key={`${window.windowDurationMins || "window"}-${index}`} />)}</div> : <p className="mt-4 mb-0 rounded-lg bg-muted p-3 text-xs text-muted-foreground">{t("usage.noWindows")}</p>}
    {(value.credits || value.individualLimit) && <footer className="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2 border-t border-border/70 pt-4"><Credits value={value.credits} t={t} />{value.individualLimit && <span className="text-xs text-muted-foreground">{t("usage.spend", { used: value.individualLimit.used, limit: value.individualLimit.limit })}</span>}</footer>}
  </article>;
}

function MetricCard({ icon: Icon, label, value, suffix = "" }) {
  return <article className="rounded-xl border bg-card px-4 py-3.5 shadow-xs">
    <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground"><Icon size={14} aria-hidden="true" />{label}</div>
    <strong className="mt-2 block truncate text-xl font-semibold tracking-tight text-foreground" title={String(value ?? "—")}>{value == null ? "—" : `${formatTokens(value)}${suffix}`}</strong>
  </article>;
}

function AccountSummary({ value = {}, t }) {
  return <section className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-label={t("usage.summary")}>
    <MetricCard icon={Activity} label={t("usage.lifetimeTokens")} value={value.lifetimeTokens} />
    <MetricCard icon={Gauge} label={t("usage.peakDailyTokens")} value={value.peakDailyTokens} />
    <MetricCard icon={Flame} label={t("usage.currentStreak")} value={value.currentStreakDays} suffix={t("usage.daySuffix")} />
    <MetricCard icon={CalendarDays} label={t("usage.longestStreak")} value={value.longestStreakDays} suffix={t("usage.daySuffix")} />
  </section>;
}

function UsageHeatmap({ dailyUsage, description, t, language }) {
  const heatmap = useMemo(() => buildUsageHeatmap(dailyUsage), [dailyUsage]);
  const cells = heatmap.weeks.flatMap((week) => week.days);
  const gridStyle = { gridTemplateColumns: "repeat(53, 0.75rem)" };
  return <section className="mt-5 rounded-xl border bg-card p-5 shadow-xs" aria-labelledby="usage-heatmap-title">
    <header className="flex flex-wrap items-start justify-between gap-3">
      <div><h2 id="usage-heatmap-title" className="m-0 text-base font-semibold text-foreground">{t("usage.heatmapTitle")}</h2><p className="mt-1 mb-0 text-xs leading-relaxed text-muted-foreground">{description}</p></div>
      <span className="rounded-md bg-muted px-2.5 py-1 text-xs font-medium text-muted-foreground">{t("usage.activeDays", { count: dailyUsage.filter((item) => Number(item.tokens) > 0).length })}</span>
    </header>
    <div className="mt-5 overflow-x-auto pb-2">
      <div className="min-w-[880px]">
        <div className="ml-8 grid gap-1 text-[10px] text-muted-foreground" style={gridStyle} aria-hidden="true">
          {heatmap.weeks.map((week, index) => <span className="overflow-visible whitespace-nowrap" key={index}>{week.monthStart ? calendarDate(week.monthStart, language, { month: "short" }) : ""}</span>)}
        </div>
        <div className="mt-1.5 flex gap-2">
          <div className="grid w-6 shrink-0 grid-rows-7 gap-1 text-[10px] leading-3 text-muted-foreground" aria-hidden="true"><span /><span>{t("usage.weekday.mon")}</span><span /><span>{t("usage.weekday.wed")}</span><span /><span>{t("usage.weekday.fri")}</span><span /></div>
          <div className="grid grid-flow-col grid-rows-7 gap-1" style={gridStyle} role="grid" aria-label={t("usage.heatmapTitle")}>
            {cells.map((day) => {
              const label = t("usage.heatmapCell", { date: calendarDate(day.date, language, { year: "numeric", month: "short", day: "numeric" }), tokens: formatTokens(day.tokens) });
              return <span className={`size-3 rounded-[3px] ${day.future ? "bg-transparent" : heatTones[day.level]}`} role="gridcell" aria-label={label} title={label} key={day.key} />;
            })}
          </div>
        </div>
      </div>
    </div>
    <footer className="mt-2 flex items-center justify-end gap-1.5 text-[10px] text-muted-foreground"><span>{t("usage.less")}</span>{heatTones.map((tone, index) => <i className={`size-3 rounded-[3px] ${tone}`} key={index} aria-hidden="true" />)}<span>{t("usage.more")}</span></footer>
  </section>;
}

function DailyUsage({ dailyUsage, t, language }) {
  const rows = useMemo(() => recentDailyUsage(dailyUsage), [dailyUsage]);
  const peak = Math.max(1, ...rows.map((row) => row.tokens));
  return <section className="mt-5 rounded-xl border bg-card p-5 shadow-xs" aria-labelledby="daily-usage-title">
    <header><h2 id="daily-usage-title" className="m-0 text-base font-semibold text-foreground">{t("usage.dailyTitle")}</h2><p className="mt-1 mb-0 text-xs leading-relaxed text-muted-foreground">{t("usage.dailyDescription")}</p></header>
    <div className="mt-4 grid gap-x-7 lg:grid-cols-2">
      {rows.map((row, index) => <div className={`grid grid-cols-[minmax(6rem,auto)_minmax(5rem,1fr)_auto] items-center gap-3 border-t border-border/60 py-2.5 first:border-t-0 ${index === 1 ? "lg:border-t-0" : ""}`} key={row.key}>
        <time className="text-xs font-medium text-foreground" dateTime={row.key}>{calendarDate(row.date, language, { month: "short", day: "numeric", weekday: "short" })}</time>
        <span className="h-1.5 overflow-hidden rounded-full bg-muted"><i className="block h-full rounded-full bg-primary/70" style={{ width: `${row.tokens > 0 ? Math.max(3, (row.tokens / peak) * 100) : 0}%` }} /></span>
        <strong className={`min-w-20 text-right text-xs tabular-nums ${row.tokens > 0 ? "text-foreground" : "font-normal text-muted-foreground"}`}>{formatTokens(row.tokens)}</strong>
      </div>)}
    </div>
  </section>;
}

function runtimeName(runtime, runtimes = []) {
  return runtimes.find((item) => item.id === runtime)?.name || runtimeFallbackNames[runtime] || runtime;
}

function RuntimeSourcePicker({ selected, onSelect, runtimeIDs, usages, runtimes, t }) {
  const options = ["all", ...runtimeIDs];
  return <section className="mt-7 rounded-xl border bg-muted/20 p-4" aria-label={t("usage.provider")}>
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div><strong className="block text-sm font-semibold text-foreground">{t("usage.provider")}</strong><span className="mt-0.5 block text-xs text-muted-foreground">{t("usage.sourceHint")}</span></div>
      <span className="inline-flex items-center gap-1.5 rounded-md bg-background px-2.5 py-1.5 text-xs text-muted-foreground shadow-xs"><Clock3 size={13} aria-hidden="true" />{t("usage.cachePolicy")}</span>
    </div>
    <div className="mt-3 flex flex-wrap gap-2" role="tablist" aria-label={t("usage.provider")}>
      {options.map((runtime) => {
        const active = selected === runtime;
        const scope = usages[runtime]?.scope;
        return <button type="button" role="tab" aria-selected={active} className={`inline-flex h-9 items-center gap-2 rounded-lg border px-3 text-xs font-medium transition-colors ${active ? "border-primary/30 bg-primary/10 text-primary" : "border-border bg-background text-muted-foreground hover:text-foreground"}`} onClick={() => onSelect(runtime)} key={runtime}>
          {runtime === "all" ? <Gauge size={15} aria-hidden="true" /> : <RuntimeHarnessIcon harness={runtime} size={15} />}
          <span>{runtime === "all" ? t("usage.allRuntimes") : runtimeName(runtime, runtimes)}</span>
          {runtime !== "all" && scope && <span className={`rounded px-1.5 py-0.5 text-[10px] ${scope === "account" ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground"}`}>{t(`usage.scopeShort.${scope}`)}</span>}
        </button>;
      })}
    </div>
  </section>;
}

export default function UsagePage({ mode = "wails", runtimes = [] }) {
  const { t, i18n } = useTranslation();
  const [selected, setSelected] = useState("all");
  const [usages, setUsages] = useState({});
  const [loading, setLoading] = useState(true);
  const [errors, setErrors] = useState({});
  const runtimeIDs = useMemo(() => {
    if (!runtimes.length) return mode === "demo" ? usageRuntimeIDs : ["codex"];
    const status = new Map(runtimes.map((runtime) => [runtime.id, runtime]));
    const visible = usageRuntimeIDs.filter((runtime) => status.get(runtime)?.enabled !== false && status.get(runtime)?.available !== false);
    return visible.length ? visible : ["codex"];
  }, [mode, runtimes]);

  useEffect(() => {
    if (selected !== "all" && !runtimeIDs.includes(selected)) setSelected("all");
  }, [runtimeIDs, selected]);

  const readUsage = useCallback(async (runtime, force) => {
    if (mode === "demo") return { ...demoUsageForRuntime(runtime), fetchedAt: new Date().toISOString() };
    return force ? RuntimeBinding.SyncAccountUsage(runtime) : RuntimeBinding.GetAccountUsage(runtime);
  }, [mode]);

  const refresh = useCallback(async (force = false, targets = runtimeIDs) => {
    setLoading(true);
    const results = await Promise.all(targets.map(async (runtime) => {
      try {
        return { runtime, usage: await readUsage(runtime, force) };
      } catch (cause) {
        return { runtime, error: errorMessage(cause) };
      }
    }));
    setUsages((current) => {
      const next = { ...current };
      for (const result of results) if (result.usage) next[result.runtime] = result.usage;
      return next;
    });
    setErrors((current) => {
      const next = { ...current };
      for (const result of results) {
        if (result.error) next[result.runtime] = result.error;
        else delete next[result.runtime];
      }
      return next;
    });
    setLoading(false);
  }, [readUsage, runtimeIDs]);

  useEffect(() => {
    void refresh(false, runtimeIDs);
    const timer = window.setInterval(() => void refresh(false, runtimeIDs), automaticSyncInterval);
    return () => window.clearInterval(timer);
  }, [refresh, runtimeIDs]);

  const selectedUsages = useMemo(() => runtimeIDs.map((runtime) => usages[runtime]).filter(Boolean), [runtimeIDs, usages]);
  const usage = useMemo(() => selected === "all" ? (selectedUsages.length ? combineAccountUsage(selectedUsages) : null) : usages[selected] || null, [selected, selectedUsages, usages]);
  const selectedErrors = selected === "all" ? runtimeIDs.filter((runtime) => errors[runtime]).map((runtime) => ({ runtime, message: errors[runtime] })) : errors[selected] ? [{ runtime: selected, message: errors[selected] }] : [];
  const selectedName = selected === "all" ? t("usage.allRuntimes") : runtimeName(selected, runtimes);
  const heatmapDescription = selected === "all"
    ? t("usage.heatmapDescriptionAll")
    : usage?.scope === "account" ? t("usage.heatmapDescriptionAccount", { runtime: selectedName }) : t("usage.heatmapDescriptionDevice", { runtime: selectedName });
  const rateLimits = usage?.rateLimits || [];
  const syncTargets = selected === "all" ? runtimeIDs : [selected];

  return <div className="min-h-0 flex-1 overflow-y-auto bg-background">
    <div className="mx-auto w-full max-w-6xl px-6 py-7 lg:px-9 lg:py-9">
      <header className="flex items-start justify-between gap-5">
        <div className="flex min-w-0 items-start gap-3.5"><span className="grid size-10 shrink-0 place-items-center rounded-xl bg-primary/10 text-primary"><Gauge size={20} aria-hidden="true" /></span><div><h1 className="m-0 text-xl font-semibold tracking-tight text-foreground">{t("usage.title")}</h1><p className="mt-1 mb-0 max-w-2xl text-sm leading-relaxed text-muted-foreground">{t("usage.description")}</p></div></div>
        <Button variant="outline" size="sm" disabled={loading} onClick={() => void refresh(true, syncTargets)}>{loading ? <RefreshCw className="animate-spin" aria-hidden="true" /> : <RefreshCw aria-hidden="true" />}{loading ? t("usage.syncing") : t("usage.syncNow")}</Button>
      </header>

      <RuntimeSourcePicker selected={selected} onSelect={setSelected} runtimeIDs={runtimeIDs} usages={usages} runtimes={runtimes} t={t} />

      {selectedErrors.length > 0 && <div className="mt-5 flex items-start gap-2.5 rounded-xl border border-destructive/30 bg-destructive/7 px-4 py-3 text-sm text-destructive" role="alert"><AlertCircle className="mt-0.5 shrink-0" size={16} aria-hidden="true" /><div><strong className="block font-semibold">{t("usage.loadFailed")}</strong>{selectedErrors.map((item) => <span className="mt-0.5 block text-xs opacity-90" key={item.runtime}>{runtimeName(item.runtime, runtimes)}：{item.message}</span>)}</div></div>}
      {loading && !usage ? <div className="mt-5 grid min-h-48 place-items-center rounded-xl border border-dashed text-sm text-muted-foreground"><span className="flex items-center gap-2"><RefreshCw className="animate-spin" size={16} aria-hidden="true" />{t("usage.loading")}</span></div>
        : usage && <>
          <AccountSummary value={usage.summary} t={t} />
          {(usage.dailyUsage || []).length > 0 ? <><UsageHeatmap dailyUsage={usage.dailyUsage} description={heatmapDescription} t={t} language={i18n.resolvedLanguage} /><DailyUsage dailyUsage={usage.dailyUsage} t={t} language={i18n.resolvedLanguage} /></> : <div className="mt-5 rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">{t("usage.noDailyData")}</div>}
          {rateLimits.length > 0 && <section className="mt-7" aria-labelledby="rolling-usage-title"><header className="mb-3"><h2 id="rolling-usage-title" className="m-0 text-base font-semibold text-foreground">{t("usage.rollingTitle")}</h2><p className="mt-1 mb-0 text-xs text-muted-foreground">{t("usage.rollingDescription")}</p></header><div className="grid gap-4">{rateLimits.map((limit, index) => <LimitCard value={limit} runtime={selected === "all" ? limit.runtime : selected} t={t} key={`${limit.runtime || selected}-${limit.id}-${index}`} />)}</div></section>}
          <footer className="mt-5 flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground"><span>{t("usage.updatedAt", { time: usage.fetchedAt ? formatDateTime(usage.fetchedAt) : "—" })} · {selected === "all" ? t("usage.scope.mixed") : t(`usage.scope.${usage.scope || "device"}`)}</span>{usage.resetCredits?.availableCount > 0 && <span className="inline-flex items-center gap-1.5 rounded-md bg-primary/8 px-2.5 py-1.5 text-primary"><RotateCcw size={13} aria-hidden="true" />{t("usage.resetCredits", { count: usage.resetCredits.availableCount })}</span>}</footer>
        </>}
    </div>
  </div>;
}
