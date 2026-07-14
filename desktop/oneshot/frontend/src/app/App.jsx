import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { CaretDown, CaretRight, Circle } from "@phosphor-icons/react";
import {
  GitBinding,
  RuntimeBinding,
  SettingsBinding,
  TaskRunBinding,
  WorkflowBinding,
  WorkspaceBinding,
  WorkerBinding,
} from "../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";
import SettingsPage, { ConfirmDialog, demoSettings } from "./SettingsPage.jsx";
import { buildRunConversation } from "./runConversation.js";
import { markResumeAccepted, resumeControlState } from "./runControls.js";
import { runtimeSessionEntries } from "./sessionCommands.js";
import { nextWorkflowItemID } from "./workflowIds.js";
import { buildRunRows, mergeRunItems, sortWorkspaces, virtualRunWindow, workspaceResults } from "./listNavigation.js";
import { Action, Kicker, ModeBadge, StatusBadge, TUISelect, Toolbar, ToolbarSpacer } from "../ui/primitives.jsx";

const statusLabel = {
  queued: "排队中",
  ready: "等待运行",
  running: "运行中",
  paused: "等待介入",
  completed: "已完成",
  failed: "失败",
  cancelled: "已终止",
  succeeded: "成功",
  interrupted: "已打断",
};

const terminalTarget = { $done: "完成", $pause: "暂停", $fail: "失败" };
const runStatusOptions = [
  { value: "", label: "全部状态" },
  { value: "queued", label: "排队中" },
  { value: "running", label: "运行中" },
  { value: "paused", label: "等待介入" },
  { value: "completed", label: "已完成" },
  { value: "failed", label: "失败" },
  { value: "cancelled", label: "已终止" },
];

const singleTemplate = {
  id: "my_single_agent",
  name: "我的单 Agent 流程",
  description: "一个 Agent 独立完成任务，需要时交还给我。",
  entryStepId: "execute",
  policy: { maxTransitions: 20, maxConsecutiveFailures: 3, stepTimeoutSeconds: 1800 },
  steps: [
    {
      id: "execute",
      name: "执行",
      runtime: "codex",
      model: "",
      sandbox: "workspace-write",
      rolePrompt: "你是负责独立完成任务的本地 Agent。",
      instruction: "完成任务、验证结果，并按 outcome contract 汇报。",
      transitions: { completed: "$done", need_human: "$pause" },
    },
  ],
};

const loopTemplate = {
  id: "my_implement_review",
  name: "我的实现与审查 Loop",
  description: "实现者完成改动，审查者检查；有问题就自动回到实现。",
  entryStepId: "implement",
  policy: { maxTransitions: 20, maxConsecutiveFailures: 3, stepTimeoutSeconds: 1800 },
  steps: [
    {
      id: "implement",
      name: "实现",
      runtime: "codex",
      model: "",
      sandbox: "workspace-write",
      rolePrompt: "你是负责落地代码和测试的实现者。",
      instruction: "实现目标、运行验证，并把变更交给审查者。",
      transitions: { ready_for_review: "review", need_human: "$pause" },
    },
    {
      id: "review",
      name: "审查",
      runtime: "claude",
      model: "",
      sandbox: "read-only",
      rolePrompt: "你是独立、严格的代码审查者。",
      instruction: "检查需求、实现、风险和测试；有问题要求修改。",
      transitions: { changes_requested: "implement", approved: "$done", need_human: "$pause" },
    },
  ],
};

const dagTemplate = {
  id: "my_parallel_review",
  name: "我的并行审查 DAG",
  description: "安全和测试并行分析，全部完成后汇总落地。",
  mode: "dag",
  entryStepId: "security",
  policy: { maxTransitions: 20, maxConsecutiveFailures: 3, stepTimeoutSeconds: 1800 },
  layout: { nodes: { security: { x: 64, y: 88 }, tests: { x: 64, y: 336 }, synthesis: { x: 412, y: 212 } } },
  steps: [
    { id: "security", name: "安全审查", runtime: "codex", workerId: "local", sandbox: "read-only", dependsOn: [], rolePrompt: "你是安全审查 Agent。", instruction: "独立检查安全风险。", transitions: { completed: "$done", need_human: "$pause" } },
    { id: "tests", name: "测试审查", runtime: "claude", workerId: "local", sandbox: "read-only", dependsOn: [], rolePrompt: "你是测试可靠性审查 Agent。", instruction: "独立检查测试和回归风险。", transitions: { completed: "$done", need_human: "$pause" } },
    { id: "synthesis", name: "汇总落地", runtime: "codex", workerId: "local", sandbox: "workspace-write", dependsOn: ["security", "tests"], rolePrompt: "你负责汇总并落地修改。", instruction: "结合依赖结果实施修改并验证。", transitions: { completed: "$done", need_human: "$pause" } },
  ],
};

const demoNow = new Date().toISOString();
const demoWorkspaces = [
  { id: "oneshot-demo", name: "oneshot", path: "~/Code/openmodu/oneshot", defaultSandbox: "workspace-write", lastOpenedAt: demoNow },
];
const demoRuntimes = [
  { id: "codex", name: "Codex", available: true, version: "codex 0.98.0" },
  { id: "claude", name: "Claude Code", available: true, version: "2.1.4" },
  { id: "modu", name: "Modu Code", available: true, version: "ACP · executable" },
];
const demoWorkflows = [
  { ...singleTemplate, id: "single_agent", name: "单 Agent 完成" },
  { ...loopTemplate, id: "implement_review", name: "实现与审查 Loop" },
  { ...dagTemplate, id: "parallel_review", name: "并行审查 DAG" },
];
const demoWorkers = [{ id: "mac-mini", name: "Mac mini", baseUrl: "http://192.168.1.20:9231", enabled: true, hasToken: true }];
const demoTasks = [
  { id: "task_demo", workspaceId: "oneshot-demo", title: "优化本地 Agent 调度流程", prompt: "梳理并改进工作流执行、暂停与恢复体验。", workflowId: "implement_review", status: "paused", createdAt: demoNow, updatedAt: demoNow },
  { id: "task_queue_demo", workspaceId: "oneshot-demo", title: "补齐任务页 Git 工作流", prompt: "接通状态、Diff、提交信息生成和推送，并验证异常提示。", workflowId: "single_agent", status: "queued", executionMode: "queued", queue: { state: "waiting", enqueuedAt: demoNow, authorized: true }, attachments: [], createdAt: demoNow, updatedAt: demoNow },
];
const demoRun = {
  run: { id: "run_demo", taskId: "task_demo", workflowId: "implement_review", status: "paused", currentStepId: "implement", transitionCount: 2, pauseReason: "workflow_signal", updatedAt: demoNow, history: [
    { fromStepId: "implement", signal: "ready_for_review", target: "review", content: "已完成调度层拆分并补充测试。", at: demoNow },
    { fromStepId: "review", signal: "changes_requested", target: "implement", content: "需要补充 UI 中断后的状态反馈。", at: demoNow },
  ] },
  task: demoTasks[0],
  workspace: demoWorkspaces[0],
  workflow: demoWorkflows[1],
  stepRuns: [
    { id: "step_1", stepId: "implement", attempt: 1, status: "succeeded", signal: "ready_for_review", content: "完成后端应用服务", sessionIdAfter: "019f4b11-codex-demo", startedAt: demoNow, finishedAt: demoNow },
    { id: "step_2", stepId: "review", attempt: 1, status: "succeeded", signal: "changes_requested", content: "要求完善中断反馈", sessionIdAfter: "45db32aa-claude-demo", startedAt: demoNow, finishedAt: demoNow },
  ],
  events: [
    { seq: 1, type: "run.started", stepId: "", at: demoNow },
    { seq: 2, type: "transition.applied", stepId: "implement", at: demoNow },
    { seq: 3, type: "transition.applied", stepId: "review", at: demoNow },
    { seq: 4, type: "run.paused", stepId: "implement", at: demoNow },
  ],
  runtimeEvents: [
    { stepRunId: "step_1", seq: 1, kind: "started", text: "019f4b11-codex-demo", at: demoNow },
    { stepRunId: "step_1", seq: 2, kind: "tool_use", text: "rg -n \"interrupt|pause\" internal desktop", at: demoNow },
    { stepRunId: "step_1", seq: 3, kind: "file_change", text: "internal/usecase/workflows/usecase.go", at: demoNow },
    { stepRunId: "step_1", seq: 4, kind: "message", text: "已完成调度层拆分并补充中断恢复测试。", at: demoNow },
    { stepRunId: "step_2", seq: 1, kind: "tool_use", text: "go test ./...", at: demoNow },
    { stepRunId: "step_2", seq: 2, kind: "message", text: "核心流程正确，但需要补充 UI 中断后的状态反馈。", at: demoNow },
    { stepRunId: "step_2", seq: 3, kind: "result", text: '{"signal":"changes_requested","content":"建议在暂停区域明确展示恢复后的当前步骤。"}', at: demoNow },
  ],
  active: false,
};

