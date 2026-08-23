import { lazy, memo, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Maximize2, Minimize2, PanelRightClose, SquareArrowOutUpRight, SquareTerminal, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { StatusBadge } from "../../ui/primitives.jsx";
import { buildRunConversation } from "../runConversation.js";
import QueuedTaskView from "./QueuedTaskView.jsx";
import ConversationTimeline from "./ConversationTimeline.jsx";
import Composer from "./Composer.jsx";
import InspectorPanel from "./inspectors/InspectorPanel.jsx";
import NewTaskView from "./NewTaskView.jsx";
import { activeWorkerID, sortQueuedTasks } from "../inspectorContext.js";
import { errorMessage } from "../format.js";
import { preferredReviewInspectorWidth } from "../reviewLayout.js";
import { supportsRuntimeProfile } from "../runtimeHarnesses.js";

const TerminalDock = lazy(() => import("./TerminalDock.jsx"));

const MIN_INSPECTOR_WIDTH = 320;
const DEFAULT_INSPECTOR_WIDTH = 380;
const INSPECTOR_SNAP_DISTANCE = 24;
// Keep the conversation titlebar controls inside their pane while the inspector
// is resized. Full-width inspection remains available through snap/maximize.
// The composer needs this much room to keep its primary controls on one row.
// Inspector resizing must yield before it turns the conversation into a narrow
// utility rail; compact viewports collapse the inspector separately.
const MIN_CONVERSATION_WIDTH = 620;

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

function TaskWorkbench({ mode, workspace, workspaceID, terminalPreferences, terminalVisible, terminalToggleVersion, terminalCommand, onTerminalVisibilityChange, tasks, runDetail, selectedRunID, selectedQueuedTaskID, busy, permissionBusy, attachments, inspectorCollapsed, onToggleInspector, onDetachInspector, newTaskOpen, taskForm, workflows, runtimes, taskRuntimeConfiguration, runtimeSettings, runtimeSettingsByHarness, allowFullSandbox, onInspectRuntimeConfiguration, onTaskFormChange, onChooseTaskAttachments, onCreateTask, onChooseAttachments, onRemoveAttachment, onSubmit, onInterrupt, onCancel, onRemoveInstruction, onLoadEarlierTranscript, onPermissionDecision, notify }) {
  const { t, i18n } = useTranslation();
  const [inspectorWidth, setInspectorWidth] = useState(DEFAULT_INSPECTOR_WIDTH);
  const [inspectorResizing, setInspectorResizing] = useState(false);
  const [inspectorMaximized, setInspectorMaximized] = useState(false);
  const [fileInspectorDirty, setFileInspectorDirty] = useState(false);
  const [terminalMounted, setTerminalMounted] = useState(false);
  const [continuationRuntimeProfile, setContinuationRuntimeProfile] = useState(null);
  const [continuationRuntimeConfiguration, setContinuationRuntimeConfiguration] = useState({ loading: false, data: null, error: "" });
  const [reviewRequest, setReviewRequest] = useState(0);
  const scrollRef = useRef(null);
  const pinnedRef = useRef(true);
  const inspectorResizeRef = useRef(null);
  const inspectorRestoreWidthRef = useRef(DEFAULT_INSPECTOR_WIDTH);
  const workbenchRef = useRef(null);
  const terminalDockRef = useRef(null);
  const pendingTerminalActionsRef = useRef([]);
  const terminalToggleVersionRef = useRef(terminalToggleVersion);
  const terminalCommandVersionRef = useRef(terminalCommand?.version || 0);

  const runTerminalAction = useCallback((action) => {
    const dock = terminalDockRef.current;
    if (dock) {
      if (action.type === "open") void dock.open(action.command);
      else dock.toggle();
      return;
    }
    pendingTerminalActionsRef.current.push(action);
    setTerminalMounted(true);
  }, []);
  const setTerminalDock = useCallback((dock) => {
    terminalDockRef.current = dock;
    if (!dock || !pendingTerminalActionsRef.current.length) return;
    const actions = pendingTerminalActionsRef.current.splice(0);
    actions.forEach((action) => {
      if (action.type === "open") void dock.open(action.command);
      else dock.toggle();
    });
  }, []);
  const toggleTerminal = useCallback(() => runTerminalAction({ type: "toggle" }), [runTerminalAction]);
  const openTerminal = useCallback((command) => runTerminalAction({ type: "open", command }), [runTerminalAction]);

  const selectedQueuedTask = useMemo(() => tasks.find((task) => task.id === selectedQueuedTaskID), [tasks, selectedQueuedTaskID]);
  const selectedTask = runDetail?.task || selectedQueuedTask;
  const queueTasks = useMemo(() => sortQueuedTasks(tasks), [tasks]);

  const signature = conversationSignature(runDetail);
  // eslint-disable-next-line react-hooks/exhaustive-deps -- signature captures every field the builder reads
  const conversation = useMemo(() => (runDetail ? buildRunConversation(runDetail, t) : []), [signature, i18n.resolvedLanguage, t]);
  // Opening a run ships only the newest slice of a long transcript. What is
  // missing is offered rather than silently dropped.
  const hiddenTranscriptCount = Math.max(0, (runDetail?.runtimeEventsTotal || 0) - (runDetail?.runtimeEvents || []).length);
  const pendingInstructions = useMemo(() => (runDetail?.instructions || []).filter((instruction) => instruction.status === "pending"), [runDetail?.instructions]);
  const activeStep = useMemo(() => {
    const steps = runDetail?.workflow?.steps || [];
    const stepRuns = runDetail?.stepRuns || [];
    const stepID = stepRuns[stepRuns.length - 1]?.stepId || runDetail?.run?.currentStepId;
    return steps.find((step) => step.id === stepID) || steps[0] || null;
  }, [runDetail]);
  const activeRuntimeProfile = useMemo(() => {
    if (!activeStep?.runtime) return null;
    const settings = runDetail?.run?.runtimeSettings?.[activeStep.runtime] || {};
    return { stepId: activeStep.id, harness: activeStep.runtime, model: activeStep.model || "", reasoningEffort: settings.reasoningEffort || "", serviceTier: settings.serviceTier || "" };
  }, [activeStep, runDetail?.run?.runtimeSettings]);
  const activeRuntimeProfileSignature = activeRuntimeProfile ? [activeRuntimeProfile.stepId, activeRuntimeProfile.harness, activeRuntimeProfile.model, activeRuntimeProfile.reasoningEffort, activeRuntimeProfile.serviceTier].join("\n") : "";
  const conversationSize = `${signature}:${conversation.length}:${conversation[conversation.length - 1]?.items?.length || 0}`;

  useEffect(() => {
    setContinuationRuntimeProfile(activeRuntimeProfile);
  }, [activeRuntimeProfileSignature, selectedRunID]);

  useEffect(() => {
    const harness = continuationRuntimeProfile?.harness;
    if (!harness || !supportsRuntimeProfile(harness) || !onInspectRuntimeConfiguration) {
      setContinuationRuntimeConfiguration({ loading: false, data: null, error: "" });
      return undefined;
    }
    let cancelled = false;
    setContinuationRuntimeConfiguration({ loading: true, data: null, error: "" });
    onInspectRuntimeConfiguration(harness)
      .then((data) => { if (!cancelled) setContinuationRuntimeConfiguration({ loading: false, data, error: "" }); })
      .catch((error) => { if (!cancelled) setContinuationRuntimeConfiguration({ loading: false, data: null, error: errorMessage(error) }); });
    return () => { cancelled = true; };
  }, [continuationRuntimeProfile?.harness, onInspectRuntimeConfiguration]);

  useEffect(() => {
    if (terminalToggleVersion === terminalToggleVersionRef.current) return;
    terminalToggleVersionRef.current = terminalToggleVersion;
    toggleTerminal();
  }, [terminalToggleVersion, toggleTerminal]);

  // The dock only exists in this window, so a "open in terminal" pressed in the
  // detached inspector arrives here as a bumped version instead of a call.
  useEffect(() => {
    const version = terminalCommand?.version || 0;
    if (version === terminalCommandVersionRef.current) return;
    terminalCommandVersionRef.current = version;
    if (terminalCommand?.command) openTerminal(terminalCommand.command);
  }, [openTerminal, terminalCommand]);

  useEffect(() => {
    if (terminalMounted) return undefined;
    const shortcut = (event) => {
      if (!(event.ctrlKey || event.metaKey) || event.code !== "Backquote") return;
      event.preventDefault();
      toggleTerminal();
    };
    window.addEventListener("keydown", shortcut);
    return () => window.removeEventListener("keydown", shortcut);
  }, [terminalMounted, toggleTerminal]);

  // Collapsing or detaching unmounts the file editor along with its buffers, so
  // the dirty flag must not survive to guard a panel that no longer exists.
  useEffect(() => {
    if (inspectorCollapsed) setFileInspectorDirty(false);
  }, [inspectorCollapsed]);

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
  const runStatus = runDetail?.run?.status;
  const runWorkerID = useMemo(() => activeWorkerID(runDetail), [runDetail]);
  const inspectorMaximumWidth = () => {
    const workbenchWidth = workbenchRef.current?.getBoundingClientRect().width || window.innerWidth;
    // Below the ideal two-pane width, share the available room instead of
    // preserving a 620px conversation track that pushes the inspector's
    // right edge outside the window. At normal sizes the conversation still
    // keeps its full minimum width.
    const reservedConversationWidth = Math.min(MIN_CONVERSATION_WIDTH, Math.floor(workbenchWidth / 2));
    return Math.max(MIN_INSPECTOR_WIDTH, workbenchWidth - reservedConversationWidth);
  };
  const clampInspectorWidth = (width) => {
    return Math.max(MIN_INSPECTOR_WIDTH, Math.min(width, inspectorMaximumWidth()));
  };
  const openReview = () => {
    const workbenchWidth = workbenchRef.current?.getBoundingClientRect().width || window.innerWidth;
    setInspectorMaximized(false);
    setInspectorWidth(clampInspectorWidth(preferredReviewInspectorWidth(workbenchWidth)));
    if (inspectorCollapsed) onToggleInspector();
    setReviewRequest((value) => value + 1);
  };
  const beginInspectorResize = (event) => {
    if (event.button !== 0) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture?.(event.pointerId);
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
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
  };
  const resizeInspectorWithKeyboard = (event) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    setInspectorWidth((width) => clampInspectorWidth(width + (event.key === "ArrowLeft" ? 20 : -20)));
  };
  const resetInspectorWidth = () => {
    const width = clampInspectorWidth(DEFAULT_INSPECTOR_WIDTH);
    inspectorRestoreWidthRef.current = width;
    setInspectorWidth(width);
  };

  useEffect(() => {
    const element = workbenchRef.current;
    if (!element || typeof ResizeObserver === "undefined") return undefined;
    const keepConversationUsable = () => setInspectorWidth((width) => clampInspectorWidth(width));
    const observer = new ResizeObserver(keepConversationUsable);
    observer.observe(element);
    keepConversationUsable();
    return () => observer.disconnect();
  }, []);
  const closeInspector = () => {
    if (fileInspectorDirty && !globalThis.confirm(t("files.discardAllConfirm"))) return;
    if (inspectorMaximized) setInspectorWidth(clampInspectorWidth(inspectorRestoreWidthRef.current));
    setInspectorMaximized(false);
    onToggleInspector();
  };
  // Detaching tears this panel down and rebuilds it in the other window, so the
  // file editor's unsaved buffers are lost exactly as they are on collapse.
  const detachInspector = () => {
    if (fileInspectorDirty && !globalThis.confirm(t("files.discardAllConfirm"))) return;
    if (inspectorMaximized) setInspectorWidth(clampInspectorWidth(inspectorRestoreWidthRef.current));
    setInspectorMaximized(false);
    onDetachInspector();
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
  return <div ref={workbenchRef} className={`task-workbench grid min-h-0 min-w-0 flex-1 overflow-visible ${inspectorCollapsed ? "inspector-collapsed" : "inspector-open"} ${inspectorResizing ? "inspector-resizing" : ""} ${inspectorMaximized ? "inspector-maximized" : ""} ${newTaskOpen ? "new-task-active" : ""}`} style={{ "--workbench-inspector-width": `${inspectorWidth}px`, "--conversation-min-width": `${MIN_CONVERSATION_WIDTH}px` }}>
    {!inspectorCollapsed && !inspectorMaximized && <div className="workbench-inspector-dock no-drag">
      <StatusBadge status={mode === "wails" ? "good" : "warn"} className="shrink-0">
        <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />
        {mode === "wails" ? t(workspace?.remoteFs ? "workspace.remoteFS" : "common.local") : t("common.preview")}
      </StatusBadge>
      <button type="button" className={`workbench-terminal-toggle ${terminalVisible ? "active" : ""}`} aria-label={terminalVisible ? t("terminal.collapse") : t("terminal.open")} aria-pressed={terminalVisible} title={`${terminalVisible ? t("terminal.collapse") : t("terminal.open")} · Ctrl + \``} onClick={toggleTerminal}><SquareTerminal size={15} strokeWidth={2} aria-hidden="true" /></button>
      <button type="button" className="workbench-inspector-dock-toggle" aria-label={t("inspector.collapse")} aria-expanded="true" aria-controls="workbench-inspector-content" title={t("inspector.collapse")} onClick={closeInspector}><PanelRightClose size={16} strokeWidth={2} aria-hidden="true" /></button>
    </div>}
    <section className="conversation-workspace flex min-h-0 min-w-0 flex-col bg-background">
      {newTaskOpen ? <NewTaskView
        workspaceID={workspaceID}
        workflows={workflows}
        runtimes={runtimes}
        form={taskForm}
        busy={busy}
        onChange={onTaskFormChange}
        onChooseAttachments={onChooseTaskAttachments}
        onSubmit={onCreateTask}
        runtimeConfiguration={taskRuntimeConfiguration}
        runtimeSettings={runtimeSettings}
        allowFullSandbox={allowFullSandbox}
      /> : selectedTask ? <>
        <div className="conversation-scroll min-h-0 min-w-0 flex-1 select-text overflow-x-hidden overflow-y-auto overscroll-contain" ref={scrollRef} onScroll={handleConversationScroll}>
          {selectedQueuedTask ? <QueuedTaskView task={selectedQueuedTask} position={queueTasks.findIndex((task) => task.id === selectedQueuedTask.id) + 1} /> : <><ConversationTimeline items={conversation} active={runDetail?.active} hiddenCount={hiddenTranscriptCount} onLoadEarlier={() => onLoadEarlierTranscript?.(runDetail?.run?.id)} permissionBusy={permissionBusy} onPermissionDecision={onPermissionDecision} onReview={openReview} />{!conversation.length && <div className="workbench-empty select-none p-8 text-center text-sm text-muted-foreground"><p>{t("task.noMessages")}</p></div>}</>}
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
          runtimeProfile={continuationRuntimeProfile || activeRuntimeProfile}
          onRuntimeProfileChange={setContinuationRuntimeProfile}
          runtimes={runtimes}
          runtimeConfiguration={continuationRuntimeConfiguration}
          runtimeSettings={runtimeSettingsByHarness?.[(continuationRuntimeProfile || activeRuntimeProfile)?.harness] || {}}
          workflowId={selectedTask?.workflowId || runDetail?.run?.workflowId}
          workflowName={runDetail?.workflow?.name}
          permission={selectedTask?.sandbox || activeStep?.sandbox || "workspace-write"}
        />}
      </> : null}
      {terminalMounted && <Suspense fallback={null}><TerminalDock ref={setTerminalDock} mode={mode} workspace={workspace} preferences={terminalPreferences} notify={notify} onVisibilityChange={onTerminalVisibilityChange} /></Suspense>}
    </section>

    {!inspectorCollapsed && (!inspectorMaximized || inspectorResizing) && <span
      className="workbench-inspector-resize"
      role="separator"
      aria-label={t("inspector.resize")}
      aria-orientation="vertical"
      aria-valuemin={MIN_INSPECTOR_WIDTH}
      aria-valuemax={Math.round(inspectorMaximumWidth())}
      aria-valuenow={Math.round(inspectorWidth)}
      tabIndex="0"
      title={t("inspector.resizeHint")}
      onDoubleClick={resetInspectorWidth}
      onPointerDown={beginInspectorResize}
      onPointerMove={moveInspectorResize}
      onPointerUp={endInspectorResize}
      onPointerCancel={endInspectorResize}
      onKeyDown={resizeInspectorWithKeyboard}
    />}

    {!inspectorCollapsed && <InspectorPanel
      className="workbench-inspector open min-h-0 min-w-0"
      mode={mode}
      workspaceID={workspaceID}
      remoteFS={workspace?.remoteFs}
      detail={runDetail}
      queuedTask={selectedQueuedTask}
      queuePosition={selectedQueuedTask ? queueTasks.findIndex((task) => task.id === selectedQueuedTask.id) + 1 : 0}
      draft={newTaskOpen}
      runWorkerID={runWorkerID}
      reviewRequest={reviewRequest}
      notify={notify}
      onOpenTerminal={openTerminal}
      onDirtyChange={setFileInspectorDirty}
      actions={<>
        {inspectorMaximized && <button type="button" className={`workbench-terminal-toggle ${terminalVisible ? "active" : ""}`} aria-label={terminalVisible ? t("terminal.collapse") : t("terminal.open")} aria-pressed={terminalVisible} title={terminalVisible ? t("terminal.collapse") : t("terminal.open")} onClick={toggleTerminal}><SquareTerminal size={15} strokeWidth={2} aria-hidden="true" /></button>}
        {onDetachInspector && <button type="button" className="workbench-inspector-detach" aria-label={t("inspector.detach")} title={t("inspector.detachHint")} onClick={detachInspector}><SquareArrowOutUpRight size={15} strokeWidth={2} aria-hidden="true" /></button>}
        <button type="button" className="workbench-inspector-maximize" aria-label={inspectorMaximized ? t("inspector.restore") : t("inspector.maximize")} aria-pressed={inspectorMaximized} title={inspectorMaximized ? t("inspector.restore") : t("inspector.maximize")} onClick={toggleInspectorMaximized}>{inspectorMaximized ? <Minimize2 size={15} strokeWidth={2} aria-hidden="true" /> : <Maximize2 size={15} strokeWidth={2} aria-hidden="true" />}</button>
        <button type="button" className="workbench-inspector-close" aria-label={t("inspector.collapse")} aria-expanded="true" aria-controls="workbench-inspector-content" title={t("inspector.collapse")} onClick={closeInspector}><X size={16} strokeWidth={2} aria-hidden="true" /></button>
      </>}
    />}
  </div>;
}

export default memo(TaskWorkbench);
