import { memo, useEffect, useMemo, useRef, useState } from "react";
import { Activity, GitBranch, ListTree, Maximize2, Minimize2, PanelRightClose, SquareTerminal, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Action, Kicker, StatusBadge } from "../../ui/primitives.jsx";
import { shortID } from "../format.js";
import { buildRunConversation } from "../runConversation.js";
import StatusPill from "./StatusPill.jsx";
import QueuedTaskView from "./QueuedTaskView.jsx";
import ConversationTimeline from "./ConversationTimeline.jsx";
import Composer from "./Composer.jsx";
import StatusInspector from "./inspectors/StatusInspector.jsx";
import GitInspector from "./inspectors/GitInspector.jsx";
import EventInspector from "./inspectors/EventInspector.jsx";
import TerminalDock from "./TerminalDock.jsx";

const MIN_INSPECTOR_WIDTH = 320;
const DEFAULT_INSPECTOR_WIDTH = 380;
const INSPECTOR_SNAP_DISTANCE = 24;

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

function TaskWorkbench({ mode, workspace, workspaceID, terminalPreferences, terminalVisible, terminalToggleVersion, onTerminalVisibilityChange, tasks, runDetail, selectedRunID, selectedQueuedTaskID, workflows, busy, permissionBusy, attachments, inspectorCollapsed, onToggleInspector, onNewTask, onChooseAttachments, onRemoveAttachment, onSubmit, onInterrupt, onCancel, onRemoveInstruction, onPermissionDecision, onRename, onDelete, notify }) {
  const { t, i18n } = useTranslation();
  const [inspectorTab, setInspectorTab] = useState("status");
  const [inspectorWidth, setInspectorWidth] = useState(DEFAULT_INSPECTOR_WIDTH);
  const [inspectorResizing, setInspectorResizing] = useState(false);
  const [inspectorMaximized, setInspectorMaximized] = useState(false);
  const scrollRef = useRef(null);
  const pinnedRef = useRef(true);
  const inspectorResizeRef = useRef(null);
  const inspectorRestoreWidthRef = useRef(DEFAULT_INSPECTOR_WIDTH);
  const workbenchRef = useRef(null);
  const terminalDockRef = useRef(null);
  const terminalToggleVersionRef = useRef(terminalToggleVersion);

  const selectedQueuedTask = useMemo(() => tasks.find((task) => task.id === selectedQueuedTaskID), [tasks, selectedQueuedTaskID]);
  const selectedTask = runDetail?.task || selectedQueuedTask;
  const queueTasks = useMemo(() => tasks.filter((task) => task.status === "queued").sort((left, right) => new Date(left.queue?.enqueuedAt || left.createdAt) - new Date(right.queue?.enqueuedAt || right.createdAt)), [tasks]);

  const signature = conversationSignature(runDetail);
  // eslint-disable-next-line react-hooks/exhaustive-deps -- signature captures every field the builder reads
  const conversation = useMemo(() => (runDetail ? buildRunConversation(runDetail, t) : []), [signature, i18n.resolvedLanguage, t]);
  const pendingInstructions = useMemo(() => (runDetail?.instructions || []).filter((instruction) => instruction.status === "pending"), [runDetail?.instructions]);
  const conversationSize = `${signature}:${conversation.length}:${conversation[conversation.length - 1]?.items?.length || 0}`;

  useEffect(() => {
    if (terminalToggleVersion === terminalToggleVersionRef.current) return;
    terminalToggleVersionRef.current = terminalToggleVersion;
    terminalDockRef.current?.toggle();
  }, [terminalToggleVersion]);

  useEffect(() => {
    pinnedRef.current = true;
    const element = scrollRef.current;
    if (element) element.scrollTop = element.scrollHeight;
  }, [selectedRunID, selectedQueuedTaskID]);
  useEffect(() => {
    const element = scrollRef.current;
    if (element && pinnedRef.current) element.scrollTop = element.scrollHeight;
  }, [attachments.length, conversationSize, pendingInstructions.length, runDetail?.run?.status]);
  const handleConversationScroll = () => {
    const element = scrollRef.current;
    if (element) pinnedRef.current = element.scrollHeight - element.scrollTop - element.clientHeight < 90;
  };
  const workflowName = selectedTask ? workflowNameFor(workflows, selectedTask.workflowId) : "";
  const runStatus = runDetail?.run?.status;
  const runWorkerID = useMemo(() => activeWorkerID(runDetail), [runDetail]);
  const inspectorTabs = [
    { value: "status", label: t("inspector.status"), icon: Activity },
    { value: "git", label: t("inspector.git"), icon: GitBranch },
    { value: "events", label: t("inspector.events"), icon: ListTree },
  ];
  const activeInspector = inspectorTabs.find((tab) => tab.value === inspectorTab) || inspectorTabs[0];
  const clampInspectorWidth = (width) => {
    const workbenchWidth = workbenchRef.current?.getBoundingClientRect().width || window.innerWidth;
    const maximum = Math.max(MIN_INSPECTOR_WIDTH, workbenchWidth - INSPECTOR_SNAP_DISTANCE);
    return Math.max(MIN_INSPECTOR_WIDTH, Math.min(width, maximum));
  };
  const beginInspectorResize = (event) => {
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    setInspectorResizing(true);
    inspectorRestoreWidthRef.current = inspectorWidth;
    inspectorResizeRef.current = { pointerID: event.pointerId, startX: event.clientX, startWidth: inspectorWidth };
  };
  const moveInspectorResize = (event) => {
    const resize = inspectorResizeRef.current;
    if (!resize || resize.pointerID !== event.pointerId) return;
    const bounds = workbenchRef.current?.getBoundingClientRect();
    if (bounds && event.clientX <= bounds.left + INSPECTOR_SNAP_DISTANCE) {
      setInspectorMaximized(true);
      return;
    }
    setInspectorMaximized(false);
    setInspectorWidth(clampInspectorWidth(resize.startWidth + resize.startX - event.clientX));
  };
  const endInspectorResize = (event) => {
    if (inspectorResizeRef.current?.pointerID !== event.pointerId) return;
    inspectorResizeRef.current = null;
    setInspectorResizing(false);
    event.currentTarget.releasePointerCapture?.(event.pointerId);
  };
  const resizeInspectorWithKeyboard = (event) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    setInspectorWidth((width) => clampInspectorWidth(width + (event.key === "ArrowLeft" ? 20 : -20)));
  };
  const closeInspector = () => {
    if (inspectorMaximized) setInspectorWidth(clampInspectorWidth(inspectorRestoreWidthRef.current));
    setInspectorMaximized(false);
    onToggleInspector();
  };
  const toggleInspectorMaximized = () => {
    if (inspectorMaximized) {
      setInspectorWidth(clampInspectorWidth(inspectorRestoreWidthRef.current));
      setInspectorMaximized(false);
      return;
    }
    inspectorRestoreWidthRef.current = inspectorWidth;
    setInspectorMaximized(true);
  };
  return <div ref={workbenchRef} className={`task-workbench grid min-h-0 min-w-0 flex-1 overflow-visible ${inspectorCollapsed ? "inspector-collapsed" : "inspector-open"} ${inspectorResizing ? "inspector-resizing" : ""} ${inspectorMaximized ? "inspector-maximized" : ""}`} style={{ "--workbench-inspector-width": `${inspectorWidth}px` }}>
    {!inspectorCollapsed && !inspectorMaximized && <div className="workbench-inspector-dock no-drag">
      <StatusBadge status={mode === "wails" ? "good" : "warn"} className="shrink-0">
        <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />
        {mode === "wails" ? t("common.local") : t("common.preview")}
      </StatusBadge>
      <button type="button" className={`workbench-terminal-toggle ${terminalVisible ? "active" : ""}`} aria-label={terminalVisible ? t("terminal.collapse") : t("terminal.open")} aria-pressed={terminalVisible} title={`${terminalVisible ? t("terminal.collapse") : t("terminal.open")} · Ctrl + \``} onClick={() => terminalDockRef.current?.toggle()}><SquareTerminal size={15} strokeWidth={2} aria-hidden="true" /></button>
      <button type="button" className="workbench-inspector-dock-toggle" aria-label={t("inspector.collapse")} aria-expanded="true" aria-controls="workbench-inspector-content" title={t("inspector.collapse")} onClick={closeInspector}><PanelRightClose size={16} strokeWidth={2} aria-hidden="true" /></button>
    </div>}
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
      <TerminalDock ref={terminalDockRef} mode={mode} workspace={workspace} preferences={terminalPreferences} notify={notify} onVisibilityChange={onTerminalVisibilityChange} />
    </section>

    {!inspectorCollapsed && <aside className="workbench-inspector open min-h-0 min-w-0" aria-label={t("inspector.aria")}>
      <span className="workbench-inspector-resize" role="separator" aria-label={t("inspector.resize", { defaultValue: "调整状态栏宽度" })} aria-orientation="vertical" tabIndex="0" onPointerDown={beginInspectorResize} onPointerMove={moveInspectorResize} onPointerUp={endInspectorResize} onPointerCancel={endInspectorResize} onKeyDown={resizeInspectorWithKeyboard} />
      <div className="workbench-inspector-toolbar"><div className="workbench-inspector-tabs" role="tablist" aria-label={t("inspector.aria")}>{inspectorTabs.map(({ value, label, icon: Icon }) => <button type="button" role="tab" className={inspectorTab === value ? "active" : ""} aria-selected={inspectorTab === value} aria-controls="workbench-inspector-content" title={label} key={value} onClick={() => setInspectorTab(value)}><Icon size={15} strokeWidth={2} aria-hidden="true" /><span className="sr-only">{label}</span></button>)}</div><strong className="workbench-inspector-title">{activeInspector.label}</strong><div className="workbench-inspector-window-actions">{inspectorMaximized && <button type="button" className={`workbench-terminal-toggle ${terminalVisible ? "active" : ""}`} aria-label={terminalVisible ? t("terminal.collapse") : t("terminal.open")} aria-pressed={terminalVisible} title={terminalVisible ? t("terminal.collapse") : t("terminal.open")} onClick={() => terminalDockRef.current?.toggle()}><SquareTerminal size={15} strokeWidth={2} aria-hidden="true" /></button>}<button type="button" className="workbench-inspector-maximize" aria-label={inspectorMaximized ? t("inspector.restore") : t("inspector.maximize")} aria-pressed={inspectorMaximized} title={inspectorMaximized ? t("inspector.restore") : t("inspector.maximize")} onClick={toggleInspectorMaximized}>{inspectorMaximized ? <Minimize2 size={15} strokeWidth={2} aria-hidden="true" /> : <Maximize2 size={15} strokeWidth={2} aria-hidden="true" />}</button><button type="button" className="workbench-inspector-close" aria-label={t("inspector.collapse")} aria-expanded="true" aria-controls="workbench-inspector-content" title={t("inspector.collapse")} onClick={closeInspector}><X size={16} strokeWidth={2} aria-hidden="true" /></button></div></div>
      <div className="workbench-inspector-body min-h-0 overflow-y-auto" id="workbench-inspector-content">{inspectorTab === "status" ? <StatusInspector detail={runDetail} queuedTask={selectedQueuedTask} queuePosition={selectedQueuedTask ? queueTasks.findIndex((task) => task.id === selectedQueuedTask.id) + 1 : 0} notify={notify} onOpenTerminal={(command) => terminalDockRef.current?.open(command)} /> : inspectorTab === "git" ? <GitInspector mode={mode} workspaceID={workspaceID} runWorkerID={runWorkerID} notify={notify} /> : <EventInspector detail={runDetail} />}</div>
    </aside>}
  </div>;
}

export default memo(TaskWorkbench);