function copy(value) { return JSON.parse(JSON.stringify(value)); }
function shortID(value = "") { return value.length > 18 ? `${value.slice(0, 8)}…${value.slice(-5)}` : value; }
function formatTime(value) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}
function errorMessage(error) { return String(error?.message || error || "未知错误").replace(/^Error:\s*/, ""); }
function formatDuration(value = 0) {
  const milliseconds = Math.max(0, Number(value) || 0);
  if (milliseconds < 1000) return `${milliseconds}ms`;
  const seconds = Math.round(milliseconds / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m ${seconds % 60}s`;
}
function formatTokens(value = 0) { return new Intl.NumberFormat("zh-CN").format(Number(value) || 0); }
function fileName(value = "") { return String(value).split(/[\\/]/).pop() || value; }

function StatusPill({ status, active }) {
  return <StatusBadge status={status || "ready"} className="status-pill">{active && <span className="pulse" />}{statusLabel[status] || status}</StatusBadge>;
}

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
  const [inspectorHidden, setInspectorHidden] = useState(false);
  const [editor, setEditor] = useState(null);
  const [validation, setValidation] = useState([]);
  const [workspaceModal, setWorkspaceModal] = useState(false);
	const [taskModal, setTaskModal] = useState(false);
  const [renameForm, setRenameForm] = useState(null);
  const [workspaceForm, setWorkspaceForm] = useState({ path: "", name: "", defaultSandbox: "" });
	const [taskForm, setTaskForm] = useState({ title: "", prompt: "", workflowId: "", executionMode: "immediate", attachmentPaths: [] });
  const [resumeInstruction, setResumeInstruction] = useState("");
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
    setResumeInstruction("");
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
    setResumeInstruction("");
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
      setRunDetail(detail);
      setRunItems((items) => items.map((item) => item.id === runID ? { ...detail.run, task: detail.task } : item));
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

  useEffect(() => {
    if (!selectedRunID || mode !== "wails") return undefined;
    const timer = window.setInterval(async () => {
      await loadRun(selectedRunID, true);
    }, runDetail?.active || runDetail?.run?.status === "running" ? 900 : 2500);
    return () => window.clearInterval(timer);
  }, [loadRun, mode, runDetail?.active, runDetail?.run?.status, selectedRunID]);

	useEffect(() => {
		if (!workspaceID || mode !== "wails") return undefined;
		const timer = window.setInterval(() => {
			loadTasks();
			loadRunList();
		}, runDetail?.active || tasks.some((task) => task.status === "queued" || task.status === "running") ? 1400 : 4500);
		return () => window.clearInterval(timer);
	}, [loadRunList, loadTasks, mode, runDetail?.active, tasks, workspaceID]);

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

  const chooseWorkspace = async () => {
    if (mode === "demo") { setWorkspaceForm((form) => ({ ...form, path: "/Users/demo/Code/my-project", name: "my-project" })); setWorkspaceModal(true); return; }
    try {
      const path = await WorkspaceBinding.ChooseDirectory();
      if (path) { setWorkspaceForm({ path, name: "", defaultSandbox: "" }); setWorkspaceModal(true); }
    } catch (error) { notify("error", errorMessage(error)); }
  };

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

  const toggleWorkspacePinned = async (workspace) => {
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
  };

  const removeWorkspace = async (workspace) => {
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
  };

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

	const submitWorkbenchComposer = async (modeName = "queue") => {
		if (!runDetail?.run?.id) { setTaskModal(true); return; }
		const run = runDetail.run;
		const content = resumeInstruction.trim();
		if (!content && !composerAttachments.length && run.status !== "paused") return;
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
			setResumeInstruction(""); setComposerAttachments([]);
			window.setTimeout(() => loadRun(run.id, true), 180);
			notify("success", modeName === "insert" ? "当前轮次将停止，并优先执行这条指令" : run.status === "running" ? "指令已加入下一轮队列" : "Run 正在恢复");
		} catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
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
        const nextStatus = action === "interrupt" ? "paused" : action === "resume" ? "running" : "cancelled";
        setRunDetail((detail) => ({ ...detail, active: action === "resume", run: { ...detail.run, status: nextStatus, pauseReason: action === "interrupt" ? "interrupted" : "" } }));
        setRunItems((items) => items.map((item) => item.id === selectedRunID ? { ...item, status: nextStatus } : item));
      } else {
        if (action === "interrupt") await TaskRunBinding.InterruptRun(runID);
        if (action === "resume") {
          await TaskRunBinding.ResumeRun(runID, resumeInstruction);
          setResumePendingRunID(runID);
          setRunDetail((detail) => markResumeAccepted(detail, runID));
        }
        if (action === "cancel") await TaskRunBinding.CancelRun(runID);
        window.setTimeout(() => {
          if (selectedRunIDRef.current === runID) loadRun(runID, true);
        }, 250);
      }
      if (action === "resume") setResumeInstruction("");
    } catch (error) {
      if (action === "resume") setResumePendingRunID("");
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

  const allRuns = runItems;
  const visibleWorkspaces = useMemo(() => workspaceResults(workspaces, { selectedID: workspaceID, query: workspaceQuery, expanded: workspaceExpanded }), [workspaceExpanded, workspaceID, workspaceQuery, workspaces]);
  const goView = (next) => { setEditor(null); setView(next); };
  const commandText = view === "settings" ? "settings @ ~/.oneshot" : selectedWorkspace ? `${selectedWorkspace.name} @ ${selectedWorkspace.path}` : "选择一个工作目录";

  if (mode === "loading") return <div className="loading-screen"><div className="brand-mark">1</div><span>正在打开本地工作台…</span></div>;

  return <div className="app-frame">
    <header className="mac-titlebar" aria-label="Oneshot window"><span aria-hidden="true" /><strong>Oneshot</strong><span /></header>
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand"><strong>ONESHOT</strong><span>// personal workspace</span></div>
        <div className="workspace-block">
          <div className="workspace-heading"><div className="sidebar-section-label">cwd <span>{workspaces.length}</span></div><button type="button" className={`workspace-search-toggle ${workspaceSearchOpen ? "active" : ""}`} aria-label="搜索工作目录" title="搜索工作目录" onClick={() => { setWorkspaceSearchOpen((open) => !open); if (workspaceSearchOpen) setWorkspaceQuery(""); }}>[ / ]</button></div>
          {workspaceSearchOpen && <div className="workspace-search"><span>/</span><input autoFocus aria-label="搜索工作目录" value={workspaceQuery} onChange={(event) => setWorkspaceQuery(event.target.value)} placeholder="名称或路径" /><button type="button" aria-label="清空搜索" onClick={() => setWorkspaceQuery("")}>[ x ]</button></div>}
          <div className={`workspace-list ${workspaceExpanded || workspaceQuery ? "expanded" : ""}`}>
            {visibleWorkspaces.map((workspace) => <div className={`workspace-row ${workspace.id === workspaceID ? "active" : ""}`} key={workspace.id}><button className="workspace-item" onClick={() => selectWorkspace(workspace.id)}><span><strong>{workspace.name}</strong><small>{workspace.path}</small></span></button><button type="button" className={`workspace-pin ${workspace.pinned ? "pinned" : ""}`} aria-label={`${workspace.pinned ? "取消置顶" : "置顶"}${workspace.name}`} aria-pressed={Boolean(workspace.pinned)} title={workspace.pinned ? "取消置顶" : "置顶"} onClick={() => toggleWorkspacePinned(workspace)}>{workspace.pinned ? "*" : "."}</button><button type="button" className="workspace-remove" aria-label={`移除${workspace.name}`} title="从列表移除" onClick={() => removeWorkspace(workspace)}>-</button></div>)}
            {!workspaces.length && <div className="sidebar-empty">还没有工作目录</div>}
            {workspaces.length > 0 && !visibleWorkspaces.length && <div className="sidebar-empty">没有匹配的工作目录</div>}
          </div>
          {!workspaceQuery && workspaces.length > 8 && <button type="button" className="workspace-expand" onClick={() => setWorkspaceExpanded((expanded) => !expanded)}>[ {workspaceExpanded ? "收起" : `全部 CWD · ${workspaces.length}`} ]</button>}
          <button className="add-workspace" onClick={chooseWorkspace}>[ + 加入工作目录 ]</button>
        </div>
        <nav className="primary-nav">
          <button className={view === "tasks" && !editor ? "active" : ""} onClick={() => goView("tasks")}><span>&gt;</span><b>任务与运行</b><small>runs</small></button>
          <button className={view === "workflows" || editor ? "active" : ""} onClick={() => goView("workflows")}><span>%</span><b>Workflow</b><small>flows</small></button>
          <button className={view === "settings" && !editor ? "active" : ""} onClick={() => goView("settings")}><span>#</span><b>设置</b><small>prefs</small></button>
        </nav>
        <div className="runtime-panel">
          <div className="sidebar-section-label">runtime</div>
          {runtimes.map((runtime) => <div className="runtime-row" key={runtime.id}><span className={`runtime-dot ${runtime.available ? "online" : "offline"}`} /><strong>{runtime.id}</strong><small>{runtime.available ? String(runtime.version || "ready").replace(`${runtime.id} `, "") : "missing"}</small></div>)}
          <div className="storage-note"><span>数据仅保存在本机</span><code>~/.oneshot/</code></div>
        </div>
      </aside>

      <main className="main-area">
        <div className="command-strip"><span>&gt;</span><strong>{commandText}</strong><span className={`connection ${mode}`}>{mode === "wails" ? "local" : "preview"}</span></div>
        {editor ? <WorkflowEditor editor={editor} setEditor={setEditor} validation={validation} validateEditor={validateEditor} saveWorkflow={saveWorkflow} busy={busy} updateStep={updateStep} updateTransition={updateTransition} removeTransition={removeTransition} runtimes={runtimes} workers={settings.experimental?.remoteWorkersEnabled ? workers : []} defaultSandbox={settings.execution.defaultSandbox} allowFullSandbox={settings.security.allowFullSandbox} onClose={() => setEditor(null)} /> : view === "tasks" ? <TaskWorkbench
          mode={mode}
          workspaceID={workspaceID}
          tasks={tasks}
          runs={allRuns}
          runDetail={runDetail}
          selectedRunID={selectedRunID}
          selectedQueuedTaskID={selectedQueuedTaskID}
          workflows={workflows}
          runtimes={runtimes}
          loading={runLoading}
          total={runTotal}
          hasMore={Boolean(runNextCursor)}
          search={runSearchDraft}
          status={runStatus}
          busy={busy}
          draft={resumeInstruction}
          attachments={composerAttachments}
          onSearch={setRunSearchDraft}
          onStatus={setRunStatus}
          onNewTask={() => setTaskModal(true)}
          onLoadMore={() => loadRunList({ cursor: runNextCursor })}
          onSelectRun={(item) => { setSelectedQueuedTaskID(""); setSelectedRunID(item.id); loadRun(item.id); }}
          onSelectQueued={(task) => { setSelectedRunID(""); setRunDetail(null); setSelectedQueuedTaskID(task.id); }}
          onDraft={setResumeInstruction}
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

function TaskWorkbench({ mode, workspaceID, tasks, runs, runDetail, selectedRunID, selectedQueuedTaskID, workflows, runtimes, loading, total, hasMore, search, status, busy, draft, attachments, onSearch, onStatus, onNewTask, onLoadMore, onSelectRun, onSelectQueued, onDraft, onChooseAttachments, onRemoveAttachment, onSubmit, onInterrupt, onCancel, onRemoveInstruction, onRename, onDelete, notify }) {
  const [inspectorTab, setInspectorTab] = useState("status");
  const scrollRef = useRef(null);
  const pinnedRef = useRef(true);
  const selectedQueuedTask = tasks.find((task) => task.id === selectedQueuedTaskID);
  const selectedTask = runDetail?.task || selectedQueuedTask;
  const queueTasks = tasks.filter((task) => task.status === "queued").sort((left, right) => new Date(left.queue?.enqueuedAt || left.createdAt) - new Date(right.queue?.enqueuedAt || right.createdAt));
  const filteredQueued = queueTasks.filter((task) => (!status || status === "queued") && (!search.trim() || `${task.title}\n${task.prompt}`.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase())));
  const visibleRuns = status === "queued" ? [] : runs;
  const conversation = runDetail ? buildRunConversation(runDetail) : [];
  const pendingInstructions = (runDetail?.instructions || []).filter((instruction) => instruction.status === "pending");
  const conversationSize = conversation.length ? `${conversation.length}:${conversation[conversation.length - 1].items?.length || 0}` : "0";

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
  const workflowName = selectedTask ? workflows.find((workflow) => workflow.id === selectedTask.workflowId)?.name || selectedTask.workflowId : "";
  const canSend = Boolean(draft.trim() || attachments.length);
  const runStatus = runDetail?.run?.status;

  return <div className="task-workbench">
    <aside className="task-history-pane">
      <div className="task-history-head"><div><Kicker>task history</Kicker><strong>任务</strong></div><Action tone="primary" disabled={!workspaceID} onClick={onNewTask}>+ 新建</Action></div>
      <div className="task-history-query"><label><span>/</span><input aria-label="搜索任务" value={search} onChange={(event) => onSearch(event.target.value)} placeholder="标题 / Run / Thread" />{search && <button type="button" onClick={() => onSearch("")}>×</button>}</label><TUISelect ariaLabel="任务状态" value={status} onChange={onStatus} options={runStatusOptions} /></div>
      <div className="task-history-list">
        {filteredQueued.length > 0 && <div className="task-history-group">Workspace 队列 · {filteredQueued.length}</div>}
        {filteredQueued.map((task) => <button type="button" className={`task-history-item ${selectedQueuedTaskID === task.id ? "selected" : ""}`} key={task.id} onClick={() => onSelectQueued(task)}><span className="task-history-status"><StatusPill status="queued" /><small>#{queueTasks.findIndex((item) => item.id === task.id) + 1}</small></span><strong>{task.title}</strong><small>{workflowNameFor(workflows, task.workflowId)} · {formatTime(task.updatedAt)}</small></button>)}
        {visibleRuns.length > 0 && <div className="task-history-group">运行记录 · {total}</div>}
        {visibleRuns.map((run) => <button type="button" className={`task-history-item ${selectedRunID === run.id ? "selected" : ""}`} key={run.id} onClick={() => onSelectRun(run)}><span className="task-history-status"><StatusPill status={run.status} active={runDetail?.run?.id === run.id && runDetail.active} /></span><strong>{run.task?.title || run.id}</strong><small>{workflowNameFor(workflows, run.workflowId)} · {formatTime(run.updatedAt || run.startedAt)}</small></button>)}
        {!filteredQueued.length && !visibleRuns.length && !loading && <div className="task-history-empty">{search || status ? "没有匹配的任务" : "还没有任务，先新建一个。"}</div>}
        {(loading || hasMore) && <button type="button" className="task-history-more" disabled={loading} onClick={onLoadMore}>{loading ? "正在读取…" : `加载更多 · ${visibleRuns.length}/${total}`}</button>}
      </div>
    </aside>

    <section className="conversation-workspace">
      {selectedTask ? <>
        <header className="conversation-workspace-head"><div><div className="conversation-title-row"><h2>{selectedTask.title}</h2><StatusPill status={runStatus || selectedTask.status} active={runDetail?.active} /></div><p>{workflowName}{runDetail?.run?.id ? ` · ${shortID(runDetail.run.id)}` : " · Workspace FIFO"}</p></div><div className="conversation-head-actions"><button type="button" onClick={onRename}>[ 重命名 ]</button><button type="button" className="danger" onClick={onDelete}>[ 删除 ]</button></div></header>
        <div className="conversation-scroll" ref={scrollRef} onScroll={handleConversationScroll}>
          {selectedQueuedTask ? <QueuedTaskView task={selectedQueuedTask} position={queueTasks.findIndex((task) => task.id === selectedQueuedTask.id) + 1} /> : <><ConversationTimeline items={conversation} active={runDetail?.active} />{!conversation.length && <div className="workbench-empty"><p>这次运行还没有生成消息。</p></div>}</>}
        </div>
        {runDetail && <div className="workbench-composer">
          {pendingInstructions.length > 0 && <div className="instruction-queue"><span>下一轮指令 · {pendingInstructions.length}</span>{pendingInstructions.map((instruction, index) => <div className={`instruction-item ${instruction.priority ? "priority" : ""}`} key={instruction.id}><b>{instruction.priority ? "优先" : `#${index + 1}`}</b><span>{instruction.content || `${instruction.attachments?.length || 0} 个附件`}</span><button type="button" onClick={() => onRemoveInstruction(instruction.id)}>移除</button></div>)}</div>}
          {attachments.length > 0 && <div className="composer-attachments">{attachments.map((path) => <span className="attachment-chip" key={path} title={path}>{fileName(path)}<button type="button" onClick={() => onRemoveAttachment(path)}>×</button></span>)}</div>}
          <textarea value={draft} disabled={!['running', 'paused'].includes(runStatus)} onChange={(event) => onDraft(event.target.value)} onKeyDown={(event) => { if ((event.metaKey || event.ctrlKey) && event.key === "Enter" && (canSend || runStatus === "paused")) onSubmit("queue"); }} placeholder={runStatus === "running" ? "补充下一轮指令，或打断当前轮次优先插入…" : runStatus === "paused" ? "补充指令并恢复运行…" : "这次运行已经结束"} />
          <div className="workbench-composer-actions"><button type="button" className="attachment-action" disabled={!['running', 'paused'].includes(runStatus)} onClick={onChooseAttachments}>[ + 附件 ]</button><span>⌘ Enter 提交</span><div>{runStatus === "running" && <><Action disabled={busy === "interrupt"} onClick={onInterrupt}>仅暂停</Action><Action disabled={!canSend || busy === "queue"} onClick={() => onSubmit("queue")}>排到下一轮</Action><Action tone="primary" disabled={!canSend || busy === "insert"} onClick={() => onSubmit("insert")}>打断并插入</Action></>}{runStatus === "paused" && <><Action tone="danger" disabled={busy === "cancel" || runDetail.active} onClick={onCancel}>终止</Action><Action tone="primary" disabled={busy === "queue" || runDetail.active} onClick={() => onSubmit("queue")}>恢复运行</Action></>}</div></div>
        </div>}
      </> : <div className="workbench-welcome"><Kicker>local agent workspace</Kicker><h2>把任务、对话和项目状态放在一个页面</h2><p>选择左侧历史任务，或新建任务立即运行 / 加入 FIFO 队列。</p><Action tone="primary" disabled={!workspaceID} onClick={onNewTask}>新建任务</Action></div>}
    </section>

    <aside className="workbench-inspector">
      <div className="workbench-tabs">{[["status", "状态"], ["git", "Git"], ["events", "事件"]].map(([value, label]) => <button type="button" className={inspectorTab === value ? "active" : ""} key={value} onClick={() => setInspectorTab(value)}>{label}</button>)}</div>
      <div className="workbench-inspector-body">{inspectorTab === "status" ? <StatusInspector detail={runDetail} queuedTask={selectedQueuedTask} queuePosition={selectedQueuedTask ? queueTasks.findIndex((task) => task.id === selectedQueuedTask.id) + 1 : 0} /> : inspectorTab === "git" ? <GitInspector mode={mode} workspaceID={workspaceID} runtimes={runtimes} notify={notify} /> : <EventInspector detail={runDetail} />}</div>
    </aside>
  </div>;
}

