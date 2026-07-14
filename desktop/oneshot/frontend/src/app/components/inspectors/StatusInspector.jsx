import { Kicker } from "../../../ui/primitives.jsx";
import { formatDuration, formatTime, formatTokens } from "../../format.js";
import { runtimeSessionEntries } from "../../sessionCommands.js";
import StatusPill from "../StatusPill.jsx";

function InspectorRow({ label, value }) { return <div className="inspector-row"><span>{label}</span><strong>{value || "—"}</strong></div>; }

export default function StatusInspector({ detail, queuedTask, queuePosition }) {
  if (queuedTask) return <div className="status-inspector"><Kicker>queue status</Kicker><div className="status-hero"><StatusPill status="queued" /><strong>队列第 {queuePosition} 位</strong></div><InspectorRow label="执行方式" value="Workspace FIFO" /><InspectorRow label="已授权" value={queuedTask.queue?.authorized ? "是" : "否"} /><InspectorRow label="入队时间" value={formatTime(queuedTask.queue?.enqueuedAt)} /><InspectorRow label="附件" value={`${queuedTask.attachments?.length || 0} 个`} /></div>;
  if (!detail) return <p className="inspector-placeholder">选择任务后显示运行状态。</p>;
  const { run, workflow, stepRuns = [] } = detail;
  const inputTokens = stepRuns.reduce((sum, step) => sum + (step.inputTokens || 0), 0);
  const outputTokens = stepRuns.reduce((sum, step) => sum + (step.outputTokens || 0), 0);
  const duration = stepRuns.reduce((sum, step) => sum + (step.durationMs || 0), 0);
  const sessions = runtimeSessionEntries(detail);
  const currentStep = workflow.steps?.find((step) => step.id === run.currentStepId);
  return <div className="status-inspector"><div className="status-hero"><StatusPill status={run.status} active={detail.active} /><strong>{currentStep?.name || run.currentStepId || "—"}</strong><small>{currentStep?.runtime || "agent"}</small></div><div className="run-metric-grid"><div><span>输入 Token</span><strong>{formatTokens(inputTokens)}</strong></div><div><span>输出 Token</span><strong>{formatTokens(outputTokens)}</strong></div><div><span>Agent 耗时</span><strong>{formatDuration(duration)}</strong></div><div><span>轮次</span><strong>{stepRuns.length}</strong></div></div><section className="inspector-block"><Kicker>workflow</Kicker>{(workflow.steps || []).map((step, index) => { const stepRun = [...stepRuns].reverse().find((item) => item.stepId === step.id); const node = run.nodes?.[step.id]; return <div className={`inspector-flow-row ${step.id === run.currentStepId ? "current" : ""}`} key={step.id}><b>{String(index + 1).padStart(2, "0")}</b><span><strong>{step.name}</strong><small>{step.runtime} · {node?.status || stepRun?.status || "pending"}</small></span></div>; })}</section><section className="inspector-block"><Kicker>sessions</Kicker>{sessions.map((session) => <div className="inspector-session" key={session.stepID}><span>{session.runtime} · {session.stepName}</span><code>{session.sessionID}</code></div>)}{!sessions.length && <p>尚未创建可恢复会话。</p>}</section>{(run.pauseReason || detail.lastError) && <section className="inspector-alert">{run.pauseReason && <p>暂停：{run.pauseReason}</p>}{detail.lastError && <p>{detail.lastError}</p>}</section>}</div>;
}
