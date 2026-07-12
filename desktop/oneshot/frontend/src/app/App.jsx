import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { CaretDown, CaretRight, Circle } from "@phosphor-icons/react";
import {
  RuntimeBinding,
  SettingsBinding,
  TaskRunBinding,
  WorkflowBinding,
  WorkspaceBinding,
  WorkerBinding,
} from "../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";
import SettingsPage, { ConfirmDialog, demoSettings } from "./SettingsPage.jsx";
import { buildRunConversation } from "./runConversation.js";
import { runtimeSessionEntries } from "./sessionCommands.js";
import { nextWorkflowItemID } from "./workflowIds.js";
import { runPairsContain } from "./workspaceRunSelection.js";
import { Action, Kicker, ModeBadge, StatusBadge, TUISelect, Toolbar, ToolbarSpacer } from "../ui/primitives.jsx";

const statusLabel = {
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
  { id: "task_demo", workspaceId: "oneshot-demo", title: "优化本地 Agent 调度流程", prompt: "梳理并改进工作流执行、暂停与恢复体验。", workflowId: "implement_review", status: "paused", updatedAt: demoNow },
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

function Icon({ name, size = 18 }) {
  const paths = {
    tasks: <><path d="M5 4h14v16H5z" /><path d="M9 4V2h6v2M8 9h8M8 13h8M8 17h5" /></>,
    workflow: <><circle cx="5" cy="6" r="2" /><circle cx="19" cy="12" r="2" /><circle cx="5" cy="18" r="2" /><path d="M7 6h4a4 4 0 0 1 4 4v0M7 18h4a4 4 0 0 0 4-4v0" /></>,
    plus: <path d="M12 5v14M5 12h14" />,
    play: <path d="m8 5 11 7-11 7z" />,
    stop: <path d="M7 7h10v10H7z" />,
    pause: <path d="M9 6v12M15 6v12" />,
    refresh: <><path d="M20 7v5h-5" /><path d="M4 17v-5h5" /><path d="M6.1 8a7 7 0 0 1 11.4-2.2L20 8M4 16l2.5 2.2A7 7 0 0 0 18 16" /></>,
    folder: <path d="M3 6h7l2 2h9v10H3z" />,
    branch: <><circle cx="7" cy="5" r="2" /><circle cx="17" cy="19" r="2" /><path d="M7 7v5a4 4 0 0 0 4 4h4M17 17V7M14 10l3-3 3 3" /></>,
    close: <path d="m6 6 12 12M18 6 6 18" />,
    edit: <><path d="m4 16-1 5 5-1L19 9l-4-4z" /><path d="m13 7 4 4" /></>,
    check: <path d="m5 12 4 4L19 6" />,
    settings: <><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.8 2.8-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-4V21a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L4.2 17l.1-.1a1.7 1.7 0 0 0 .3-1.9A1.7 1.7 0 0 0 3 14H3v-4h.1a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9L4.2 7 7 4.2l.1.1a1.7 1.7 0 0 0 1.9.3A1.7 1.7 0 0 0 10 3V3h4v.1a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1L19.8 7l-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2v4H21a1.7 1.7 0 0 0-1.6 1z" /></>,
  };
  return <svg className="icon" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">{paths[name]}</svg>;
}

function copy(value) { return JSON.parse(JSON.stringify(value)); }
function shortID(value = "") { return value.length > 18 ? `${value.slice(0, 8)}…${value.slice(-5)}` : value; }
function formatTime(value) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}
function errorMessage(error) { return String(error?.message || error || "未知错误").replace(/^Error:\s*/, ""); }

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
  const [tasks, setTasks] = useState([]);
  const [runs, setRuns] = useState({});
  const [selectedRunID, setSelectedRunID] = useState("");
  const [runDetail, setRunDetail] = useState(null);
  const [editor, setEditor] = useState(null);
  const [validation, setValidation] = useState([]);
  const [workspaceModal, setWorkspaceModal] = useState(false);
  const [workspaceForm, setWorkspaceForm] = useState({ path: "", name: "", defaultSandbox: "" });
  const [taskForm, setTaskForm] = useState({ title: "", prompt: "", workflowId: "" });
  const [resumeInstruction, setResumeInstruction] = useState("");
  const [notice, setNotice] = useState(null);
  const [busy, setBusy] = useState("");
  const [workers, setWorkers] = useState([]);
  const [workerHealth, setWorkerHealth] = useState({});
  const [workerModal, setWorkerModal] = useState(false);
  const [workerForm, setWorkerForm] = useState({ id: "", name: "", baseUrl: "http://", token: "", enabled: true });
  const [settings, setSettings] = useState(demoSettings);
  const [appDialog, setAppDialog] = useState(null);
  const appDialogResolve = useRef(null);
  const taskLoadVersion = useRef(0);
  const runLoadVersion = useRef(0);

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
    if (!nextWorkspaceID || nextWorkspaceID === workspaceID) return;
    taskLoadVersion.current += 1;
    runLoadVersion.current += 1;
    setWorkspaceID(nextWorkspaceID);
    setTasks([]);
    setRuns({});
    setSelectedRunID("");
    setRunDetail(null);
    setResumeInstruction("");
  }, [workspaceID]);

  const boot = useCallback(async () => {
    try {
      const [runtimeItems, workspaceItems, workflowItems, workerItems, settingsValue] = await Promise.all([
        RuntimeBinding.ListRuntimes(), WorkspaceBinding.ListWorkspaces(), WorkflowBinding.ListDefinitions(), WorkerBinding.ListWorkers(), SettingsBinding.GetSettings(),
      ]);
      setMode("wails");
      setRuntimes(runtimeItems || []);
      setWorkspaces(workspaceItems || []);
      setWorkflows(workflowItems || []);
      setWorkers(workerItems || []);
      setSettings(settingsValue || demoSettings);
      setWorkspaceID((current) => current || workspaceItems?.[0]?.id || "");
      setTaskForm((current) => ({ ...current, workflowId: current.workflowId || workflowItems?.[0]?.id || "" }));
    } catch {
      setMode("demo");
      setRuntimes(demoRuntimes);
      setWorkspaces(demoWorkspaces);
      setWorkflows(demoWorkflows);
      setWorkers(demoWorkers);
      setSettings(demoSettings);
      setWorkspaceID(demoWorkspaces[0].id);
      setTaskForm((current) => ({ ...current, workflowId: "implement_review" }));
      setTasks(demoTasks);
      setRuns({ task_demo: [{ ...demoRun.run }] });
      setSelectedRunID("run_demo");
      setRunDetail(demoRun);
    }
  }, []);

  useEffect(() => { boot(); }, [boot]);

  const loadTasks = useCallback(async () => {
    if (!workspaceID || mode !== "wails") return;
    const loadVersion = ++taskLoadVersion.current;
    try {
      const items = await TaskRunBinding.ListTasks(workspaceID);
      const runPairs = await Promise.all((items || []).map(async (task) => [task.id, await TaskRunBinding.ListRunsByTask(task.id)]));
      if (loadVersion !== taskLoadVersion.current) return;
      const nextRuns = Object.fromEntries(runPairs);
      setTasks(items || []);
      setRuns(nextRuns);
      if (selectedRunID && !runPairsContain(runPairs, selectedRunID)) {
        runLoadVersion.current += 1;
        setSelectedRunID("");
        setRunDetail(null);
        setResumeInstruction("");
      }
    } catch (error) { if (loadVersion === taskLoadVersion.current) notify("error", errorMessage(error)); }
  }, [mode, notify, selectedRunID, workspaceID]);

  useEffect(() => {
    if (mode === "wails") loadTasks();
    if (mode === "demo") setTasks(demoTasks.filter((task) => task.workspaceId === workspaceID));
  }, [loadTasks, mode, workspaceID]);

  const loadRun = useCallback(async (runID, silent = false) => {
    if (!runID) return;
    const loadVersion = ++runLoadVersion.current;
    if (mode === "demo") { setRunDetail((current) => current?.run?.id === runID ? current : demoRun); return; }
    try {
      const detail = await TaskRunBinding.GetRun(runID);
      if (loadVersion !== runLoadVersion.current) return;
      setRunDetail(detail);
      if (!silent) setSelectedRunID(runID);
    } catch (error) { if (loadVersion === runLoadVersion.current && !silent) notify("error", errorMessage(error)); }
  }, [mode, notify]);

  useEffect(() => { if (selectedRunID) loadRun(selectedRunID, true); }, [loadRun, selectedRunID]);

  useEffect(() => {
    if (!selectedRunID || mode !== "wails") return undefined;
    const timer = window.setInterval(async () => {
      await loadRun(selectedRunID, true);
      await loadTasks();
    }, runDetail?.active || runDetail?.run?.status === "running" ? 900 : 2500);
    return () => window.clearInterval(timer);
  }, [loadRun, loadTasks, mode, runDetail?.active, runDetail?.run?.status, selectedRunID]);

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
    try {
      if (mode === "demo") {
        setSelectedRunID("run_demo"); setRunDetail({ ...demoRun, run: { ...demoRun.run, status: "running" }, active: true });
      } else {
        const task = await TaskRunBinding.CreateTask({ workspaceId: workspaceID, ...taskForm });
        const preview = await TaskRunBinding.PreviewRun(task.id);
        const run = await TaskRunBinding.StartRun(task.id, preview.confirmationToken || "");
        setSelectedRunID(run.id); await loadRun(run.id); await loadTasks();
      }
      setTaskForm((form) => ({ ...form, title: "", prompt: "" })); notify("success", "Run 已启动，Agent 正在后台执行");
    } catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
  };

  const runAction = async (action) => {
    if (!selectedRunID) return;
    setBusy(action);
    try {
      if (mode === "demo") {
        const nextStatus = action === "interrupt" ? "paused" : action === "resume" ? "running" : "cancelled";
        setRunDetail((detail) => ({ ...detail, active: action === "resume", run: { ...detail.run, status: nextStatus, pauseReason: action === "interrupt" ? "interrupted" : "" } }));
      } else {
        if (action === "interrupt") await TaskRunBinding.InterruptRun(selectedRunID);
        if (action === "resume") await TaskRunBinding.ResumeRun(selectedRunID, resumeInstruction);
        if (action === "cancel") await TaskRunBinding.CancelRun(selectedRunID);
        window.setTimeout(() => loadRun(selectedRunID, true), 250);
      }
      if (action === "resume") setResumeInstruction("");
    } catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
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

  const allRuns = useMemo(() => tasks.flatMap((task) => (runs[task.id] || []).map((run) => ({ ...run, task }))).sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt)), [runs, tasks]);
  const goView = (next) => { setEditor(null); setView(next); };
  const commandText = view === "settings" ? "settings @ ~/.oneshot" : selectedWorkspace ? `${selectedWorkspace.name} @ ${selectedWorkspace.path}` : "选择一个工作目录";

  if (mode === "loading") return <div className="loading-screen"><div className="brand-mark">1</div><span>正在打开本地工作台…</span></div>;

  return <div className="app-frame">
    <header className="mac-titlebar" aria-label="Oneshot window"><span aria-hidden="true" /><strong>Oneshot</strong><span /></header>
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand"><strong>ONESHOT</strong><span>// personal workspace</span></div>
        <div className="workspace-block">
          <div className="sidebar-section-label">cwd</div>
          <div className="workspace-list">
            {workspaces.map((workspace) => <button key={workspace.id} className={`workspace-item ${workspace.id === workspaceID ? "active" : ""}`} onClick={() => selectWorkspace(workspace.id)}><span><strong>{workspace.name}</strong><small>{workspace.path}</small></span></button>)}
            {!workspaces.length && <div className="sidebar-empty">还没有工作目录</div>}
          </div>
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
        {editor ? <WorkflowEditor editor={editor} setEditor={setEditor} validation={validation} validateEditor={validateEditor} saveWorkflow={saveWorkflow} busy={busy} updateStep={updateStep} updateTransition={updateTransition} removeTransition={removeTransition} runtimes={runtimes} workers={settings.experimental?.remoteWorkersEnabled ? workers : []} defaultSandbox={settings.execution.defaultSandbox} allowFullSandbox={settings.security.allowFullSandbox} onClose={() => setEditor(null)} /> : view === "tasks" ? <div className="workspace-grid">
          <section className="content-column">
            <div className="composer">
              <Kicker>new run</Kicker>
              <input className="title-input" placeholder="这次要完成什么？" value={taskForm.title} onChange={(event) => setTaskForm((form) => ({ ...form, title: event.target.value }))} />
              <textarea placeholder="描述目标、约束和验收方式。每个步骤会继承这段任务上下文。" value={taskForm.prompt} onChange={(event) => setTaskForm((form) => ({ ...form, prompt: event.target.value }))} />
              <div className="composer-footer"><label><span>workflow</span><TUISelect ariaLabel="Workflow" value={taskForm.workflowId} onChange={(workflowId) => setTaskForm((form) => ({ ...form, workflowId }))} options={workflows.map((workflow) => ({ value: workflow.id, label: workflow.name }))} /></label><Action tone="primary" disabled={busy === "run" || !selectedWorkspace} onClick={createTaskAndRun}>{busy === "run" ? "starting…" : "start run"}</Action></div>
            </div>
            <div className="section-title"><div><h2>最近运行</h2></div><span>{allRuns.length} runs</span></div>
            <div className="run-list">{allRuns.map((item) => <button key={item.id} className={`run-card ${selectedRunID === item.id ? "selected" : ""}`} onClick={() => { setSelectedRunID(item.id); loadRun(item.id); }}><StatusPill status={item.status} /><h3>{item.task.title}</h3><p>{workflows.find((workflow) => workflow.id === item.workflowId)?.name || item.workflowId} · {formatTime(item.updatedAt)}</p></button>)}{!allRuns.length && <div className="empty-state"><h3>还没有运行记录</h3><p>写下任务目标并选择 Workflow，Oneshot 会在后台调度本地 Agent。</p></div>}</div>
          </section>
          <aside className="inspector">{runDetail ? <RunInspector detail={runDetail} busy={busy} resumeInstruction={resumeInstruction} setResumeInstruction={setResumeInstruction} runAction={runAction} notify={notify} /> : <div className="inspector-empty"><h3>{allRuns.length ? "选择一个 Run" : "暂无运行详情"}</h3><p>{allRuns.length ? "这里会显示当前步骤、Agent 输出、回边和人工介入操作。" : "当前工作目录还没有 Run，右侧不会保留其他工作目录的数据。"}</p></div>}</aside>
        </div> : view === "workflows" ? <WorkflowLibrary workflows={workflows} runtimes={runtimes} openEditor={openEditor} /> : <SettingsPage mode={mode} value={settings} runtimes={runtimes} onChange={setSettings} notify={notify} workersPanel={<WorkerPage workers={workers} health={workerHealth} checkWorker={checkWorker} deleteWorker={deleteWorker} openWorker={(worker) => { setWorkerForm(worker ? { id: worker.id, name: worker.name, baseUrl: worker.baseUrl, token: "", enabled: worker.enabled } : { id: "", name: "", baseUrl: "http://", token: "", enabled: true }); setWorkerModal(true); }} />} />}
      </main>
    </div>
    {workspaceModal && <Modal title="加入工作目录" subtitle="Agent 只会在你授权的目录中工作" onClose={() => setWorkspaceModal(false)}><div className="form-stack"><label>目录路径<input autoFocus value={workspaceForm.path} onChange={(event) => setWorkspaceForm((form) => ({ ...form, path: event.target.value }))} placeholder="/Users/me/Code/project" /></label><label>显示名称（可选）<input value={workspaceForm.name} onChange={(event) => setWorkspaceForm((form) => ({ ...form, name: event.target.value }))} placeholder="默认使用目录名" /></label><label>默认 Sandbox<TUISelect ariaLabel="默认 Sandbox" value={workspaceForm.defaultSandbox} onChange={(defaultSandbox) => setWorkspaceForm((form) => ({ ...form, defaultSandbox }))} options={[{ value: "", label: "使用设置中的全局默认" }, { value: "read-only", label: "Read only" }, { value: "workspace-write", label: "Workspace write" }, ...(settings.security?.allowFullSandbox ? [{ value: "full", label: "Full access（危险）" }] : [])]} /></label><div className="modal-actions"><button className="secondary-button" onClick={() => setWorkspaceModal(false)}>[ 取消 ]</button><button className="primary-button" onClick={addWorkspace} disabled={busy === "workspace"}>[ 加入目录 ]</button></div></div></Modal>}
    {workerModal && <Modal title="远端 Worker" subtitle="仅用于受信任 LAN / VPN；token 保存于本机 0600 文件" onClose={() => setWorkerModal(false)}><div className="form-stack"><label>Worker ID<input value={workerForm.id} onChange={(event) => setWorkerForm((form) => ({ ...form, id: event.target.value }))} placeholder="mac-mini" /></label><label>名称<input value={workerForm.name} onChange={(event) => setWorkerForm((form) => ({ ...form, name: event.target.value }))} placeholder="Build Mac mini" /></label><label>Base URL<input value={workerForm.baseUrl} onChange={(event) => setWorkerForm((form) => ({ ...form, baseUrl: event.target.value }))} placeholder="http://192.168.1.20:9231" /></label><label>Bearer token<input type="password" value={workerForm.token} onChange={(event) => setWorkerForm((form) => ({ ...form, token: event.target.value }))} placeholder="更新时留空则保留原 token" /></label><label className="checkbox-label"><input type="checkbox" checked={workerForm.enabled} onChange={(event) => setWorkerForm((form) => ({ ...form, enabled: event.target.checked }))} />启用调度</label><div className="modal-actions"><button className="secondary-button" onClick={() => setWorkerModal(false)}>[ 取消 ]</button><button className="primary-button" disabled={busy === "worker"} onClick={saveWorker}>[ 保存 Worker ]</button></div></div></Modal>}
    <ConfirmDialog dialog={appDialog} onCancel={() => resolveConfirm(false)} onConfirm={() => resolveConfirm(true)} />
    {notice && <div className={`toast ${notice.type}`}><span>{notice.text}</span></div>}
  </div>;
}