function workflowNameFor(workflows, workflowID) { return workflows.find((workflow) => workflow.id === workflowID)?.name || workflowID; }

function QueuedTaskView({ task, position }) {
  return <div className="queued-task-view"><div className="queue-orbit"><span>{position}</span></div><Kicker>workspace fifo</Kicker><h3>正在等待前面的任务</h3><p>同一个 Workspace 一次只激活一个排队任务；前序任务完成或终止后会自动启动。</p><dl><div><dt>任务目标</dt><dd>{task.prompt}</dd></div><div><dt>入队时间</dt><dd>{formatTime(task.queue?.enqueuedAt || task.createdAt)}</dd></div><div><dt>附件</dt><dd>{task.attachments?.length || 0} 个</dd></div></dl></div>;
}

function StatusInspector({ detail, queuedTask, queuePosition }) {
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

function InspectorRow({ label, value }) { return <div className="inspector-row"><span>{label}</span><strong>{value || "—"}</strong></div>; }

function GitInspector({ mode, workspaceID, runtimes, notify }) {
  const [snapshot, setSnapshot] = useState(null);
  const [diff, setDiff] = useState("");
  const [staged, setStaged] = useState(false);
  const [message, setMessage] = useState("");
  const [runtime, setRuntime] = useState(runtimes.find((item) => item.available)?.id || "codex");
  const [push, setPush] = useState(false);
  const [gitBusy, setGitBusy] = useState("");
  const load = useCallback(async (nextStaged = staged) => {
    if (!workspaceID) return;
    try {
      if (mode === "demo") { setSnapshot({ isRepo: true, branch: "feature/workbench", head: "019f5ed", ahead: 0, behind: 0, files: [{ path: "src/app/App.jsx", index: " ", worktree: "M" }], diffStat: "1 file changed" }); setDiff("diff --git a/src/app/App.jsx b/src/app/App.jsx\n+三栏任务工作台"); return; }
      const [statusValue, diffValue] = await Promise.all([GitBinding.Status(workspaceID), GitBinding.Diff(workspaceID, nextStaged)]);
      setSnapshot(statusValue); setDiff(diffValue || "");
    } catch (error) { notify("error", errorMessage(error)); }
  }, [mode, notify, staged, workspaceID]);
  useEffect(() => { load(staged); }, [load, staged]);
  const stageAll = async () => { setGitBusy("stage"); try { if (mode !== "demo") await GitBinding.StageAll(workspaceID); setStaged(true); await load(true); notify("success", "所有变更已暂存"); } catch (error) { notify("error", errorMessage(error)); } finally { setGitBusy(""); } };
  const generate = async () => { setGitBusy("generate"); try { const value = mode === "demo" ? "feat: add task workbench" : await GitBinding.GenerateCommitMessage(workspaceID, runtime); setMessage(value); } catch (error) { notify("error", errorMessage(error)); } finally { setGitBusy(""); } };
  const commit = async () => { if (!message.trim()) { notify("error", "请填写提交信息"); return; } setGitBusy("commit"); try { const result = mode === "demo" ? { commitHash: "019f5ed", pushed: push } : await GitBinding.CommitAndPush({ workspaceId: workspaceID, message: message.trim(), remote: "", push }); notify("success", result.pushed ? `已提交并推送 ${shortID(result.commitHash)}` : `已提交 ${shortID(result.commitHash)}`); setMessage(""); setStaged(false); await load(false); } catch (error) { notify("error", errorMessage(error)); } finally { setGitBusy(""); } };
  if (snapshot && !snapshot.isRepo) return <p className="inspector-placeholder">当前 Workspace 不是 Git 仓库。</p>;
  return <div className="git-inspector"><div className="git-head"><div><Kicker>repository</Kicker><strong>{snapshot?.branch || "读取中…"}</strong><small>{snapshot?.head || ""}</small></div><button type="button" onClick={() => load(staged)}>[ 刷新 ]</button></div><div className="git-sync"><span>↑ {snapshot?.ahead || 0}</span><span>↓ {snapshot?.behind || 0}</span><span>{snapshot?.files?.length || 0} files</span></div><div className="git-files">{snapshot?.files?.map((file) => <div key={file.path}><b>{`${file.index || " "}${file.worktree || " "}`}</b><span title={file.path}>{file.path}</span></div>)}{snapshot && !snapshot.files?.length && <p>工作区干净。</p>}</div><div className="git-diff-tabs"><button type="button" className={!staged ? "active" : ""} onClick={() => setStaged(false)}>未暂存</button><button type="button" className={staged ? "active" : ""} onClick={() => setStaged(true)}>已暂存</button><button type="button" disabled={gitBusy === "stage" || !snapshot?.files?.length} onClick={stageAll}>全部暂存</button></div><pre className="git-diff">{diff || "没有可显示的 Diff。"}</pre><div className="commit-box"><div><TUISelect ariaLabel="生成提交信息的 Runtime" value={runtime} onChange={setRuntime} options={runtimes.filter((item) => item.available).map((item) => ({ value: item.id, label: item.name || item.id }))} /><button type="button" disabled={gitBusy === "generate" || !snapshot?.files?.length} onClick={generate}>{gitBusy === "generate" ? "生成中…" : "生成提交信息"}</button></div><textarea value={message} onChange={(event) => setMessage(event.target.value)} placeholder="feat: describe the change" /><label><input type="checkbox" checked={push} onChange={(event) => setPush(event.target.checked)} /> commit 后推送当前分支</label><Action tone="primary" disabled={gitBusy === "commit" || !message.trim()} onClick={commit}>{gitBusy === "commit" ? "提交中…" : push ? "Commit & Push" : "Commit"}</Action></div></div>;
}

function EventInspector({ detail }) {
  if (!detail) return <p className="inspector-placeholder">选择运行后显示过程事件。</p>;
  return <div className="event-inspector">{[...(detail.events || [])].reverse().map((event) => <article key={`${event.seq}-${event.type}`}><div><b>{event.seq}</b><strong>{event.type}</strong><time>{formatTime(event.at)}</time></div>{event.stepId && <span>{event.stepId}</span>}{event.payload && event.payload !== "{}" && <pre>{prettyPayload(event.payload)}</pre>}</article>)}{!detail.events?.length && <p className="inspector-placeholder">还没有过程事件。</p>}</div>;
}

function prettyPayload(value) {
  try { return JSON.stringify(JSON.parse(value), null, 2); } catch { return String(value); }
}

function VirtualRunList({ items, selectedRunID, workflows, loading, hasMore, onLoadMore, onSelect, emptyFiltered }) {
  const listRef = useRef(null);
  const loadingMoreRef = useRef(false);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(480);
  const rows = useMemo(() => buildRunRows(items), [items]);
  const virtualWindow = useMemo(() => virtualRunWindow(rows, scrollTop, viewportHeight), [rows, scrollTop, viewportHeight]);

  useEffect(() => {
    const element = listRef.current;
    if (!element) return undefined;
    const update = () => setViewportHeight(element.clientHeight || 480);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (items.length) return;
    if (listRef.current) listRef.current.scrollTop = 0;
    setScrollTop(0);
  }, [items.length]);

  useEffect(() => {
    if (!loading) loadingMoreRef.current = false;
  }, [loading]);

  const handleScroll = (event) => {
    const element = event.currentTarget;
    setScrollTop(element.scrollTop);
    if (hasMore && !loading && !loadingMoreRef.current && element.scrollHeight - element.scrollTop - element.clientHeight < 180) {
      loadingMoreRef.current = true;
      Promise.resolve(onLoadMore()).finally(() => { loadingMoreRef.current = false; });
    }
  };

  if (!items.length && !loading) return <div className="run-list empty"><div className="empty-state"><h3>{emptyFiltered ? "没有匹配的运行记录" : "还没有运行记录"}</h3><p>{emptyFiltered ? "调整搜索词或状态筛选，右侧详情会保持为空。" : "写下任务目标并选择 Workflow，Oneshot 会在后台调度本地 Agent。"}</p></div></div>;

  return <div className="run-list virtual" ref={listRef} onScroll={handleScroll}><div className="run-list-spacer" style={{ height: virtualWindow.totalHeight }}>
    {virtualWindow.visible.map((row) => row.type === "group" ? <div className="run-date-group" key={row.key} style={{ transform: `translateY(${row.top}px)`, height: row.height }}>{row.label}</div> : <button key={row.key} style={{ transform: `translateY(${row.top}px)`, height: row.height }} className={`run-card ${selectedRunID === row.item.id ? "selected" : ""}`} onClick={() => onSelect(row.item)}><StatusPill status={row.item.status} /><h3>{row.item.task.title}</h3><p>{workflows.find((workflow) => workflow.id === row.item.workflowId)?.name || row.item.workflowId} · {formatTime(row.item.updatedAt)}</p></button>)}
  </div></div>;
}

function RunInspector({ detail, busy, resumePending, resumeInstruction, setResumeInstruction, runAction, notify, onCollapse }) {
  const { run, workflow, stepRuns = [], runtimeEvents = [] } = detail;
  const dagNodes = run.nodes || {};
  const currentNodeID = workflow.mode === "dag" ? Object.values(dagNodes).find((node) => node.status === "running" || node.status === "paused")?.stepId : run.currentStepId;
  const currentStep = workflow.steps?.find((step) => step.id === currentNodeID) || workflow.steps?.[0];
  const conversation = buildRunConversation(detail);
  const sessions = runtimeSessionEntries(detail);
  const resumeControl = resumeControlState(detail, busy, resumePending);
  const scrollRef = useRef(null);
  // Pinned-to-latest: default to the newest message; release the pin only
  // while the user has scrolled away from the bottom.
  const pinnedRef = useRef(true);
  const conversationSize = conversation.length ? `${conversation.length}:${conversation[conversation.length - 1].items?.length || 0}` : "0";
  useEffect(() => {
    pinnedRef.current = true;
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [run.id]);
  useEffect(() => {
    const el = scrollRef.current;
    if (el && pinnedRef.current) el.scrollTop = el.scrollHeight;
  }, [conversationSize, run.status]);
  const trackScrollPin = () => {
    const el = scrollRef.current;
    if (el) pinnedRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  };
  const visiblePauseReason = run.pauseReason && run.pauseReason !== "workflow_signal" ? run.pauseReason : "";
  const copySessionValue = async (value, label) => {
    try {
      await copyText(value);
      notify("success", `${label}已复制`);
    } catch {
      notify("error", "复制失败，请手动选择文本");
    }
  };
  return <>
    <div className="inspector-scroll" ref={scrollRef} onScroll={trackScrollPin}>
    <section className="run-summary">
      <div className="run-summary-head"><div><h2>{detail.task.title}</h2><StatusPill status={run.status} active={detail.active} /></div><div className="run-summary-stats"><span>{run.transitionCount || 0} / {workflow.policy?.maxTransitions || 20}</span><time>{formatTime(run.updatedAt || run.startedAt)}</time><button type="button" className="text-button inspector-collapse" aria-label="收起运行详情" title="收起运行详情" onClick={onCollapse}>[ » ]</button></div></div>
      <details className="run-summary-details"><summary><span className="run-summary-current">当前：<strong>{currentStep?.name || run.currentStepId}</strong><i>·</i><span className={`runtime-name ${currentStep?.runtime}`}>{currentStep?.runtime || "—"}</span></span><span className="run-summary-recovery">会话与恢复 <CaretRight className="details-caret closed" weight="bold" /><CaretDown className="details-caret opened" weight="bold" /></span></summary><div className="run-summary-detail-body">
        <div className="run-summary-flow">{(workflow.steps || []).map((step, index) => { const nodeStatus = dagNodes[step.id]?.status; return <div className={`run-summary-step ${step.id === currentNodeID ? "current" : ""}`} key={step.id}><b>{String(index + 1).padStart(2, "0")}</b><span><strong>{step.name}</strong><small>{step.runtime} · {step.workerId || "local"} · {nodeStatus || step.sandbox || "workspace-write"}</small></span>{nodeStatus && <em>{nodeStatus}</em>}</div>; })}</div>
        <div className="run-summary-id"><span>RUN ID</span><code>{run.id}</code></div>
        {sessions.map((session) => <div className="run-summary-session" key={session.stepID}><div><span className={`runtime-badge ${session.runtime}`}>{session.runtime}</span><strong>{session.stepName}</strong></div><div className="session-value"><span>{session.idLabel}</span><code>{session.sessionID}</code><button type="button" onClick={() => copySessionValue(session.sessionID, "会话 ID")}>[ 复制 ID ]</button></div>{session.command && <div className="session-value command"><span>resume</span><code>{session.command}</code><button type="button" onClick={() => copySessionValue(session.command, "恢复命令")}>[ 复制命令 ]</button></div>}</div>)}
      </div></details>
      {(visiblePauseReason || detail.lastError) && <div className="run-summary-notice">{visiblePauseReason && <span>暂停：{visiblePauseReason}</span>}{detail.lastError && <span className="error-copy">{detail.lastError}</span>}</div>}
    </section>
    <ConversationTimeline items={conversation} active={detail.active} />
    </div>
    {["running", "paused"].includes(run.status) && <div className="run-controls">
      <textarea value={resumeInstruction} disabled={run.status === "running" || resumeControl.disabled} onChange={(event) => setResumeInstruction(event.target.value)} onKeyDown={(event) => { if ((event.metaKey || event.ctrlKey) && event.key === "Enter" && run.status === "paused" && !resumeControl.disabled) runAction("resume"); }} placeholder={run.status === "running" ? "运行中，暂停后可补充指令…" : "补充指令（可选），恢复后注入当前步骤…"} />
      <div>
        {run.status === "running" && <Action tone="danger" disabled={busy === "interrupt"} onClick={() => runAction("interrupt")}>打断并暂停</Action>}
        {run.status === "paused" && <><Action tone="danger" disabled={busy === "cancel" || detail.active} onClick={() => runAction("cancel")}>终止</Action><Action tone="primary" disabled={resumeControl.disabled} aria-live="polite" onClick={() => runAction("resume")}>{resumeControl.label}</Action></>}
      </div>
    </div>}
  </>;
}

function ConversationTimeline({ items, active }) {
  const rounds = items.filter((item) => item.type === "round");
  // Consecutive events often share the same second; repeating the identical
  // timestamp on every row is noise, so only the first of a same-second run
  // shows it.
  let lastTimeLabel = "";
  const timeLabel = (at) => {
    const label = formatTime(at);
    if (!label || label === lastTimeLabel) return "";
    lastTimeLabel = label;
    return label;
  };
  return <div className="conversation-section">
    <div className="conversation-list">
      {items.map((item) => item.type === "user" ? <div className="conversation-user" key={item.id}>
        <div className="conversation-speaker"><span className="conversation-identity"><Circle className="conversation-event-dot user" weight="fill" aria-label="用户消息" /></span><span className="conversation-message-meta"><time>{timeLabel(item.at)}</time></span></div>
        <p>{item.text}</p>
      </div> : <article className="conversation-round" key={item.id}>
        <div className="conversation-round-body">{item.items.map((entry, index) => entry.type === "message" ? <div className={`conversation-agent ${entry.tone}`} key={`message-${index}`}><div className="conversation-speaker"><span className="conversation-identity"><Circle className="conversation-event-dot agent" weight="fill" aria-label="Agent 消息" /><strong>{item.runtime}</strong></span><span className="conversation-message-meta"><span className="conversation-round-index">第 {item.round} 轮</span><time>{timeLabel(entry.at || item.finishedAt || item.startedAt)}</time></span></div><p>{entry.text}</p></div> : <ToolTimelineItem key={`tool-${index}`} entry={entry} time={timeLabel(entry.at)} running={active && item === rounds[rounds.length - 1] && index === item.items.length - 1 && entry.kind === "tool_use"} failed={item.status === "failed"} />)}</div>
      </article>)}
      {!items.length && <p className="muted-copy">Agent 消息会按执行轮次显示在这里。</p>}
    </div>
  </div>;
}

function ToolTimelineItem({ entry, running, failed, time }) {
  const labels = { tool_use: "TOOL USE", tool_result: "RESULT", file_change: "FILE CHANGE", reasoning: "PROCESS" };
  const state = failed ? "失败" : running ? "执行中" : entry.kind === "reasoning" ? "过程" : "完成";
  return <details className={`conversation-tool kind-${entry.kind} ${running ? "running" : ""} ${failed ? "failed" : ""}`}>
    <summary aria-label={`${labels[entry.kind] || entry.kind}: ${entry.title}`}><span className="conversation-tool-summary"><span className="conversation-tool-caret"><CaretRight className="closed" weight="bold" /><CaretDown className="opened" weight="bold" /></span><strong title={entry.text}>{entry.title}</strong><span className="conversation-tool-state">{running && <span className="pulse" />}{state}</span><time>{time ?? formatTime(entry.at)}</time></span></summary>
    <div className="conversation-tool-body"><div><span>{entry.kind === "file_change" ? "PATH" : entry.kind === "reasoning" ? "PROCESS" : "COMMAND"}</span><pre>{entry.text}</pre></div>{entry.details.map((detail, index) => <div key={`${detail.kind}-${index}`}><span>{labels[detail.kind] || detail.kind}</span><pre>{detail.text}</pre></div>)}</div>
  </details>;
}

async function copyText(value) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value);
      return;
    } catch {
      // WebView and local preview permissions vary; fall back to a selected
      // temporary textarea when the modern API is present but denied.
    }
  }
  const input = document.createElement("textarea");
  input.value = value;
  input.setAttribute("readonly", "");
  input.style.position = "fixed";
  input.style.opacity = "0";
  document.body.appendChild(input);
  input.select();
  const copied = document.execCommand("copy");
  input.remove();
  if (!copied) throw new Error("clipboard unavailable");
}

