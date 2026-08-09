import { memo, useEffect, useMemo, useRef, useState } from "react";
import { ChevronLeft } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Action, Kicker } from "../../ui/primitives.jsx";
import { shortID } from "../format.js";
import { buildRunConversation } from "../runConversation.js";
import { INSPECTOR_COMPACT_QUERY, readInspectorPreference, resolveInspectorCollapsed, writeInspectorPreference } from "../inspectorLayout.js";
import StatusPill from "./StatusPill.jsx";
import QueuedTaskView from "./QueuedTaskView.jsx";
import ConversationTimeline from "./ConversationTimeline.jsx";
import Composer from "./Composer.jsx";
import StatusInspector from "./inspectors/StatusInspector.jsx";
import GitInspector from "./inspectors/GitInspector.jsx";
import EventInspector from "./inspectors/EventInspector.jsx";

function workflowNameFor(workflows, workflowID) { return workflows.find((workflow) => workflow.id === workflowID)?.name || workflowID; }

// Read-only remote steps may inspect worker-local state, so follow that clone.
// Writable remote steps synchronize their patch back and clean the worker;
// those must default to local so the Git panel shows the delivered changes.
function activeWorkerID(runDetail) {
  const steps = runDetail?.workflow?.steps || [];
  const stepRuns = runDetail?.stepRuns || [];
  const stepID = stepRuns[stepRuns.length - 1]?.stepId || runDetail?.run?.currentStepId;
  const step = steps.find((item) => item.id === stepID);
  return step?.sandbox === "read-only" && step.workerId && step.workerId !== "local" ? step.workerId : "";
}

function initialInspectorPreference() {
  try {
    return typeof window === "undefined" ? null : readInspectorPreference(window.localStorage);
  } catch {
    return null;
  }
}

function initialCompactViewport() {
  return typeof window !== "undefined" && typeof window.matchMedia === "function" && window.matchMedia(INSPECTOR_COMPACT_QUERY).matches;
}

// Cheap fingerprint of everything buildRunConversation reads, so a poll tick
// that returns a fresh `runDetail` object with identical content does not force
// the (potentially hundreds of rows) timeline to be rebuilt and reconciled.
function conversationSignature(detail) {
  if (!detail) return "";
  const runtimeEvents = detail.runtimeEvents || [];
  const last = runtimeEvents[runtimeEvents.length - 1];
  return [
    detail.run?.id,
    detail.run?.status,
    runtimeEvents.length,
    last ? `${last.stepRunId}:${last.seq}` : "",
    runtimeEvents.filter((event) => event.streamId).map((event) => `${event.stepRunId}:${event.streamId}:${event.revision || 0}:${event.text?.length || 0}:${event.streaming ? 1 : 0}`).join(","),
    (detail.stepRuns || []).length,
    (detail.stepRuns || []).map((step) => step.status).join(","),
    (detail.events || []).length,
    (detail.instructions || []).map((instruction) => instruction.status).join(","),
    detail.task?.prompt ? 1 : 0,
  ].join("|");
}

