import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Events } from "@wailsio/runtime";
import {
  RuntimeBinding,
  SettingsBinding,
  TaskRunBinding,
  WorkflowBinding,
  WorkspaceBinding,
  WorkerBinding,
} from "../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";
import SettingsPage, { ConfirmDialog, demoSettings } from "./SettingsPage.jsx";
import { mergeRunItems, preserveEqualValue, sortWorkspaces, workspaceSections } from "./listNavigation.js";
import { Action, TUISelect } from "../ui/primitives.jsx";
import { copy, errorMessage, fileName } from "./format.js";
import { loopTemplate } from "./templates.js";
import { demoRun, demoRuntimes, demoTasks, demoWorkers, demoWorkflows, demoWorkspaces } from "./demoData.js";
import Sidebar from "./components/Sidebar.jsx";
import TaskWorkbench from "./components/TaskWorkbench.jsx";
import WorkflowLibrary from "./components/workflow/WorkflowLibrary.jsx";
import WorkflowEditor from "./components/workflow/WorkflowEditor.jsx";
import WorkerPage from "./components/WorkerPage.jsx";
import Modal from "./components/Modal.jsx";
import { applyRunState, applyRuntimeFrames } from "./runtimeStream.js";
import { nextWorkflowDefinitionID } from "./workflowIds.js";

const runtimeFrameEvent = "oneshot:runtime-frame";
const runStateEvent = "oneshot:run-state";

