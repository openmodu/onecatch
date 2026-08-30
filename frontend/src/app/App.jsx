import { hasRemoteFSHarness, hydrateRuntimeHarnesses, supportsRuntimeProfile } from "./runtimeHarnesses.js";
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Folder, Lock, Minus, PanelRightClose, PanelRightOpen, PictureInPicture2, Square, SquareTerminal, X } from "lucide-react";
import { Events, Window } from "@wailsio/runtime";
import { Button } from "@/components/ui/button";
import { DialogFooter } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  RuntimeBinding,
  SettingsBinding,
  TaskRunBinding,
  WorkflowBinding,
  WorkspaceBinding,
  WorkerBinding,
  WindowBinding,
} from "../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { ConfirmDialog } from "./components/settings/ConfirmDialog.jsx";
import { demoSettings } from "./settingsDefaults.js";
import { mergeRunItems, preserveByFingerprint, runDetailFingerprint, runItemFingerprint, sortWorkspaces, workspaceResults } from "./listNavigation.js";
import { StatusBadge, TUISelect } from "../ui/primitives.jsx";
import { copy, errorMessage, fileName, taskTitleFromPrompt } from "./format.js";
import { loopTemplate } from "./templates.js";
// Demo fixtures back the fallback the UI drops into when the Wails bindings are
// missing — running the frontend standalone in a browser. That is a development
// path, so the fixtures are fetched on demand instead of riding along in the
// workbench's first load. Every reader below is behind `mode === "demo"`, which
// is only set once this holder is populated.
let demo = null;
import Sidebar from "./components/Sidebar.jsx";
import TaskWorkbench from "./components/TaskWorkbench.jsx";
import StatusPill from "./components/StatusPill.jsx";
import Modal from "./components/Modal.jsx";

// Everything below is reachable from the workbench but never on screen when it
// first paints. Importing them eagerly put the settings screen and the
// workflow editor into the first load the user waits through.
const SettingsPage = lazy(() => import("./SettingsPage.jsx"));
const WorkflowLibrary = lazy(() => import("./components/workflow/WorkflowLibrary.jsx"));
const WorkflowEditor = lazy(() => import("./components/workflow/WorkflowEditor.jsx"));
const WorkerPage = lazy(() => import("./components/WorkerPage.jsx"));
const WorkerModal = lazy(() => import("./components/WorkerModal.jsx"));
import { applyRunState, applyRuntimeFrames } from "./runtimeStream.js";
import { nextWorkflowDefinitionID } from "./workflowIds.js";
import LockScreen from "./components/LockScreen.jsx";
import { buildLockSignal, completionEdge, LOCK_PHASE } from "./lockSignal.js";
import { notifyStandby } from "./standbyNotify.js";
import { runtimesChangedEvent, settingsChangedEvent, workflowsChangedEvent } from "./auxiliaryWindowEvents.js";
import { readInspectorDetached, resolveInspectorCollapsed, writeInspectorDetached } from "./inspectorLayout.js";
import { buildInspectorContext, inspectorContextSignature, INSPECTOR_ACTION_EVENT, INSPECTOR_CONTEXT_EVENT, INSPECTOR_REQUEST_EVENT, INSPECTOR_WINDOW_EVENT } from "./inspectorContext.js";
import { demoClaudeConfiguration, demoCodexConfiguration } from "./codexRuntimeOptions.js";
import { collapsePanelAtCompact, COMPACT_LAYOUT_QUERY } from "./responsiveLayout.js";
import { scheduleIdle } from "./scheduleIdle.js";
import { REMOTE_FS_HEALTH_INTERVAL_MS, shouldAutoCheckRemoteFS } from "./remoteFSHealth.js";
import { newestTaskRun, normalizeTrayNavigation, TRAY_ACTION_EVENT } from "./trayNavigation.js";
import { LANGUAGE_CHANGED_EVENT } from "../i18n.js";
import { SIDEBAR_DEFAULT_WIDTH } from "./sidebarLayout.js";

const runtimeFrameEvent = "onecatch:runtime-frame";
const runStateEvent = "onecatch:run-state";
const listsChangedEvent = "onecatch:lists-changed";
// A dropped event should cost a stale list for a while, not forever. Long
// enough that an idle window is genuinely idle.
const LIST_HEARTBEAT_MS = 30_000;
const directAgentWorkflowID = "single_agent";

const emptyWorkspaceForm = () => ({ source: "local", path: "", remoteHost: "", remoteRoot: "", remoteUsername: "", remotePassword: "", name: "", defaultSandbox: "" });

function workspaceLocation(workspace) {
  return workspace?.remoteFs ? `${workspace.remoteFs.username ? `${workspace.remoteFs.username}@` : ""}${workspace.remoteFs.host}:${workspace.remoteFs.root}` : workspace?.path || "";
}

function firstWorkflowID() {
  return directAgentWorkflowID;
}

function isAvailableExecutionTarget(items = [], id = "") {
  return id === directAgentWorkflowID || items.some((item) => item.id === id);
}

function selectedTaskExecution(form) {
  const directAgent = form.workflowId === directAgentWorkflowID;
  return {
    directAgent,
    workflowId: form.workflowId,
    harness: directAgent ? form.harness || "codex" : "",
    model: directAgent ? form.model : "",
    reasoningEffort: directAgent ? form.reasoningEffort : "",
    serviceTier: directAgent ? form.serviceTier : "",
    sandbox: form.sandbox || "workspace-write",
  };
}

function DesktopTitleBar({ title = "", workspaceName = "", taskStatus = "", taskActive = false, showTaskStatus = false, showWorkbenchControls = false, terminalVisible = false, inspectorCollapsed = true, inspectorDetached = false, lockActive = 0, sidebarWidth = SIDEBAR_DEFAULT_WIDTH, onLock, onToggleTerminal, onToggleInspector, onDockInspector }) {
  const { t } = useTranslation();
  // Hyprland (and most Wayland tiling WMs) has no minimize concept and no
  // separate maximized size for a tiled window, so those two buttons would
  // just be dead weight there — same reasoning as GTK apps like Files, which
  // drop them under a tiling WM rather than ship controls that do nothing.
  const platform = typeof document === "undefined" ? "" : document.documentElement.dataset.platform;
  const isLinux = platform === "linux";
  const titlebarSidebarWidth = Math.max(sidebarWidth, 136);
  return <div className="windows-titlebar drag-region hidden h-9 items-center bg-background pl-3 text-xs text-foreground" style={{ "--windows-titlebar-sidebar-width": `${titlebarSidebarWidth}px` }} onDoubleClick={isLinux ? undefined : () => void Window.ToggleMaximise()}>
    <span className="windows-titlebar-brand flex items-center gap-2 font-medium"><img className="size-4 rounded-[4px]" src="/appicon.png" alt="" aria-hidden="true" /></span>
    {title && <span className="windows-titlebar-task pointer-events-none flex min-w-0 flex-1 items-center gap-2 px-3">
      {workspaceName && <Tooltip>
        <TooltipTrigger asChild>
          <span className="no-drag pointer-events-auto grid size-6 shrink-0 place-items-center rounded-md text-muted-foreground" aria-label={`${t("workspace.currentProject")}: ${workspaceName}`} onDoubleClick={(event) => event.stopPropagation()}>
            <Folder size={14} strokeWidth={2.1} aria-hidden="true" />
          </span>
        </TooltipTrigger>
        <TooltipContent side="bottom" sideOffset={6}>{workspaceName}</TooltipContent>
      </Tooltip>}
      <strong className="min-w-0 truncate text-[13px] font-semibold text-foreground" title={title}>{title}</strong>
      {showTaskStatus && <StatusPill status={taskStatus} active={taskActive} />}
    </span>}
    <div className="windows-titlebar-workbench-actions no-drag flex h-full shrink-0 items-center gap-1 px-2" onDoubleClick={(event) => event.stopPropagation()}>
      <button type="button" className="windows-titlebar-control relative" aria-label={t("lock.enter")} title={`${t("lock.enter")} · Ctrl+L`} onClick={onLock}>
        <Lock size={13} strokeWidth={2.5} aria-hidden="true" />
        {lockActive > 0 && <em className="absolute -top-0.5 -right-0.5 grid h-3.5 min-w-3.5 place-items-center rounded-full bg-primary px-1 text-[10px] font-bold not-italic text-primary-foreground">{lockActive}</em>}
      </button>
      {showWorkbenchControls && <>
        <button type="button" className={`windows-titlebar-control ${terminalVisible ? "active" : ""}`} aria-label={terminalVisible ? t("terminal.collapse") : t("terminal.open")} aria-pressed={terminalVisible} title={`${terminalVisible ? t("terminal.collapse") : t("terminal.open")} · Ctrl + \``} onClick={onToggleTerminal}><SquareTerminal size={15} strokeWidth={2} aria-hidden="true" /></button>
        {inspectorDetached
          ? <button type="button" className="windows-titlebar-control" aria-label={t("inspector.dock")} title={t("inspector.dockHint")} onClick={onDockInspector}><PictureInPicture2 size={16} strokeWidth={2} aria-hidden="true" /></button>
          : <button type="button" className={`windows-titlebar-control ${inspectorCollapsed ? "" : "active"}`} aria-label={inspectorCollapsed ? t("inspector.expand") : t("inspector.collapse")} aria-expanded={!inspectorCollapsed} aria-controls="workbench-inspector-content" title={inspectorCollapsed ? t("inspector.expand") : t("inspector.collapse")} onClick={onToggleInspector}>{inspectorCollapsed ? <PanelRightOpen size={16} strokeWidth={2} aria-hidden="true" /> : <PanelRightClose size={16} strokeWidth={2} aria-hidden="true" />}</button>}
      </>}
    </div>
    <div className="no-drag ml-auto flex h-full" onDoubleClick={(event) => event.stopPropagation()}>
      {!isLinux && <button type="button" className="windows-caption-button" aria-label="最小化" onClick={() => void Window.Minimise()}><Minus size={14} aria-hidden="true" /></button>}
      {!isLinux && <button type="button" className="windows-caption-button" aria-label="最大化或还原" onClick={() => void Window.ToggleMaximise()}><Square size={11} aria-hidden="true" /></button>}
      <button type="button" className="windows-caption-button close" aria-label="关闭" onClick={() => void Window.Close()}><X size={15} aria-hidden="true" /></button>
    </div>
  </div>;
}

function initialCompactViewport() {
  return typeof window !== "undefined" && typeof window.matchMedia === "function" && window.matchMedia(COMPACT_LAYOUT_QUERY).matches;
}

// Read before the first paint so a restored second-display layout never flashes
// the docked panel on the way to reopening its own window.
// Fallback for the lazily loaded views. They are all whole-pane screens, so the
// placeholder holds the pane rather than collapsing the layout under it.
function ViewLoading() {
  const { t } = useTranslation();
  return <div className="flex min-h-0 flex-1 items-center justify-center bg-background text-sm text-muted-foreground">{t("task.opening")}</div>;
}

function initialInspectorDetached() {
  try {
    return typeof window === "undefined" ? false : readInspectorDetached(window.localStorage);
  } catch {
    return false;
  }
}

