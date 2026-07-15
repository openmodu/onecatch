import { useEffect, useState } from "react";
import { Clipboard } from "@wailsio/runtime";
import { Action, Kicker } from "../../../ui/primitives.jsx";
import { errorMessage, formatDuration, formatTime, formatTokens } from "../../format.js";
import { runtimeSessionEntries } from "../../sessionCommands.js";
import { summarizeTokenUsage } from "../../tokenUsage.js";
import StatusPill from "../StatusPill.jsx";

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
      notify?.("error", `复制失败：${errorMessage(error)}`);
    }
  };

  if (queuedTask) return <div className="status-inspector"><Kicker>queue status</Kicker><div className="status-hero"><StatusPill status="queued" /><strong>队列第 {queuePosition} 位</strong></div><InspectorRow label="执行方式" value="Workspace FIFO" /><InspectorRow label="已授权" value={queuedTask.queue?.authorized ? "是" : "否"} /><InspectorRow label="入队时间" value={formatTime(queuedTask.queue?.enqueuedAt)} /><InspectorRow label="附件" value={`${queuedTask.attachments?.length || 0} 个`} /></div>;
  if (!detail) return <p className="inspector-placeholder">选择任务后显示运行状态。</p>;
  const { run, workflow, stepRuns = [] } = detail;
  const tokenUsage = summarizeTokenUsage(stepRuns);
  const duration = stepRuns.reduce((sum, step) => sum + (step.durationMs || 0), 0);
  const sessions = runtimeSessionEntries(detail);
  const currentStep = workflow.steps?.find((step) => step.id === run.currentStepId);
  return <div className="status-inspector"><div className="status-hero"><StatusPill status={run.status} active={detail.active} /><strong>{currentStep?.name || run.currentStepId || "—"}</strong><small>{currentStep?.runtime || "agent"}</small></div><div className="run-metric-grid"><TokenMetric label="输入总 Token" total={tokenUsage.inputTokens} details={[{ label: "缓存读取", value: tokenUsage.cachedInputTokens }, { label: "缓存写入", value: tokenUsage.cacheCreationInputTokens }]} /><TokenMetric label="输出总 Token" total={tokenUsage.outputTokens} details={[{ label: "其中推理", value: tokenUsage.reasoningOutputTokens }]} /><div><span>Agent 耗时</span><strong>{formatDuration(duration)}</strong></div><div><span>轮次</span><strong>{stepRuns.length}</strong></div></div><section className="inspector-block"><Kicker>workflow</Kicker>{(workflow.steps || []).map((step, index) => { const stepRun = [...stepRuns].reverse().find((item) => item.stepId === step.id); const node = run.nodes?.[step.id]; return <div className={`inspector-flow-row ${step.id === run.currentStepId ? "current" : ""}`} key={step.id}><b>{String(index + 1).padStart(2, "0")}</b><span><strong>{step.name}</strong><small>{step.runtime} · {node?.status || stepRun?.status || "pending"}</small></span></div>; })}</section><section className="inspector-block"><Kicker>resume sessions</Kicker>{sessions.map((session) => <div className="inspector-session" key={session.stepID}><div className="inspector-session-head"><span><strong>{session.runtime}</strong> · {session.stepName}</span><small>{session.idLabel}</small></div><code>{session.command || session.sessionID}</code><div className="inspector-session-action"><span>{session.command ? "在终端恢复此 Agent" : "复制会话 ID"}</span><Action bracket={false} tone={copiedStepID === session.stepID ? "cyan" : "muted"} onClick={() => copySession(session)}>{copiedStepID === session.stepID ? "已复制" : "复制"}</Action></div></div>)}{!sessions.length && <p>尚未创建可恢复会话。</p>}</section>{(run.pauseReason || detail.lastError) && <section className="inspector-alert">{run.pauseReason && <p>暂停：{run.pauseReason}</p>}{detail.lastError && <p>{detail.lastError}</p>}</section>}</div>;
}