function WorkflowLibrary({ workflows, runtimes, openEditor }) {
  const path = (workflow) => {
    if (workflow.mode === "dag") {
      const roots = (workflow.steps || []).filter((step) => !(step.dependsOn || []).length).map((step) => step.name);
      const joins = (workflow.steps || []).filter((step) => (step.dependsOn || []).length).map((step) => step.name);
      return `${roots.join(" ∥ ")}${joins.length ? ` → ${joins.join(" → ")}` : ""}`;
    }
    return `${(workflow.steps || []).map((step) => step.name).join(" → ")}${workflow.steps?.length > 1 ? " ↺" : " → $done"}`;
  };
  return <div className="library-page">
    <div className="library-hero"><div><Kicker>loops &amp; parallel dag</Kicker><h2>Workflow</h2></div><div className="hero-actions"><Action onClick={() => openEditor(loopTemplate)}>+ loop</Action><Action tone="primary" onClick={() => openEditor(dagTemplate)}>+ 并行 dag</Action></div></div>
    <div className="workflow-grid">{workflows.map((workflow) => <button className="workflow-card" key={workflow.id} onClick={() => openEditor(workflow)}><ModeBadge mode={workflow.mode || "serial"} /><strong>{workflow.name}</strong><span className="workflow-path">{path(workflow)}</span><small>{workflow.steps?.length || 0} {workflow.mode === "dag" ? "nodes · all join" : `${workflow.steps?.length === 1 ? "step" : "steps"} · ${workflow.policy?.maxTransitions || 20} max`}</small><b>[ edit ]</b></button>)}</div>
    <div className="runtime-callout">Workflow 不会自动替换缺失的 runtime，以免破坏角色语义。{runtimes.some((runtime) => !runtime.available) && <strong>存在缺失 Runtime</strong>}</div>
  </div>;
}

