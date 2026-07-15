import { memo, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Action, Kicker, TUISelect } from "../../ui/primitives.jsx";
import { formatTime, shortID } from "../format.js";
import { runStatusOptions } from "../constants.js";
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

// The worker the run's latest step executed on, or "" when it ran locally. The
// Git panel uses it to auto-show the machine that actually holds the changes,
// since a remote step's edits live only on its worker's clone.
function activeWorkerID(runDetail) {
  const steps = runDetail?.workflow?.steps || [];
  const stepRuns = runDetail?.stepRuns || [];
  const stepID = stepRuns[stepRuns.length - 1]?.stepId || runDetail?.run?.currentStepId;
  const workerID = steps.find((step) => step.id === stepID)?.workerId;
  return workerID && workerID !== "local" ? workerID : "";
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

// Rows are memoized on primitive/ref-stable props (the run objects keep their
// reference across polls via mergeRunItems), so a background refresh only
// re-renders the row that actually changed instead of the whole history list.
const RunHistoryRow = memo(function RunHistoryRow({ run, selected, active, workflowName, onSelect }) {
  return <button type="button" className={`task-history-item ${selected ? "selected" : ""}`} onClick={() => onSelect(run)}><span className="task-history-status"><StatusPill status={run.status} active={active} /></span><strong>{run.task?.title || run.id}</strong><small>{workflowName} · {formatTime(run.updatedAt || run.startedAt)}</small></button>;
});

const QueuedHistoryRow = memo(function QueuedHistoryRow({ task, selected, position, workflowName, onSelect }) {
  return <button type="button" className={`task-history-item ${selected ? "selected" : ""}`} onClick={() => onSelect(task)}><span className="task-history-status"><StatusPill status="queued" /><small>#{position}</small></span><strong>{task.title}</strong><small>{workflowName} · {formatTime(task.updatedAt)}</small></button>;
});

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

export default function TaskWorkbench({ mode, workspaceID, tasks, runs, runDetail, selectedRunID, selectedQueuedTaskID, workflows, loading, total, hasMore, search, status, busy, attachments, onSearch, onStatus, onNewTask, onLoadMore, onSelectRun, onSelectQueued, onChooseAttachments, onRemoveAttachment, onSubmit, onInterrupt, onCancel, onRemoveInstruction, onRename, onDelete, notify }) {
  const { t, i18n } = useTranslation();
  const [inspectorTab, setInspectorTab] = useState("status");
  const [inspectorPreference, setInspectorPreference] = useState(initialInspectorPreference);
  const [compactViewport, setCompactViewport] = useState(initialCompactViewport);
  const scrollRef = useRef(null);
  const pinnedRef = useRef(true);

  const selectedQueuedTask = useMemo(() => tasks.find((task) => task.id === selectedQueuedTaskID), [tasks, selectedQueuedTaskID]);
  const selectedTask = runDetail?.task || selectedQueuedTask;
  const queueTasks = useMemo(() => tasks.filter((task) => task.status === "queued").sort((left, right) => new Date(left.queue?.enqueuedAt || left.createdAt) - new Date(right.queue?.enqueuedAt || right.createdAt)), [tasks]);
  const filteredQueued = useMemo(() => queueTasks.filter((task) => (!status || status === "queued") && (!search.trim() || `${task.title}\n${task.prompt}`.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase()))), [queueTasks, status, search]);
  const visibleRuns = status === "queued" ? [] : runs;

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

  return <div className={`task-workbench ${inspectorCollapsed ? "inspector-collapsed" : ""}`}>
    <aside className="task-history-pane">
      <div className="task-history-head"><div><Kicker>{t("task.history")}</Kicker><strong>{t("task.tasks")}</strong></div><Action tone="primary" disabled={!workspaceID} onClick={onNewTask}>+ {t("task.new")}</Action></div>
      <div className="task-history-query"><label><span>/</span><input aria-label={t("task.search")} value={search} onChange={(event) => onSearch(event.target.value)} placeholder={t("task.searchPlaceholder")} />{search && <button type="button" onClick={() => onSearch("")}>×</button>}</label><TUISelect ariaLabel={t("task.status")} value={status} onChange={onStatus} options={runStatusOptions(t)} /></div>
      <div className="task-history-list">
        {filteredQueued.length > 0 && <div className="task-history-group">{t("task.workspaceQueue", { count: filteredQueued.length })}</div>}
        {filteredQueued.map((task) => <QueuedHistoryRow key={task.id} task={task} selected={selectedQueuedTaskID === task.id} position={queueTasks.findIndex((item) => item.id === task.id) + 1} workflowName={workflowNameFor(workflows, task.workflowId)} onSelect={onSelectQueued} />)}
        {visibleRuns.length > 0 && <div className="task-history-group">{t("task.runHistory", { count: total })}</div>}
        {visibleRuns.map((run) => <RunHistoryRow key={run.id} run={run} selected={selectedRunID === run.id} active={runDetail?.run?.id === run.id && runDetail.active} workflowName={workflowNameFor(workflows, run.workflowId)} onSelect={onSelectRun} />)}
        {!filteredQueued.length && !visibleRuns.length && !loading && <div className="task-history-empty">{search || status ? t("task.noMatches") : t("task.empty")}</div>}
        {(loading || hasMore) && <button type="button" className="task-history-more" disabled={loading} onClick={onLoadMore}>{loading ? t("task.loading") : t("task.loadMore", { visible: visibleRuns.length, total })}</button>}
      </div>
    </aside>

    <section className="conversation-workspace">
      {selectedTask ? <>
        <header className="conversation-workspace-head"><div><div className="conversation-title-row"><h2>{selectedTask.title}</h2><StatusPill status={runStatus || selectedTask.status} active={runDetail?.active} /></div><p>{workflowName}{runDetail?.run?.id ? ` · ${shortID(runDetail.run.id)}` : ` · ${t("task.workspaceFIFO")}`}</p></div><div className="conversation-head-actions"><button type="button" onClick={onRename}>[ {t("task.rename")} ]</button><button type="button" className="danger" onClick={onDelete}>[ {t("task.delete")} ]</button></div></header>
        <div className="conversation-scroll" ref={scrollRef} onScroll={handleConversationScroll}>
          {selectedQueuedTask ? <QueuedTaskView task={selectedQueuedTask} position={queueTasks.findIndex((task) => task.id === selectedQueuedTask.id) + 1} /> : <><ConversationTimeline items={conversation} active={runDetail?.active} />{!conversation.length && <div className="workbench-empty"><p>{t("task.noMessages")}</p></div>}</>}
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
      </> : <div className="workbench-welcome"><Kicker>{t("task.welcomeKicker")}</Kicker><h2>{t("task.welcomeTitle")}</h2><p>{t("task.welcomeDescription")}</p><Action tone="primary" disabled={!workspaceID} onClick={onNewTask}>{t("task.newTask")}</Action></div>}
    </section>

    <aside className={`workbench-inspector ${inspectorCollapsed ? "collapsed" : ""}`} aria-label={t("inspector.aria")}>
      {inspectorCollapsed ? <div className="workbench-inspector-rail"><button type="button" aria-label={t("inspector.expand")} aria-expanded="false" aria-controls="workbench-inspector-content" title={t("inspector.expand")} onClick={toggleInspector}><span aria-hidden="true">‹</span><strong>{t("inspector.rail")}</strong></button></div> : <div className="workbench-tabs">{[["status", t("inspector.status")], ["git", t("inspector.git")], ["events", t("inspector.events")]].map(([value, label]) => <button type="button" className={inspectorTab === value ? "active" : ""} aria-pressed={inspectorTab === value} key={value} onClick={() => setInspectorTab(value)}>{label}</button>)}<button type="button" className="workbench-inspector-toggle" aria-label={t("inspector.collapse")} aria-expanded="true" aria-controls="workbench-inspector-content" title={t("inspector.collapse")} onClick={toggleInspector}><span aria-hidden="true">›</span></button></div>}
      <div className="workbench-inspector-body" id="workbench-inspector-content" hidden={inspectorCollapsed}>{!inspectorCollapsed && (inspectorTab === "status" ? <StatusInspector detail={runDetail} queuedTask={selectedQueuedTask} queuePosition={selectedQueuedTask ? queueTasks.findIndex((task) => task.id === selectedQueuedTask.id) + 1 : 0} notify={notify} /> : inspectorTab === "git" ? <GitInspector mode={mode} workspaceID={workspaceID} runWorkerID={runWorkerID} notify={notify} /> : <EventInspector detail={runDetail} />)}</div>
    </aside>
  </div>;
}