function RunInspector({ detail, busy, resumeInstruction, setResumeInstruction, runAction, notify }) {
  const { run, workflow, stepRuns = [], runtimeEvents = [] } = detail;
  const dagNodes = run.nodes || {};
  const currentNodeID = workflow.mode === "dag" ? Object.values(dagNodes).find((node) => node.status === "running" || node.status === "paused")?.stepId : run.currentStepId;
  const currentStep = workflow.steps?.find((step) => step.id === currentNodeID) || workflow.steps?.[0];
  const conversation = buildRunConversation(detail);
  const sessions = runtimeSessionEntries(detail);
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
    <section className="run-summary">
      <div className="run-summary-head"><div><h2>{detail.task.title}</h2><StatusPill status={run.status} active={detail.active} /></div><div className="run-summary-stats"><span>{run.transitionCount || 0} / {workflow.policy?.maxTransitions || 20}</span><time>{formatTime(run.updatedAt || run.startedAt)}</time></div></div>
      <details className="run-summary-details"><summary><span className="run-summary-current">当前：<strong>{currentStep?.name || run.currentStepId}</strong><i>·</i><span className={`runtime-name ${currentStep?.runtime}`}>{currentStep?.runtime || "—"}</span></span><span className="run-summary-recovery">会话与恢复 <CaretRight className="details-caret closed" weight="bold" /><CaretDown className="details-caret opened" weight="bold" /></span></summary><div className="run-summary-detail-body">
        <div className="run-summary-flow">{(workflow.steps || []).map((step, index) => { const nodeStatus = dagNodes[step.id]?.status; return <div className={`run-summary-step ${step.id === currentNodeID ? "current" : ""}`} key={step.id}><b>{String(index + 1).padStart(2, "0")}</b><span><strong>{step.name}</strong><small>{step.runtime} · {step.workerId || "local"} · {nodeStatus || step.sandbox || "workspace-write"}</small></span>{nodeStatus && <em>{nodeStatus}</em>}</div>; })}</div>
        <div className="run-summary-id"><span>RUN ID</span><code>{run.id}</code></div>
        {sessions.map((session) => <div className="run-summary-session" key={session.stepID}><div><span className={`runtime-badge ${session.runtime}`}>{session.runtime}</span><strong>{session.stepName}</strong></div><div className="session-value"><span>{session.idLabel}</span><code>{session.sessionID}</code><button type="button" onClick={() => copySessionValue(session.sessionID, "会话 ID")}>[ 复制 ID ]</button></div>{session.command && <div className="session-value command"><span>resume</span><code>{session.command}</code><button type="button" onClick={() => copySessionValue(session.command, "恢复命令")}>[ 复制命令 ]</button></div>}</div>)}
      </div></details>
      {(visiblePauseReason || detail.lastError) && <div className="run-summary-notice">{visiblePauseReason && <span>暂停：{visiblePauseReason}</span>}{detail.lastError && <span className="error-copy">{detail.lastError}</span>}</div>}
    </section>
    <ConversationTimeline items={conversation} active={detail.active} />
    {["running", "paused"].includes(run.status) && <div className="run-controls">
      {run.status === "running" && <Action tone="danger" disabled={busy === "interrupt"} onClick={() => runAction("interrupt")}>打断并暂停</Action>}
      {run.status === "paused" && <><textarea value={resumeInstruction} onChange={(event) => setResumeInstruction(event.target.value)} placeholder="补充指令（可选），恢复后注入当前步骤…" /><div><Action tone="danger" disabled={busy === "cancel"} onClick={() => runAction("cancel")}>终止</Action><Action tone="primary" disabled={busy === "resume"} onClick={() => runAction("resume")}>恢复运行</Action></div></>}
    </div>}
  </>;
}