function WorkerPage({ workers, health, checkWorker, deleteWorker, openWorker }) {
  return <div className="library-page worker-page"><div className="library-hero"><div><span className="kicker">TRUSTED LAN / VPN</span><h2>把 DAG 节点派到另一台机器</h2><p>远端机器需要相同 Workspace ID 的本地 clone。Oneshot 只传 prompt、outcome 和事件，不同步项目文件。</p></div><Action tone="primary" onClick={() => openWorker(null)}>注册 Worker</Action></div><div className="worker-grid">{workers.map((worker) => <article className="worker-card panel" key={worker.id}><div className="worker-card-head"><div className="worker-machine"><span><strong>{worker.name}</strong><small>{worker.id}</small></span></div><StatusBadge status={worker.enabled ? "completed" : "cancelled"} className="status-pill">{worker.enabled ? "Enabled" : "Disabled"}</StatusBadge></div><code>{worker.baseUrl}</code><div className="worker-health">{health[worker.id]?.checking ? "正在检查…" : health[worker.id]?.ok ? <>{Object.entries(health[worker.id].runtimes || {}).map(([runtime, ok]) => <span key={runtime} className={ok ? "ok" : "missing"}><i />{runtime}</span>)}</> : health[worker.id]?.error || "尚未检查连接"}</div><div className="worker-actions"><Action onClick={() => checkWorker(worker)}>健康检查</Action><Action onClick={() => openWorker(worker)}>编辑</Action><Action tone="danger" onClick={() => deleteWorker(worker.id)}>删除</Action></div></article>)}{!workers.length && <div className="empty-state"><h3>还没有远端 Worker</h3><p>先在另一台机器启动 oneshot-worker，再在这里注册地址与共享 token。</p></div>}</div><div className="worker-command panel"><span className="kicker">REMOTE COMMAND</span><code>ONESHOT_WORKER_TOKEN=... oneshot-worker --listen 0.0.0.0:9231 --id mac-mini --workspace workspace-id=/path/to/clone</code><p>建议通过 Tailscale / WireGuard 地址连接，不要把首版 HTTP worker 直接暴露到公网。</p></div></div>;
}

