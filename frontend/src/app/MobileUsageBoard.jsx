import { useEffect, useMemo, useRef, useState } from "react";
import { Activity, CalendarDays, CircleAlert, Flame, Gauge, LoaderCircle, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { accountRateLimitName, buildUsageHeatmap, clampUsagePercent, combineAccountUsage, recentDailyUsage, usageWindowDuration } from "./accountUsage.js";
import { compactTokens } from "./format.js";
import RuntimeHarnessIcon from "./components/RuntimeHarnessIcon.jsx";
import { runtimeHarnesses } from "./runtimeHarnesses.js";

const WINDOW_UNITS = { minutes: "分钟", hours: "小时", days: "天", weeks: "周" };
export const USAGE_RANGES = [14, 30];

function windowLabel(minutes) {
  const span = usageWindowDuration(minutes);
  return span ? `${span.value} ${WINDOW_UNITS[span.unit]}` : "";
}

function runtimeLabel(runtime) {
  return runtimeHarnesses.find((item) => item.id === runtime)?.label || runtime;
}

function dayLabel(date) {
  return `${date.getMonth() + 1}/${date.getDate()}`;
}

// The same year-long grid the desktop draws. A phone cannot show 53 weeks at
// once, so the grid scrolls sideways rather than being cropped to a fortnight:
// the shape of a year is the point of the chart.
function Heatmap({ dailyUsage }) {
  const heatmap = useMemo(() => buildUsageHeatmap(dailyUsage), [dailyUsage]);
  const activeDays = dailyUsage.filter((item) => Number(item.tokens) > 0).length;
  // The grid is wider than the phone, and the interesting end is today's.
  const scroller = useRef(null);
  useEffect(() => {
    const element = scroller.current;
    if (element) element.scrollLeft = element.scrollWidth;
  }, [heatmap]);
  return <section className="mobile-usage-heatmap">
    <header><h3>活跃热力图</h3><small>{activeDays} 天有活动</small></header>
    <div className="mobile-usage-heatmap-scroll" ref={scroller}>
      <div className="mobile-usage-heatmap-grid">
        {heatmap.weeks.flatMap((week) => week.days).map((day) => (
          <i className={`level-${day.future ? "future" : day.level}`} key={day.key} title={`${day.key} · ${compactTokens(day.tokens)}`} />
        ))}
      </div>
    </div>
    <footer><span>少</span>{[0, 1, 2, 3, 4].map((level) => <i className={`level-${level}`} key={level} />)}<span>多</span></footer>
  </section>;
}

function Metric({ icon: Icon, label, value, suffix = "" }) {
  return <div className="mobile-usage-metric">
    <span><Icon />{label}</span>
    <strong>{value == null ? "—" : `${compactTokens(value)}${suffix}`}</strong>
  </div>;
}

export default function MobileUsageBoard({ usage = [], loading, error, onRefresh }) {
  const [selected, setSelected] = useState("all");
  const [range, setRange] = useState(USAGE_RANGES[0]);
  const items = usage.filter(Boolean);
  const combined = useMemo(() => combineAccountUsage(items), [items]);
  const current = selected === "all" ? combined : items.find((item) => item.runtime === selected) || combined;
  const daily = current.dailyUsage || [];
  const rows = useMemo(() => recentDailyUsage(daily, new Date(), range), [daily, range]);
  const peak = Math.max(1, ...rows.map((row) => row.tokens));

  if (error) return <div className="mobile-page mobile-usage-page"><div className="mobile-list-empty"><CircleAlert /><p>{error}</p></div>
    <Button className="mobile-main-action" variant="ghost" disabled={loading} onClick={onRefresh}><RefreshCw />重新读取</Button>
  </div>;
  if (!items.length) return <div className="mobile-page mobile-usage-page"><div className="mobile-list-empty"><Gauge /><p>{loading ? "正在读取用量…" : "这台电脑没有可报告用量的运行时"}</p></div>
    <Button className="mobile-main-action" variant="ghost" disabled={loading} onClick={onRefresh}><RefreshCw />重新读取</Button>
  </div>;

  return <div className="mobile-page mobile-usage-page">
    <div className="mobile-usage-sources">
      <button type="button" className={selected === "all" ? "selected" : ""} onClick={() => setSelected("all")}>全部</button>
      {items.map((item) => <button type="button" className={selected === item.runtime ? "selected" : ""} key={item.runtime} onClick={() => setSelected(item.runtime)}>
        <RuntimeHarnessIcon harness={item.runtime} size={13} />{runtimeLabel(item.runtime)}
      </button>)}
    </div>

    <div className="mobile-usage-metrics">
      <Metric icon={Activity} label="累计" value={current.summary?.lifetimeTokens} />
      <Metric icon={Gauge} label="单日峰值" value={current.summary?.peakDailyTokens} />
      <Metric icon={Flame} label="连续活跃" value={current.summary?.currentStreakDays} suffix=" 天" />
      <Metric icon={CalendarDays} label="最长连续" value={current.summary?.longestStreakDays} suffix=" 天" />
    </div>

    {(current.rateLimits || []).flatMap((limit, index) => [limit.primary, limit.secondary].filter(Boolean).map((quota, position) => {
      const span = windowLabel(quota.windowDurationMins);
      const percent = clampUsagePercent(quota.usedPercent);
      return <div className="mobile-usage-limit" key={`${limit.runtime || ""}-${limit.id || index}-${position}`}>
        <div className="mobile-usage-limit-head">
          <span>{limit.runtime && selected === "all" ? <RuntimeHarnessIcon harness={limit.runtime} size={12} /> : null}{accountRateLimitName(limit)}{span ? ` · ${span}` : ""}</span>
          <b>{Math.round(percent)}%</b>
        </div>
        <div className="mobile-usage-bar"><span style={{ width: `${percent}%` }} /></div>
      </div>;
    }))}

    <Heatmap dailyUsage={daily} />

    <section className="mobile-usage-daily">
      <header>
        <h3>每日用量</h3>
        <div className="mobile-usage-ranges">
          {USAGE_RANGES.map((days) => <button type="button" className={range === days ? "selected" : ""} key={days} onClick={() => setRange(days)}>{days} 天</button>)}
        </div>
      </header>
      {rows.map((row) => <div className="mobile-usage-day" key={row.key}>
        <time dateTime={row.key}>{dayLabel(row.date)}</time>
        <span><i style={{ width: `${row.tokens > 0 ? Math.max(3, (row.tokens / peak) * 100) : 0}%` }} /></span>
        <b className={row.tokens ? "" : "quiet"}>{compactTokens(row.tokens)}</b>
      </div>)}
    </section>

    <Button className="mobile-main-action" variant="ghost" disabled={loading} onClick={onRefresh}>{loading ? <LoaderCircle className="animate-spin" /> : <RefreshCw />}重新读取</Button>
  </div>;
}