function ConversationTimeline({ items, active }) {
  const rounds = items.filter((item) => item.type === "round");
  return <div className="conversation-section">
    <div className="conversation-list">
      {items.map((item) => item.type === "user" ? <div className="conversation-user" key={item.id}>
        <div className="conversation-speaker"><span className="conversation-identity"><Circle className="conversation-event-dot user" weight="fill" aria-label="用户消息" /></span><span className="conversation-message-meta"><time>{formatTime(item.at)}</time></span></div>
        <p>{item.text}</p>
      </div> : <article className="conversation-round" key={item.id}>
        <div className="conversation-round-body">{item.items.map((entry, index) => entry.type === "message" ? <div className={`conversation-agent ${entry.tone}`} key={`message-${index}`}><div className="conversation-speaker"><span className="conversation-identity"><Circle className="conversation-event-dot agent" weight="fill" aria-label="Agent 消息" /><strong>{item.runtime}</strong></span><span className="conversation-message-meta"><span className="conversation-round-index">第 {item.round} 轮</span><time>{formatTime(entry.at || item.finishedAt || item.startedAt)}</time></span></div><p>{entry.text}</p></div> : <ToolTimelineItem key={`tool-${index}`} entry={entry} running={active && item === rounds[rounds.length - 1] && index === item.items.length - 1 && entry.kind === "tool_use"} failed={item.status === "failed"} />)}</div>
      </article>)}
      {!items.length && <p className="muted-copy">Agent 消息会按执行轮次显示在这里。</p>}
    </div>
  </div>;
}