function Modal({ title, subtitle, onClose, children, wide = false }) {
  if (wide) return <section className="workflow-editor-surface legacy-wide-editor"><Toolbar className="modal-header editor-toolbar"><Action onClick={onClose}>&lt; 返回</Action><div><h2>{title}</h2><p>{subtitle}</p></div></Toolbar>{children}</section>;
  return <div className="modal-backdrop"><div className="modal"><div className="modal-header"><div><h2>{title}</h2><p>{subtitle}</p></div><button className="text-button" onClick={onClose}>[ 关闭 ]</button></div>{children}</div></div>;
}

function WorkflowEditor({ editor, setEditor, validation, validateEditor, saveWorkflow, busy, updateStep, updateTransition, removeTransition, runtimes, workers, defaultSandbox, allowFullSandbox, onClose }) {
  if (editor.mode === "dag") return <DAGWorkflowEditor editor={editor} setEditor={setEditor} validation={validation} validateEditor={validateEditor} saveWorkflow={saveWorkflow} busy={busy} runtimes={runtimes} workers={workers} defaultSandbox={defaultSandbox} allowFullSandbox={allowFullSandbox} onClose={onClose} />;
  const addStep = () => setEditor((current) => {
    const id = nextWorkflowItemID("step", current.steps);
    return { ...current, steps: [...current.steps, { id, name: "新步骤", runtime: "codex", model: "", sandbox: defaultSandbox, rolePrompt: "你在这个流程中的角色。", instruction: "描述这个步骤要完成的事情。", transitions: { completed: "$done" } }] };
  });
  const removeStep = (index) => setEditor((current) => ({ ...current, steps: current.steps.filter((_, itemIndex) => itemIndex !== index) }));
  return <section className="workflow-editor-surface">
    <Toolbar className="editor-toolbar"><Action onClick={onClose}>&lt; 返回</Action><strong>{editor.name}</strong><code>{editor.id} · serial</code><ToolbarSpacer /><Action tone="cyan" onClick={validateEditor}>校验</Action><Action tone="primary" disabled={busy === "workflow"} onClick={saveWorkflow}>{busy === "workflow" ? "保存中" : "保存"}</Action></Toolbar>
    <div className="editor-layout"><div className="editor-main"><div className="step-editor-list">{editor.steps.map((step, stepIndex) => <section className="step-editor" key={`${step.id}-${stepIndex}`}>
      <div className="step-editor-head"><b>{String(stepIndex + 1).padStart(2, "0")}</b><input aria-label="步骤名称" value={step.name} onChange={(event) => updateStep(stepIndex, "name", event.target.value)} /><TUISelect ariaLabel="Runtime" value={step.runtime} onChange={(runtime) => updateStep(stepIndex, "runtime", runtime)} options={runtimes.map((runtime) => ({ value: runtime.id, label: runtime.id, meta: runtime.available ? "" : "missing" }))} /><TUISelect ariaLabel="Sandbox" value={step.sandbox || "workspace-write"} onChange={(sandbox) => updateStep(stepIndex, "sandbox", sandbox)} options={[{ value: "read-only", label: "read-only" }, { value: "workspace-write", label: "workspace-write" }, { value: "full", label: "full-access", disabled: !allowFullSandbox && step.sandbox !== "full" }]} /><button className="text-button danger" disabled={editor.steps.length <= 1} onClick={() => removeStep(stepIndex)}>[ 删除步骤 ]</button></div>
      <div className="step-editor-grid"><label>角色提示<textarea value={step.rolePrompt} onChange={(event) => updateStep(stepIndex, "rolePrompt", event.target.value)} /></label><label>步骤指令<textarea value={step.instruction} onChange={(event) => updateStep(stepIndex, "instruction", event.target.value)} /></label></div>
      <div className="transition-editor"><span>signals → targets</span>{Object.entries(step.transitions || {}).map(([signal, target]) => <div className="transition-row" key={signal}><input value={signal} onChange={(event) => updateTransition(stepIndex, signal, event.target.value, target)} /><span>→</span><input value={target} onChange={(event) => updateTransition(stepIndex, signal, signal, event.target.value)} /><button onClick={() => removeTransition(stepIndex, signal)}>× 删除</button></div>)}<button className="text-button" onClick={() => updateTransition(stepIndex, `signal_${Object.keys(step.transitions || {}).length + 1}`, `signal_${Object.keys(step.transitions || {}).length + 1}`, "$done")}>[ + signal ]</button></div>
    </section>)}</div><button className="text-button add-step" onClick={addStep}>[ + 添加步骤 ]</button></div>
    <aside className="editor-side"><span className="kicker">flow preview</span><pre className="flow-ascii">{editor.steps.map((step, index) => `${index ? "│\n" : ""}┌─ ${index + 1} ${step.name}      ${step.runtime}\n${Object.entries(step.transitions || {}).map(([signal, target]) => `│  ${signal} → ${target}`).join("\n")}`).join("\n")}</pre><div className="policy-box"><span>policy</span><p>max transitions <b>{editor.policy?.maxTransitions || 20}</b></p><p>连续失败上限 <b>{editor.policy?.maxConsecutiveFailures || 3}</b></p><p>单步超时 <b>{editor.policy?.stepTimeoutSeconds || 1800}s</b></p></div><div className={`validation-box ${validation.length ? "has-errors" : "valid"}`}><div><strong>validation</strong><b>{validation.length ? `${validation.length} errors` : "ok"}</b></div>{validation.map((issue, index) => <p key={`${issue.path}-${index}`}><code>{issue.path}</code>{issue.message}</p>)}{!validation.length && <p>保存时会检查入口、可达性、完成路径和 policy 边界。</p>}</div></aside></div>
  </section>;
}

