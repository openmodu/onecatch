import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Clipboard } from "@wailsio/runtime";
import { Action, Kicker } from "../../../ui/primitives.jsx";
import { errorMessage, formatDuration, formatTime, formatTokens } from "../../format.js";
import { runtimeSessionEntries } from "../../sessionCommands.js";
import { currentWorkflowStepID, effectiveWorkflowStepStatus, latestStepRunsByStep, summarizeAgentDuration } from "../../statusMetrics.js";
import { summarizeTokenUsage } from "../../tokenUsage.js";
import StatusPill from "../StatusPill.jsx";
import { statusKey } from "../../constants.js";

function InspectorRow({ label, value }) {
  return <div className="mt-1.5 flex items-center justify-between gap-2.5 rounded-md bg-muted/45 px-2.5 py-2.5">
    <span className="text-xs text-muted-foreground">{label}</span>
    <strong className="text-xs font-medium text-foreground">{value || "—"}</strong>
  </div>;
}

function TokenMetric({ label, total, details = [] }) {
  const visibleDetails = details.filter((item) => item.value > 0);
  return <div className="px-1.5 py-3" title={`${label}：${formatTokens(total)}`}>
    <span className="block text-[11px] text-muted-foreground">{label}</span>
    <strong className="mt-1 block text-[17px] font-semibold tabular-nums text-foreground">{formatTokens(total)}</strong>
    {visibleDetails.length > 0 && <small className="mt-1 block text-[11px] leading-snug text-muted-foreground">{visibleDetails.map((item) => `${item.label} ${formatTokens(item.value)}`).join(" · ")}</small>}
  </div>;
}

async function copyText(value) {
  try {
    await Clipboard.SetText(value);
  } catch (wailsError) {
    if (!navigator.clipboard?.writeText) throw wailsError;
    await navigator.clipboard.writeText(value);
  }
}

