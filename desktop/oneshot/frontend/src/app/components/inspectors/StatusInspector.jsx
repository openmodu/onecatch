import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Clipboard } from "@wailsio/runtime";
import { Action, Kicker } from "../../../ui/primitives.jsx";
import { errorMessage, formatDuration, formatTime, formatTokens } from "../../format.js";
import { runtimeSessionEntries } from "../../sessionCommands.js";
import { summarizeTokenUsage } from "../../tokenUsage.js";
import StatusPill from "../StatusPill.jsx";
import { statusKey } from "../../constants.js";

function InspectorRow({ label, value }) { return <div className="inspector-row"><span>{label}</span><strong>{value || "—"}</strong></div>; }

function TokenMetric({ label, total, details = [] }) {
  const visibleDetails = details.filter((item) => item.value > 0);
  return <div title={`${label}：${formatTokens(total)}`}><span>{label}</span><strong>{formatTokens(total)}</strong>{visibleDetails.length > 0 && <small>{visibleDetails.map((item) => `${item.label} ${formatTokens(item.value)}`).join(" · ")}</small>}</div>;
}

async function copyText(value) {
  try {
    await Clipboard.SetText(value);
  } catch (wailsError) {
    if (!navigator.clipboard?.writeText) throw wailsError;
    await navigator.clipboard.writeText(value);
  }
}

export default function StatusInspector({ detail, queuedTask, queuePosition, notify }) {
  const { t } = useTranslation();
  const [copiedStepID, setCopiedStepID] = useState("");
  useEffect(() => {
    if (!copiedStepID) return undefined;
    const timer = window.setTimeout(() => setCopiedStepID(""), 1800);
    return () => window.clearTimeout(timer);
  }, [copiedStepID]);
  useEffect(() => { setCopiedStepID(""); }, [detail?.run?.id]);

  const copySession = async (session) => {
    try {
      await copyText(session.command || session.sessionID);
      setCopiedStepID(session.stepID);
    } catch (error) {
      notify?.("error", t("inspector.copyFailed", { error: errorMessage(error) }));
    }
  };

  if (queuedTask) return <div className="status-inspector"><Kicker>{t("inspector.queueStatus")}</Kicker><div className="status-hero"><StatusPill status="queued" /><strong>{t("inspector.queuePosition", { position: queuePosition })}</strong></div><InspectorRow label={t("inspector.executionMode")} value={t("task.workspaceFIFO")} /><InspectorRow label={t("inspector.authorized")} value={queuedTask.queue?.authorized ? t("common.yes") : t("common.no")} /><InspectorRow label={t("inspector.enqueuedAt")} value={formatTime(queuedTask.queue?.enqueuedAt)} /><InspectorRow label={t("inspector.attachments")} value={t("common.itemsCount", { count: queuedTask.attachments?.length || 0 })} /></div>;
  if (!detail) return <p className="inspector-placeholder">{t("inspector.selectTask")}</p>;
  const { run, workflow, stepRuns = [] } = detail;
  const tokenUsage = summarizeTokenUsage(stepRuns);
  const duration = stepRuns.reduce((sum, step) => sum + (step.durationMs || 0), 0);
  const sessions = runtimeSessionEntries(detail, t);
  const currentStep = workflow.steps?.find((step) => step.id === run.currentStepId);
  return <div className="status-inspector"><div className="status-hero"><StatusPill status={run.status} active={detail.active} /><strong>{currentStep?.name || run.currentStepId || "—"}</strong><small>{currentStep?.runtime || "agent"}</small></div><div className="run-metric-grid"><TokenMetric label={t("inspector.inputTokens")} total={tokenUsage.inputTokens} details={[{ label: t("inspector.cacheRead"), value: tokenUsage.cachedInputTokens }, { label: t("inspector.cacheWrite"), value: tokenUsage.cacheCreationInputTokens }]} /><TokenMetric label={t("inspector.outputTokens")} total={tokenUsage.outputTokens} details={[{ label: t("inspector.reasoning"), value: tokenUsage.reasoningOutputTokens }]} /><div><span>{t("inspector.duration")}</span><strong>{formatDuration(duration)}</strong></div><div><span>{t("inspector.rounds")}</span><strong>{stepRuns.length}</strong></div></div><section className="inspector-block"><Kicker>{t("inspector.workflow")}</Kicker>{(workflow.steps || []).map((step, index) => { const stepRun = [...stepRuns].reverse().find((item) => item.stepId === step.id); const node = run.nodes?.[step.id]; const stepStatus = node?.status || stepRun?.status || "pending"; return <div className={`inspector-flow-row ${step.id === run.currentStepId ? "current" : ""}`} key={step.id}><b>{String(index + 1).padStart(2, "0")}</b><span><strong>{step.name}</strong><small>{step.runtime} · {t(statusKey(stepStatus), { defaultValue: stepStatus })}</small></span></div>; })}</section><section className="inspector-block"><Kicker>{t("inspector.resumeSessions")}</Kicker>{sessions.map((session) => <div className="inspector-session" key={session.stepID}><div className="inspector-session-head"><span><strong>{session.runtime}</strong> · {session.stepName}</span><small>{session.idLabel}</small></div><code>{session.command || session.sessionID}</code><div className="inspector-session-action"><span>{session.command ? t("inspector.resumeTerminal") : t("inspector.copySession")}</span><Action bracket={false} tone={copiedStepID === session.stepID ? "cyan" : "muted"} onClick={() => copySession(session)}>{copiedStepID === session.stepID ? t("inspector.copied") : t("inspector.copy")}</Action></div></div>)}{!sessions.length && <p>{t("inspector.noSessions")}</p>}</section>{(run.pauseReason || detail.lastError) && <section className="inspector-alert">{run.pauseReason && <p>{t("inspector.paused", { reason: t(`pauseReason.${run.pauseReason}`, { defaultValue: run.pauseReason }) })}</p>}{detail.lastError && <p>{detail.lastError}</p>}</section>}</div>;
}
