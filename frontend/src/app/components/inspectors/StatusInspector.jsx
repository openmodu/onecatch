import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Clipboard } from "@wailsio/runtime";
import { Activity, Clock3, Download, Upload } from "lucide-react";
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

function TokenLedger({ icon: Icon, label, total, details = [] }) {
  return <section className="status-token-ledger rounded-lg bg-muted/35 px-3 py-3" aria-label={label}>
    <div className="status-token-ledger-heading grid min-w-0 grid-cols-[32px_minmax(0,1fr)_auto] items-center gap-2.5">
      <span className="grid size-8 place-items-center rounded-lg border border-border/60 bg-background/70 text-primary" aria-hidden="true">
        <Icon size={15} strokeWidth={1.8} />
      </span>
      <span className="min-w-0 truncate text-[11px] text-muted-foreground">{label}</span>
      <strong className="status-token-ledger-total whitespace-nowrap font-semibold tracking-[-0.02em] tabular-nums text-foreground" title={`${label}：${formatTokens(total)}`}>{formatTokens(total)}</strong>
    </div>
    {details.length > 0 && <div className="status-token-ledger-details mt-2 grid gap-1 rounded-md bg-background/55 px-2.5 py-2">
      {details.map((item) => <div className="flex min-w-0 items-baseline justify-between gap-3" key={item.label} title={`${item.label}：${item.value}`}>
        <span className="min-w-0 truncate text-[10px] text-muted-foreground">{item.label}</span>
        <strong className={`shrink-0 whitespace-nowrap text-[11px] font-medium tabular-nums ${item.accent ? "text-primary" : "text-foreground"}`}>{item.value}</strong>
      </div>)}
    </div>}
  </section>;
}

function RunMetric({ icon: Icon, label, value }) {
  return <div className="status-run-metric flex min-w-0 items-center gap-2.5 px-1 py-1">
    <span className="grid size-8 shrink-0 place-items-center rounded-lg border border-border/60 bg-background/70 text-primary" aria-hidden="true">
      <Icon size={15} strokeWidth={1.8} />
    </span>
    <span className="min-w-0">
      <span className="block truncate text-[10px] text-muted-foreground">{label}</span>
      <strong className="mt-0.5 block truncate text-[15px] font-semibold tabular-nums text-foreground">{value}</strong>
    </span>
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
  const inputTokenDetails = [
    tokenUsage.inputTokens > 0 && { label: t("inspector.cacheHitRate"), value: `${tokenUsage.cacheHitRate.toFixed(1)}%`, accent: true },
    tokenUsage.cachedInputTokens > 0 && { label: t("inspector.cacheRead"), value: formatTokens(tokenUsage.cachedInputTokens) },
    tokenUsage.cacheCreationInputTokens > 0 && { label: t("inspector.cacheWrite"), value: formatTokens(tokenUsage.cacheCreationInputTokens) },
  ].filter(Boolean);
  const outputTokenDetails = [
    tokenUsage.reasoningOutputTokens > 0 && { label: t("inspector.reasoning"), value: formatTokens(tokenUsage.reasoningOutputTokens) },
  ].filter(Boolean);
  return <div className="status-inspector p-3.5">
    <div className="flex items-center gap-2.5 rounded-lg bg-muted/60 px-3 py-2.5">
      <StatusPill status={run.status} active={detail.active && run.status === "running"} />
      <strong className="min-w-0 truncate text-sm font-semibold text-foreground">{currentStep?.name || currentStepID || "—"}</strong>
      <small className="ml-auto shrink-0 text-[11px] font-medium text-info">{currentStep?.runtime || "agent"}</small>
    </div>
    <div className="status-metrics mt-2 grid gap-2">
      <TokenLedger icon={Download} label={t("inspector.inputTokens")} total={tokenUsage.inputTokens} details={inputTokenDetails} />
      <TokenLedger icon={Upload} label={t("inspector.outputTokens")} total={tokenUsage.outputTokens} details={outputTokenDetails} />
      <div className="status-run-metrics grid grid-cols-2 gap-2 rounded-lg bg-muted/35 p-2.5">
        <RunMetric icon={Clock3} label={t("inspector.duration")} value={formatDuration(duration)} />
        <RunMetric icon={Activity} label={t("inspector.executions")} value={stepRuns.length} />
      </div>
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