function TaskWorkbench({ mode, workspaceID, tasks, runDetail, selectedRunID, selectedQueuedTaskID, workflows, busy, permissionBusy, attachments, onNewTask, onChooseAttachments, onRemoveAttachment, onSubmit, onInterrupt, onCancel, onRemoveInstruction, onPermissionDecision, onRename, onDelete, notify }) {
  const { t, i18n } = useTranslation();
  const [inspectorTab, setInspectorTab] = useState("status");
  const [inspectorPreference, setInspectorPreference] = useState(initialInspectorPreference);
  const [compactViewport, setCompactViewport] = useState(initialCompactViewport);
  const scrollRef = useRef(null);
  const pinnedRef = useRef(true);

  const selectedQueuedTask = useMemo(() => tasks.find((task) => task.id === selectedQueuedTaskID), [tasks, selectedQueuedTaskID]);
  const selectedTask = runDetail?.task || selectedQueuedTask;
  const queueTasks = useMemo(() => tasks.filter((task) => task.status === "queued").sort((left, right) => new Date(left.queue?.enqueuedAt || left.createdAt) - new Date(right.queue?.enqueuedAt || right.createdAt)), [tasks]);

  const signature = conversationSignature(runDetail);
  // eslint-disable-next-line react-hooks/exhaustive-deps -- signature captures every field the builder reads
  const conversation = useMemo(() => (runDetail ? buildRunConversation(runDetail, t) : []), [signature, i18n.resolvedLanguage, t]);
  const pendingInstructions = useMemo(() => (runDetail?.instructions || []).filter((instruction) => instruction.status === "pending"), [runDetail?.instructions]);
  const conversationSize = `${signature}:${conversation.length}:${conversation[conversation.length - 1]?.items?.length || 0}`;

  useEffect(() => {
    pinnedRef.current = true;
    const element = scrollRef.current;
    if (element) element.scrollTop = element.scrollHeight;
  }, [selectedRunID, selectedQueuedTaskID]);
  useEffect(() => {
    const element = scrollRef.current;
    if (element && pinnedRef.current) element.scrollTop = element.scrollHeight;
  }, [attachments.length, conversationSize, pendingInstructions.length, runDetail?.run?.status]);
  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") return undefined;
    const media = window.matchMedia(INSPECTOR_COMPACT_QUERY);
    const update = (event) => setCompactViewport(event.matches);
    setCompactViewport(media.matches);
    if (typeof media.addEventListener === "function") {
      media.addEventListener("change", update);
      return () => media.removeEventListener("change", update);
    }
    media.addListener?.(update);
    return () => media.removeListener?.(update);
  }, []);

  const handleConversationScroll = () => {
    const element = scrollRef.current;
    if (element) pinnedRef.current = element.scrollHeight - element.scrollTop - element.clientHeight < 90;
  };
  const workflowName = selectedTask ? workflowNameFor(workflows, selectedTask.workflowId) : "";
  const runStatus = runDetail?.run?.status;
  const runWorkerID = useMemo(() => activeWorkerID(runDetail), [runDetail]);
  const inspectorCollapsed = resolveInspectorCollapsed(inspectorPreference, compactViewport);
  const toggleInspector = () => {
    const next = !inspectorCollapsed;
    setInspectorPreference(next);
    try {
      writeInspectorPreference(window.localStorage, next);
    } catch {
      // Storage is best effort; the current session should still respond.
    }
  };

  return <div className={`task-workbench grid min-h-0 min-w-0 flex-1 overflow-hidden transition-[grid-template-columns] duration-150 ease-out motion-reduce:transition-none ${inspectorCollapsed ? "inspector-collapsed grid-cols-[minmax(360px,1fr)_32px]" : "grid-cols-[minmax(360px,1fr)_310px]"}`}>
    <section className="conversation-workspace flex min-h-0 min-w-0 flex-col bg-background">
      {selectedTask ? <>
        <header className="conversation-workspace-head mx-2 mt-2 flex min-h-[58px] shrink-0 items-center justify-between gap-3 rounded-lg bg-muted/45 px-3.5 py-2"><div className="min-w-0"><div className="conversation-title-row flex min-w-0 items-center gap-2.5"><h2 className="m-0 min-w-0 truncate text-[15px] font-semibold text-foreground">{selectedTask.title}</h2><StatusPill status={runStatus || selectedTask.status} active={runDetail?.active} /></div><p className="mt-1 truncate text-xs text-muted-foreground">{workflowName}{runDetail?.run?.id ? ` · ${shortID(runDetail.run.id)}` : ` · ${t("task.workspaceFIFO")}`}</p></div><div className="conversation-head-actions flex shrink-0 items-center gap-1.5"><Action size="compact" tone="muted" onClick={onRename}>{t("task.rename")}</Action><Action size="compact" tone="danger" onClick={onDelete}>{t("task.delete")}</Action></div></header>
        <div className="conversation-scroll min-h-0 min-w-0 flex-1 select-text overflow-x-hidden overflow-y-auto overscroll-contain" ref={scrollRef} onScroll={handleConversationScroll}>
          {selectedQueuedTask ? <QueuedTaskView task={selectedQueuedTask} position={queueTasks.findIndex((task) => task.id === selectedQueuedTask.id) + 1} /> : <><ConversationTimeline items={conversation} active={runDetail?.active} permissionBusy={permissionBusy} onPermissionDecision={onPermissionDecision} />{!conversation.length && <div className="workbench-empty select-none p-8 text-center text-sm text-muted-foreground"><p>{t("task.noMessages")}</p></div>}</>}
        </div>
        {runDetail && <Composer
          runStatus={runStatus}
          active={runDetail.active}
          busy={busy}
          attachments={attachments}
          pendingInstructions={pendingInstructions}
          onChooseAttachments={onChooseAttachments}
          onRemoveAttachment={onRemoveAttachment}
          onRemoveInstruction={onRemoveInstruction}
          onInterrupt={onInterrupt}
          onCancel={onCancel}
          onSubmit={onSubmit}
        />}
      </> : <div className="workbench-welcome m-auto max-w-xl select-none p-10 text-center text-muted-foreground"><Kicker>{t("task.welcomeKicker")}</Kicker><h2 className="my-3 text-lg font-semibold text-foreground">{t("task.welcomeTitle")}</h2><p className="mb-6 text-sm leading-relaxed">{t("task.welcomeDescription")}</p><Action tone="primary" disabled={!workspaceID} onClick={onNewTask}>{t("task.newTask")}</Action></div>}
    </section>

    <aside className={`workbench-inspector grid min-h-0 min-w-0 border-l bg-muted/40 ${inspectorCollapsed ? "collapsed grid-rows-[minmax(0,1fr)]" : "grid-rows-[42px_minmax(0,1fr)]"}`} aria-label={t("inspector.aria")}>
      {inspectorCollapsed ? <div className="workbench-inspector-rail min-h-0"><button type="button" className="grid h-full w-full grid-rows-[32px_minmax(0,1fr)] justify-items-center bg-muted/60 pt-1.5 pb-2.5 text-muted-foreground transition-colors hover:bg-accent hover:text-primary" aria-label={t("inspector.expand")} aria-expanded="false" aria-controls="workbench-inspector-content" title={t("inspector.expand")} onClick={toggleInspector}><ChevronLeft size={16} aria-hidden="true" /><strong className="self-start text-[11px] font-bold tracking-[0.12em] [writing-mode:vertical-rl]">{t("inspector.rail")}</strong></button></div> : <div className="workbench-tabs m-2 mb-0 grid grid-cols-[repeat(3,minmax(0,1fr))_auto] items-center gap-1 rounded-lg bg-muted/70 p-1">{[["status", t("inspector.status")], ["git", t("inspector.git")], ["events", t("inspector.events")]].map(([value, label]) => <button type="button" className={`rounded-sm px-2 py-1.5 text-xs transition-colors hover:text-foreground ${inspectorTab === value ? "active bg-background text-foreground shadow-xs" : "text-muted-foreground"}`} aria-pressed={inspectorTab === value} key={value} onClick={() => setInspectorTab(value)}>{label}</button>)}<Action size="compact" tone="muted" className="workbench-inspector-toggle" aria-label={t("inspector.collapse")} aria-expanded="true" aria-controls="workbench-inspector-content" title={t("inspector.collapse")} onClick={toggleInspector}>{t("inspector.collapse")}</Action></div>}
      <div className="workbench-inspector-body min-h-0 overflow-y-auto" id="workbench-inspector-content" hidden={inspectorCollapsed}>{!inspectorCollapsed && (inspectorTab === "status" ? <StatusInspector detail={runDetail} queuedTask={selectedQueuedTask} queuePosition={selectedQueuedTask ? queueTasks.findIndex((task) => task.id === selectedQueuedTask.id) + 1 : 0} notify={notify} /> : inspectorTab === "git" ? <GitInspector mode={mode} workspaceID={workspaceID} runWorkerID={runWorkerID} notify={notify} /> : <EventInspector detail={runDetail} />)}</div>
    </aside>
  </div>;
}

export default memo(TaskWorkbench);
