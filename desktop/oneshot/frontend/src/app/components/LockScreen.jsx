import { memo, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { LOCK_PHASE } from "../lockSignal.js";

// The traffic light maps the three phases onto the colour everyone already
// reads across a room: red = stop and deal with it, amber = work in progress,
// green = all clear. Exactly one lamp is lit per phase; the rest go dark.
const LAMPS = [
  { key: "red", phase: LOCK_PHASE.waiting },
  { key: "amber", phase: LOCK_PHASE.working },
  { key: "green", phase: LOCK_PHASE.done },
];

function useClock() {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  const pad = (value) => String(value).padStart(2, "0");
  return `${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`;
}

// A calm, screensaver-style standby overlay. It renders over everything so a
// glance tells you whether the agents are still working, need you, or are done
// — without exposing the workspace. It is intentionally not a security lock:
// any click, Esc, or the exit control returns to the app.
function LockScreen({ signal, workspaceName, onExit }) {
  const { t } = useTranslation();
  const clock = useClock();
  const phase = signal.phase;

  useEffect(() => {
    const onKey = (event) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onExit();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onExit]);

  const headline = phase === LOCK_PHASE.waiting ? t("lock.needsAttention")
    : phase === LOCK_PHASE.working ? t("lock.working")
      : t("lock.allDone");

  return <div className={`lock-screen phase-${phase}`} role="dialog" aria-modal="true" aria-label={t("lock.title")} onClick={onExit}>
    <div className="lock-clock" aria-hidden="true">{clock}</div>
    <div className="lock-stage" onClick={(event) => event.stopPropagation()}>
      <div className="lock-signal" role="img" aria-label={headline}>
        {LAMPS.map((lamp) => <span key={lamp.key} className={`lock-lamp lamp-${lamp.key} ${lamp.phase === phase ? "lit" : ""}`} aria-hidden="true" />)}
      </div>

      <div className="lock-info">
        <h1>{headline}</h1>
        {workspaceName && <p className="lock-workspace">{workspaceName}</p>}

        <div className="lock-counts" aria-label={t("lock.title")}>
          <div className="lock-count"><b>{signal.running}</b><span>{t("lock.running")}</span></div>
          <div className="lock-count"><b>{signal.queued}</b><span>{t("lock.queued")}</span></div>
          <div className={`lock-count ${signal.paused > 0 ? "flag" : ""}`}><b>{signal.paused}</b><span>{t("lock.paused")}</span></div>
        </div>

        {signal.items.length > 0 && <ul className="lock-list">
          {signal.items.slice(0, 4).map((item) => <li key={item.id} className={`status-${item.status}`}>
            <span className="lock-dot" aria-hidden="true" />
            <span className="lock-item-title" title={item.title}>{item.title}</span>
            <span className="lock-item-status">{t(`lock.state.${item.status}`)}</span>
          </li>)}
          {signal.items.length > 4 && <li className="lock-more">{t("lock.andMore", { count: signal.items.length - 4 })}</li>}
        </ul>}

        <button type="button" className="lock-exit" onClick={onExit}>{t("lock.exit")}</button>
      </div>
    </div>
    <div className="lock-hint" aria-hidden="true">{t("lock.dismissHint")}</div>
  </div>;
}

export default memo(LockScreen);
