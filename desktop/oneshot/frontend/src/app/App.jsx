import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  RuntimeBinding,
  SettingsBinding,
  TaskRunBinding,
  WorkflowBinding,
  WorkspaceBinding,
  WorkerBinding,
} from "../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";
import SettingsPage, { ConfirmDialog, demoSettings } from "./SettingsPage.jsx";
import { mergeRunItems, preserveEqualValue, sortWorkspaces, workspaceResults } from "./listNavigation.js";
import { TUISelect } from "../ui/primitives.jsx";
import { copy, errorMessage, fileName } from "./format.js";
import { loopTemplate } from "./templates.js";
import { demoRun, demoRuntimes, demoTasks, demoWorkers, demoWorkflows, demoWorkspaces } from "./demoData.js";
import Sidebar from "./components/Sidebar.jsx";
import TaskWorkbench from "./components/TaskWorkbench.jsx";
import WorkflowLibrary from "./components/workflow/WorkflowLibrary.jsx";
import WorkflowEditor from "./components/workflow/WorkflowEditor.jsx";
import WorkerPage from "./components/WorkerPage.jsx";
import Modal from "./components/Modal.jsx";

function App() {
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
  const [workspaceQuery, setWorkspaceQuery] = useState("");
  const [workspaceExpanded, setWorkspaceExpanded] = useState(false);
  const [workspaceSearchOpen, setWorkspaceSearchOpen] = useState(false);
  const [selectedRunID, setSelectedRunID] = useState("");
  const [selectedQueuedTaskID, setSelectedQueuedTaskID] = useState("");
  const [runDetail, setRunDetail] = useState(null);
  const [editor, setEditor] = useState(null);
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
  const [workers, setWorkers] = useState([]);
  const [workerHealth, setWorkerHealth] = useState({});
  const [workerModal, setWorkerModal] = useState(false);
  const [workerForm, setWorkerForm] = useState({ id: "", name: "", baseUrl: "http://", token: "", enabled: true });
  const [settings, setSettings] = useState(demoSettings);
  const [appDialog, setAppDialog] = useState(null);
  const appDialogResolve = useRef(null);
  const runListLoadVersion = useRef(0);
  const runLoadVersion = useRef(0);
  const runActionPending = useRef("");
  const selectedRunIDRef = useRef("");
  // Latest-value refs so the polling loops can read fresh state without listing
  // `tasks`/`runDetail` as effect deps (which would tear down and rebuild the
  // timer on every tick — a big source of the jank).
  const runDetailRef = useRef(null);
  runDetailRef.current = runDetail;
  const tasksRef = useRef([]);
  tasksRef.current = tasks;

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
      setWorkspaceQuery("");
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
    setWorkspaceQuery("");
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
    if (mode === "demo") { setRunDetail((current) => current?.run?.id === runID ? current : demoRun); return; }
    try {
      const detail = await TaskRunBinding.GetRun(runID);
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

  useEffect(() => { selectedRunIDRef.current = selectedRunID; }, [selectedRunID]);

  useEffect(() => {
    if (!resumePendingRunID) return;
    if (selectedRunID !== resumePendingRunID || runDetail?.run?.id !== resumePendingRunID || runDetail.run.status !== "paused" || !runDetail.active) {
      setResumePendingRunID("");
    }
  }, [resumePendingRunID, runDetail?.active, runDetail?.run?.id, runDetail?.run?.status, selectedRunID]);

  // Self-scheduling poll for the open run. Cadence adapts from a ref each tick,
  // so the timer is created once per selection instead of on every state change.
  useEffect(() => {
    if (!selectedRunID || mode !== "wails") return undefined;
    let stopped = false;
    let timer;
    const cadence = () => (runDetailRef.current?.active || runDetailRef.current?.run?.status === "running" ? 900 : 2500);
    const tick = async () => {
      await loadRun(selectedRunID, true);
      if (!stopped) timer = window.setTimeout(tick, cadence());
    };
    timer = window.setTimeout(tick, cadence());
    return () => { stopped = true; window.clearTimeout(timer); };
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
    if (!workspaceForm.path.trim()) { notify("error", "请输入工作目录路径"); return; }
    if (workspaceForm.defaultSandbox === "full" && !await requestConfirm({ title: "以 Full access 加入工作目录？", description: "Agent 可以在这个工作目录之外读取和写入。只应对完全信任的项目和 Workflow 使用。", detail: "这会成为该工作目录的新 Run 默认权限。", confirmLabel: "确认 Full access", dangerous: true })) return;
    setBusy("workspace");
    try {
      if (mode === "demo") {
        const item = { ...workspaceForm, id: `workspace-${Date.now()}`, name: workspaceForm.name || workspaceForm.path.split("/").pop(), lastOpenedAt: new Date().toISOString() };
        setWorkspaces((items) => [item, ...items]); selectWorkspace(item.id);
      } else {
        const item = await WorkspaceBinding.AddWorkspace(workspaceForm);
        setWorkspaces(await WorkspaceBinding.ListWorkspaces()); selectWorkspace(item.id);
      }
      setWorkspaceModal(false); notify("success", "工作目录已加入");
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
    if (!await requestConfirm({ title: `从列表移除“${workspace.name}”？`, description: "只移除工作台中的目录引用，不删除项目文件、Task 或 Run 历史。以后重新加入同一路径会恢复历史关联。", detail: workspace.path, confirmLabel: "从列表移除", dangerous: true })) return;
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
      notify("success", "已从 CWD 列表移除，磁盘文件未改动");
    } catch (error) {
      notify("error", errorMessage(error));
    }
  }, [mode, notify, requestConfirm, selectWorkspace, workspaceID, workspaces]);

  const saveWorker = async () => {
    if (!workerForm.id.trim() || !workerForm.name.trim() || !workerForm.baseUrl.trim()) { notify("error", "请填写 Worker ID、名称和地址"); return; }
    setBusy("worker");
    try {
      if (mode === "demo") setWorkers((items) => [...items.filter((item) => item.id !== workerForm.id), { ...workerForm, hasToken: Boolean(workerForm.token) }]);
      else { await WorkerBinding.SaveWorker(workerForm); setWorkers(await WorkerBinding.ListWorkers()); }
      setWorkerModal(false); notify("success", "Worker 已保存，token 不会在列表接口返回");
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
    if (!await requestConfirm({ title: `删除 Worker“${worker?.name || id}”？`, description: "本机保存的 Worker 地址和 token 会被删除，但历史 Run 与事件记录会继续保留。", confirmLabel: "删除 Worker", dangerous: true })) return;
    try {
      if (mode === "demo") setWorkers((items) => items.filter((item) => item.id !== id));
      else { await WorkerBinding.DeleteWorker(id); setWorkers(await WorkerBinding.ListWorkers()); }
    } catch (error) { notify("error", errorMessage(error)); }
  };

  const createTaskAndRun = async () => {
    if (!workspaceID || !taskForm.title.trim() || !taskForm.prompt.trim() || !taskForm.workflowId) { notify("error", "请填写标题、任务目标并选择 Workflow"); return; }
    const selectedWorkflow = workflows.find((item) => item.id === taskForm.workflowId);
    if (selectedWorkflow?.steps?.some((step) => step.sandbox === "full") && !await requestConfirm({ title: "启动包含 Full access 的 Run？", description: "这个 Workflow 的 Agent 可以在工作目录之外执行读写。请确认任务内容和 Workflow 都可信。", detail: `Workflow：${selectedWorkflow.name}`, confirmLabel: "确认并启动", dangerous: true })) return;
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
      setTaskForm((form) => ({ ...form, title: "", prompt: "", attachmentPaths: [] })); setTaskModal(false); notify("success", taskForm.executionMode === "queued" ? "任务已加入 Workspace 队列" : "Run 已启动，Agent 正在后台执行");
    } catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
  };

  const chooseAttachments = async (target) => {
    try {
      const paths = mode === "demo" ? ["/Users/demo/Desktop/reference.png"] : await WorkspaceBinding.ChooseAttachments();
      if (!paths?.length) return;
      if (target === "task") setTaskForm((form) => ({ ...form, attachmentPaths: [...new Set([...(form.attachmentPaths || []), ...paths])].slice(0, 8) }));
      else setComposerAttachments((items) => [...new Set([...items, ...paths])].slice(0, 8));
    } catch (error) { notify("error", errorMessage(error)); }
  };

  // Returns true when the submit was accepted so the composer can clear its
  // local draft; false keeps whatever the user typed.
  const submitWorkbenchComposer = async (modeName = "queue", content = "") => {
    if (!runDetail?.run?.id) { setTaskModal(true); return false; }
    const run = runDetail.run;
    if (!content && !composerAttachments.length && run.status !== "paused") return false;
    setBusy(modeName);
    try {
      if (mode === "demo") {
        if (run.status === "running") {
          const instruction = { id: `instruction_${Date.now()}`, content: content || "查看附件并继续任务", attachments: [...composerAttachments], status: "pending", priority: modeName === "insert", createdAt: new Date().toISOString() };
          setRunDetail((detail) => ({ ...detail, instructions: [...(detail.instructions || []), instruction] }));
        } else if (run.status === "paused") {
          setRunDetail((detail) => ({ ...detail, active: true, run: { ...detail.run, status: "running", pauseReason: "" } }));
          setRunItems((items) => items.map((item) => item.id === run.id ? { ...item, status: "running" } : item));
        }
      } else if (run.status === "running") {
        const input = { content, attachmentPaths: composerAttachments };
        if (modeName === "insert") await TaskRunBinding.InterruptAndInsert(run.id, input);
        else await TaskRunBinding.EnqueueInstruction(run.id, input);
      } else if (run.status === "paused") {
        if (composerAttachments.length) {
          await TaskRunBinding.EnqueueInstruction(run.id, { content: content || "查看附件并继续任务", attachmentPaths: composerAttachments });
          await TaskRunBinding.ResumeRun(run.id, "");
        } else {
          await TaskRunBinding.ResumeRun(run.id, content);
        }
        setResumePendingRunID(run.id);
      }
      setComposerAttachments([]);
      window.setTimeout(() => loadRun(run.id, true), 180);
      notify("success", modeName === "insert" ? "当前轮次将停止，并优先执行这条指令" : run.status === "running" ? "指令已加入下一轮队列" : "Run 正在恢复");
      return true;
    } catch (error) { notify("error", errorMessage(error)); return false; } finally { setBusy(""); }
  };

  const removeQueuedInstruction = async (instructionID) => {
    if (!runDetail?.run?.id) return;
    try { if (mode !== "demo") await TaskRunBinding.RemoveInstruction(runDetail.run.id, instructionID); await loadRun(runDetail.run.id, true); }
    catch (error) { notify("error", errorMessage(error)); }
  };

  const openRenameTask = () => {
    const task = runDetail?.task || tasks.find((item) => item.id === selectedQueuedTaskID);
    if (task) setRenameForm({ taskId: task.id, title: task.title, originalTitle: task.title });
  };

  const renameSelectedTask = async () => {
    const title = renameForm?.title.trim();
    if (!renameForm || !title) { notify("error", "任务名称不能为空"); return; }
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
      notify("success", "任务名称已更新");
    } catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
  };

  const deleteSelectedTask = async () => {
    const task = runDetail?.task || tasks.find((item) => item.id === selectedQueuedTaskID);
    if (!task || !await requestConfirm({ title: `删除任务“${task.title}”？`, description: "任务会从工作台隐藏，运行中的任务必须先打断。项目文件不会被删除。", confirmLabel: "删除任务", dangerous: true })) return;
    try { if (mode !== "demo") await TaskRunBinding.DeleteTask(task.id); setSelectedRunID(""); setSelectedQueuedTaskID(""); setRunDetail(null); await loadTasks(); await loadRunList(); }
    catch (error) { notify("error", errorMessage(error)); }
  };

  const composerSubmitKey = (event) => {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter" && busy !== "run" && selectedWorkspace) createTaskAndRun();
  };

  const runAction = async (action) => {
    if (!selectedRunID || runActionPending.current) return;
    const runID = selectedRunID;
    runActionPending.current = action;
    setBusy(action);
    try {
      if (mode === "demo") {
        const nextStatus = action === "interrupt" ? "paused" : "cancelled";
        setRunDetail((detail) => ({ ...detail, active: false, run: { ...detail.run, status: nextStatus, pauseReason: action === "interrupt" ? "interrupted" : "" } }));
        setRunItems((items) => items.map((item) => item.id === selectedRunID ? { ...item, status: nextStatus } : item));
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
  };

  const openEditor = (definition) => { const next = copy(definition || loopTemplate); if (!workflows.some((item) => item.id === next.id)) next.policy = { maxTransitions: settings.execution.maxTransitions, maxConsecutiveFailures: settings.execution.maxConsecutiveFailures, stepTimeoutSeconds: settings.execution.stepTimeoutSeconds }; setEditor(next); setValidation([]); };

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
    if (mode === "demo") {
      const issues = [];
      if (!/^[a-z][a-z0-9_-]*$/.test(editor.id)) issues.push({ path: "id", message: "必须是小写标识符" });
      if (!editor.steps?.length) issues.push({ path: "steps", message: "至少需要一个步骤" });
      setValidation(issues); return issues;
    }
    const issues = await WorkflowBinding.ValidateDefinition(editor); setValidation(issues || []); return issues || [];
  };

  const saveWorkflow = async () => {
    const issues = await validateEditor();
    if (issues.length) { notify("error", `有 ${issues.length} 个配置问题，请先修正`); return; }
    if (editor.steps.some((step) => step.sandbox === "full") && !await requestConfirm({ title: "保存包含 Full access 的 Workflow？", description: "这个 Workflow 包含可在工作目录之外读写的节点。保存后每次启动仍会再次确认。", detail: `Workflow：${editor.name || editor.id}`, confirmLabel: "确认并保存", dangerous: true })) return;
    setBusy("workflow");
    try {
      let saved = editor;
      if (mode !== "demo") {
        const exists = workflows.some((item) => item.id === editor.id);
        saved = exists ? await WorkflowBinding.UpdateDefinition(editor.id, editor) : await WorkflowBinding.CreateDefinition(editor);
        setWorkflows(await WorkflowBinding.ListDefinitions());
      } else setWorkflows((items) => [...items.filter((item) => item.id !== editor.id), editor]);
      setTaskForm((form) => ({ ...form, workflowId: saved.id })); setEditor(null); notify("success", "Workflow 已保存到 ~/.oneshot/");
    } catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
  };

  const visibleWorkspaces = useMemo(() => workspaceResults(workspaces, { selectedID: workspaceID, query: workspaceQuery, expanded: workspaceExpanded }), [workspaceExpanded, workspaceID, workspaceQuery, workspaces]);
  const goView = useCallback((next) => { setEditor(null); setView(next); }, []);
  const commandText = view === "settings" ? "settings @ ~/.oneshot" : selectedWorkspace ? `${selectedWorkspace.name} @ ${selectedWorkspace.path}` : "选择一个工作目录";

  const toggleWorkspaceSearch = useCallback(() => { setWorkspaceSearchOpen((open) => !open); if (workspaceSearchOpen) setWorkspaceQuery(""); }, [workspaceSearchOpen]);
  const toggleWorkspaceExpanded = useCallback(() => setWorkspaceExpanded((expanded) => !expanded), []);
  const selectRun = useCallback((item) => { setSelectedQueuedTaskID(""); setSelectedRunID(item.id); loadRun(item.id); }, [loadRun]);
  const selectQueued = useCallback((task) => { setSelectedRunID(""); setRunDetail(null); setSelectedQueuedTaskID(task.id); }, []);

  if (mode === "loading") return <div className="loading-screen"><div className="brand-mark">1</div><span>正在打开本地工作台…</span></div>;

  return <div className="app-frame">
    <header className="mac-titlebar" aria-label="Oneshot window"><span aria-hidden="true" /><strong>Oneshot</strong><span /></header>
    <div className="app-shell">
      <Sidebar
        workspaces={workspaces}
        workspaceID={workspaceID}
        visibleWorkspaces={visibleWorkspaces}
        workspaceQuery={workspaceQuery}
        workspaceExpanded={workspaceExpanded}
        workspaceSearchOpen={workspaceSearchOpen}
        runtimes={runtimes}
        view={view}
        editor={editor}
        onToggleSearch={toggleWorkspaceSearch}
        onQueryChange={setWorkspaceQuery}
        onSelectWorkspace={selectWorkspace}
        onTogglePinned={toggleWorkspacePinned}
        onRemoveWorkspace={removeWorkspace}
        onToggleExpanded={toggleWorkspaceExpanded}
        onAddWorkspace={chooseWorkspace}
        onGoView={goView}
      />

      <main className="main-area">
        <div className="command-strip"><span>&gt;</span><strong>{commandText}</strong><span className={`connection ${mode}`}>{mode === "wails" ? "local" : "preview"}</span></div>
        {editor ? <WorkflowEditor editor={editor} setEditor={setEditor} validation={validation} validateEditor={validateEditor} saveWorkflow={saveWorkflow} busy={busy} updateStep={updateStep} updateTransition={updateTransition} removeTransition={removeTransition} runtimes={runtimes} workers={settings.experimental?.remoteWorkersEnabled ? workers : []} defaultSandbox={settings.execution.defaultSandbox} allowFullSandbox={settings.security.allowFullSandbox} onClose={() => setEditor(null)} /> : view === "tasks" ? <TaskWorkbench
          mode={mode}
          workspaceID={workspaceID}
          tasks={tasks}
          runs={runItems}
          runDetail={runDetail}
          selectedRunID={selectedRunID}
          selectedQueuedTaskID={selectedQueuedTaskID}
          workflows={workflows}
          loading={runLoading}
          total={runTotal}
          hasMore={Boolean(runNextCursor)}
          search={runSearchDraft}
          status={runStatus}
          busy={busy}
          attachments={composerAttachments}
          onSearch={setRunSearchDraft}
          onStatus={setRunStatus}
          onNewTask={() => setTaskModal(true)}
          onLoadMore={() => loadRunList({ cursor: runNextCursor })}
          onSelectRun={selectRun}
          onSelectQueued={selectQueued}
          onChooseAttachments={() => chooseAttachments("composer")}
          onRemoveAttachment={(path) => setComposerAttachments((items) => items.filter((item) => item !== path))}
          onSubmit={submitWorkbenchComposer}
          onInterrupt={() => runAction("interrupt")}
          onCancel={() => runAction("cancel")}
          onRemoveInstruction={removeQueuedInstruction}
          onRename={openRenameTask}
          onDelete={deleteSelectedTask}
          notify={notify}
        /> : view === "workflows" ? <WorkflowLibrary workflows={workflows} runtimes={runtimes} openEditor={openEditor} /> : <SettingsPage mode={mode} value={settings} runtimes={runtimes} onChange={setSettings} notify={notify} workersPanel={<WorkerPage workers={workers} health={workerHealth} checkWorker={checkWorker} deleteWorker={deleteWorker} openWorker={(worker) => { setWorkerForm(worker ? { id: worker.id, name: worker.name, baseUrl: worker.baseUrl, token: "", enabled: worker.enabled } : { id: "", name: "", baseUrl: "http://", token: "", enabled: true }); setWorkerModal(true); }} />} />}
      </main>
    </div>
    {workspaceModal && <Modal title="加入工作目录" subtitle="Agent 只会在你授权的目录中工作" onClose={() => setWorkspaceModal(false)}><div className="form-stack"><label>目录路径<input autoFocus value={workspaceForm.path} onChange={(event) => setWorkspaceForm((form) => ({ ...form, path: event.target.value }))} placeholder="/Users/me/Code/project" /></label><label>显示名称（可选）<input value={workspaceForm.name} onChange={(event) => setWorkspaceForm((form) => ({ ...form, name: event.target.value }))} placeholder="默认使用目录名" /></label><label>默认 Sandbox<TUISelect ariaLabel="默认 Sandbox" value={workspaceForm.defaultSandbox} onChange={(defaultSandbox) => setWorkspaceForm((form) => ({ ...form, defaultSandbox }))} options={[{ value: "", label: "使用设置中的全局默认" }, { value: "read-only", label: "Read only" }, { value: "workspace-write", label: "Workspace write" }, ...(settings.security?.allowFullSandbox ? [{ value: "full", label: "Full access（危险）" }] : [])]} /></label><div className="modal-actions"><button className="secondary-button" onClick={() => setWorkspaceModal(false)}>[ 取消 ]</button><button className="primary-button" onClick={addWorkspace} disabled={busy === "workspace"}>[ 加入目录 ]</button></div></div></Modal>}
    {taskModal && <Modal title="新建任务" subtitle="立即运行，或加入当前 Workspace 的 FIFO 队列" onClose={() => setTaskModal(false)}><div className="form-stack task-create-form"><label>任务名称<input autoFocus value={taskForm.title} onChange={(event) => setTaskForm((form) => ({ ...form, title: event.target.value }))} onKeyDown={composerSubmitKey} placeholder="例如：补齐 Buddy 工作台能力" /></label><label>目标与验收<textarea value={taskForm.prompt} onChange={(event) => setTaskForm((form) => ({ ...form, prompt: event.target.value }))} onKeyDown={composerSubmitKey} placeholder="描述目标、约束和验收方式。" /></label><label>Workflow<TUISelect ariaLabel="Workflow" value={taskForm.workflowId} onChange={(workflowId) => setTaskForm((form) => ({ ...form, workflowId }))} options={workflows.map((workflow) => ({ value: workflow.id, label: workflow.name }))} /></label><label>执行方式<TUISelect ariaLabel="执行方式" value={taskForm.executionMode} onChange={(executionMode) => setTaskForm((form) => ({ ...form, executionMode }))} options={[{ value: "immediate", label: "立即运行" }, { value: "queued", label: "加入 Workspace 队列" }]} /></label><div className="attachment-picker"><span>附件 · 最多 8 个</span><button type="button" className="text-button" onClick={() => chooseAttachments("task")}>[ + 选择文件 ]</button>{taskForm.attachmentPaths?.map((path) => <div className="attachment-chip" key={path}><span title={path}>{fileName(path)}</span><button type="button" onClick={() => setTaskForm((form) => ({ ...form, attachmentPaths: form.attachmentPaths.filter((item) => item !== path) }))}>×</button></div>)}</div><div className="modal-actions"><button className="secondary-button" onClick={() => setTaskModal(false)}>[ 取消 ]</button><button className="primary-button" onClick={createTaskAndRun} disabled={busy === "run" || !selectedWorkspace}>[ {busy === "run" ? "创建中…" : taskForm.executionMode === "queued" ? "加入队列" : "创建并运行"} ]</button></div></div></Modal>}
    {renameForm && <Modal title="重命名任务" subtitle="名称会同步更新任务历史和运行详情" onClose={() => busy !== "rename" && setRenameForm(null)}><div className="form-stack"><label>任务名称<input autoFocus maxLength={160} value={renameForm.title} onChange={(event) => setRenameForm((form) => ({ ...form, title: event.target.value }))} onKeyDown={(event) => { if (event.key === "Enter" && !event.nativeEvent.isComposing && busy !== "rename") renameSelectedTask(); }} /></label><div className="modal-actions"><button className="secondary-button" disabled={busy === "rename"} onClick={() => setRenameForm(null)}>[ 取消 ]</button><button className="primary-button" disabled={busy === "rename" || !renameForm.title.trim()} onClick={renameSelectedTask}>[ {busy === "rename" ? "保存中…" : "保存名称"} ]</button></div></div></Modal>}
    {workerModal && <Modal title="远端 Worker" subtitle="仅用于受信任 LAN / VPN；token 保存于本机 0600 文件" onClose={() => setWorkerModal(false)}><div className="form-stack"><label>Worker ID<input value={workerForm.id} onChange={(event) => setWorkerForm((form) => ({ ...form, id: event.target.value }))} placeholder="mac-mini" /></label><label>名称<input value={workerForm.name} onChange={(event) => setWorkerForm((form) => ({ ...form, name: event.target.value }))} placeholder="Build Mac mini" /></label><label>Base URL<input value={workerForm.baseUrl} onChange={(event) => setWorkerForm((form) => ({ ...form, baseUrl: event.target.value }))} placeholder="http://192.168.1.20:9231" /></label><label>Bearer token<input type="password" value={workerForm.token} onChange={(event) => setWorkerForm((form) => ({ ...form, token: event.target.value }))} placeholder="更新时留空则保留原 token" /></label><label className="checkbox-label"><input type="checkbox" checked={workerForm.enabled} onChange={(event) => setWorkerForm((form) => ({ ...form, enabled: event.target.checked }))} />启用调度</label><div className="modal-actions"><button className="secondary-button" onClick={() => setWorkerModal(false)}>[ 取消 ]</button><button className="primary-button" disabled={busy === "worker"} onClick={saveWorker}>[ 保存 Worker ]</button></div></div></Modal>}
    <ConfirmDialog dialog={appDialog} onCancel={() => resolveConfirm(false)} onConfirm={() => resolveConfirm(true)} />
    {notice && <div className={`toast ${notice.type}`}><span>{notice.text}</span></div>}
  </div>;
}

export default App;