function ToolTimelineItem({ entry, running, failed }) {
  const labels = { tool_use: "TOOL USE", tool_result: "RESULT", file_change: "FILE CHANGE", reasoning: "PROCESS" };
  const state = failed ? "失败" : running ? "执行中" : entry.kind === "reasoning" ? "过程" : "完成";
  return <details className={`conversation-tool kind-${entry.kind} ${running ? "running" : ""} ${failed ? "failed" : ""}`}>
    <summary aria-label={`${labels[entry.kind] || entry.kind}: ${entry.title}`}><span className="conversation-tool-summary"><span className="conversation-tool-caret"><CaretRight className="closed" weight="bold" /><CaretDown className="opened" weight="bold" /></span><strong title={entry.text}>{entry.title}</strong><span className="conversation-tool-state">{running && <span className="pulse" />}{state}</span><time>{formatTime(entry.at)}</time></span></summary>
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
    <div className="library-hero"><div><Kicker>loops &amp; parallel dag</Kicker><h2>把你的工作方式变成 Workflow</h2></div><div className="hero-actions"><Action onClick={() => openEditor(loopTemplate)}>+ loop</Action><Action tone="primary" onClick={() => openEditor(dagTemplate)}>+ 并行 dag</Action></div></div>
    <div className="workflow-grid">{workflows.map((workflow) => <button className="workflow-card" key={workflow.id} onClick={() => openEditor(workflow)}><ModeBadge mode={workflow.mode || "serial"} /><strong>{workflow.name}</strong><span className="workflow-path">{path(workflow)}</span><small>{workflow.steps?.length || 0} {workflow.mode === "dag" ? "nodes · all join" : `${workflow.steps?.length === 1 ? "step" : "steps"} · ${workflow.policy?.maxTransitions || 20} max`}</small><b>[ edit ]</b></button>)}</div>
    <div className="runtime-callout">Workflow 不会自动替换缺失的 runtime，以免破坏角色语义。{runtimes.some((runtime) => !runtime.available) && <strong>存在缺失 Runtime</strong>}</div>
  </div>;
}