function DAGWorkflowEditor({ editor, setEditor, validation, validateEditor, saveWorkflow, busy, runtimes, workers, defaultSandbox, allowFullSandbox, onClose }) {
  const [selectedID, setSelectedID] = useState(editor.steps[0]?.id || "");
  const [connectFrom, setConnectFrom] = useState("");
  const drag = useRef(null);
  const canvas = useRef(null);
  const selectedIndex = editor.steps.findIndex((step) => step.id === selectedID);
  const selected = editor.steps[selectedIndex];
  const positions = editor.layout?.nodes || {};
  const workerOptions = [{ id: "local", name: "Local" }, ...workers.filter((worker) => worker.enabled)];

  const setStep = (field, value) => setEditor((current) => ({ ...current, steps: current.steps.map((step) => step.id === selectedID ? { ...step, [field]: value } : step) }));
  const renameStep = (nextID) => setEditor((current) => {
    const oldID = selectedID;
    const nodes = { ...(current.layout?.nodes || {}) }; nodes[nextID] = nodes[oldID] || { x: 80, y: 80 }; delete nodes[oldID];
    const steps = current.steps.map((step) => ({ ...step, id: step.id === oldID ? nextID : step.id, dependsOn: (step.dependsOn || []).map((id) => id === oldID ? nextID : id) }));
    return { ...current, entryStepId: current.entryStepId === oldID ? nextID : current.entryStepId, steps, layout: { nodes } };
  });
  const addNode = () => {
    const id = nextWorkflowItemID("node", editor.steps);
    setEditor((current) => ({ ...current, steps: [...current.steps, { id, name: "新节点", runtime: "codex", workerId: "local", sandbox: defaultSandbox, dependsOn: [], rolePrompt: "你在 DAG 中的角色。", instruction: "描述这个节点的任务。", transitions: { completed: "$done", need_human: "$pause" } }], layout: { nodes: { ...(current.layout?.nodes || {}), [id]: { x: 120 + current.steps.length * 34, y: 110 + current.steps.length * 34 } } } }));
    setSelectedID(id);
  };
  const deleteNode = () => {
    if (!selected || editor.steps.length <= 1) return;
    const remaining = editor.steps.filter((step) => step.id !== selected.id).map((step) => ({ ...step, dependsOn: (step.dependsOn || []).filter((id) => id !== selected.id) }));
    const nodes = { ...positions }; delete nodes[selected.id];
    setEditor({ ...editor, steps: remaining, entryStepId: editor.entryStepId === selected.id ? remaining[0].id : editor.entryStepId, layout: { nodes } });
    setSelectedID(remaining[0].id);
  };
  const connect = (targetID) => {
    if (!connectFrom) { setConnectFrom(targetID); return; }
    if (connectFrom !== targetID) setEditor((current) => ({ ...current, steps: current.steps.map((step) => step.id === targetID ? { ...step, dependsOn: [...new Set([...(step.dependsOn || []), connectFrom])] } : step) }));
    setConnectFrom("");
  };
  const deleteEdge = (source, target) => setEditor((current) => ({ ...current, steps: current.steps.map((step) => step.id === target ? { ...step, dependsOn: (step.dependsOn || []).filter((id) => id !== source) } : step) }));
  const autoLayout = () => {
    const levels = {};
    const levelOf = (id, trail = []) => { if (levels[id] != null) return levels[id]; if (trail.includes(id)) return 0; const step = editor.steps.find((item) => item.id === id); return levels[id] = step?.dependsOn?.length ? Math.max(...step.dependsOn.map((dep) => levelOf(dep, [...trail, id]))) + 1 : 0; };
    editor.steps.forEach((step) => levelOf(step.id));
    const rows = {};
    const nodes = {}; editor.steps.forEach((step) => { const level = levels[step.id] || 0; rows[level] = (rows[level] || 0) + 1; nodes[step.id] = { x: 60 + level * 310, y: 55 + (rows[level] - 1) * 155 }; });
    setEditor({ ...editor, layout: { nodes } });
  };
  const moveNode = (event, stepID) => {
    if (!drag.current || drag.current.id !== stepID) return;
    const rect = canvas.current.getBoundingClientRect();
    const x = Math.max(10, Math.min(700, event.clientX - rect.left - drag.current.offsetX));
    const y = Math.max(10, Math.min(460, event.clientY - rect.top - drag.current.offsetY));
    setEditor((current) => ({ ...current, layout: { nodes: { ...(current.layout?.nodes || {}), [stepID]: { x, y } } } }));
  };
  const updateSignal = (oldSignal, signal, target) => setStep("transitions", Object.fromEntries(Object.entries(selected.transitions || {}).filter(([key]) => key !== oldSignal).concat([[signal, target]])));

  return <Modal wide title="并行 DAG 画布" subtitle="拖动节点；点击节点右上连接点，再点击目标节点连接点创建依赖" onClose={onClose}><div className="dag-editor-shell"><div className="dag-toolbar"><div className="dag-meta"><input value={editor.id} onChange={(event) => setEditor({ ...editor, id: event.target.value })} /><input value={editor.name} onChange={(event) => setEditor({ ...editor, name: event.target.value })} /></div><div className="dag-actions"><span className="mode-chip dag">DAG · ALL JOIN</span><Action onClick={autoLayout}>自动布局</Action><Action tone="primary" onClick={addNode}>节点</Action><Action tone="primary" disabled={busy === "workflow"} onClick={saveWorkflow}>{busy === "workflow" ? "保存中" : "保存 DAG"}</Action></div></div><div className="dag-workspace"><div className="dag-canvas" ref={canvas} onPointerUp={() => { drag.current = null; }} onPointerLeave={() => { drag.current = null; }}><svg className="dag-edges" viewBox="0 0 900 560" preserveAspectRatio="none">{editor.steps.flatMap((step) => (step.dependsOn || []).map((source) => { const from = positions[source] || { x: 30, y: 30 }; const to = positions[step.id] || { x: 400, y: 200 }; const x1 = from.x + 190, y1 = from.y + 48, x2 = to.x, y2 = to.y + 48; return <g key={`${source}-${step.id}`} className="dag-edge" onClick={() => deleteEdge(source, step.id)}><path d={`M ${x1} ${y1} C ${x1 + 70} ${y1}, ${x2 - 70} ${y2}, ${x2} ${y2}`} /><text x={(x1 + x2) / 2} y={(y1 + y2) / 2 - 7}>×</text></g>; }))}</svg>{editor.steps.map((step) => { const point = positions[step.id] || { x: 50, y: 50 }; return <div key={step.id} className={`dag-node-card ${selectedID === step.id ? "selected" : ""} ${connectFrom === step.id ? "connecting" : ""}`} style={{ left: point.x, top: point.y }} onClick={() => setSelectedID(step.id)} onPointerDown={(event) => { if (event.target.closest("button")) return; const rect = event.currentTarget.getBoundingClientRect(); drag.current = { id: step.id, offsetX: event.clientX - rect.left, offsetY: event.clientY - rect.top }; event.currentTarget.setPointerCapture(event.pointerId); }} onPointerMove={(event) => moveNode(event, step.id)}><button className="dag-port input" title="连接到这里" aria-label={`连接到 ${step.name}`} onClick={(event) => { event.stopPropagation(); connect(step.id); }} /><div className="dag-node-head"><span className={`runtime-badge ${step.runtime}`}>{step.runtime}</span><small>{step.workerId || "local"}</small></div><strong>{step.name}</strong><p>{(step.dependsOn || []).length ? `等待 ${(step.dependsOn || []).length} 个依赖` : "Root · 可并行"}</p><button className="dag-port output" title="从这里连接" aria-label={`从 ${step.name} 连接`} onClick={(event) => { event.stopPropagation(); connect(step.id); }} /></div>; })}<div className="canvas-hint">{connectFrom ? `正在从 ${connectFrom} 连接，点击目标节点连接点` : "拖动节点调整布局 · 点击边上的 × 删除依赖"}</div></div><aside className="dag-inspector">{selected ? <><div className="dag-inspector-title"><div><span className="kicker">NODE INSPECTOR</span><h3>{selected.name}</h3></div><Action className="dag-delete-node" tone="danger" disabled={editor.steps.length <= 1} onClick={deleteNode}>删除节点</Action></div><label>Node ID<input value={selected.id} onChange={(event) => { const next = event.target.value; renameStep(next); setSelectedID(next); }} /></label><label>名称<input value={selected.name} onChange={(event) => setStep("name", event.target.value)} /></label><div className="two-fields"><label>Runtime<TUISelect ariaLabel="Runtime" value={selected.runtime} onChange={(runtime) => setStep("runtime", runtime)} options={runtimes.map((runtime) => ({ value: runtime.id, label: runtime.name }))} /></label><label>Worker<TUISelect ariaLabel="Worker" value={selected.workerId || "local"} onChange={(workerId) => setStep("workerId", workerId)} options={workerOptions.map((worker) => ({ value: worker.id, label: worker.name }))} /></label></div><label>Sandbox<TUISelect ariaLabel="Sandbox" value={selected.sandbox || "read-only"} onChange={(sandbox) => setStep("sandbox", sandbox)} options={[{ value: "read-only", label: "Read only" }, { value: "workspace-write", label: "Workspace write" }, { value: "full", label: "Full access" }]} /></label><label>角色提示<textarea value={selected.rolePrompt} onChange={(event) => setStep("rolePrompt", event.target.value)} /></label><label>节点指令<textarea value={selected.instruction} onChange={(event) => setStep("instruction", event.target.value)} /></label><div className="transition-editor"><span>终点 Signals</span>{Object.entries(selected.transitions || {}).map(([signal, target]) => <div className="transition-row" key={signal}><input value={signal} onChange={(event) => updateSignal(signal, event.target.value, target)} /><span>→</span><TUISelect ariaLabel={`${signal} target`} value={target} onChange={(nextTarget) => updateSignal(signal, signal, nextTarget)} options={[{ value: "$done", label: "$done" }, { value: "$pause", label: "$pause" }, { value: "$fail", label: "$fail" }]} /></div>)}</div></> : null}</aside></div><div className={`dag-validation ${validation.length ? "has-errors" : ""}`}><button className="text-button" onClick={validateEditor}>校验 DAG</button>{validation.length ? validation.map((issue) => <span key={`${issue.path}-${issue.code}`}><code>{issue.path}</code>{issue.message}</span>) : <span>保存前会检查环、未知依赖和并行写冲突。</span>}</div></div></Modal>;
}

export default App;