function App() {
  const { t, i18n } = useTranslation();
  const [mode, setMode] = useState("loading");
  const [view, setView] = useState("tasks");
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [sidebarWidth, setSidebarWidth] = useState(SIDEBAR_DEFAULT_WIDTH);
  const [terminalVisible, setTerminalVisible] = useState(false);
  const [terminalToggleVersion, setTerminalToggleVersion] = useState(0);
  const [inspectorToggleVersion, setInspectorToggleVersion] = useState(0);
  const [runtimes, setRuntimes] = useState([]);
  const [workspaces, setWorkspaces] = useState([]);
  const [workspaceID, setWorkspaceID] = useState("");
  const [workflows, setWorkflows] = useState([]);
  const [runItems, setRunItems] = useState([]);
  const [tasks, setTasks] = useState([]);
  const [pinnedTasks, setPinnedTasks] = useState([]);
  const [runTotal, setRunTotal] = useState(0);
  const [runNextCursor, setRunNextCursor] = useState("");
  const [runLoading, setRunLoading] = useState(false);
  const [runStatus, setRunStatus] = useState("");
  const [runSearchDraft, setRunSearchDraft] = useState("");
  const [runKeyword, setRunKeyword] = useState("");
  const [globalSearchQuery, setGlobalSearchQuery] = useState("");
  const [globalTaskItems, setGlobalTaskItems] = useState([]);
  const [globalTaskSearchLoading, setGlobalTaskSearchLoading] = useState(false);
  const [workspaceExpanded, setWorkspaceExpanded] = useState(false);
  const [workspaceSearchOpen, setWorkspaceSearchOpen] = useState(false);
  const [selectedRunID, setSelectedRunID] = useState("");
  const [selectedQueuedTaskID, setSelectedQueuedTaskID] = useState("");
  const [runDetail, setRunDetail] = useState(null);
  const [editor, setEditor] = useState(null);
  const [editorSourceID, setEditorSourceID] = useState("");
  const [validation, setValidation] = useState([]);
  const [workspaceModal, setWorkspaceModal] = useState(false);
  const [workspaceEditingID, setWorkspaceEditingID] = useState("");
  // A task-less cold start is still an actionable chat surface. Starting in
  // compose mode avoids flashing the legacy welcome card while the first
  // workspace's task history is loaded; selecting any history row closes it.
  const [taskModal, setTaskModal] = useState(true);
  const [renameForm, setRenameForm] = useState(null);
  const [workspaceForm, setWorkspaceForm] = useState(emptyWorkspaceForm);
  const [taskForm, setTaskForm] = useState({ prompt: "", workflowId: directAgentWorkflowID, executionMode: "immediate", attachmentPaths: [], harness: "codex", model: "", reasoningEffort: "", serviceTier: "", sandbox: "workspace-write" });
  const [taskRuntimeConfiguration, setTaskRuntimeConfiguration] = useState({ loading: false, data: null, error: "" });
  const [composerAttachments, setComposerAttachments] = useState([]);
  const [resumePendingRunID, setResumePendingRunID] = useState("");
  const [notice, setNotice] = useState(null);
  const [busy, setBusy] = useState("");
  const [locked, setLocked] = useState(false);
  const [permissionBusy, setPermissionBusy] = useState("");
  const [workers, setWorkers] = useState([]);
  const [workerHealth, setWorkerHealth] = useState({});
  const [remoteWorkspaceHealth, setRemoteWorkspaceHealth] = useState({});
  const [workerModal, setWorkerModal] = useState(false);
  const [workerForm, setWorkerForm] = useState({ id: "", name: "", baseUrl: "https://", caFile: "", clientCertFile: "", clientKeyFile: "", serverName: "", serverCertificateSha256: "", enabled: true });
  const [settings, setSettings] = useState(demoSettings);
  const [appDialog, setAppDialog] = useState(null);
  // The inspector opens on request and closes with the window. Its expanded
  // state is deliberately not carried across launches: it is a panel you open
  // to answer a question, and starting every session with whatever was on
  // screen when the last one ended is not the same thing as wanting it there.
  const [inspectorPreference, setInspectorPreference] = useState(true);
  const [inspectorDetached, setInspectorDetached] = useState(initialInspectorDetached);
  const [compactViewport, setCompactViewport] = useState(initialCompactViewport);
  const [terminalCommand, setTerminalCommand] = useState({ version: 0, command: "" });
  const appDialogResolve = useRef(null);
  const inspectorRestoredRef = useRef(false);
  const runListLoadVersion = useRef(0);
  const runLoadVersion = useRef(0);
  const globalTaskSearchVersion = useRef(0);
  const runActionPending = useRef("");
  const trayTaskRequestRef = useRef(null);
  const selectedRunIDRef = useRef("");
  const liveFramesRef = useRef([]);
  const liveFrameHistoryRef = useRef([]);
  const liveFrameGenerationRef = useRef(0);
  const liveFlushTimerRef = useRef(0);
  // Latest-value refs so the polling loops can read fresh state without listing
  // `tasks`/`runDetail` as effect deps (which would tear down and rebuild the
  // timer on every tick — a big source of the jank).
  const runDetailRef = useRef(null);
  runDetailRef.current = runDetail;
  selectedRunIDRef.current = selectedRunID;
  const tasksRef = useRef([]);
  tasksRef.current = tasks;
  // Handlers passed down to memoized children read these instead of closing over
  // the fast-changing state directly, so the callbacks stay reference-stable and
  // the children's memo() actually holds across the 80ms streaming cadence.
  const selectedQueuedTaskIDRef = useRef("");
  selectedQueuedTaskIDRef.current = selectedQueuedTaskID;
  const composerAttachmentsRef = useRef([]);
  composerAttachmentsRef.current = composerAttachments;
  const runNextCursorRef = useRef("");
  runNextCursorRef.current = runNextCursor;
  const remoteWorkspaceHealthRef = useRef({});
  remoteWorkspaceHealthRef.current = remoteWorkspaceHealth;
  const remoteWorkspaceHealthVersions = useRef(new Map());
  const remoteWorkspacesRef = useRef([]);
  remoteWorkspacesRef.current = workspaces.filter((workspace) => workspace.remoteFs);

  const selectedWorkspace = workspaces.find((item) => item.id === workspaceID);
  const remoteFSHarnessAvailable = hasRemoteFSHarness(runtimes, settings.runtimes);
  const remoteWorkspaceSignature = useMemo(() => workspaces
    .filter((workspace) => workspace.remoteFs)
    .map((workspace) => `${workspace.id}\0${workspace.remoteFs.host}\0${workspace.remoteFs.username || ""}\0${workspace.remoteFs.root}`)
    .join("\n"), [workspaces]);
  // A detached inspector leaves no dock behind, so the workbench sees the same
  // "no panel here" layout it uses for a deliberate collapse.
  const inspectorCollapsed = inspectorDetached || resolveInspectorCollapsed(inspectorPreference);
  const desktopChrome = typeof document !== "undefined" && ["windows", "linux"].includes(document.documentElement.dataset.platform);

  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") return undefined;
    const media = window.matchMedia(COMPACT_LAYOUT_QUERY);
    const update = (event) => {
      setCompactViewport(event.matches);
      setInspectorPreference((current) => collapsePanelAtCompact(current, event.matches));
    };
    update(media);
    if (typeof media.addEventListener === "function") {
      media.addEventListener("change", update);
      return () => media.removeEventListener("change", update);
    }
    media.addListener?.(update);
    return () => media.removeListener?.(update);
  }, []);

  const toggleInspector = useCallback(() => {
    setInspectorPreference((current) => !resolveInspectorCollapsed(current));
  }, []);

  // Both toggles move optimistically so the layout responds immediately; the
  // native window then confirms (or corrects) the state on INSPECTOR_WINDOW_EVENT.
  const detachInspector = useCallback(() => {
    setInspectorDetached(true);
    writeInspectorDetached(window.localStorage, true);
    void WindowBinding.OpenInspector();
  }, []);

  const dockInspector = useCallback(() => {
    setInspectorDetached(false);
    writeInspectorDetached(window.localStorage, false);
    void WindowBinding.CloseInspector();
  }, []);

  const notify = useCallback((type, text) => {
    setNotice({ type, text });
    window.setTimeout(() => setNotice(null), 4200);
  }, []);

  const checkRemoteWorkspaceHealth = useCallback(async (workspace) => {
    if (!workspace?.remoteFs || mode === "loading") return;
    const version = (remoteWorkspaceHealthVersions.current.get(workspace.id) || 0) + 1;
    remoteWorkspaceHealthVersions.current.set(workspace.id, version);
    setRemoteWorkspaceHealth((current) => ({
      ...current,
      [workspace.id]: { ...current[workspace.id], checking: true, error: "" },
    }));
    try {
      if (mode === "wails") await WorkspaceBinding.GetWorkspaceStatus(workspace.id);
      if (remoteWorkspaceHealthVersions.current.get(workspace.id) !== version) return;
      setRemoteWorkspaceHealth((current) => ({
        ...current,
        [workspace.id]: { healthy: true, checking: false, error: "", checkedAt: new Date().toISOString() },
      }));
    } catch (error) {
      if (remoteWorkspaceHealthVersions.current.get(workspace.id) !== version) return;
      setRemoteWorkspaceHealth((current) => ({
        ...current,
        [workspace.id]: { healthy: false, checking: false, error: errorMessage(error), checkedAt: new Date().toISOString() },
      }));
    }
  }, [mode]);

  const requestConfirm = useCallback((options) => new Promise((resolve) => {
    appDialogResolve.current?.(false);
    appDialogResolve.current = resolve;
    setAppDialog(options);
  }), []);
  const resolveConfirm = useCallback((accepted) => {
    const resolve = appDialogResolve.current;
    appDialogResolve.current = null;
    setAppDialog(null);
    resolve?.(accepted);
  }, []);

  const selectWorkspace = useCallback((nextWorkspaceID) => {
    if (!nextWorkspaceID) return;
    if (nextWorkspaceID === workspaceID) {
      setGlobalSearchQuery("");
      setWorkspaceSearchOpen(false);
      return;
    }
    runListLoadVersion.current += 1;
    runLoadVersion.current += 1;
    setWorkspaceID(nextWorkspaceID);
    setWorkspaces((items) => items.map((item) => item.id === nextWorkspaceID ? { ...item, lastOpenedAt: new Date().toISOString() } : item));
    setRunItems([]);
    setTasks([]);
    setRunTotal(0);
    setRunNextCursor("");
    setSelectedRunID("");
    setSelectedQueuedTaskID("");
    setRunDetail(null);
    setResumePendingRunID("");
    setTaskForm((form) => ({ ...form, attachmentPaths: [] }));
    setComposerAttachments([]);
    setGlobalSearchQuery("");
    setWorkspaceSearchOpen(false);
    if (mode === "wails") WorkspaceBinding.OpenWorkspace(nextWorkspaceID).then((opened) => setWorkspaces((items) => items.map((item) => item.id === opened.id ? opened : item))).catch((error) => notify("error", errorMessage(error)));
  }, [mode, notify, workspaceID]);

  // Boot is split by what the shell actually cannot draw without.
  //
  // Only the workspace list and settings qualify: everything else fills in
  // behind a rendered window. Waiting on all five calls meant the slowest one
  // set the time to first paint, and the slowest is ListRuntimes, which may
  // spawn `<cli> --version` per runtime. Half a second of process startup used
  // to sit in front of an empty window.
  const boot = useCallback(async () => {
    let firstWorkspaceID = "";
    try {
      const [workspaceItems, settingsValue] = await Promise.all([
        WorkspaceBinding.ListWorkspaces(), SettingsBinding.GetSettings(),
      ]);
      const orderedWorkspaces = sortWorkspaces(workspaceItems || []);
      firstWorkspaceID = orderedWorkspaces[0]?.id || "";
      setMode("wails");
      setWorkspaces(orderedWorkspaces);
      setSettings(settingsValue || demoSettings);
      setWorkspaceID((current) => current || firstWorkspaceID);
    } catch {
      demo = await import("./demoData.js");
      setRuntimes(hydrateRuntimeHarnesses(demo.demoRuntimes));
      setWorkspaces(sortWorkspaces(demo.demoWorkspaces));
      setWorkflows(demo.demoWorkflows);
      setWorkers(demo.demoWorkers);
      setSettings(demoSettings);
      setWorkspaceID(demo.demoWorkspaces[0].id);
      setTaskForm((current) => ({ ...current, workflowId: firstWorkflowID() }));
      setRunItems([{ ...demo.demoRun.run, task: demo.demoTasks[0] }]);
      setTasks(demo.demoTasks);
      setRunTotal(1);
      setSelectedRunID("run_demo");
      setRunDetail(demo.demoRun);
      // Set last: everything reading `demo` is guarded by this mode.
      setMode("demo");
      return;
    }
    // Marking the workspace as opened is bookkeeping for the next launch's sort
    // order. It never gated the first paint's correctness, only its timing, so
    // it no longer runs before it.
    if (!firstWorkspaceID) return;
    WorkspaceBinding.OpenWorkspace(firstWorkspaceID)
      .then((opened) => setWorkspaces((items) => sortWorkspaces(items.map((item) => item.id === opened.id ? opened : item))))
      .catch(() => {
        // Preference bookkeeping must not disturb an open workbench.
      });
  }, []);

  useEffect(() => { boot(); }, [boot]);

  // Language is a WebView preference, while the macOS tray lives in Go.
  // Announce the persisted value after the bridge is ready so a fresh launch
  // does not briefly lock the native menu to its default language.
  useEffect(() => {
    if (mode !== "wails") return;
    void Events.Emit(LANGUAGE_CHANGED_EVENT, i18n.resolvedLanguage || i18n.language || "en");
  }, [i18n.language, i18n.resolvedLanguage, mode]);

  useEffect(() => {
    if (mode === "loading") return undefined;
    const checkEligibleTargets = () => {
      for (const workspace of remoteWorkspacesRef.current) {
        if (shouldAutoCheckRemoteFS(remoteWorkspaceHealthRef.current[workspace.id])) {
          void checkRemoteWorkspaceHealth(workspace);
        }
      }
    };
    checkEligibleTargets();
    const interval = window.setInterval(checkEligibleTargets, REMOTE_FS_HEALTH_INTERVAL_MS);
    return () => window.clearInterval(interval);
  }, [checkRemoteWorkspaceHealth, mode, remoteWorkspaceSignature]);

  // The rest of the workbench's reference data. None of it is needed to draw
  // the shell, so it loads once the window is idle rather than in front of it.
  useEffect(() => {
    if (mode !== "wails") return undefined;
    let active = true;
    const cancelIdle = scheduleIdle(() => {
      Promise.allSettled([RuntimeBinding.ListRuntimes(), WorkflowBinding.ListDefinitions(), WorkerBinding.ListWorkers()])
        .then(([runtimeResult, workflowResult, workerResult]) => {
          if (!active) return;
          if (runtimeResult.status === "fulfilled") setRuntimes(hydrateRuntimeHarnesses(runtimeResult.value || []));
          if (workerResult.status === "fulfilled") setWorkers(workerResult.value || []);
          if (workflowResult.status !== "fulfilled") return;
          const workflowItems = workflowResult.value || [];
          setWorkflows(workflowItems);
          setTaskForm((current) => ({ ...current, workflowId: isAvailableExecutionTarget(workflowItems, current.workflowId) ? current.workflowId : firstWorkflowID() }));
        });
    });
    return () => { active = false; cancelIdle(); };
  }, [mode]);

  // A cached runtime status is served immediately and re-probed in the
  // background; this is how the corrected list arrives.
  useEffect(() => {
    if (mode !== "wails") return undefined;
    return Events.On(runtimesChangedEvent, (event) => {
      if (Array.isArray(event?.data)) setRuntimes(hydrateRuntimeHarnesses(event.data));
    });
  }, [mode]);

  const inspectRuntimeConfiguration = useCallback(async (harness) => {
    if (!supportsRuntimeProfile(harness)) return null;
    if (mode === "demo") {
      if (harness === "codex") return demoCodexConfiguration;
      if (harness === "claude") return demoClaudeConfiguration;
      return null;
    }
    const runtimeSettings = settings.runtimes?.[harness] || {};
    if (harness === "codex") return SettingsBinding.InspectCodexConfiguration(runtimeSettings);
    if (harness === "claude") return SettingsBinding.InspectClaudeConfiguration(runtimeSettings);
    return SettingsBinding.InspectHarnessConfiguration(harness, runtimeSettings);
  }, [mode, settings.runtimes]);

  useEffect(() => {
    if (!taskModal || mode === "loading") return undefined;
    if (taskForm.workflowId !== directAgentWorkflowID) {
      setTaskRuntimeConfiguration({ loading: false, data: null, error: "" });
      return undefined;
    }
    const harness = taskForm.harness || "codex";
    if (!supportsRuntimeProfile(harness)) {
      setTaskRuntimeConfiguration({ loading: false, data: null, error: "" });
      return undefined;
    }
    let cancelled = false;
    setTaskRuntimeConfiguration({ loading: true, data: null, error: "" });
    inspectRuntimeConfiguration(harness)
      .then((data) => { if (!cancelled) setTaskRuntimeConfiguration({ loading: false, data, error: "" }); })
      .catch((error) => { if (!cancelled) setTaskRuntimeConfiguration((current) => ({ ...current, loading: false, error: errorMessage(error) })); });
    return () => { cancelled = true; };
  }, [inspectRuntimeConfiguration, mode, taskForm.harness, taskForm.workflowId, taskModal]);

  // Settings and workflow definitions are edited in their own WebViews. Wails
  // custom events are application-wide, so the main task window can refresh
  // just the shared data that changed without running duplicate polling loops.
  useEffect(() => {
    if (mode !== "wails") return undefined;
    const offSettings = Events.On(settingsChangedEvent, async () => {
      try {
        const [value, workerItems] = await Promise.all([SettingsBinding.GetSettings(), WorkerBinding.ListWorkers()]);
        setSettings(value || demoSettings);
        setWorkers(workerItems || []);
      } catch (error) {
        notify("error", errorMessage(error));
      }
    });
    const offWorkflows = Events.On(workflowsChangedEvent, async () => {
      try {
        const items = await WorkflowBinding.ListDefinitions();
        setWorkflows(items || []);
        setTaskForm((form) => isAvailableExecutionTarget(items, form.workflowId) ? form : { ...form, workflowId: firstWorkflowID() });
      } catch (error) {
        notify("error", errorMessage(error));
      }
    });
    return () => { offSettings(); offWorkflows(); };
  }, [mode, notify]);

  // --- Detached inspector -------------------------------------------------
  // The inspector may live in its own window. Go owns that window's lifecycle
  // and reports it here, so a close from the native title bar re-docks the
  // panel just like the re-dock button does.
  useEffect(() => {
    if (mode !== "wails") return undefined;
    return Events.On(INSPECTOR_WINDOW_EVENT, (event) => {
      const open = Boolean(event?.data?.open);
      setInspectorDetached(open);
      writeInspectorDetached(window.localStorage, open);
    });
  }, [mode]);

  // Reopen a previously detached inspector once per launch, so a second-display
  // setup survives a restart. A fresh installation is docked and does nothing.
  useEffect(() => {
    if (mode !== "wails" || inspectorRestoredRef.current) return;
    inspectorRestoredRef.current = true;
    if (readInspectorDetached(window.localStorage)) void WindowBinding.OpenInspector();
  }, [mode]);

  const inspectorContext = useMemo(
    () => buildInspectorContext({ mode, workspaceID, remoteFS: selectedWorkspace?.remoteFs || null, runDetail, tasks, selectedQueuedTaskID, draft: taskModal }),
    [mode, runDetail, selectedQueuedTaskID, selectedWorkspace?.remoteFs, taskModal, tasks, workspaceID],
  );
  const inspectorContextRef = useRef(inspectorContext);
  inspectorContextRef.current = inspectorContext;
  const inspectorSignature = inspectorContextSignature(inspectorContext);

  // Keyed on the signature, not the object: `runDetail` is replaced every 80ms
  // while an agent streams, but none of the transcript reaches the inspector,
  // so this fires only when something the panel actually renders changed.
  useEffect(() => {
    if (!inspectorDetached || mode !== "wails") return;
    void Events.Emit(INSPECTOR_CONTEXT_EVENT, inspectorContextRef.current);
  }, [inspectorDetached, inspectorSignature, mode]);

  useEffect(() => {
    if (mode !== "wails") return undefined;
    const offRequest = Events.On(INSPECTOR_REQUEST_EVENT, () => {
      void Events.Emit(INSPECTOR_CONTEXT_EVENT, inspectorContextRef.current);
    });
    // The terminal dock only exists in this window.
    const offAction = Events.On(INSPECTOR_ACTION_EVENT, (event) => {
      if (event?.data?.type !== "open-terminal" || !event.data.command) return;
      setTerminalCommand((current) => ({ version: current.version + 1, command: event.data.command }));
    });
    return () => { offRequest(); offAction(); };
  }, [mode]);
  // ------------------------------------------------------------------------

  useEffect(() => {
    const timer = window.setTimeout(() => setRunKeyword(runSearchDraft.trim()), 220);
    return () => window.clearTimeout(timer);
  }, [runSearchDraft]);

  useEffect(() => {
    if (!workspaceSearchOpen || mode === "loading") return undefined;
    const loadVersion = ++globalTaskSearchVersion.current;
    const timer = window.setTimeout(async () => {
      setGlobalTaskSearchLoading(true);
      try {
        if (mode === "demo") {
          const keyword = globalSearchQuery.trim().toLocaleLowerCase();
          const workspaceByID = new Map(demo.demoWorkspaces.map((workspace) => [workspace.id, workspace]));
          const items = demo.demoTasks.map((task) => ({ task, workspace: workspaceByID.get(task.workspaceId), latestRun: demo.demoRun.run.taskId === task.id ? demo.demoRun.run : null }))
            .filter((item) => item.workspace && (!keyword || `${item.task.title}\n${item.task.prompt}\n${item.workspace.name}\n${item.workspace.path}\n${item.workspace.remoteFs?.username || ""}\n${item.workspace.remoteFs?.host || ""}`.toLocaleLowerCase().includes(keyword)))
            .slice(0, 50);
          if (loadVersion === globalTaskSearchVersion.current) setGlobalTaskItems(items);
          return;
        }
        const page = await TaskRunBinding.SearchTasks({ keyword: globalSearchQuery.trim(), limit: 50 });
        if (loadVersion === globalTaskSearchVersion.current) setGlobalTaskItems(page.items || []);
      } catch (error) {
        if (loadVersion === globalTaskSearchVersion.current) notify("error", errorMessage(error));
      } finally {
        if (loadVersion === globalTaskSearchVersion.current) setGlobalTaskSearchLoading(false);
      }
    }, 140);
    return () => window.clearTimeout(timer);
  }, [globalSearchQuery, mode, notify, workspaceSearchOpen]);

  const loadRunList = useCallback(async ({ cursor = "" } = {}) => {
    const append = Boolean(cursor);
    if (!workspaceID || mode === "loading") return;
    const loadVersion = ++runListLoadVersion.current;
    setRunLoading(true);
    try {
      if (runStatus === "queued") {
        if (loadVersion !== runListLoadVersion.current) return;
        setRunItems([]);
        setRunTotal(0);
        setRunNextCursor("");
        return;
      }
      if (mode === "demo") {
        const keyword = runKeyword.toLocaleLowerCase();
        const items = [{ ...demo.demoRun.run, task: demo.demoTasks[0] }].filter((item) => (!runStatus || item.status === runStatus) && (!keyword || `${item.id}\n${item.task.title}\n${Object.values(item.sessions || {}).join("\n")}`.toLocaleLowerCase().includes(keyword)));
        if (loadVersion !== runListLoadVersion.current) return;
        setRunItems(items);
        setRunTotal(items.length);
        setRunNextCursor("");
        return;
      }
      const page = await TaskRunBinding.ListRuns({ workspaceId: workspaceID, status: runStatus, keyword: runKeyword, cursor, limit: 50 });
      if (loadVersion !== runListLoadVersion.current) return;
      const incoming = (page.items || []).map((item) => ({ ...item.run, task: item.task }));
      setRunItems((current) => mergeRunItems(current, incoming, !append));
      setRunTotal(page.total || 0);
      setRunNextCursor(page.nextCursor || "");
    } catch (error) {
      if (loadVersion === runListLoadVersion.current) notify("error", errorMessage(error));
    } finally {
      if (loadVersion === runListLoadVersion.current) setRunLoading(false);
    }
  }, [mode, notify, runKeyword, runStatus, workspaceID]);

  const loadTasks = useCallback(async () => {
    if (!workspaceID || mode === "loading") return;
    try {
      if (mode === "demo") {
        setTasks(demo.demoTasks);
        setPinnedTasks(demo.demoTasks.filter((task) => task.pinned));
        return;
      }
      const [workspaceTasks, allTasks] = await Promise.all([
        TaskRunBinding.ListTasks(workspaceID),
        TaskRunBinding.ListTasks(""),
      ]);
      setTasks(workspaceTasks || []);
      setPinnedTasks((allTasks || []).filter((task) => task.pinned));
    } catch (error) {
      notify("error", errorMessage(error));
    }
  }, [mode, notify, workspaceID]);

  useEffect(() => {
    if (mode === "loading" || !workspaceID) return;
    runListLoadVersion.current += 1;
    runLoadVersion.current += 1;
    setRunItems([]);
    setTasks([]);
    setRunTotal(0);
    setRunNextCursor("");
    setSelectedRunID("");
    setSelectedQueuedTaskID("");
    setRunDetail(null);
    loadRunList();
    loadTasks();
  }, [loadRunList, loadTasks, mode, runKeyword, runStatus, workspaceID]);

  // Opening a run ships only the newest slice of its transcript, because every
  // entry becomes a mounted Markdown component. These runs are the ones the user
  // asked to see in full; the choice has to survive the reloads that follow.
  const expandedTranscripts = useRef(new Set());

  const withFullTranscript = useCallback(async (runID, detail) => {
    if (!expandedTranscripts.current.has(runID)) return detail;
    try {
      const runtimeEvents = await TaskRunBinding.GetRunTranscript(runID) || [];
      return { ...detail, runtimeEvents, runtimeEventsTotal: runtimeEvents.length };
    } catch {
      // The window is still a usable view of the run; the control stays offered.
      return detail;
    }
  }, []);

  const loadEarlierTranscript = useCallback(async (runID) => {
    if (!runID || mode !== "wails") return;
    expandedTranscripts.current.add(runID);
    try {
      const runtimeEvents = await TaskRunBinding.GetRunTranscript(runID) || [];
      setRunDetail((current) => current?.run?.id === runID
        ? { ...current, runtimeEvents, runtimeEventsTotal: runtimeEvents.length }
        : current);
    } catch (error) {
      expandedTranscripts.current.delete(runID);
      notify("error", errorMessage(error));
    }
  }, [mode, notify]);

  const loadRun = useCallback(async (runID, silent = false) => {
    if (!runID) return;
    const loadVersion = ++runLoadVersion.current;
    const liveGeneration = liveFrameGenerationRef.current;
    if (mode === "demo") { setRunDetail((current) => current?.run?.id === runID ? current : demo.demoRun); return; }
    try {
      let detail = await TaskRunBinding.GetRun(runID);
      detail = await withFullTranscript(runID, detail);
      // GetRun is durable truth; the in-memory snapshot fills the small gap
      // between the latest 500ms persistence batch and this read. Revisions
      // make overlap with already-delivered Wails events harmless.
      try {
        detail = applyRuntimeFrames(detail, await TaskRunBinding.GetRunStreamSnapshot(runID));
      } catch {
        // Streaming is best effort. A durable detail still keeps the run usable
        // with an older backend or during application shutdown.
      }
      // Frames emitted after this load began may race the snapshot response.
      // Replaying this short revisioned history closes that final window; any
      // overlap with the snapshot is discarded by applyRuntimeFrames.
      detail = applyRuntimeFrames(detail, liveFrameHistoryRef.current
        .filter((entry) => entry.generation > liveGeneration && entry.frame.runId === runID)
        .map((entry) => entry.frame));
      if (loadVersion !== runLoadVersion.current) return;
      // Keep the same object reference when a poll returns identical data, so an
      // idle refresh (or the fast post-action nudge) triggers no re-render and
      // therefore no flicker.
      setRunDetail((current) => preserveByFingerprint(current, detail, runDetailFingerprint));
      setRunItems((items) => {
        let changed = false;
        const next = items.map((item) => {
          if (item.id !== runID) return item;
          const merged = preserveByFingerprint(item, { ...detail.run, task: detail.task }, runItemFingerprint);
          if (merged !== item) changed = true;
          return merged;
        });
        return changed ? next : items;
      });
      if (!silent) setSelectedRunID(runID);
    } catch (error) { if (loadVersion === runLoadVersion.current && !silent) notify("error", errorMessage(error)); }
  }, [mode, notify, withFullTranscript]);

  useEffect(() => { if (selectedRunID) loadRun(selectedRunID, true); }, [loadRun, selectedRunID]);

  useEffect(() => {
    if (mode !== "wails") return undefined;
    const flush = () => {
      liveFlushTimerRef.current = 0;
      const frames = liveFramesRef.current;
      liveFramesRef.current = [];
      if (!frames.length) return;
      setRunDetail((current) => applyRuntimeFrames(current, frames));
    };
    const off = Events.On(runtimeFrameEvent, (event) => {
      const frame = event.data;
      if (!frame || frame.runId !== selectedRunIDRef.current) return;
      liveFrameGenerationRef.current += 1;
      liveFrameHistoryRef.current.push({ generation: liveFrameGenerationRef.current, frame });
      if (liveFrameHistoryRef.current.length > 512) liveFrameHistoryRef.current.splice(0, 256);
      liveFramesRef.current.push(frame);
      if (!liveFlushTimerRef.current) liveFlushTimerRef.current = window.setTimeout(flush, 80);
      // Run/step status no longer needs a transcript re-read here: the backend
      // pushes the bounded run state on the onecatch:run-state channel below.
    });
    return () => {
      off();
      window.clearTimeout(liveFlushTimerRef.current);
      liveFlushTimerRef.current = 0;
      liveFramesRef.current = [];
      liveFrameHistoryRef.current = [];
    };
  }, [mode]);

  // The bounded half of a run — status, step runs, instructions, active — is
  // pushed from Go whenever it changes, so the UI header/inspector/composer stay
  // live without ever re-reading the (unbounded) transcript on a timer.
  useEffect(() => {
    if (mode !== "wails") return undefined;
    const off = Events.On(runStateEvent, (event) => {
      const view = event.data;
      if (!view || view.runId !== selectedRunIDRef.current) return;
      setRunDetail((current) => applyRunState(current, view));
    });
    return () => off();
  }, [mode]);

  useEffect(() => {
    if (!resumePendingRunID) return;
    if (selectedRunID !== resumePendingRunID || runDetail?.run?.id !== resumePendingRunID || runDetail.run.status !== "paused" || !runDetail.active) {
      setResumePendingRunID("");
    }
  }, [resumePendingRunID, runDetail?.active, runDetail?.run?.id, runDetail?.run?.status, selectedRunID]);

  // Desktop events are best-effort. Instead of continuously polling to recover a
  // dropped run-state push, reconcile once when the window regains focus or
  // becomes visible again — the only moments a stale header would be seen.
  useEffect(() => {
    if (!selectedRunID || mode !== "wails") return undefined;
    const reconcile = () => {
      if (document.visibilityState === "hidden") return;
      if (selectedRunIDRef.current) loadRun(selectedRunIDRef.current, true);
    };
    window.addEventListener("focus", reconcile);
    document.addEventListener("visibilitychange", reconcile);
    return () => {
      window.removeEventListener("focus", reconcile);
      document.removeEventListener("visibilitychange", reconcile);
    };
  }, [loadRun, mode, selectedRunID]);

  // The task queue and run list refresh when Go says a write moved them.
  //
  // This used to be a self-scheduling poll running as often as every 1.4
  // seconds, which re-read every task file twice and up to fifty run files per
  // tick to almost always find nothing had changed. The timer that remains is a
  // safety net for a dropped event, not the mechanism — and it stops entirely
  // while the window is hidden, where a refresh has nobody to show anything to.
  useEffect(() => {
    if (!workspaceID || mode !== "wails") return undefined;
    let stopped = false;
    let pending = 0;
    let timer = 0;
    const refresh = () => {
      if (stopped || document.visibilityState === "hidden") return;
      const refreshes = [loadTasks(), loadRunList()];
      if (selectedRunIDRef.current) refreshes.push(loadRun(selectedRunIDRef.current, true));
      void Promise.all(refreshes);
    };
    // Go already coalesces a burst of writes; this absorbs the rest, and keeps
    // a single user action from firing two refreshes.
    const scheduleRefresh = () => {
      if (stopped || pending) return;
      pending = window.setTimeout(() => { pending = 0; refresh(); }, 120);
    };
    const off = Events.On(listsChangedEvent, scheduleRefresh);
    const heartbeat = () => {
      refresh();
      if (!stopped) timer = window.setTimeout(heartbeat, LIST_HEARTBEAT_MS);
    };
    timer = window.setTimeout(heartbeat, LIST_HEARTBEAT_MS);
    document.addEventListener("visibilitychange", scheduleRefresh);
    return () => {
      stopped = true;
      off();
      document.removeEventListener("visibilitychange", scheduleRefresh);
      window.clearTimeout(timer);
      window.clearTimeout(pending);
    };
  }, [loadRun, loadRunList, loadTasks, mode, workspaceID]);

  useEffect(() => {
    if (!selectedQueuedTaskID || mode !== "wails") return undefined;
    const task = tasks.find((item) => item.id === selectedQueuedTaskID);
    if (!task || task.status === "queued") return undefined;
    let cancelled = false;
    TaskRunBinding.ListRunsByTask(task.id).then((runs) => {
      if (cancelled || !runs?.[0]) return;
      setSelectedQueuedTaskID("");
      setSelectedRunID(runs[0].id);
      loadRun(runs[0].id);
    }).catch((error) => { if (!cancelled) notify("error", errorMessage(error)); });
    return () => { cancelled = true; };
  }, [loadRun, mode, notify, selectedQueuedTaskID, tasks]);

  const chooseWorkspace = useCallback(async () => {
    setWorkspaceEditingID("");
    setWorkspaceForm(mode === "demo" ? { ...emptyWorkspaceForm(), path: "/Users/demo/Code/my-project", name: "my-project" } : emptyWorkspaceForm());
    setWorkspaceModal(true);
  }, [mode]);

  const editWorkspace = useCallback(() => {
    if (!selectedWorkspace) return;
    setWorkspaceEditingID(selectedWorkspace.id);
    setWorkspaceForm({
      source: selectedWorkspace.remoteFs ? "remote" : "local",
      path: selectedWorkspace.remoteFs ? "" : selectedWorkspace.path,
      remoteHost: selectedWorkspace.remoteFs?.host || "",
      remoteRoot: selectedWorkspace.remoteFs?.root || "",
      remoteUsername: selectedWorkspace.remoteFs?.username || "",
      remotePassword: "",
      name: selectedWorkspace.name || "",
      defaultSandbox: selectedWorkspace.defaultSandbox || "workspace-write",
    });
    setWorkspaceModal(true);
  }, [selectedWorkspace]);

  const browseWorkspace = useCallback(async () => {
    if (mode === "demo") { setWorkspaceForm((form) => ({ ...form, source: "local", path: "/Users/demo/Code/my-project" })); return; }
    try {
      const path = await WorkspaceBinding.ChooseDirectory();
      if (path) setWorkspaceForm((form) => ({ ...form, source: "local", path }));
    } catch (error) { notify("error", errorMessage(error)); }
  }, [mode, notify]);

  const saveWorkspace = async () => {
    const remote = workspaceForm.source === "remote";
    if (!remote && !workspaceForm.path.trim()) { notify("error", t("app.workspacePathRequired")); return; }
    if (remote && (!workspaceForm.remoteHost.trim() || !workspaceForm.remoteRoot.trim())) { notify("error", t("app.remoteFSFieldsRequired")); return; }
    if (remote && workspaceForm.remotePassword && !workspaceForm.remoteUsername.trim()) { notify("error", t("app.remoteFSCredentialsRequired")); return; }
    if (remote && !remoteFSHarnessAvailable) { notify("error", t("app.remoteFSHarnessRequired")); return; }
    if (workspaceForm.defaultSandbox === "full" && !await requestConfirm({ title: t("app.fullWorkspaceTitle"), description: t("app.fullWorkspaceDescription"), detail: t("app.fullWorkspaceDetail"), confirmLabel: t("app.confirmFullAccess"), dangerous: true })) return;
    setBusy("workspace");
    try {
      const payload = remote
        ? { path: workspaceForm.remoteRoot.trim(), name: workspaceForm.name, defaultSandbox: workspaceForm.defaultSandbox || "workspace-write", remoteFs: { host: workspaceForm.remoteHost.trim(), root: workspaceForm.remoteRoot.trim(), username: workspaceForm.remoteUsername.trim() }, ...(workspaceForm.remotePassword ? { password: workspaceForm.remotePassword } : {}) }
        : { path: workspaceForm.path.trim(), name: workspaceForm.name, defaultSandbox: workspaceForm.defaultSandbox };
      if (mode === "demo") {
        const { password: _password, ...safePayload } = payload;
        const item = { ...safePayload, id: workspaceEditingID || `workspace-${Date.now()}`, name: payload.name || payload.path.split("/").pop(), lastOpenedAt: new Date().toISOString() };
        setWorkspaces((items) => workspaceEditingID ? items.map((current) => current.id === workspaceEditingID ? { ...current, ...item } : current) : [item, ...items]); selectWorkspace(item.id);
      } else {
        const item = workspaceEditingID ? await WorkspaceBinding.UpdateWorkspace({ id: workspaceEditingID, ...payload }) : await WorkspaceBinding.AddWorkspace(payload);
        setWorkspaces(await WorkspaceBinding.ListWorkspaces()); selectWorkspace(item.id);
      }
      setWorkspaceModal(false); setWorkspaceEditingID(""); notify("success", t(workspaceEditingID ? "app.workspaceUpdated" : "app.workspaceAdded"));
    } catch (error) { notify("error", errorMessage(error)); } finally { setWorkspaceForm((form) => ({ ...form, remotePassword: "" })); setBusy(""); }
  };

  const toggleTaskPinned = useCallback(async (task) => {
    const pinned = !task.pinned;
    const applyPinned = (item) => item.id === task.id ? { ...item, pinned } : item;
    setTasks((items) => items.map(applyPinned));
    setRunItems((items) => items.map((run) => run.task?.id === task.id ? { ...run, task: { ...run.task, pinned } } : run));
    setPinnedTasks((items) => pinned ? [{ ...task, pinned: true }, ...items.filter((item) => item.id !== task.id)] : items.filter((item) => item.id !== task.id));
    try {
      if (mode !== "demo") {
        await TaskRunBinding.SetTaskPinned(task.id, pinned);
        await loadTasks();
        await loadRunList();
      }
    } catch (error) {
      setTasks((items) => items.map((item) => item.id === task.id ? { ...item, pinned: task.pinned } : item));
      setRunItems((items) => items.map((run) => run.task?.id === task.id ? { ...run, task: { ...run.task, pinned: task.pinned } } : run));
      setPinnedTasks((items) => task.pinned ? [{ ...task }, ...items.filter((item) => item.id !== task.id)] : items.filter((item) => item.id !== task.id));
      notify("error", errorMessage(error));
    }
  }, [loadRunList, loadTasks, mode, notify]);

  const removeWorkspace = useCallback(async (workspace) => {
    if (!await requestConfirm({ title: t("app.removeWorkspaceTitle", { name: workspace.name }), description: t("app.removeWorkspaceDescription"), detail: workspace.path, confirmLabel: t("sidebar.removeFromList"), dangerous: true })) return;
    try {
      if (mode !== "demo") await WorkspaceBinding.RemoveWorkspace(workspace.id);
      const remaining = mode === "demo" ? workspaces.filter((item) => item.id !== workspace.id) : sortWorkspaces(await WorkspaceBinding.ListWorkspaces());
      setWorkspaces(remaining);
      if (workspace.id === workspaceID) {
        if (remaining[0]) selectWorkspace(remaining[0].id);
        else {
          runListLoadVersion.current += 1;
          runLoadVersion.current += 1;
          setWorkspaceID("");
          setRunItems([]);
          setRunTotal(0);
          setRunNextCursor("");
          setSelectedRunID("");
          setRunDetail(null);
        }
      }
      notify("success", t("app.workspaceRemoved"));
    } catch (error) {
      notify("error", errorMessage(error));
    }
  }, [mode, notify, requestConfirm, selectWorkspace, workspaceID, workspaces]);

  const updateWorker = async () => {
    if (!workerForm.id.trim() || !workerForm.name.trim() || !workerForm.baseUrl.trim()) { notify("error", t("app.workerFieldsRequired")); return; }
    setBusy("worker");
    try {
      if (mode === "demo") setWorkers((items) => [...items.filter((item) => item.id !== workerForm.id), { ...workerForm, hasToken: true }]);
      else { await WorkerBinding.UpdateWorker(workerForm); setWorkers(await WorkerBinding.ListWorkers()); }
      setWorkerModal(false); notify("success", t("app.workerSaved"));
    } catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
  };

  const pairWorker = async (baseURL, code) => {
    if (!baseURL.trim() || !code.trim()) { notify("error", t("app.workerPairingFieldsRequired")); return; }
    setBusy("worker-pair");
    try {
      if (mode === "demo") {
        setWorkers((items) => [...items, { id: "paired-worker", name: "Paired Worker", baseUrl: baseURL, hasToken: true, enabled: true }]);
      } else {
        await WorkerBinding.PairWorker(baseURL, code);
        setWorkers(await WorkerBinding.ListWorkers());
      }
      setWorkerModal(false);
      notify("success", t("app.workerPaired"));
    } catch (error) {
      notify("error", t("app.workerPairingFailed", { error: errorMessage(error) }));
    } finally {
      setBusy("");
    }
  };

  const checkWorker = async (worker) => {
    setWorkerHealth((current) => ({ ...current, [worker.id]: { ...current[worker.id], checking: true } }));
    try {
      const status = mode === "demo" ? { health: { workerId: worker.id, name: worker.name, runtimes: { codex: true, claude: true, modu: true } } } : await WorkerBinding.CheckWorker(worker.id);
      setWorkerHealth((current) => ({ ...current, [worker.id]: { ok: true, checking: false, ...status.health, checkedAt: status.checkedAt, latencyMilliseconds: status.latencyMilliseconds } }));
    } catch (error) { setWorkerHealth((current) => ({ ...current, [worker.id]: { ok: false, error: errorMessage(error) } })); }
  };

  const deleteWorker = async (id) => {
    const worker = workers.find((item) => item.id === id);
    if (!await requestConfirm({ title: t("app.deleteWorkerTitle", { name: worker?.name || id }), description: t("app.deleteWorkerDescription"), confirmLabel: t("app.deleteWorker"), dangerous: true })) return;
    try {
      if (mode === "demo") setWorkers((items) => items.filter((item) => item.id !== id));
      else { await WorkerBinding.DeleteWorker(id); setWorkers(await WorkerBinding.ListWorkers()); }
    } catch (error) { notify("error", errorMessage(error)); }
  };

  const createTaskAndRun = async () => {
    const execution = selectedTaskExecution(taskForm);
    if (!workspaceID || !taskForm.prompt.trim() || !execution.workflowId || !execution.sandbox || (execution.directAgent && !execution.harness)) { notify("error", t("app.taskFieldsRequired")); return; }
    const taskTitle = taskTitleFromPrompt(taskForm.prompt, t("task.createTitle"));
    const selectedWorkflow = workflows.find((item) => item.id === execution.workflowId);
    const usesFullSandbox = execution.sandbox === "full" && (execution.directAgent || selectedWorkflow?.steps?.some((step) => step.sandbox === "full"));
    if (usesFullSandbox && !await requestConfirm({ title: t("app.fullRunTitle"), description: t("app.fullRunDescription"), detail: t("app.workflowDetail", { name: selectedWorkflow.name }), confirmLabel: t("app.confirmStart"), dangerous: true })) return;
    setBusy("run");
    setRunStatus("");
    setRunSearchDraft("");
    setRunKeyword("");
    try {
      if (mode === "demo") {
        if (taskForm.executionMode === "queued") {
          const queuedTask = { id: `task_${Date.now()}`, workspaceId: workspaceID, title: taskTitle, prompt: taskForm.prompt, workflowId: execution.workflowId, harness: execution.harness, model: execution.model, reasoningEffort: execution.reasoningEffort, serviceTier: execution.serviceTier, sandbox: execution.sandbox, status: "queued", executionMode: "queued", queue: { state: "waiting", enqueuedAt: new Date().toISOString(), authorized: true }, attachments: taskForm.attachmentPaths.map((path) => ({ id: path, name: fileName(path), storedPath: path })), createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() };
          setTasks((items) => [...items, queuedTask]); setSelectedRunID(""); setRunDetail(null); setSelectedQueuedTaskID(queuedTask.id);
        } else {
          const demoTask = { ...demo.demoTasks[0], title: taskTitle, prompt: taskForm.prompt, workflowId: execution.workflowId, harness: execution.harness, model: execution.model, reasoningEffort: execution.reasoningEffort, serviceTier: execution.serviceTier, sandbox: execution.sandbox, updatedAt: new Date().toISOString() };
          setTasks((items) => items.map((item) => item.id === demoTask.id ? demoTask : item)); setRunItems([{ ...demo.demoRun.run, workflowId: demoTask.workflowId, status: "running", task: demoTask }]); setRunTotal(1); setSelectedQueuedTaskID(""); setSelectedRunID("run_demo"); setRunDetail({ ...demo.demoRun, task: demoTask, run: { ...demo.demoRun.run, workflowId: demoTask.workflowId, status: "running" }, active: true });
        }
      } else {
        const task = await TaskRunBinding.CreateTask({ workspaceId: workspaceID, title: "", prompt: taskForm.prompt, workflowId: execution.workflowId, harness: execution.harness, model: execution.model, reasoningEffort: execution.reasoningEffort, serviceTier: execution.serviceTier, sandbox: execution.sandbox, attachmentPaths: taskForm.attachmentPaths });
        const preview = await TaskRunBinding.PreviewRun(task.id);
        if (taskForm.executionMode === "queued") {
          const queued = await TaskRunBinding.EnqueueTask(task.id, preview.confirmationToken || "");
          setSelectedRunID(""); setRunDetail(null); setSelectedQueuedTaskID(task.id);
          if (queued.status === "running") {
            const runs = await TaskRunBinding.ListRunsByTask(task.id);
            if (runs?.[0]) { setSelectedQueuedTaskID(""); setSelectedRunID(runs[0].id); await loadRun(runs[0].id); }
          }
        } else {
          const run = await TaskRunBinding.StartRun(task.id, preview.confirmationToken || "");
          setRunItems((items) => mergeRunItems([{ ...run, task }], items)); setRunTotal((total) => total + 1); setSelectedQueuedTaskID(""); setSelectedRunID(run.id); await loadRun(run.id);
        }
        await loadTasks(); await loadRunList();
      }
      setTaskForm((form) => ({ ...form, prompt: "", attachmentPaths: [] })); setTaskModal(false); notify("success", taskForm.executionMode === "queued" ? t("app.taskQueued") : t("app.runStarted"));
    } catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
  };

  const chooseAttachments = useCallback(async (target) => {
    try {
      const paths = mode === "demo" ? ["/Users/demo/Desktop/reference.png"] : await WorkspaceBinding.ChooseAttachments();
      if (!paths?.length) return;
      if (target === "task") setTaskForm((form) => ({ ...form, attachmentPaths: [...new Set([...(form.attachmentPaths || []), ...paths])].slice(0, 8) }));
      else setComposerAttachments((items) => [...new Set([...items, ...paths])].slice(0, 8));
    } catch (error) { notify("error", errorMessage(error)); }
  }, [mode, notify]);

  // Returns true when the submit was accepted so the composer can clear its
  // local draft; false keeps whatever the user typed.
  const submitWorkbenchComposer = useCallback(async (modeName = "queue", content = "", runtimeProfile = null) => {
    const runDetailNow = runDetailRef.current;
    if (!runDetailNow?.run?.id) { setTaskModal(true); return false; }
    const run = runDetailNow.run;
    const attachments = composerAttachmentsRef.current;
    if (!content && !attachments.length && run.status !== "paused") return false;
    setBusy(modeName);
    try {
      if (mode === "demo") {
        if (run.status === "running") {
          const instruction = { id: `instruction_${Date.now()}`, content: content || t("composer.attachmentInstruction"), attachments: [...attachments], status: "pending", priority: modeName === "insert", createdAt: new Date().toISOString() };
          setRunDetail((detail) => ({ ...detail, instructions: [...(detail.instructions || []), instruction] }));
        } else if (run.status === "paused" || run.status === "completed") {
          const instruction = content || (attachments.length ? t("composer.attachmentInstruction") : "");
          const at = new Date().toISOString();
          setRunDetail((detail) => ({
            ...detail,
            active: true,
            run: { ...detail.run, status: "running", pauseReason: "", completedAt: "", updatedAt: at },
            events: [...(detail.events || []), { seq: Math.max(0, ...(detail.events || []).map((event) => event.seq || 0)) + 1, type: "run.resumed", payload: JSON.stringify({ instruction }), at }],
          }));
          setRunItems((items) => items.map((item) => item.id === run.id ? { ...item, status: "running" } : item));
        }
      } else if (run.status === "running") {
        const input = { content, attachmentPaths: attachments };
        if (modeName === "insert") await TaskRunBinding.InterruptAndInsert(run.id, input);
        else await TaskRunBinding.EnqueueInstruction(run.id, input);
      } else if (run.status === "paused" || run.status === "completed") {
        if (attachments.length) {
          await TaskRunBinding.EnqueueInstruction(run.id, { content: content || t("composer.attachmentInstruction"), attachmentPaths: attachments });
          await TaskRunBinding.ResumeRunConfigured(run.id, { instruction: "", ...runtimeProfile });
        } else {
          await TaskRunBinding.ResumeRunConfigured(run.id, { instruction: content, ...runtimeProfile });
        }
        setResumePendingRunID(run.id);
      }
      setComposerAttachments([]);
      window.setTimeout(() => loadRun(run.id, true), 180);
      notify("success", modeName === "insert" ? t("app.instructionInserted") : run.status === "running" ? t("app.instructionQueued") : t("app.runResuming"));
      return true;
    } catch (error) { notify("error", errorMessage(error)); return false; } finally { setBusy(""); }
  }, [loadRun, mode, notify, t]);

  const removeQueuedInstruction = useCallback(async (instructionID) => {
    const runID = runDetailRef.current?.run?.id;
    if (!runID) return;
    try { if (mode !== "demo") await TaskRunBinding.RemoveInstruction(runID, instructionID); await loadRun(runID, true); }
    catch (error) { notify("error", errorMessage(error)); }
  }, [loadRun, mode, notify]);

  const respondPermission = useCallback(async (requestID, decision) => {
    const runID = runDetailRef.current?.run?.id;
    if (!runID || !requestID) return;
    setPermissionBusy(requestID);
    try {
      if (mode !== "demo") await TaskRunBinding.RespondPermission({ runId: runID, requestId: requestID, decision });
      window.setTimeout(() => loadRun(runID, true), 120);
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setPermissionBusy("");
    }
  }, [loadRun, mode, notify]);

  const openRenameTask = useCallback((requestedTask) => {
    const task = requestedTask || runDetailRef.current?.task || tasksRef.current.find((item) => item.id === selectedQueuedTaskIDRef.current);
    if (task) setRenameForm({ taskId: task.id, title: task.title, originalTitle: task.title });
  }, []);

  const renameSelectedTask = async () => {
    const title = renameForm?.title.trim();
    if (!renameForm || !title) { notify("error", t("task.nameRequired")); return; }
    if (title === renameForm.originalTitle) { setRenameForm(null); return; }
    setBusy("rename");
    try {
      if (mode === "demo") {
        setTasks((items) => items.map((task) => task.id === renameForm.taskId ? { ...task, title, updatedAt: new Date().toISOString() } : task));
        setRunItems((items) => items.map((run) => run.task?.id === renameForm.taskId ? { ...run, task: { ...run.task, title } } : run));
        setRunDetail((detail) => detail?.task?.id === renameForm.taskId ? { ...detail, task: { ...detail.task, title } } : detail);
      } else {
        await TaskRunBinding.RenameTask(renameForm.taskId, title);
        await loadTasks();
        await loadRunList();
        if (selectedRunID) await loadRun(selectedRunID, true);
      }
      setRenameForm(null);
      notify("success", t("app.taskRenamed"));
    } catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
  };

  const deleteTask = useCallback(async (task) => {
    if (!task || !await requestConfirm({ title: t("app.deleteTaskTitle", { name: task.title }), description: t("app.deleteTaskDescription"), confirmLabel: t("app.deleteTask"), dangerous: true })) return;
    const selected = runDetailRef.current?.task?.id === task.id || selectedQueuedTaskIDRef.current === task.id;
    try {
      if (mode === "demo") {
        setTasks((items) => items.filter((item) => item.id !== task.id));
        setRunItems((items) => items.filter((run) => run.task?.id !== task.id));
        setPinnedTasks((items) => items.filter((item) => item.id !== task.id));
      } else {
        await TaskRunBinding.DeleteTask(task.id);
        await loadTasks();
        await loadRunList();
      }
      if (selected) {
        setSelectedRunID("");
        setSelectedQueuedTaskID("");
        setRunDetail(null);
      }
    }
    catch (error) { notify("error", errorMessage(error)); }
  }, [loadRunList, loadTasks, mode, notify, requestConfirm, t]);

  const runAction = useCallback(async (action) => {
    const runID = selectedRunIDRef.current;
    if (!runID || runActionPending.current) return;
    runActionPending.current = action;
    setBusy(action);
    try {
      if (mode === "demo") {
        const nextStatus = action === "interrupt" ? "paused" : "cancelled";
        setRunDetail((detail) => ({ ...detail, active: false, run: { ...detail.run, status: nextStatus, pauseReason: action === "interrupt" ? "interrupted" : "" } }));
        setRunItems((items) => items.map((item) => item.id === runID ? { ...item, status: nextStatus } : item));
      } else {
        if (action === "interrupt") await TaskRunBinding.InterruptRun(runID);
        if (action === "cancel") await TaskRunBinding.CancelRun(runID);
        window.setTimeout(() => {
          if (selectedRunIDRef.current === runID) loadRun(runID, true);
        }, 250);
      }
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      runActionPending.current = "";
      setBusy("");
    }
  }, [loadRun, mode, notify]);

  const openEditor = (definition, isNew = false) => {
    const next = copy(definition || loopTemplate);
    if (isNew) next.id = nextWorkflowDefinitionID(next.id, workflows);
    if (isNew || !workflows.some((item) => item.id === next.id)) next.policy = { maxTransitions: settings.execution.maxTransitions, maxConsecutiveFailures: settings.execution.maxConsecutiveFailures, stepTimeoutSeconds: settings.execution.stepTimeoutSeconds };
    setEditorSourceID(isNew ? "" : next.id);
    setEditor(next);
    setValidation([]);
  };

  const updateStep = (index, field, value) => setEditor((current) => ({ ...current, steps: current.steps.map((step, i) => i === index ? { ...step, [field]: value } : step) }));
  const updateTransition = (stepIndex, oldSignal, signal, target) => setEditor((current) => {
    const steps = current.steps.map((step, i) => {
      if (i !== stepIndex) return step;
      const transitions = { ...step.transitions }; delete transitions[oldSignal]; transitions[signal] = target;
      return { ...step, transitions };
    });
    return { ...current, steps };
  });
  const removeTransition = (stepIndex, signal) => setEditor((current) => ({ ...current, steps: current.steps.map((step, i) => {
    if (i !== stepIndex) return step; const transitions = { ...step.transitions }; delete transitions[signal]; return { ...step, transitions };
  }) }));

  const validateEditor = async () => {
    if (!editor) return [];
    const duplicateID = workflows.some((item) => item.id === editor.id && item.id !== editorSourceID);
    let issues;
    if (mode === "demo") {
      issues = [];
      if (!/^[a-z][a-z0-9_-]*$/.test(editor.id)) issues.push({ path: "id", message: t("workflow.lowercaseID") });
      if (!editor.steps?.length) issues.push({ path: "steps", message: t("workflow.stepRequired") });
    } else {
      issues = await WorkflowBinding.ValidateDefinition(editor) || [];
    }
    if (duplicateID) issues = [...issues, { path: "id", code: "duplicate", message: t("workflow.idExists") }];
    setValidation(issues);
    return issues;
  };

  const saveWorkflow = async () => {
    const issues = await validateEditor();
    if (issues.length) { notify("error", t("workflow.configIssues", { count: issues.length })); return; }
    if (editor.steps.some((step) => step.sandbox === "full") && !await requestConfirm({ title: t("app.fullWorkflowTitle"), description: t("app.fullWorkflowDescription"), detail: t("app.workflowDetail", { name: editor.name || editor.id }), confirmLabel: t("app.confirmSave"), dangerous: true })) return;
    setBusy("workflow");
    try {
      let saved = editor;
      if (mode !== "demo") {
        saved = editorSourceID ? await WorkflowBinding.UpdateDefinition(editorSourceID, editor) : await WorkflowBinding.CreateDefinition(editor);
        setWorkflows(await WorkflowBinding.ListDefinitions());
      } else setWorkflows((items) => [...items.filter((item) => item.id !== editor.id && item.id !== editorSourceID), editor]);
      setTaskForm((form) => ({ ...form, workflowId: saved.id, harness: "", model: "", reasoningEffort: "", serviceTier: "" })); setEditor(null); setEditorSourceID(""); notify("success", t("app.workflowSaved"));
    } catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
  };

  const deleteWorkflow = async (workflow) => {
    if (!await requestConfirm({ title: t("app.deleteWorkflowTitle", { name: workflow.name }), description: t("app.deleteWorkflowDescription"), detail: t("app.workflowDetail", { name: workflow.name || workflow.id }), confirmLabel: t("app.deleteWorkflow"), dangerous: true })) return;
    setBusy("delete-workflow");
    try {
      let items;
      if (mode === "demo") items = workflows.filter((item) => item.id !== workflow.id);
      else {
        await WorkflowBinding.DeleteDefinition(workflow.id);
        items = await WorkflowBinding.ListDefinitions();
      }
      setWorkflows(items);
      setTaskForm((form) => form.workflowId === workflow.id ? { ...form, workflowId: firstWorkflowID() } : form);
      notify("success", t("app.workflowDeleted"));
    } catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
  };

  const sidebarWorkspaces = useMemo(() => workspaceResults(workspaces, { query: "", expanded: workspaceExpanded }), [workspaceExpanded, workspaces]);
  const goView = useCallback((next) => {
    if (next !== "tasks") setTaskModal(false);
    if (next === "settings" || next === "workflows") {
      if (mode === "wails") {
        const open = next === "settings" ? WindowBinding.OpenSettings() : WindowBinding.OpenWorkflows();
        void open.catch((error) => notify("error", errorMessage(error)));
        return;
      }
    }
    // Browser preview has no native window host, so keeping the legacy inline
    // view there makes design work possible without changing desktop behavior.
    setEditor(null);
    setEditorSourceID("");
    setView(next);
  }, [mode, notify]);

  const openTaskFromTray = useCallback(async (request) => {
    if (!request?.taskId || request.started || mode !== "wails") return;
    request.started = true;
    try {
      const run = request.runId
        ? { id: request.runId }
        : newestTaskRun(await TaskRunBinding.ListRunsByTask(request.taskId));
      if (trayTaskRequestRef.current !== request) return;
      setEditor(null);
      setEditorSourceID("");
      setView("tasks");
      setTaskModal(false);
      setRunDetail(null);
      if (run) {
        setSelectedQueuedTaskID("");
        setSelectedRunID("");
        await loadRun(run.id);
      } else {
        setSelectedRunID("");
        setSelectedQueuedTaskID(request.taskId);
      }
    } catch (error) {
      if (trayTaskRequestRef.current === request) notify("error", errorMessage(error));
    } finally {
      if (trayTaskRequestRef.current === request) trayTaskRequestRef.current = null;
    }
  }, [loadRun, mode, notify]);

  // A cross-project tray selection is held until the workspace-change effects
  // have cleared the previous list and selection. Opening it afterwards avoids
  // that normal reset racing away the tray request.
  useEffect(() => {
    const request = trayTaskRequestRef.current;
    if (mode === "wails" && request?.workspaceId === workspaceID) void openTaskFromTray(request);
  }, [mode, openTaskFromTray, workspaceID]);

  useEffect(() => {
    if (mode !== "wails") return undefined;
    return Events.On(TRAY_ACTION_EVENT, (event) => {
      const action = normalizeTrayNavigation(event?.data);
      if (!action) return;
      if (action?.action === "new") {
        trayTaskRequestRef.current = null;
        setEditor(null);
        setEditorSourceID("");
        setView("tasks");
        setTaskModal(true);
        return;
      }
      if (action?.action !== "open" || !action.taskId) return;
      const request = { taskId: action.taskId, workspaceId: action.workspaceId, runId: action.runId };
      trayTaskRequestRef.current = request;
      setTaskModal(false);
      if (request.workspaceId && request.workspaceId !== workspaceID) {
        selectWorkspace(request.workspaceId);
        return;
      }
      void openTaskFromTray(request);
    });
  }, [mode, openTaskFromTray, selectWorkspace, workspaceID]);

  // Split rather than one "name @ path" string: the shell-prompt framing was
  // the loudest terminal cue in the chrome. The label is prose, the path is a
  // real filesystem path and keeps the mono face.
  const location = view === "settings"
    ? { label: t("sidebar.settings"), path: "~/.onecatch" }
    : selectedWorkspace
      ? { label: selectedWorkspace.name, path: workspaceLocation(selectedWorkspace) }
      : { label: t("app.selectWorkspace"), path: "" };
  const commandText = location.path ? `${location.label} · ${location.path}` : location.label;
  const selectedTask = runDetail?.task || tasks.find((task) => task.id === selectedQueuedTaskID);
  const selectedTaskStatus = runDetail?.run?.status || selectedTask?.status;
  const taskCreateVisible = view === "tasks" && !editor && (taskModal || !selectedTask);
  const taskTitleVisible = view === "tasks" && !editor && !taskModal && selectedTask;

  const toggleWorkspaceSearch = useCallback(() => setWorkspaceSearchOpen((open) => !open), []);
  const clearSidebarSearch = useCallback(() => { setGlobalSearchQuery(""); setGlobalTaskItems([]); }, []);
  const toggleWorkspaceExpanded = useCallback(() => setWorkspaceExpanded((expanded) => !expanded), []);
  const selectRun = useCallback((item) => { setTaskModal(false); setSelectedQueuedTaskID(""); setSelectedRunID(item.id); loadRun(item.id); }, [loadRun]);
  const selectQueued = useCallback((task) => { setTaskModal(false); setSelectedRunID(""); setRunDetail(null); setSelectedQueuedTaskID(task.id); }, []);
  // Stable handler identities keep the memoized Sidebar/TaskWorkbench from
  // re-rendering on every unrelated App state change (toasts, busy flips, the
  // 80ms streaming cadence).
  const openTaskModal = useCallback(() => setTaskModal(true), []);
  const loadMoreRuns = useCallback(() => loadRunList({ cursor: runNextCursorRef.current }), [loadRunList]);
  const chooseTaskAttachments = useCallback(() => chooseAttachments("task"), [chooseAttachments]);
  const chooseComposerAttachments = useCallback(() => chooseAttachments("composer"), [chooseAttachments]);
  const removeComposerAttachment = useCallback((path) => setComposerAttachments((items) => items.filter((item) => item !== path)), []);
  const interruptRun = useCallback(() => runAction("interrupt"), [runAction]);
  const cancelRun = useCallback(() => runAction("cancel"), [runAction]);

  // Standby lock: one glanceable aggregate over the whole workspace, derived
  // from state that already updates live (tasks poll + run-state push).
  const lockSignal = useMemo(() => buildLockSignal(tasks, runItems), [tasks, runItems]);
  const enterLock = useCallback(() => setLocked(true), []);
  const exitLock = useCallback(() => setLocked(false), []);

  useEffect(() => {
    const nativeSidebar = globalThis.webkit?.messageHandlers?.onecatchSidebar;
    if (!nativeSidebar) return undefined;
    nativeSidebar.postMessage({ hidden: locked });
    return () => {
      if (locked) nativeSidebar.postMessage({ hidden: false });
    };
  }, [locked]);

  // Fire a system notification on the meaningful edges even if the window is
  // behind another app: work finished, or a run now needs you. Watching the
  // signal (not the lock) means the ping arrives whether or not you locked.
  const previousSignalRef = useRef(null);
  useEffect(() => {
    if (mode === "loading") return;
    const edge = completionEdge(previousSignalRef.current, lockSignal);
    previousSignalRef.current = lockSignal;
    if (!edge) return;
    if (edge === LOCK_PHASE.done) notifyStandby(t("lock.notifyDoneTitle"), t("lock.notifyDoneBody"));
    else if (edge === LOCK_PHASE.waiting) notifyStandby(t("lock.notifyWaitingTitle"), t("lock.notifyWaitingBody"));
  }, [lockSignal, mode, t]);

  // Cmd/Ctrl+L toggles standby; Escape is handled inside the overlay. Ignored
  // while typing so it never eats an L in a prompt.
  useEffect(() => {
    if (mode === "loading") return undefined;
    const onKey = (event) => {
      if (!(event.metaKey || event.ctrlKey) || event.key.toLocaleLowerCase() !== "l") return;
      const tag = event.target?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || event.target?.isContentEditable) return;
      event.preventDefault();
      setLocked((value) => !value);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [mode]);

  // Keep the webview shortcut in sync with the native application menu.
  // Windows conventionally opens Settings with Ctrl+,.
  useEffect(() => {
    if (mode === "loading") return undefined;
    const openSettings = (event) => {
      if (!(event.metaKey || event.ctrlKey) || event.altKey || event.key !== ",") return;
      event.preventDefault();
      void WindowBinding.OpenSettings();
    };
    window.addEventListener("keydown", openSettings);
    return () => window.removeEventListener("keydown", openSettings);
  }, [mode]);

  if (mode === "loading") return <div className="flex h-full flex-col items-center justify-center gap-4 bg-background text-sm text-muted-foreground">
    <div className="grid size-11 place-items-center rounded-xl bg-primary text-xl font-bold text-primary-foreground">1</div>
    <span>{t("task.opening")}</span>
  </div>;

  {/* No title band: the sidebar runs the full height of the window and the
      traffic lights sit on it, the way Finder and Music do. The window is
      MacTitleBarHiddenInsetUnified, so the lights are inset — the rail
      reserves that strip itself (see .traffic-light-gutter). */}
  return <div className="app-window-root relative grid h-full grid-rows-[minmax(0,1fr)] bg-transparent text-foreground">
    <DesktopTitleBar
      title={taskCreateVisible ? t("task.createTitle") : taskTitleVisible ? selectedTask.title : commandText}
      workspaceName={view === "tasks" ? selectedWorkspace?.name || "" : ""}
      taskStatus={selectedTaskStatus}
      taskActive={runDetail?.active}
      showTaskStatus={Boolean(taskTitleVisible && selectedTask.workflowId && selectedTask.workflowId !== directAgentWorkflowID)}
      showWorkbenchControls={view === "tasks" && !editor}
      terminalVisible={terminalVisible}
      inspectorCollapsed={inspectorCollapsed}
      inspectorDetached={inspectorDetached}
      lockActive={lockSignal.active}
      sidebarWidth={sidebarCollapsed ? 136 : sidebarWidth}
      onLock={enterLock}
      onToggleTerminal={() => setTerminalToggleVersion((value) => value + 1)}
      onToggleInspector={() => setInspectorToggleVersion((value) => value + 1)}
      onDockInspector={dockInspector}
    />
    <div className={`app-shell grid min-h-0 grid-cols-[auto_minmax(0,1fr)] ${sidebarCollapsed ? "sidebar-is-collapsed" : ""}`}>
      <Sidebar
        workspaces={workspaces}
        workspaceID={workspaceID}
        projectWorkspaces={sidebarWorkspaces}
        searchQuery={globalSearchQuery}
        searchTaskItems={globalTaskItems}
        searchLoading={globalTaskSearchLoading}
        workspaceExpanded={workspaceExpanded}
        workspaceSearchOpen={workspaceSearchOpen}
        workspaceHealth={remoteWorkspaceHealth}
        tasks={tasks}
        pinnedTasks={pinnedTasks}
        runs={runItems}
        selectedRunID={taskModal ? "" : selectedRunID}
        selectedQueuedTaskID={taskModal ? "" : selectedQueuedTaskID}
        runLoading={runLoading}
        runTotal={runTotal}
        runHasMore={Boolean(runNextCursor)}
        taskSearch={runSearchDraft}
        taskStatus={runStatus}
        view={view}
        editor={editor}
        onToggleSearch={toggleWorkspaceSearch}
        onClearSearch={clearSidebarSearch}
        onSearchQueryChange={setGlobalSearchQuery}
        onSelectWorkspace={selectWorkspace}
        onCheckWorkspaceHealth={checkRemoteWorkspaceHealth}
        onToggleTaskPinned={toggleTaskPinned}
        onDeleteTask={deleteTask}
        onRenameTask={openRenameTask}
        onRemoveWorkspace={removeWorkspace}
        onToggleExpanded={toggleWorkspaceExpanded}
        onAddWorkspace={chooseWorkspace}
        onNewTask={openTaskModal}
        onLoadMoreRuns={loadMoreRuns}
        onSelectRun={selectRun}
        onSelectQueued={selectQueued}
        onGoView={goView}
        onCollapsedChange={setSidebarCollapsed}
        onWidthChange={setSidebarWidth}
        compactViewport={compactViewport}
      />

      <main className="flex min-h-0 min-w-0 flex-col overflow-hidden bg-background">
        {!desktopChrome && <div className="app-titlebar drag-region flex h-[52px] shrink-0 cursor-default items-center gap-3 bg-background/80 px-5">
          {view === "tasks" && selectedWorkspace && <Tooltip>
            <TooltipTrigger asChild>
              <span className="no-drag grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground" aria-label={`${t("workspace.currentProject")}: ${selectedWorkspace.name}`}>
                <Folder size={14} strokeWidth={2.1} aria-hidden="true" />
              </span>
            </TooltipTrigger>
            <TooltipContent side="bottom" sideOffset={6}>{selectedWorkspace.name}</TooltipContent>
          </Tooltip>}
          {taskCreateVisible ? <span className="app-titlebar-task flex min-w-0 items-center gap-2" title={t("task.createTitle")}>
            <strong className="min-w-0 truncate text-[13px] font-semibold text-foreground">{t("task.createTitle")}</strong>
          </span> : taskTitleVisible ? <span className="app-titlebar-task flex min-w-0 items-center gap-2" title={selectedTask.title}>
            <strong className="min-w-0 truncate text-[13px] font-semibold text-foreground">{selectedTask.title}</strong>
            {selectedTask.workflowId && selectedTask.workflowId !== directAgentWorkflowID && <StatusPill status={selectedTaskStatus} active={runDetail?.active} />}
          </span> : view === "tasks" ? <span className="min-w-0 flex-1" /> : <span className="flex min-w-0 items-baseline gap-2" title={commandText}>
            <strong className="shrink-0 text-[13px] font-semibold text-foreground">{location.label}</strong>
            {location.path && <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">{location.path}</span>}
          </span>}
          <button type="button" className="no-drag relative inline-flex size-7 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground" aria-label={t("lock.enter")} title={`${t("lock.enter")} · ⌘L`} onClick={enterLock}>
            <Lock size={13} strokeWidth={2.5} aria-hidden="true" />
            {lockSignal.active > 0 && <em className="absolute -top-0.5 -right-0.5 grid h-3.5 min-w-3.5 place-items-center rounded-full bg-primary px-1 text-[10px] font-bold not-italic text-primary-foreground">{lockSignal.active}</em>}
          </button>
          {view !== "tasks" && <StatusBadge status={mode === "wails" ? "good" : "warn"} className="ml-auto shrink-0">
            <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />
            {mode === "wails" ? t(view === "tasks" && selectedWorkspace?.remoteFs ? "workspace.remoteFS" : "common.local") : t("common.preview")}
          </StatusBadge>}
          {view === "tasks" && !editor && inspectorCollapsed && <button type="button" className={`no-drag ml-auto grid size-7 shrink-0 place-items-center rounded-md transition-colors hover:bg-accent hover:text-foreground ${terminalVisible ? "bg-accent text-foreground" : "text-muted-foreground"}`} aria-label={terminalVisible ? t("terminal.collapse") : t("terminal.open")} aria-pressed={terminalVisible} title={`${terminalVisible ? t("terminal.collapse") : t("terminal.open")} · Ctrl + \``} onClick={() => setTerminalToggleVersion((value) => value + 1)}><SquareTerminal size={15} strokeWidth={2} aria-hidden="true" /></button>}
          {view === "tasks" && (inspectorDetached
            ? <button type="button" className="no-drag grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground" aria-label={t("inspector.dock")} title={t("inspector.dockHint")} onClick={dockInspector}><PictureInPicture2 size={16} strokeWidth={2} aria-hidden="true" /></button>
            : inspectorCollapsed && <button type="button" className="no-drag grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground" aria-label={t("inspector.expand")} aria-expanded="false" aria-controls="workbench-inspector-content" title={t("inspector.expand")} onClick={toggleInspector}><PanelRightOpen size={16} strokeWidth={2} aria-hidden="true" /></button>)}
        </div>}
        {editor ? <Suspense fallback={<ViewLoading />}><WorkflowEditor editor={editor} setEditor={setEditor} validation={validation} validateEditor={validateEditor} saveWorkflow={saveWorkflow} busy={busy} updateStep={updateStep} updateTransition={updateTransition} removeTransition={removeTransition} runtimes={runtimes} workers={settings.experimental?.remoteWorkersEnabled ? workers : []} defaultSandbox={settings.execution.defaultSandbox} allowFullSandbox={settings.security.allowFullSandbox} onClose={() => { setEditor(null); setEditorSourceID(""); }} showBack /></Suspense> : view === "tasks" ? <TaskWorkbench
          mode={mode}
          workspace={selectedWorkspace}
          terminalPreferences={settings.terminal}
          terminalVisible={terminalVisible}
          terminalToggleVersion={terminalToggleVersion}
          terminalCommand={terminalCommand}
          onTerminalVisibilityChange={setTerminalVisible}
          workspaceID={workspaceID}
          tasks={tasks}
          runDetail={runDetail}
          selectedRunID={selectedRunID}
          selectedQueuedTaskID={selectedQueuedTaskID}
          busy={busy}
          permissionBusy={permissionBusy}
          attachments={composerAttachments}
          inspectorCollapsed={inspectorCollapsed}
          inspectorToggleVersion={inspectorToggleVersion}
          integratedDesktopTitlebar={desktopChrome}
          onToggleInspector={toggleInspector}
          onDetachInspector={mode === "wails" ? detachInspector : null}
          onEditWorkspace={editWorkspace}
          newTaskOpen={taskCreateVisible}
          taskForm={taskForm}
          workflows={workflows}
          runtimes={runtimes}
          taskRuntimeConfiguration={taskRuntimeConfiguration}
          runtimeSettings={settings.runtimes?.[taskForm.harness]}
          runtimeSettingsByHarness={settings.runtimes}
          allowFullSandbox={Boolean(settings.security?.allowFullSandbox && selectedWorkspace?.defaultSandbox === "full")}
          onInspectRuntimeConfiguration={inspectRuntimeConfiguration}
          onTaskFormChange={setTaskForm}
          onChooseTaskAttachments={selectedWorkspace?.remoteFs ? null : chooseTaskAttachments}
          onCreateTask={createTaskAndRun}
          onChooseAttachments={selectedWorkspace?.remoteFs ? null : chooseComposerAttachments}
          onRemoveAttachment={removeComposerAttachment}
          onSubmit={submitWorkbenchComposer}
          onInterrupt={interruptRun}
          onCancel={cancelRun}
          onRemoveInstruction={removeQueuedInstruction}
          onLoadEarlierTranscript={loadEarlierTranscript}
          onPermissionDecision={respondPermission}
          notify={notify}
        /> : view === "workflows" ? <Suspense fallback={<ViewLoading />}><WorkflowLibrary workflows={workflows} runtimes={runtimes} openEditor={openEditor} deleteWorkflow={deleteWorkflow} busy={busy} /></Suspense> : <Suspense fallback={<ViewLoading />}><SettingsPage mode={mode} value={settings} runtimes={runtimes} onChange={setSettings} notify={notify} workersPanel={<WorkerPage mode={mode} workspace={selectedWorkspace} workers={workers} health={workerHealth} checkWorker={checkWorker} deleteWorker={deleteWorker} notify={notify} openWorker={(worker) => { setWorkerForm(worker ? { id: worker.id, name: worker.name, baseUrl: worker.baseUrl, caFile: worker.caFile || "", clientCertFile: worker.clientCertFile || "", clientKeyFile: worker.clientKeyFile || "", serverName: worker.serverName || "", serverCertificateSha256: worker.serverCertificateSha256 || "", enabled: worker.enabled } : { id: "", name: "", baseUrl: "https://", caFile: "", clientCertFile: "", clientKeyFile: "", serverName: "", serverCertificateSha256: "", enabled: true }); setWorkerModal(true); }} />} /></Suspense>}
      </main>
    </div>
    {workspaceModal && <Modal className="workspace-create-dialog max-h-[calc(100vh-2rem)] gap-0 overflow-y-auto p-0 sm:max-w-[480px]" title={t(workspaceEditingID ? "workspace.editTitle" : "workspace.addTitle")} subtitle={t(workspaceEditingID ? "workspace.editSubtitle" : "workspace.addSubtitle")} onClose={() => { if (busy !== "workspace") { setWorkspaceForm((form) => ({ ...form, remotePassword: "" })); setWorkspaceEditingID(""); setWorkspaceModal(false); } }}>
      <form className="grid gap-4 px-5 pt-4 pb-5" aria-busy={busy === "workspace"} onSubmit={(event) => { event.preventDefault(); if (!event.nativeEvent.isComposing && busy !== "workspace") void saveWorkspace(); }}>
        <div className="grid gap-1.5">
          <Label id="workspace-create-source-label">{t("workspace.source")}</Label>
          <TUISelect ariaLabel={t("workspace.source")} value={workspaceForm.source} onChange={(source) => setWorkspaceForm((form) => ({ ...form, source, defaultSandbox: source === "remote" && (form.defaultSandbox === "" || form.defaultSandbox === "full") ? "workspace-write" : form.defaultSandbox }))} options={[{ value: "local", label: t("workspace.localDirectory") }, { value: "remote", label: t("workspace.remoteFS"), meta: remoteFSHarnessAvailable ? "" : t("workspace.remoteFSNoHarness"), disabled: !remoteFSHarnessAvailable }]} />
        </div>
        {workspaceForm.source === "local" ? <div className="grid gap-1.5">
          <Label htmlFor="workspace-create-path">{t("workspace.path")}</Label>
          <div className="flex gap-2"><Input id="workspace-create-path" className="min-w-0 flex-1 font-mono text-[13px]" autoFocus value={workspaceForm.path} onChange={(event) => setWorkspaceForm((form) => ({ ...form, path: event.target.value }))} placeholder="/Users/me/Code/project" /><Button type="button" variant="outline" disabled={busy === "workspace"} onClick={browseWorkspace}>{t("workspace.browse")}</Button></div>
        </div> : <>
          <div className="grid gap-1.5">
            <Label htmlFor="workspace-create-remote-host">{t("workspace.sshHost")}</Label>
            <Input id="workspace-create-remote-host" className="font-mono text-[13px]" autoFocus autoCapitalize="none" autoCorrect="off" spellCheck="false" value={workspaceForm.remoteHost} onChange={(event) => setWorkspaceForm((form) => ({ ...form, remoteHost: event.target.value }))} placeholder="192.168.5.98:22" />
            <p className="m-0 text-[11px] leading-relaxed text-muted-foreground">{t("workspace.sshHostHint")}</p>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="workspace-create-remote-root">{t("workspace.remoteRoot")}</Label>
            <Input id="workspace-create-remote-root" className="font-mono text-[13px]" autoCapitalize="none" autoCorrect="off" spellCheck="false" value={workspaceForm.remoteRoot} onChange={(event) => setWorkspaceForm((form) => ({ ...form, remoteRoot: event.target.value }))} placeholder="/srv/project" />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="grid min-w-0 gap-1.5">
              <Label htmlFor="workspace-create-remote-username">{t("workspace.sshUsername")}</Label>
              <Input id="workspace-create-remote-username" autoComplete="username" autoCapitalize="none" autoCorrect="off" spellCheck="false" value={workspaceForm.remoteUsername} onChange={(event) => setWorkspaceForm((form) => ({ ...form, remoteUsername: event.target.value }))} placeholder="deploy" />
            </div>
            <div className="grid min-w-0 gap-1.5">
              <Label htmlFor="workspace-create-remote-password">{t("workspace.sshPassword")}</Label>
              <Input id="workspace-create-remote-password" type="password" autoComplete="current-password" value={workspaceForm.remotePassword} onChange={(event) => setWorkspaceForm((form) => ({ ...form, remotePassword: event.target.value }))} placeholder={t("workspace.sshPasswordPlaceholder")} />
            </div>
          </div>
          <p className="-mt-1 m-0 text-[11px] leading-relaxed text-muted-foreground">{t("workspace.sshPasswordHint")}</p>
          <p className="-mt-1 m-0 rounded-md border bg-muted/40 px-3 py-2 text-[11px] leading-relaxed text-muted-foreground">{t("workspace.remoteFSDescription")}</p>
        </>}
        <div className="grid gap-1.5">
          <Label htmlFor="workspace-create-name">{t("workspace.displayName")}</Label>
          <Input id="workspace-create-name" value={workspaceForm.name} onChange={(event) => setWorkspaceForm((form) => ({ ...form, name: event.target.value }))} placeholder={t("workspace.defaultName")} />
        </div>
        <div className="grid gap-1.5">
          <Label id="workspace-create-sandbox-label">{t("workspace.defaultSandbox")}</Label>
          <TUISelect ariaLabel={t("workspace.defaultSandbox")} value={workspaceForm.defaultSandbox} onChange={(defaultSandbox) => setWorkspaceForm((form) => ({ ...form, defaultSandbox }))} options={workspaceForm.source === "remote" ? [{ value: "workspace-write", label: t("workspace.write") }, { value: "read-only", label: t("workspace.readOnly") }] : [{ value: "", label: t("workspace.globalDefault") }, { value: "read-only", label: t("workspace.readOnly") }, { value: "workspace-write", label: t("workspace.write") }, ...(settings.security?.allowFullSandbox ? [{ value: "full", label: t("workspace.fullDanger") }] : [])]} />
        </div>
        <DialogFooter className="mt-1">
          <Button type="button" variant="ghost" disabled={busy === "workspace"} onClick={() => { setWorkspaceForm((form) => ({ ...form, remotePassword: "" })); setWorkspaceEditingID(""); setWorkspaceModal(false); }}>{t("common.cancel")}</Button>
          <Button type="submit" disabled={busy === "workspace" || (workspaceForm.source === "remote" ? !workspaceForm.remoteHost.trim() || !workspaceForm.remoteRoot.trim() || (Boolean(workspaceForm.remotePassword) && !workspaceForm.remoteUsername.trim()) : !workspaceForm.path.trim())}>{busy === "workspace" ? t("common.processing") : t(workspaceEditingID ? "common.save" : "workspace.add")}</Button>
        </DialogFooter>
      </form>
    </Modal>}
    {renameForm && <Modal className="gap-5 p-5 sm:max-w-md" title={t("task.renameTitle")} subtitle={t("task.renameSubtitle")} onClose={() => busy !== "rename" && setRenameForm(null)}><form className="grid gap-5" onSubmit={(event) => { event.preventDefault(); if (!event.nativeEvent.isComposing && busy !== "rename") void renameSelectedTask(); }}><div className="grid gap-2"><Label htmlFor="rename-task-title">{t("task.name")}</Label><Input id="rename-task-title" autoFocus maxLength={160} value={renameForm.title} onChange={(event) => setRenameForm((form) => ({ ...form, title: event.target.value }))} /></div><DialogFooter className="pt-1"><Button type="button" variant="outline" disabled={busy === "rename"} onClick={() => setRenameForm(null)}>{t("common.cancel")}</Button><Button type="submit" disabled={busy === "rename" || !renameForm.title.trim()}>{busy === "rename" ? t("common.saving") : t("task.saveName")}</Button></DialogFooter></form></Modal>}
    {workerModal && <Suspense fallback={null}><WorkerModal form={workerForm} setForm={setWorkerForm} busy={busy} onClose={() => setWorkerModal(false)} onUpdate={updateWorker} onPair={pairWorker} /></Suspense>}
    <ConfirmDialog dialog={appDialog} onCancel={() => resolveConfirm(false)} onConfirm={() => resolveConfirm(true)} />
    {notice && <div className={`toast ${notice.type}`}><span>{notice.text}</span></div>}
    {locked && <LockScreen signal={lockSignal} workspaceName={selectedWorkspace?.name || ""} onExit={exitLock} />}
  </div>;
}

export default App;