function WorkerPage({ workers, health, checkWorker, deleteWorker, openWorker }) {
  return <div className="library-page worker-page"><div className="library-hero"><div><span className="kicker">TRUSTED LAN / VPN</span><h2>把 DAG 节点派到另一台机器</h2><p>远端机器需要相同 Workspace ID 的本地 clone。Oneshot 只传 prompt、outcome 和事件，不同步项目文件。</p></div><button className="primary-button" onClick={() => openWorker(null)}><Icon name="plus" />注册 Worker</button></div><div className="worker-grid">{workers.map((worker) => <article className="worker-card panel" key={worker.id}><div className="worker-card-head"><div className="worker-machine"><Icon name="branch" /><span><strong>{worker.name}</strong><small>{worker.id}</small></span></div><span className={`status-pill ${worker.enabled ? "completed" : "cancelled"}`}>{worker.enabled ? "Enabled" : "Disabled"}</span></div><code>{worker.baseUrl}</code><div className="worker-health">{health[worker.id]?.checking ? "正在检查…" : health[worker.id]?.ok ? <>{Object.entries(health[worker.id].runtimes || {}).map(([runtime, ok]) => <span key={runtime} className={ok ? "ok" : "missing"}><i />{runtime}</span>)}</> : health[worker.id]?.error || "尚未检查连接"}</div><div className="worker-actions"><button className="secondary-button compact" onClick={() => checkWorker(worker)}>健康检查</button><button className="secondary-button compact" onClick={() => openWorker(worker)}>编辑</button><button className="text-button danger" onClick={() => deleteWorker(worker.id)}>删除</button></div></article>)}{!workers.length && <div className="empty-state"><div className="empty-icon"><Icon name="branch" /></div><h3>还没有远端 Worker</h3><p>先在另一台机器启动 oneshot-worker，再在这里注册地址与共享 token。</p></div>}</div><div className="worker-command panel"><span className="kicker">REMOTE COMMAND</span><code>ONESHOT_WORKER_TOKEN=... oneshot-worker --listen 0.0.0.0:9231 --id mac-mini --workspace workspace-id=/path/to/clone</code><p>建议通过 Tailscale / WireGuard 地址连接，不要把首版 HTTP worker 直接暴露到公网。</p></div></div>;
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
      <div className="step-editor-head"><b>{String(stepIndex + 1).padStart(2, "0")}</b><input aria-label="步骤名称" value={step.name} onChange={(event) => updateStep(stepIndex, "name", event.target.value)} /><TUISelect ariaLabel="Runtime" value={step.runtime} onChange={(runtime) => updateStep(stepIndex, "runtime", runtime)} options={runtimes.map((runtime) => ({ value: runtime.id, label: runtime.id, meta: runtime.available ? "" : "missing", disabled: !runtime.available && runtime.id !== step.runtime }))} /><TUISelect ariaLabel="Sandbox" value={step.sandbox || "workspace-write"} onChange={(sandbox) => updateStep(stepIndex, "sandbox", sandbox)} options={[{ value: "read-only", label: "read-only" }, { value: "workspace-write", label: "workspace-write" }, { value: "full", label: "full-access", disabled: !allowFullSandbox && step.sandbox !== "full" }]} /><button className="text-button danger" disabled={editor.steps.length <= 1} onClick={() => removeStep(stepIndex)}>[ 删除步骤 ]</button></div>
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

  return <Modal wide title="并行 DAG 画布" subtitle="拖动节点；点击节点右上连接点，再点击目标节点连接点创建依赖" onClose={onClose}><div className="dag-editor-shell"><div className="dag-toolbar"><div className="dag-meta"><input value={editor.id} onChange={(event) => setEditor({ ...editor, id: event.target.value })} /><input value={editor.name} onChange={(event) => setEditor({ ...editor, name: event.target.value })} /></div><div><span className="mode-chip dag">DAG · ALL JOIN</span><button className="secondary-button compact" onClick={autoLayout}>自动布局</button><button className="primary-button compact" onClick={addNode}><Icon name="plus" />节点</button></div></div><div className="dag-workspace"><div className="dag-canvas" ref={canvas} onPointerUp={() => { drag.current = null; }} onPointerLeave={() => { drag.current = null; }}><svg className="dag-edges" viewBox="0 0 900 560" preserveAspectRatio="none">{editor.steps.flatMap((step) => (step.dependsOn || []).map((source) => { const from = positions[source] || { x: 30, y: 30 }; const to = positions[step.id] || { x: 400, y: 200 }; const x1 = from.x + 190, y1 = from.y + 48, x2 = to.x, y2 = to.y + 48; return <g key={`${source}-${step.id}`} className="dag-edge" onClick={() => deleteEdge(source, step.id)}><path d={`M ${x1} ${y1} C ${x1 + 70} ${y1}, ${x2 - 70} ${y2}, ${x2} ${y2}`} /><text x={(x1 + x2) / 2} y={(y1 + y2) / 2 - 7}>×</text></g>; }))}</svg>{editor.steps.map((step) => { const point = positions[step.id] || { x: 50, y: 50 }; return <div key={step.id} className={`dag-node-card ${selectedID === step.id ? "selected" : ""} ${connectFrom === step.id ? "connecting" : ""}`} style={{ left: point.x, top: point.y }} onClick={() => setSelectedID(step.id)} onPointerDown={(event) => { if (event.target.closest("button")) return; const rect = event.currentTarget.getBoundingClientRect(); drag.current = { id: step.id, offsetX: event.clientX - rect.left, offsetY: event.clientY - rect.top }; event.currentTarget.setPointerCapture(event.pointerId); }} onPointerMove={(event) => moveNode(event, step.id)}><button className="dag-port input" title="连接到这里" onClick={(event) => { event.stopPropagation(); connect(step.id); }} /><div className="dag-node-head"><span className={`runtime-badge ${step.runtime}`}>{step.runtime}</span><small>{step.workerId || "local"}</small></div><strong>{step.name}</strong><p>{(step.dependsOn || []).length ? `等待 ${(step.dependsOn || []).length} 个依赖` : "Root · 可并行"}</p><button className="dag-port output" title="从这里连接" onClick={(event) => { event.stopPropagation(); connect(step.id); }} /></div>; })}<div className="canvas-hint">{connectFrom ? `正在从 ${connectFrom} 连接，点击目标节点连接点` : "拖动节点调整布局 · 点击边上的 × 删除依赖"}</div></div><aside className="dag-inspector">{selected ? <><div className="dag-inspector-title"><div><span className="kicker">NODE INSPECTOR</span><h3>{selected.name}</h3></div><button className="icon-button" disabled={editor.steps.length <= 1} onClick={deleteNode}><Icon name="close" /></button></div><label>Node ID<input value={selected.id} onChange={(event) => { const next = event.target.value; renameStep(next); setSelectedID(next); }} /></label><label>名称<input value={selected.name} onChange={(event) => setStep("name", event.target.value)} /></label><div className="two-fields"><label>Runtime<TUISelect ariaLabel="Runtime" value={selected.runtime} onChange={(runtime) => setStep("runtime", runtime)} options={runtimes.map((runtime) => ({ value: runtime.id, label: runtime.name }))} /></label><label>Worker<TUISelect ariaLabel="Worker" value={selected.workerId || "local"} onChange={(workerId) => setStep("workerId", workerId)} options={workerOptions.map((worker) => ({ value: worker.id, label: worker.name }))} /></label></div><label>Sandbox<TUISelect ariaLabel="Sandbox" value={selected.sandbox || "read-only"} onChange={(sandbox) => setStep("sandbox", sandbox)} options={[{ value: "read-only", label: "Read only" }, { value: "workspace-write", label: "Workspace write" }, { value: "full", label: "Full access" }]} /></label><label>角色提示<textarea value={selected.rolePrompt} onChange={(event) => setStep("rolePrompt", event.target.value)} /></label><label>节点指令<textarea value={selected.instruction} onChange={(event) => setStep("instruction", event.target.value)} /></label><div className="transition-editor"><span>终点 Signals</span>{Object.entries(selected.transitions || {}).map(([signal, target]) => <div className="transition-row" key={signal}><input value={signal} onChange={(event) => updateSignal(signal, event.target.value, target)} /><span>→</span><TUISelect ariaLabel={`${signal} target`} value={target} onChange={(nextTarget) => updateSignal(signal, signal, nextTarget)} options={[{ value: "$done", label: "$done" }, { value: "$pause", label: "$pause" }, { value: "$fail", label: "$fail" }]} /></div>)}</div></> : null}</aside></div><div className={`dag-validation ${validation.length ? "has-errors" : ""}`}><button className="text-button" onClick={validateEditor}>校验 DAG</button>{validation.length ? validation.map((issue) => <span key={`${issue.path}-${issue.code}`}><code>{issue.path}</code>{issue.message}</span>) : <span>保存前会检查环、未知依赖和并行写冲突。</span>}</div></div><div className="editor-footer"><button className="secondary-button" onClick={onClose}>取消</button><button className="primary-button" disabled={busy === "workflow"} onClick={saveWorkflow}>校验并保存 DAG</button></div></Modal>;
}

export default App;