function App() {
  const { t } = useTranslation();
  const [mode, setMode] = useState("loading");
  const [view, setView] = useState("tasks");
  const [runtimes, setRuntimes] = useState([]);
  const [workspaces, setWorkspaces] = useState([]);
  const [workspaceID, setWorkspaceID] = useState("");
  const [workflows, setWorkflows] = useState([]);
  const [runItems, setRunItems] = useState([]);
  const [tasks, setTasks] = useState([]);
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
  const [taskModal, setTaskModal] = useState(false);
  const [renameForm, setRenameForm] = useState(null);
  const [workspaceForm, setWorkspaceForm] = useState({ path: "", name: "", defaultSandbox: "" });
  const [taskForm, setTaskForm] = useState({ title: "", prompt: "", workflowId: "", executionMode: "immediate", attachmentPaths: [] });
  const [composerAttachments, setComposerAttachments] = useState([]);
  const [resumePendingRunID, setResumePendingRunID] = useState("");
  const [notice, setNotice] = useState(null);
  const [busy, setBusy] = useState("");
  const [permissionBusy, setPermissionBusy] = useState("");
  const [workers, setWorkers] = useState([]);
  const [workerHealth, setWorkerHealth] = useState({});
  const [workerModal, setWorkerModal] = useState(false);
  const [workerForm, setWorkerForm] = useState({ id: "", name: "", baseUrl: "http://", token: "", enabled: true });
  const [settings, setSettings] = useState(demoSettings);
  const [appDialog, setAppDialog] = useState(null);
  const appDialogResolve = useRef(null);
  const runListLoadVersion = useRef(0);
  const runLoadVersion = useRef(0);
  const globalTaskSearchVersion = useRef(0);
  const runActionPending = useRef("");
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

  const selectedWorkspace = workspaces.find((item) => item.id === workspaceID);

  const notify = useCallback((type, text) => {
    setNotice({ type, text });
    window.setTimeout(() => setNotice(null), 4200);
  }, []);

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
    setWorkspaces((items) => sortWorkspaces(items.map((item) => item.id === nextWorkspaceID ? { ...item, lastOpenedAt: new Date().toISOString() } : item)));
    setRunItems([]);
    setTasks([]);
    setRunTotal(0);
    setRunNextCursor("");
    setSelectedRunID("");
    setSelectedQueuedTaskID("");
    setRunDetail(null);
    setResumePendingRunID("");
    setGlobalSearchQuery("");
    setWorkspaceSearchOpen(false);
    if (mode === "wails") WorkspaceBinding.OpenWorkspace(nextWorkspaceID).then(() => WorkspaceBinding.ListWorkspaces()).then((items) => setWorkspaces(items || [])).catch((error) => notify("error", errorMessage(error)));
  }, [mode, notify, workspaceID]);

  const boot = useCallback(async () => {
    try {
      const [runtimeItems, workspaceItems, workflowItems, workerItems, settingsValue] = await Promise.all([
        RuntimeBinding.ListRuntimes(), WorkspaceBinding.ListWorkspaces(), WorkflowBinding.ListDefinitions(), WorkerBinding.ListWorkers(), SettingsBinding.GetSettings(),
      ]);
      let orderedWorkspaces = sortWorkspaces(workspaceItems || []);
      if (orderedWorkspaces[0]) {
        try {
          const openedWorkspace = await WorkspaceBinding.OpenWorkspace(orderedWorkspaces[0].id);
          orderedWorkspaces = sortWorkspaces(orderedWorkspaces.map((item) => item.id === openedWorkspace.id ? openedWorkspace : item));
        } catch {
          // Preference bookkeeping must not prevent the local workbench from opening.
        }
      }
      setMode("wails");
      setRuntimes(runtimeItems || []);
      setWorkspaces(orderedWorkspaces);
      setWorkflows(workflowItems || []);
      setWorkers(workerItems || []);
      setSettings(settingsValue || demoSettings);
      setWorkspaceID((current) => current || orderedWorkspaces[0]?.id || "");
      setTaskForm((current) => ({ ...current, workflowId: current.workflowId || workflowItems?.[0]?.id || "" }));
    } catch {
      setMode("demo");
      setRuntimes(demoRuntimes);
      setWorkspaces(sortWorkspaces(demoWorkspaces));
      setWorkflows(demoWorkflows);
      setWorkers(demoWorkers);
      setSettings(demoSettings);
      setWorkspaceID(demoWorkspaces[0].id);
      setTaskForm((current) => ({ ...current, workflowId: "implement_review" }));
      setRunItems([{ ...demoRun.run, task: demoTasks[0] }]);
      setTasks(demoTasks);
      setRunTotal(1);
      setSelectedRunID("run_demo");
      setRunDetail(demoRun);
    }
  }, []);

  useEffect(() => { boot(); }, [boot]);

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
          const workspaceByID = new Map(demoWorkspaces.map((workspace) => [workspace.id, workspace]));
          const items = demoTasks.map((task) => ({ task, workspace: workspaceByID.get(task.workspaceId), latestRun: demoRun.run.taskId === task.id ? demoRun.run : null }))
            .filter((item) => item.workspace && (!keyword || `${item.task.title}\n${item.task.prompt}\n${item.workspace.name}\n${item.workspace.path}`.toLocaleLowerCase().includes(keyword)))
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
        const items = [{ ...demoRun.run, task: demoTasks[0] }].filter((item) => (!runStatus || item.status === runStatus) && (!keyword || `${item.id}\n${item.task.title}\n${Object.values(item.sessions || {}).join("\n")}`.toLocaleLowerCase().includes(keyword)));
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
      setTasks(mode === "demo" ? demoTasks : (await TaskRunBinding.ListTasks(workspaceID)) || []);
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

  const loadRun = useCallback(async (runID, silent = false) => {
    if (!runID) return;
    const loadVersion = ++runLoadVersion.current;
    const liveGeneration = liveFrameGenerationRef.current;
    if (mode === "demo") { setRunDetail((current) => current?.run?.id === runID ? current : demoRun); return; }
    try {
      let detail = await TaskRunBinding.GetRun(runID);
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
      setRunDetail((current) => preserveEqualValue(current, detail));
      setRunItems((items) => {
        let changed = false;
        const next = items.map((item) => {
          if (item.id !== runID) return item;
          const merged = preserveEqualValue(item, { ...detail.run, task: detail.task });
          if (merged !== item) changed = true;
          return merged;
        });
        return changed ? next : items;
      });
      if (!silent) setSelectedRunID(runID);
    } catch (error) { if (loadVersion === runLoadVersion.current && !silent) notify("error", errorMessage(error)); }
  }, [mode, notify]);

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
      // pushes the bounded run state on the oneshot:run-state channel below.
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

  // Self-scheduling poll for the task queue + run list, same ref-driven cadence.
  useEffect(() => {
    if (!workspaceID || mode !== "wails") return undefined;
    let stopped = false;
    let timer;
    const cadence = () => {
      const tasksActive = tasksRef.current.some((task) => task.status === "queued" || task.status === "running");
      return runDetailRef.current?.active || tasksActive ? 1400 : 4500;
    };
    const tick = async () => {
      await Promise.all([loadTasks(), loadRunList()]);
      if (!stopped) timer = window.setTimeout(tick, cadence());
    };
    timer = window.setTimeout(tick, cadence());
    return () => { stopped = true; window.clearTimeout(timer); };
  }, [loadRunList, loadTasks, mode, workspaceID]);

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
    if (mode === "demo") { setWorkspaceForm((form) => ({ ...form, path: "/Users/demo/Code/my-project", name: "my-project" })); setWorkspaceModal(true); return; }
    try {
      const path = await WorkspaceBinding.ChooseDirectory();
      if (path) { setWorkspaceForm({ path, name: "", defaultSandbox: "" }); setWorkspaceModal(true); }
    } catch (error) { notify("error", errorMessage(error)); }
  }, [mode, notify]);

  const addWorkspace = async () => {
    if (!workspaceForm.path.trim()) { notify("error", t("app.workspacePathRequired")); return; }
    if (workspaceForm.defaultSandbox === "full" && !await requestConfirm({ title: t("app.fullWorkspaceTitle"), description: t("app.fullWorkspaceDescription"), detail: t("app.fullWorkspaceDetail"), confirmLabel: t("app.confirmFullAccess"), dangerous: true })) return;
    setBusy("workspace");
    try {
      if (mode === "demo") {
        const item = { ...workspaceForm, id: `workspace-${Date.now()}`, name: workspaceForm.name || workspaceForm.path.split("/").pop(), lastOpenedAt: new Date().toISOString() };
        setWorkspaces((items) => [item, ...items]); selectWorkspace(item.id);
      } else {
        const item = await WorkspaceBinding.AddWorkspace(workspaceForm);
        setWorkspaces(await WorkspaceBinding.ListWorkspaces()); selectWorkspace(item.id);
      }
      setWorkspaceModal(false); notify("success", t("app.workspaceAdded"));
    } catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
  };

  const toggleWorkspacePinned = useCallback(async (workspace) => {
    const pinned = !workspace.pinned;
    setWorkspaces((items) => sortWorkspaces(items.map((item) => item.id === workspace.id ? { ...item, pinned } : item)));
    try {
      if (mode !== "demo") {
        await WorkspaceBinding.SetWorkspacePinned(workspace.id, pinned);
        setWorkspaces(sortWorkspaces(await WorkspaceBinding.ListWorkspaces()));
      }
    } catch (error) {
      setWorkspaces((items) => sortWorkspaces(items.map((item) => item.id === workspace.id ? { ...item, pinned: workspace.pinned } : item)));
      notify("error", errorMessage(error));
    }
  }, [mode, notify]);

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

  const saveWorker = async () => {
    if (!workerForm.id.trim() || !workerForm.name.trim() || !workerForm.baseUrl.trim()) { notify("error", t("app.workerFieldsRequired")); return; }
    setBusy("worker");
    try {
      if (mode === "demo") setWorkers((items) => [...items.filter((item) => item.id !== workerForm.id), { ...workerForm, hasToken: Boolean(workerForm.token) }]);
      else { await WorkerBinding.SaveWorker(workerForm); setWorkers(await WorkerBinding.ListWorkers()); }
      setWorkerModal(false); notify("success", t("app.workerSaved"));
    } catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
  };

  const checkWorker = async (worker) => {
    setWorkerHealth((current) => ({ ...current, [worker.id]: { checking: true } }));
    try {
      const status = mode === "demo" ? { health: { workerId: worker.id, name: worker.name, runtimes: { codex: true, claude: true, modu: true } } } : await WorkerBinding.CheckWorker(worker.id);
      setWorkerHealth((current) => ({ ...current, [worker.id]: { ok: true, ...status.health } }));
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
    if (!workspaceID || !taskForm.title.trim() || !taskForm.prompt.trim() || !taskForm.workflowId) { notify("error", t("app.taskFieldsRequired")); return; }
    const selectedWorkflow = workflows.find((item) => item.id === taskForm.workflowId);
    if (selectedWorkflow?.steps?.some((step) => step.sandbox === "full") && !await requestConfirm({ title: t("app.fullRunTitle"), description: t("app.fullRunDescription"), detail: t("app.workflowDetail", { name: selectedWorkflow.name }), confirmLabel: t("app.confirmStart"), dangerous: true })) return;
    setBusy("run");
    setRunStatus("");
    setRunSearchDraft("");
    setRunKeyword("");
    try {
      if (mode === "demo") {
        if (taskForm.executionMode === "queued") {
          const queuedTask = { id: `task_${Date.now()}`, workspaceId: workspaceID, title: taskForm.title, prompt: taskForm.prompt, workflowId: taskForm.workflowId, status: "queued", executionMode: "queued", queue: { state: "waiting", enqueuedAt: new Date().toISOString(), authorized: true }, attachments: taskForm.attachmentPaths.map((path) => ({ id: path, name: fileName(path), storedPath: path })), createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() };
          setTasks((items) => [...items, queuedTask]); setSelectedRunID(""); setRunDetail(null); setSelectedQueuedTaskID(queuedTask.id);
        } else {
          const demoTask = { ...demoTasks[0], title: taskForm.title, prompt: taskForm.prompt, workflowId: taskForm.workflowId, updatedAt: new Date().toISOString() };
          setTasks((items) => items.map((item) => item.id === demoTask.id ? demoTask : item)); setRunItems([{ ...demoRun.run, workflowId: demoTask.workflowId, status: "running", task: demoTask }]); setRunTotal(1); setSelectedQueuedTaskID(""); setSelectedRunID("run_demo"); setRunDetail({ ...demoRun, task: demoTask, run: { ...demoRun.run, workflowId: demoTask.workflowId, status: "running" }, active: true });
        }
      } else {
        const task = await TaskRunBinding.CreateTask({ workspaceId: workspaceID, title: taskForm.title, prompt: taskForm.prompt, workflowId: taskForm.workflowId, attachmentPaths: taskForm.attachmentPaths });
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
      setTaskForm((form) => ({ ...form, title: "", prompt: "", attachmentPaths: [] })); setTaskModal(false); notify("success", taskForm.executionMode === "queued" ? t("app.taskQueued") : t("app.runStarted"));
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
  const submitWorkbenchComposer = useCallback(async (modeName = "queue", content = "") => {
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
        } else if (run.status === "paused") {
          setRunDetail((detail) => ({ ...detail, active: true, run: { ...detail.run, status: "running", pauseReason: "" } }));
          setRunItems((items) => items.map((item) => item.id === run.id ? { ...item, status: "running" } : item));
        }
      } else if (run.status === "running") {
        const input = { content, attachmentPaths: attachments };
        if (modeName === "insert") await TaskRunBinding.InterruptAndInsert(run.id, input);
        else await TaskRunBinding.EnqueueInstruction(run.id, input);
      } else if (run.status === "paused") {
        if (attachments.length) {
          await TaskRunBinding.EnqueueInstruction(run.id, { content: content || t("composer.attachmentInstruction"), attachmentPaths: attachments });
          await TaskRunBinding.ResumeRun(run.id, "");
        } else {
          await TaskRunBinding.ResumeRun(run.id, content);
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

  const openRenameTask = useCallback(() => {
    const task = runDetailRef.current?.task || tasksRef.current.find((item) => item.id === selectedQueuedTaskIDRef.current);
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

  const deleteSelectedTask = useCallback(async () => {
    const task = runDetailRef.current?.task || tasksRef.current.find((item) => item.id === selectedQueuedTaskIDRef.current);
    if (!task || !await requestConfirm({ title: t("app.deleteTaskTitle", { name: task.title }), description: t("app.deleteTaskDescription"), confirmLabel: t("app.deleteTask"), dangerous: true })) return;
    try { if (mode !== "demo") await TaskRunBinding.DeleteTask(task.id); setSelectedRunID(""); setSelectedQueuedTaskID(""); setRunDetail(null); await loadTasks(); await loadRunList(); }
    catch (error) { notify("error", errorMessage(error)); }
  }, [loadRunList, loadTasks, mode, notify, requestConfirm, t]);

  const composerSubmitKey = (event) => {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter" && busy !== "run" && selectedWorkspace) createTaskAndRun();
  };

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
      setTaskForm((form) => ({ ...form, workflowId: saved.id })); setEditor(null); setEditorSourceID(""); notify("success", t("app.workflowSaved"));
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
      setTaskForm((form) => form.workflowId === workflow.id ? { ...form, workflowId: items[0]?.id || "" } : form);
      notify("success", t("app.workflowDeleted"));
    } catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
  };

  const sidebarWorkspaces = useMemo(() => workspaceSections(workspaces, { selectedID: workspaceID, query: "", expanded: workspaceExpanded }), [workspaceExpanded, workspaceID, workspaces]);
  const goView = useCallback((next) => { setEditor(null); setEditorSourceID(""); setView(next); }, []);
  const commandText = view === "settings" ? `${t("sidebar.settings")} @ ~/.oneshot` : selectedWorkspace ? `${selectedWorkspace.name} @ ${selectedWorkspace.path}` : t("app.selectWorkspace");

  const toggleWorkspaceSearch = useCallback(() => setWorkspaceSearchOpen((open) => !open), []);
  const clearSidebarSearch = useCallback(() => { setGlobalSearchQuery(""); setGlobalTaskItems([]); }, []);
  const toggleWorkspaceExpanded = useCallback(() => setWorkspaceExpanded((expanded) => !expanded), []);
  const selectRun = useCallback((item) => { setSelectedQueuedTaskID(""); setSelectedRunID(item.id); loadRun(item.id); }, [loadRun]);
  const selectQueued = useCallback((task) => { setSelectedRunID(""); setRunDetail(null); setSelectedQueuedTaskID(task.id); }, []);
  // Stable handler identities keep the memoized Sidebar/TaskWorkbench from
  // re-rendering on every unrelated App state change (toasts, busy flips, the
  // 80ms streaming cadence).
  const openTaskModal = useCallback(() => setTaskModal(true), []);
  const loadMoreRuns = useCallback(() => loadRunList({ cursor: runNextCursorRef.current }), [loadRunList]);
  const chooseComposerAttachments = useCallback(() => chooseAttachments("composer"), [chooseAttachments]);
  const removeComposerAttachment = useCallback((path) => setComposerAttachments((items) => items.filter((item) => item !== path)), []);
  const interruptRun = useCallback(() => runAction("interrupt"), [runAction]);
  const cancelRun = useCallback(() => runAction("cancel"), [runAction]);

  if (mode === "loading") return <div className="loading-screen"><div className="brand-mark">1</div><span>{t("task.opening")}</span></div>;

  return <div className="app-frame">
    <header className="mac-titlebar" aria-label={t("app.windowAria")}><span aria-hidden="true" /><strong>Oneshot</strong><span /></header>
    <div className="app-shell">
      <Sidebar
        workspaces={workspaces}
        workspaceID={workspaceID}
        pinnedWorkspaces={sidebarWorkspaces.pinned}
        projectWorkspaces={sidebarWorkspaces.projects}
        searchQuery={globalSearchQuery}
        searchTaskItems={globalTaskItems}
        searchLoading={globalTaskSearchLoading}
        workspaceExpanded={workspaceExpanded}
        workspaceSearchOpen={workspaceSearchOpen}
        tasks={tasks}
        runs={runItems}
        selectedRunID={selectedRunID}
        selectedQueuedTaskID={selectedQueuedTaskID}
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
        onTogglePinned={toggleWorkspacePinned}
        onRemoveWorkspace={removeWorkspace}
        onToggleExpanded={toggleWorkspaceExpanded}
        onAddWorkspace={chooseWorkspace}
        onNewTask={openTaskModal}
        onLoadMoreRuns={loadMoreRuns}
        onSelectRun={selectRun}
        onSelectQueued={selectQueued}
        onGoView={goView}
      />

      <main className="main-area">
        <div className="command-strip"><span>&gt;</span><strong>{commandText}</strong><span className={`connection ${mode}`}>{mode === "wails" ? t("common.local") : t("common.preview")}</span></div>
        {editor ? <WorkflowEditor editor={editor} setEditor={setEditor} validation={validation} validateEditor={validateEditor} saveWorkflow={saveWorkflow} busy={busy} updateStep={updateStep} updateTransition={updateTransition} removeTransition={removeTransition} runtimes={runtimes} workers={settings.experimental?.remoteWorkersEnabled ? workers : []} defaultSandbox={settings.execution.defaultSandbox} allowFullSandbox={settings.security.allowFullSandbox} onClose={() => { setEditor(null); setEditorSourceID(""); }} /> : view === "tasks" ? <TaskWorkbench
          mode={mode}
          workspaceID={workspaceID}
          tasks={tasks}
          runDetail={runDetail}
          selectedRunID={selectedRunID}
          selectedQueuedTaskID={selectedQueuedTaskID}
          workflows={workflows}
          busy={busy}
          permissionBusy={permissionBusy}
          attachments={composerAttachments}
          onNewTask={openTaskModal}
          onChooseAttachments={chooseComposerAttachments}
          onRemoveAttachment={removeComposerAttachment}
          onSubmit={submitWorkbenchComposer}
          onInterrupt={interruptRun}
          onCancel={cancelRun}
          onRemoveInstruction={removeQueuedInstruction}
          onPermissionDecision={respondPermission}
          onRename={openRenameTask}
          onDelete={deleteSelectedTask}
          notify={notify}
        /> : view === "workflows" ? <WorkflowLibrary workflows={workflows} runtimes={runtimes} openEditor={openEditor} deleteWorkflow={deleteWorkflow} busy={busy} /> : <SettingsPage mode={mode} value={settings} runtimes={runtimes} onChange={setSettings} notify={notify} workersPanel={<WorkerPage workers={workers} health={workerHealth} checkWorker={checkWorker} deleteWorker={deleteWorker} openWorker={(worker) => { setWorkerForm(worker ? { id: worker.id, name: worker.name, baseUrl: worker.baseUrl, token: "", enabled: worker.enabled } : { id: "", name: "", baseUrl: "http://", token: "", enabled: true }); setWorkerModal(true); }} />} />}
      </main>
    </div>
    {workspaceModal && <Modal title={t("workspace.addTitle")} subtitle={t("workspace.addSubtitle")} onClose={() => setWorkspaceModal(false)}><div className="form-stack"><label>{t("workspace.path")}<input autoFocus value={workspaceForm.path} onChange={(event) => setWorkspaceForm((form) => ({ ...form, path: event.target.value }))} placeholder="/Users/me/Code/project" /></label><label>{t("workspace.displayName")}<input value={workspaceForm.name} onChange={(event) => setWorkspaceForm((form) => ({ ...form, name: event.target.value }))} placeholder={t("workspace.defaultName")} /></label><label>{t("workspace.defaultSandbox")}<TUISelect ariaLabel={t("workspace.defaultSandbox")} value={workspaceForm.defaultSandbox} onChange={(defaultSandbox) => setWorkspaceForm((form) => ({ ...form, defaultSandbox }))} options={[{ value: "", label: t("workspace.globalDefault") }, { value: "read-only", label: t("workspace.readOnly") }, { value: "workspace-write", label: t("workspace.write") }, ...(settings.security?.allowFullSandbox ? [{ value: "full", label: t("workspace.fullDanger") }] : [])]} /></label><div className="modal-actions"><Action tone="muted" onClick={() => setWorkspaceModal(false)}>{t("common.cancel")}</Action><Action tone="primary" onClick={addWorkspace} disabled={busy === "workspace"}>{t("workspace.add")}</Action></div></div></Modal>}
    {taskModal && <Modal title={t("task.createTitle")} subtitle={t("task.createSubtitle")} onClose={() => setTaskModal(false)}><div className="form-stack task-create-form"><label>{t("task.name")}<input autoFocus value={taskForm.title} onChange={(event) => setTaskForm((form) => ({ ...form, title: event.target.value }))} onKeyDown={composerSubmitKey} placeholder={t("task.namePlaceholder")} /></label><label>{t("task.goal")}<textarea value={taskForm.prompt} onChange={(event) => setTaskForm((form) => ({ ...form, prompt: event.target.value }))} onKeyDown={composerSubmitKey} placeholder={t("task.goalPlaceholder")} /></label><label>{t("task.workflow")}<TUISelect ariaLabel={t("task.workflow")} value={taskForm.workflowId} onChange={(workflowId) => setTaskForm((form) => ({ ...form, workflowId }))} options={workflows.map((workflow) => ({ value: workflow.id, label: workflow.name }))} /></label><label>{t("task.executionMode")}<TUISelect ariaLabel={t("task.executionMode")} value={taskForm.executionMode} onChange={(executionMode) => setTaskForm((form) => ({ ...form, executionMode }))} options={[{ value: "immediate", label: t("task.runNow") }, { value: "queued", label: t("task.joinQueue") }]} /></label><div className="attachment-picker"><span>{t("task.attachmentsLimit")}</span><Action size="compact" onClick={() => chooseAttachments("task")}>+ {t("task.chooseFiles")}</Action>{taskForm.attachmentPaths?.map((path) => <div className="attachment-chip" key={path}><span title={path}>{fileName(path)}</span><Action size="compact" tone="danger" onClick={() => setTaskForm((form) => ({ ...form, attachmentPaths: form.attachmentPaths.filter((item) => item !== path) }))}>{t("common.remove")}</Action></div>)}</div><div className="modal-actions"><Action tone="muted" onClick={() => setTaskModal(false)}>{t("common.cancel")}</Action><Action tone="primary" onClick={createTaskAndRun} disabled={busy === "run" || !selectedWorkspace}>{busy === "run" ? t("task.creating") : taskForm.executionMode === "queued" ? t("task.joinQueue") : t("task.createAndRun")}</Action></div></div></Modal>}
    {renameForm && <Modal title={t("task.renameTitle")} subtitle={t("task.renameSubtitle")} onClose={() => busy !== "rename" && setRenameForm(null)}><div className="form-stack"><label>{t("task.name")}<input autoFocus maxLength={160} value={renameForm.title} onChange={(event) => setRenameForm((form) => ({ ...form, title: event.target.value }))} onKeyDown={(event) => { if (event.key === "Enter" && !event.nativeEvent.isComposing && busy !== "rename") renameSelectedTask(); }} /></label><div className="modal-actions"><Action tone="muted" disabled={busy === "rename"} onClick={() => setRenameForm(null)}>{t("common.cancel")}</Action><Action tone="primary" disabled={busy === "rename" || !renameForm.title.trim()} onClick={renameSelectedTask}>{busy === "rename" ? t("common.saving") : t("task.saveName")}</Action></div></div></Modal>}
    {workerModal && <Modal title={t("worker.modalTitle")} subtitle={t("worker.modalSubtitle")} onClose={() => setWorkerModal(false)}><div className="form-stack"><label>{t("worker.id")}<input value={workerForm.id} onChange={(event) => setWorkerForm((form) => ({ ...form, id: event.target.value }))} placeholder="mac-mini" /></label><label>{t("worker.name")}<input value={workerForm.name} onChange={(event) => setWorkerForm((form) => ({ ...form, name: event.target.value }))} placeholder="Build Mac mini" /></label><label>{t("worker.baseUrl")}<input value={workerForm.baseUrl} onChange={(event) => setWorkerForm((form) => ({ ...form, baseUrl: event.target.value }))} placeholder="http://192.168.1.20:9231" /></label><label>{t("worker.bearerToken")}<input type="password" value={workerForm.token} onChange={(event) => setWorkerForm((form) => ({ ...form, token: event.target.value }))} placeholder={t("worker.keepToken")} /></label><label className="checkbox-label"><input type="checkbox" checked={workerForm.enabled} onChange={(event) => setWorkerForm((form) => ({ ...form, enabled: event.target.checked }))} />{t("worker.enableScheduling")}</label><div className="modal-actions"><Action tone="muted" onClick={() => setWorkerModal(false)}>{t("common.cancel")}</Action><Action tone="primary" disabled={busy === "worker"} onClick={saveWorker}>{t("worker.save")}</Action></div></div></Modal>}
    <ConfirmDialog dialog={appDialog} onCancel={() => resolveConfirm(false)} onConfirm={() => resolveConfirm(true)} />
    {notice && <div className={`toast ${notice.type}`}><span>{notice.text}</span></div>}
  </div>;
}

export default App;