export default function StatusInspector({ detail, queuedTask, queuePosition, draft = false, notify, onOpenTerminal }) {
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

  if (queuedTask) return <div className="status-inspector p-3.5"><Kicker>{t("inspector.queueStatus")}</Kicker><div className="mt-2 mb-2 flex items-center gap-2.5 rounded-lg bg-muted/60 px-3 py-2.5"><StatusPill status="queued" /><strong>{t("inspector.queuePosition", { position: queuePosition })}</strong></div><InspectorRow label={t("inspector.executionMode")} value={t("task.workspaceFIFO")} /><InspectorRow label={t("inspector.authorized")} value={queuedTask.queue?.authorized ? t("common.yes") : t("common.no")} /><InspectorRow label={t("inspector.enqueuedAt")} value={formatTime(queuedTask.queue?.enqueuedAt)} /><InspectorRow label={t("inspector.attachments")} value={t("common.itemsCount", { count: queuedTask.attachments?.length || 0 })} /></div>;
  if (!detail) return <p className="m-0 px-4 py-5 text-xs leading-relaxed text-muted-foreground">{t(draft ? "inspector.newTaskStatus" : "inspector.selectTask")}</p>;
  const { run, workflow, stepRuns = [] } = detail;
  const tokenUsage = summarizeTokenUsage(stepRuns);
  const duration = summarizeAgentDuration(stepRuns);
  const latestStepRuns = latestStepRunsByStep(stepRuns);
  const sessions = runtimeSessionEntries(detail, t);
  const currentStepID = currentWorkflowStepID(run, workflow, stepRuns);
  const statusRun = currentStepID === run.currentStepId ? run : { ...run, currentStepId: currentStepID };
  const currentStep = workflow.steps?.find((step) => step.id === currentStepID);
  return <div className="status-inspector p-3.5">
    <div className="flex items-center gap-2.5 rounded-lg bg-muted/60 px-3 py-2.5">
      <StatusPill status={run.status} active={detail.active && run.status === "running"} />
      <strong className="min-w-0 truncate text-sm font-semibold text-foreground">{currentStep?.name || currentStepID || "—"}</strong>
      <small className="ml-auto shrink-0 text-[11px] font-medium text-info">{currentStep?.runtime || "agent"}</small>
    </div>
    {/* Spacing and a shared tint group the four metrics without table rules. */}
    <div className="mt-2 grid grid-cols-2 gap-1.5 rounded-lg bg-muted/35 p-1.5">
      <TokenMetric label={t("inspector.inputTokens")} total={tokenUsage.inputTokens} details={[{ label: t("inspector.cacheRead"), value: tokenUsage.cachedInputTokens }, { label: t("inspector.cacheWrite"), value: tokenUsage.cacheCreationInputTokens }]} />
      <TokenMetric label={t("inspector.outputTokens")} total={tokenUsage.outputTokens} details={[{ label: t("inspector.reasoning"), value: tokenUsage.reasoningOutputTokens }]} />
      <div className="px-1.5 py-3"><span className="block text-[11px] text-muted-foreground">{t("inspector.duration")}</span><strong className="mt-1 block text-[17px] font-semibold tabular-nums text-foreground">{formatDuration(duration)}</strong></div>
      <div className="px-1.5 py-3"><span className="block text-[11px] text-muted-foreground">{t("inspector.executions")}</span><strong className="mt-1 block text-[17px] font-semibold tabular-nums text-foreground">{stepRuns.length}</strong></div>
    </div>
    <section className="mt-3 rounded-lg bg-muted/30 px-3 py-3">
      <Kicker className="mb-2 block">{t("inspector.workflow")}</Kicker>
      {(workflow.steps || []).map((step, index) => {
        const stepRun = latestStepRuns.get(step.id);
        const node = run.nodes?.[step.id];
        const stepStatus = effectiveWorkflowStepStatus(statusRun, step.id, node, stepRun);
        const current = step.id === currentStepID;
        return <div className="grid grid-cols-[20px_minmax(0,1fr)] items-start gap-2.5 py-1.5" key={step.id}>
          <b className={`grid size-5 place-items-center rounded-full text-[11px] font-semibold tabular-nums ${current ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground"}`}>{index + 1}</b>
          <span>
            <strong className={`block text-xs font-medium ${current ? "text-primary" : "text-foreground"}`}>{step.name}</strong>
            <small className="mt-0.5 block text-[11px] text-muted-foreground">{step.runtime} · {t(statusKey(stepStatus), { defaultValue: stepStatus })}</small>
          </span>
        </div>;
      })}
    </section>
    <section className="mt-3 rounded-lg bg-muted/30 px-3 py-3">
      <Kicker className="mb-2 block">{t("inspector.resumeSessions")}</Kicker>
      {sessions.map((session) => <div className="mt-2 grid gap-2 rounded-md bg-background/70 p-2.5" key={session.stepID}>
        <div className="flex items-center justify-between gap-2">
          <span className="min-w-0 truncate text-[11px] text-muted-foreground"><strong className="font-medium text-foreground capitalize">{session.runtime}</strong> · {session.stepName}</span>
          <small className="shrink-0 text-[11px] uppercase text-muted-foreground">{session.idLabel}</small>
        </div>
        <code className="select-text rounded-md border-l-2 border-l-info bg-muted px-2 py-1.5 font-mono text-[11px] leading-relaxed break-all text-muted-foreground">{session.command || session.sessionID}</code>
        <div className="flex items-center justify-between gap-2">
          <span className="min-w-0 text-[11px] text-muted-foreground">{session.command ? t("inspector.resumeTerminal") : t("inspector.copySession")}</span>
          <span className="flex shrink-0 items-center gap-1"><Action size="compact" tone={copiedStepID === session.stepID ? "cyan" : "muted"} onClick={() => copySession(session)}>{copiedStepID === session.stepID ? t("inspector.copied") : t("inspector.copy")}</Action>{session.command && onOpenTerminal && <Action size="compact" tone="muted" onClick={() => onOpenTerminal(session.command)}>{t("terminal.open")}</Action>}</span>
        </div>
      </div>)}
      {!sessions.length && <p className="m-0 text-xs text-muted-foreground">{t("inspector.noSessions")}</p>}
    </section>
    {(run.pauseReason || detail.lastError) && <section className="mt-3 rounded-md border border-destructive/35 border-l-2 border-l-destructive bg-destructive/6 p-2.5 text-destructive [&>p]:my-0.5 [&>p]:text-xs">{run.pauseReason && <p>{t("inspector.paused", { reason: t(`pauseReason.${run.pauseReason}`, { defaultValue: run.pauseReason }) })}</p>}{detail.lastError && <p>{errorMessage(detail.lastError)}</p>}</section>}
  </div>;
}
