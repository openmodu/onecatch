import { useCallback, useEffect, useMemo, useState } from "react";
import {
  RuntimeBinding,
  TaskRunBinding,
  WorkflowBinding,
  WorkspaceBinding,
} from "../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";

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

const demoNow = new Date().toISOString();
const demoWorkspaces = [
  { id: "oneshot-demo", name: "oneshot", path: "~/Code/openmodu/oneshot", defaultSandbox: "workspace-write", lastOpenedAt: demoNow },
];
const demoRuntimes = [
  { id: "codex", name: "Codex", available: true, version: "codex 0.98.0" },
  { id: "claude", name: "Claude Code", available: true, version: "2.1.4" },
];
const demoWorkflows = [
  { ...singleTemplate, id: "single_agent", name: "单 Agent 完成" },
  { ...loopTemplate, id: "implement_review", name: "实现与审查 Loop" },
];
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
    { id: "step_1", stepId: "implement", attempt: 1, status: "succeeded", signal: "ready_for_review", content: "完成后端应用服务", startedAt: demoNow, finishedAt: demoNow },
    { id: "step_2", stepId: "review", attempt: 1, status: "succeeded", signal: "changes_requested", content: "要求完善中断反馈", startedAt: demoNow, finishedAt: demoNow },
  ],
  events: [
    { seq: 1, type: "run.started", stepId: "", at: demoNow },
    { seq: 2, type: "transition.applied", stepId: "implement", at: demoNow },
    { seq: 3, type: "transition.applied", stepId: "review", at: demoNow },
    { seq: 4, type: "run.paused", stepId: "implement", at: demoNow },
  ],
  runtimeEvents: [
    { stepRunId: "step_2", seq: 1, kind: "tool_use", text: "go test ./...", at: demoNow },
    { stepRunId: "step_2", seq: 2, kind: "message", text: "核心流程正确，建议补充中断态 UI。", at: demoNow },
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
  return <span className={`status-pill ${status || "ready"}`}>{active && <span className="pulse" />}{statusLabel[status] || status}</span>;
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
  const [workspaceForm, setWorkspaceForm] = useState({ path: "", name: "", defaultSandbox: "workspace-write" });
  const [taskForm, setTaskForm] = useState({ title: "", prompt: "", workflowId: "" });
  const [resumeInstruction, setResumeInstruction] = useState("");
  const [notice, setNotice] = useState(null);
  const [busy, setBusy] = useState("");

  const selectedWorkspace = workspaces.find((item) => item.id === workspaceID);

  const notify = useCallback((type, text) => {
    setNotice({ type, text });
    window.setTimeout(() => setNotice(null), 4200);
  }, []);

  const boot = useCallback(async () => {
    try {
      const [runtimeItems, workspaceItems, workflowItems] = await Promise.all([
        RuntimeBinding.ListRuntimes(), WorkspaceBinding.ListWorkspaces(), WorkflowBinding.ListDefinitions(),
      ]);
      setMode("wails");
      setRuntimes(runtimeItems || []);
      setWorkspaces(workspaceItems || []);
      setWorkflows(workflowItems || []);
      setWorkspaceID((current) => current || workspaceItems?.[0]?.id || "");
      setTaskForm((current) => ({ ...current, workflowId: current.workflowId || workflowItems?.[0]?.id || "" }));
    } catch {
      setMode("demo");
      setRuntimes(demoRuntimes);
      setWorkspaces(demoWorkspaces);
      setWorkflows(demoWorkflows);
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
    try {
      const items = await TaskRunBinding.ListTasks(workspaceID);
      setTasks(items || []);
      const runPairs = await Promise.all((items || []).map(async (task) => [task.id, await TaskRunBinding.ListRunsByTask(task.id)]));
      setRuns(Object.fromEntries(runPairs));
    } catch (error) { notify("error", errorMessage(error)); }
  }, [mode, notify, workspaceID]);

  useEffect(() => {
    if (mode === "wails") loadTasks();
    if (mode === "demo") setTasks(demoTasks.filter((task) => task.workspaceId === workspaceID));
  }, [loadTasks, mode, workspaceID]);

  const loadRun = useCallback(async (runID, silent = false) => {
    if (!runID) return;
    if (mode === "demo") { setRunDetail((current) => current?.run?.id === runID ? current : demoRun); return; }
    try {
      const detail = await TaskRunBinding.GetRun(runID);
      setRunDetail(detail);
      if (!silent) setSelectedRunID(runID);
    } catch (error) { if (!silent) notify("error", errorMessage(error)); }
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
      if (path) { setWorkspaceForm({ path, name: "", defaultSandbox: "workspace-write" }); setWorkspaceModal(true); }
    } catch (error) { notify("error", errorMessage(error)); }
  };

  const addWorkspace = async () => {
    if (!workspaceForm.path.trim()) { notify("error", "请输入工作目录路径"); return; }
    if (workspaceForm.defaultSandbox === "full" && !window.confirm("Full sandbox 会允许 Agent 在工作目录外执行操作。确定继续吗？")) return;
    setBusy("workspace");
    try {
      if (mode === "demo") {
        const item = { ...workspaceForm, id: `workspace-${Date.now()}`, name: workspaceForm.name || workspaceForm.path.split("/").pop(), lastOpenedAt: new Date().toISOString() };
        setWorkspaces((items) => [item, ...items]); setWorkspaceID(item.id);
      } else {
        const item = await WorkspaceBinding.AddWorkspace(workspaceForm);
        setWorkspaces(await WorkspaceBinding.ListWorkspaces()); setWorkspaceID(item.id);
      }
      setWorkspaceModal(false); notify("success", "工作目录已加入");
    } catch (error) { notify("error", errorMessage(error)); } finally { setBusy(""); }
  };

  const createTaskAndRun = async () => {
    if (!workspaceID || !taskForm.title.trim() || !taskForm.prompt.trim() || !taskForm.workflowId) { notify("error", "请填写标题、任务目标并选择 Workflow"); return; }
    const selectedWorkflow = workflows.find((item) => item.id === taskForm.workflowId);
    if (selectedWorkflow?.steps?.some((step) => step.sandbox === "full") && !window.confirm("这个 Workflow 包含 Full sandbox 步骤。确定启动吗？")) return;
    setBusy("run");
    try {
      if (mode === "demo") {
        setSelectedRunID("run_demo"); setRunDetail({ ...demoRun, run: { ...demoRun.run, status: "running" }, active: true });
      } else {
        const task = await TaskRunBinding.CreateTask({ workspaceId: workspaceID, ...taskForm });
        const run = await TaskRunBinding.StartRun(task.id);
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

  const openEditor = (definition) => { setEditor(copy(definition || loopTemplate)); setValidation([]); };

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
    if (editor.steps.some((step) => step.sandbox === "full") && !window.confirm("这个 Workflow 包含 Full sandbox 步骤。确定保存吗？")) return;
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

  if (mode === "loading") return <div className="loading-screen"><div className="brand-mark">1</div><span>正在打开本地工作台…</span></div>;

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand"><div className="brand-mark">1</div><div><strong>Oneshot</strong><span>LOCAL ORCHESTRATOR</span></div></div>
        <div className="sidebar-section-label">工作目录</div>
        <div className="workspace-list">
          {workspaces.map((workspace) => <button key={workspace.id} className={`workspace-item ${workspace.id === workspaceID ? "active" : ""}`} onClick={() => setWorkspaceID(workspace.id)}><span className="workspace-avatar">{workspace.name.slice(0, 1).toUpperCase()}</span><span><strong>{workspace.name}</strong><small>{workspace.path}</small></span></button>)}
          {!workspaces.length && <div className="sidebar-empty">还没有工作目录</div>}
        </div>
        <button className="add-workspace" onClick={chooseWorkspace}><Icon name="plus" /> 加入工作目录</button>
        <nav>
          <button className={view === "tasks" ? "active" : ""} onClick={() => setView("tasks")}><Icon name="tasks" />任务与运行</button>
          <button className={view === "workflows" ? "active" : ""} onClick={() => setView("workflows")}><Icon name="workflow" />Workflow</button>
        </nav>
        <div className="runtime-panel">
          <div className="sidebar-section-label">本地 Runtime</div>
          {runtimes.map((runtime) => <div className="runtime-row" key={runtime.id}><span className={`runtime-dot ${runtime.available ? "online" : "offline"}`} /><span><strong>{runtime.name}</strong><small>{runtime.available ? runtime.version || "可用" : "未安装"}</small></span></div>)}
        </div>
        <div className="storage-note"><span>数据仅保存在本机</span><code>~/.oneshot/</code></div>
      </aside>

      <main className="main-area">
        <header className="topbar"><div><span className="eyebrow">{selectedWorkspace ? selectedWorkspace.path : "选择一个工作目录"}</span><h1>{view === "tasks" ? "任务与运行" : "Workflow 设计"}</h1></div><div className="top-actions"><span className={`connection ${mode}`}>{mode === "wails" ? "本地服务已连接" : "界面预览"}</span><button className="icon-button" title="刷新" onClick={() => { boot(); loadTasks(); }}><Icon name="refresh" /></button></div></header>

        {view === "tasks" ? (
          <div className="workspace-grid">
            <section className="content-column">
              <div className="composer panel">
                <div className="panel-heading"><div><span className="kicker">NEW RUN</span><h2>交给本地 Agent</h2></div><Icon name="branch" size={22} /></div>
                <input className="title-input" placeholder="这次要完成什么？" value={taskForm.title} onChange={(event) => setTaskForm((form) => ({ ...form, title: event.target.value }))} />
                <textarea placeholder="描述目标、约束和验收方式。每个步骤会继承这段任务上下文。" value={taskForm.prompt} onChange={(event) => setTaskForm((form) => ({ ...form, prompt: event.target.value }))} />
                <div className="composer-footer"><label><span>执行流程</span><select value={taskForm.workflowId} onChange={(event) => setTaskForm((form) => ({ ...form, workflowId: event.target.value }))}>{workflows.map((workflow) => <option key={workflow.id} value={workflow.id}>{workflow.name}</option>)}</select></label><button className="primary-button" disabled={busy === "run" || !selectedWorkspace} onClick={createTaskAndRun}><Icon name="play" />{busy === "run" ? "正在启动…" : "启动 Run"}</button></div>
              </div>

              <div className="section-title"><div><h2>最近运行</h2><span>{allRuns.length} runs</span></div></div>
              <div className="run-list">
                {allRuns.map((item) => <button key={item.id} className={`run-card ${selectedRunID === item.id ? "selected" : ""}`} onClick={() => { setSelectedRunID(item.id); loadRun(item.id); }}><div className="run-card-top"><StatusPill status={item.status} /><span>{formatTime(item.updatedAt)}</span></div><h3>{item.task.title}</h3><p>{workflows.find((workflow) => workflow.id === item.workflowId)?.name || item.workflowId}</p><div className="run-meta"><span><Icon name="branch" size={14} /> {item.transitionCount || 0} 次转移</span><code>{shortID(item.id)}</code></div></button>)}
                {!allRuns.length && <div className="empty-state"><div className="empty-icon"><Icon name="play" size={26} /></div><h3>还没有运行记录</h3><p>写下一个任务目标并选择 Workflow，Oneshot 会在后台调度本地 Agent。</p></div>}
              </div>
            </section>

            <aside className="inspector panel">
              {runDetail ? <RunInspector detail={runDetail} workflows={workflows} busy={busy} resumeInstruction={resumeInstruction} setResumeInstruction={setResumeInstruction} runAction={runAction} /> : <div className="inspector-empty"><Icon name="branch" size={30} /><h3>选择一个 Run</h3><p>这里会显示当前步骤、Agent 输出、回边和人工介入操作。</p></div>}
            </aside>
          </div>
        ) : (
          <WorkflowLibrary workflows={workflows} runtimes={runtimes} openEditor={openEditor} />
        )}
      </main>

      {workspaceModal && <Modal title="加入工作目录" subtitle="Agent 只会在你授权的目录中工作" onClose={() => setWorkspaceModal(false)}><div className="form-stack"><label>目录路径<input autoFocus value={workspaceForm.path} onChange={(event) => setWorkspaceForm((form) => ({ ...form, path: event.target.value }))} placeholder="/Users/me/Code/project" /></label><label>显示名称（可选）<input value={workspaceForm.name} onChange={(event) => setWorkspaceForm((form) => ({ ...form, name: event.target.value }))} placeholder="默认使用目录名" /></label><label>默认 Sandbox<select value={workspaceForm.defaultSandbox} onChange={(event) => setWorkspaceForm((form) => ({ ...form, defaultSandbox: event.target.value }))}><option value="read-only">Read only</option><option value="workspace-write">Workspace write</option><option value="full">Full access（危险）</option></select></label><div className="modal-actions"><button className="secondary-button" onClick={() => setWorkspaceModal(false)}>取消</button><button className="primary-button" onClick={addWorkspace} disabled={busy === "workspace"}>加入目录</button></div></div></Modal>}
      {editor && <WorkflowEditor editor={editor} setEditor={setEditor} validation={validation} validateEditor={validateEditor} saveWorkflow={saveWorkflow} busy={busy} updateStep={updateStep} updateTransition={updateTransition} removeTransition={removeTransition} runtimes={runtimes} onClose={() => setEditor(null)} />}
      {notice && <div className={`toast ${notice.type}`}>{notice.type === "success" && <Icon name="check" />}<span>{notice.text}</span></div>}
    </div>
  );
}

function RunInspector({ detail, busy, resumeInstruction, setResumeInstruction, runAction }) {
  const { run, workflow, stepRuns = [], runtimeEvents = [] } = detail;
  const currentStep = workflow.steps?.find((step) => step.id === run.currentStepId);
  const visibleEvents = runtimeEvents.filter((event) => event.text).slice(-8).reverse();
  return <>
    <div className="inspector-header"><div><span className="kicker">RUN INSPECTOR</span><h2>{detail.task.title}</h2></div><StatusPill status={run.status} active={detail.active} /></div>
    <div className="run-id-row"><code>{shortID(run.id)}</code><span>{run.transitionCount || 0} / {workflow.policy?.maxTransitions || 20} 次转移</span></div>
    <div className="current-step"><span>当前步骤</span><div><span className={`runtime-badge ${currentStep?.runtime}`}>{currentStep?.runtime || "—"}</span><strong>{currentStep?.name || run.currentStepId}</strong></div>{run.pauseReason && <p>暂停原因：{run.pauseReason}</p>}{detail.lastError && <p className="error-copy">{detail.lastError}</p>}</div>
    <div className="flow-mini">
      {(workflow.steps || []).map((step, index) => <div className={`flow-step ${step.id === run.currentStepId ? "current" : ""}`} key={step.id}><div className="flow-node"><span>{index + 1}</span></div><div><strong>{step.name}</strong><small>{step.runtime} · {step.sandbox || "workspace-write"}</small></div>{step.id === run.currentStepId && <span className="you-are-here">NOW</span>}</div>)}
    </div>
    <div className="inspector-section"><div className="inspector-section-title"><h3>步骤记录</h3><span>{stepRuns.length}</span></div><div className="timeline">{stepRuns.map((stepRun) => <div className="timeline-item" key={stepRun.id}><span className={`timeline-dot ${stepRun.status}`} /><div><div><strong>{workflow.steps?.find((step) => step.id === stepRun.stepId)?.name || stepRun.stepId}</strong><StatusPill status={stepRun.status} /></div><p>{stepRun.content || stepRun.error || `第 ${stepRun.attempt} 次执行`}</p><small>{formatTime(stepRun.finishedAt || stepRun.startedAt)}{stepRun.signal ? ` · signal: ${stepRun.signal}` : ""}</small></div></div>)}</div></div>
    <div className="inspector-section runtime-stream"><div className="inspector-section-title"><h3>Agent 动态</h3><span>LIVE</span></div>{visibleEvents.map((event) => <div className="event-row" key={`${event.stepRunId}-${event.seq}`}><span>{event.kind}</span><p>{event.text}</p></div>)}{!visibleEvents.length && <p className="muted-copy">Agent 输出会实时保存在当前 StepRun 的 events.jsonl。</p>}</div>
    <div className="run-controls">
      {run.status === "running" && <button className="danger-outline" disabled={busy === "interrupt"} onClick={() => runAction("interrupt")}><Icon name="pause" />打断并暂停</button>}
      {run.status === "paused" && <><textarea value={resumeInstruction} onChange={(event) => setResumeInstruction(event.target.value)} placeholder="补充指令（可选），恢复后注入当前步骤…" /><div><button className="secondary-button danger" disabled={busy === "cancel"} onClick={() => runAction("cancel")}><Icon name="stop" />终止</button><button className="primary-button" disabled={busy === "resume"} onClick={() => runAction("resume")}><Icon name="play" />恢复运行</button></div></>}
      {["completed", "failed", "cancelled"].includes(run.status) && <div className="terminal-note">这个 Run 已结束，完整记录仍保存在本机。</div>}
    </div>
  </>;
}

function WorkflowLibrary({ workflows, runtimes, openEditor }) {
  return <div className="library-page"><div className="library-hero"><div><span className="kicker">SIGNAL-DRIVEN LOOPS</span><h2>把你的工作方式变成 Workflow</h2><p>每一步由一个本地 Agent 执行。Agent 只返回 signal，流程决定下一步；回边就是 loop。</p></div><div className="hero-actions"><button className="secondary-button" onClick={() => openEditor(singleTemplate)}>从单 Agent 开始</button><button className="primary-button" onClick={() => openEditor(loopTemplate)}><Icon name="plus" />创建 Review Loop</button></div></div><div className="workflow-grid">{workflows.map((workflow) => <article className="workflow-card panel" key={workflow.id}><div className="workflow-card-top"><div className="workflow-symbol"><Icon name="workflow" /></div><button className="icon-button" onClick={() => openEditor(workflow)}><Icon name="edit" /></button></div><h3>{workflow.name}</h3><p>{workflow.description || "自定义本地 Agent 调度流程"}</p><div className="workflow-path">{workflow.steps?.map((step, index) => <span key={step.id}><b className={step.runtime}>{step.runtime.slice(0, 1).toUpperCase()}</b><em>{step.name}</em>{index < workflow.steps.length - 1 && <i>→</i>}</span>)}</div><div className="workflow-card-footer"><span>{workflow.steps?.length || 0} steps</span><span>{workflow.policy?.maxTransitions || 20} max transitions</span></div></article>)}</div><div className="runtime-callout panel"><div><h3>Runtime 可用性</h3><p>Workflow 不会自动替换缺失的 runtime，以免破坏角色语义。</p></div>{runtimes.map((runtime) => <span key={runtime.id} className={runtime.available ? "available" : "missing"}><i />{runtime.name} · {runtime.available ? "Ready" : "Missing"}</span>)}</div></div>;
}

function Modal({ title, subtitle, onClose, children, wide = false }) {
  return <div className="modal-backdrop"><div className={`modal ${wide ? "wide" : ""}`}><div className="modal-header"><div><h2>{title}</h2><p>{subtitle}</p></div><button className="icon-button" onClick={onClose}><Icon name="close" /></button></div>{children}</div></div>;
}

function WorkflowEditor({ editor, setEditor, validation, validateEditor, saveWorkflow, busy, updateStep, updateTransition, removeTransition, runtimes, onClose }) {
  const addStep = () => setEditor((current) => ({ ...current, steps: [...current.steps, { id: `step_${current.steps.length + 1}`, name: "新步骤", runtime: "codex", model: "", sandbox: "workspace-write", rolePrompt: "你在这个流程中的角色。", instruction: "描述这个步骤要完成的事情。", transitions: { completed: "$done" } }] }));
  return <Modal wide title="Workflow 编辑器" subtitle="步骤发出 signal，转移表决定继续、回环、暂停或完成" onClose={onClose}><div className="editor-layout"><div className="editor-main"><div className="editor-basics"><label>Workflow ID<input value={editor.id} onChange={(event) => setEditor({ ...editor, id: event.target.value })} /></label><label>名称<input value={editor.name} onChange={(event) => setEditor({ ...editor, name: event.target.value })} /></label><label className="full">描述<textarea value={editor.description || ""} onChange={(event) => setEditor({ ...editor, description: event.target.value })} /></label><label>入口步骤<select value={editor.entryStepId} onChange={(event) => setEditor({ ...editor, entryStepId: event.target.value })}>{editor.steps.map((step) => <option key={step.id} value={step.id}>{step.name}</option>)}</select></label><label>最大转移次数<input type="number" value={editor.policy.maxTransitions} onChange={(event) => setEditor({ ...editor, policy: { ...editor.policy, maxTransitions: Number(event.target.value) } })} /></label></div><div className="editor-section-title"><div><h3>步骤与转移</h3><p>普通 target 指向步骤，保留 target 为 $done / $pause / $fail。</p></div><button className="secondary-button compact" onClick={addStep}><Icon name="plus" />添加步骤</button></div><div className="step-editor-list">{editor.steps.map((step, stepIndex) => <div className="step-editor" key={`${step.id}-${stepIndex}`}><div className="step-editor-number">{stepIndex + 1}</div><div className="step-editor-body"><div className="step-editor-grid"><label>Step ID<input value={step.id} onChange={(event) => updateStep(stepIndex, "id", event.target.value)} /></label><label>显示名称<input value={step.name} onChange={(event) => updateStep(stepIndex, "name", event.target.value)} /></label><label>Runtime<select value={step.runtime} onChange={(event) => updateStep(stepIndex, "runtime", event.target.value)}>{runtimes.map((runtime) => <option key={runtime.id} value={runtime.id}>{runtime.name}{runtime.available ? "" : "（未安装）"}</option>)}</select></label><label>Sandbox<select value={step.sandbox || "workspace-write"} onChange={(event) => updateStep(stepIndex, "sandbox", event.target.value)}><option value="read-only">Read only</option><option value="workspace-write">Workspace write</option><option value="full">Full access</option></select></label><label className="full">角色提示<textarea value={step.rolePrompt} onChange={(event) => updateStep(stepIndex, "rolePrompt", event.target.value)} /></label><label className="full">步骤指令<textarea value={step.instruction} onChange={(event) => updateStep(stepIndex, "instruction", event.target.value)} /></label></div><div className="transition-editor"><span>Signals & targets</span>{Object.entries(step.transitions || {}).map(([signal, target]) => <div className="transition-row" key={signal}><input value={signal} onChange={(event) => updateTransition(stepIndex, signal, event.target.value, target)} /><span>→</span><input value={target} onChange={(event) => updateTransition(stepIndex, signal, signal, event.target.value)} /><button onClick={() => removeTransition(stepIndex, signal)}><Icon name="close" size={14} /></button></div>)}<button className="text-button" onClick={() => updateTransition(stepIndex, `signal_${Object.keys(step.transitions || {}).length + 1}`, `signal_${Object.keys(step.transitions || {}).length + 1}`, "$done")}><Icon name="plus" size={14} />添加 signal</button></div></div></div>)}</div></div><aside className="editor-side"><div className="graph-preview"><span className="kicker">FLOW PREVIEW</span>{editor.steps.map((step, index) => <div className="graph-node" key={step.id}><span>{index + 1}</span><div><strong>{step.name || step.id}</strong><small>{step.runtime}</small></div></div>)}<div className="graph-legend"><span><i className="done" />$done</span><span><i className="pause" />$pause</span><span><i className="loop" />回边 / 下一步</span></div></div><div className={`validation-box ${validation.length ? "has-errors" : "valid"}`}><div><strong>{validation.length ? `${validation.length} 个配置问题` : "等待校验"}</strong><button className="text-button" onClick={validateEditor}>立即校验</button></div>{validation.map((issue, index) => <p key={`${issue.path}-${index}`}><code>{issue.path}</code>{issue.message}</p>)}{!validation.length && <p>保存时会检查入口、可达性、完成路径和 policy 边界。</p>}</div></aside></div><div className="editor-footer"><button className="secondary-button" onClick={onClose}>取消</button><button className="primary-button" disabled={busy === "workflow"} onClick={saveWorkflow}>{busy === "workflow" ? "保存中…" : "校验并保存"}</button></div></Modal>;
}

export default App;
