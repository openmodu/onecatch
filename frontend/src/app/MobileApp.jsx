import { useCallback, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import {
  Bot,
  Check,
  ChevronDown,
  CircleAlert,
  Cpu,
  FolderGit2,
  Link2,
  LoaderCircle,
  MoreHorizontal,
  Play,
  Plus,
  Radio,
  RefreshCw,
  Server,
  Settings2,
  Square,
  Trash2,
  Wifi,
  WifiOff,
  X,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { MobileBinding } from "../../bindings/github.com/openmodu/oneshot/internal/transport/wails/index.js";
import { errorMessage, formatTime, shortID } from "./format.js";
import "../mobile.css";

const RUN_EVENT = "mobile:run";
const RUNTIMES = [
  { id: "codex", label: "Codex" },
  { id: "claude", label: "Claude Code" },
  { id: "modu", label: "Modu Code" },
];

function workerLabel(worker) {
  return worker?.name || worker?.id || "远端 Worker";
}

function workspaceLabel(workspace) {
  const pieces = String(workspace?.path || "").split(/[\\/]/).filter(Boolean);
  return pieces.at(-1) || workspace?.id || "Workspace";
}

function runStatusLabel(status) {
  return { running: "运行中", succeeded: "已完成", failed: "失败" }[status] || status || "等待";
}

function eventLabel(kind) {
  return {
    started: "已连接",
    reasoning: "思考",
    message: "回复",
    tool_use: "调用工具",
    tool_result: "工具结果",
    file_change: "文件变化",
    usage: "用量",
    result: "结果",
    error: "错误",
    permission_request: "等待审批",
    permission_resolved: "审批完成",
  }[kind] || kind;
}

function Notice({ value, onClose }) {
  if (!value) return null;
  return <div className={`mobile-notice ${value.type || "info"}`} role="status">
    <span>{value.message}</span>
    <button type="button" aria-label="关闭" onClick={onClose}><X size={15} /></button>
  </div>;
}

function PairSheet({ open, busy, onClose, onPair }) {
  const [baseURL, setBaseURL] = useState("https://");
  const [code, setCode] = useState("");
  if (!open) return null;
  return <div className="mobile-sheet-backdrop" role="presentation" onPointerDown={(event) => event.target === event.currentTarget && !busy && onClose()}>
    <section className="mobile-sheet" role="dialog" aria-modal="true" aria-labelledby="pair-title">
      <div className="mobile-sheet-handle" />
      <header>
        <div><span className="mobile-kicker">安全配对</span><h2 id="pair-title">连接远端 Worker</h2></div>
        <button type="button" className="mobile-icon-button" aria-label="关闭" disabled={busy} onClick={onClose}><X size={18} /></button>
      </header>
      <p className="mobile-sheet-copy">在远端机器执行 <code>oneshot-worker --pair</code>，然后输入地址和日志中的一次性配对码。</p>
      <form onSubmit={(event) => { event.preventDefault(); void onPair({ baseURL, code }).then((ok) => { if (ok) { setCode(""); onClose(); } }); }}>
        <label><span>Worker 地址</span><Input value={baseURL} inputMode="url" autoCapitalize="none" autoCorrect="off" placeholder="https://192.168.1.20:9231" onChange={(event) => setBaseURL(event.target.value)} /></label>
        <label><span>一次性配对码</span><Input value={code} autoCapitalize="characters" autoCorrect="off" maxLength={16} placeholder="8 位配对码" onChange={(event) => setCode(event.target.value.toUpperCase())} /></label>
        <Button className="mobile-cta" type="submit" disabled={busy || !baseURL.trim() || !code.trim()}>{busy ? <><LoaderCircle className="animate-spin" />正在连接</> : <><Link2 />连接 Worker</>}</Button>
      </form>
      <p className="mobile-security-note">首次 HTTPS 配对会固定服务器证书指纹，之后连接会拒绝证书变化。</p>
    </section>
  </div>;
}

function EmptyWorkers({ onAdd }) {
  return <section className="mobile-empty-card">
    <div className="mobile-empty-icon"><Server size={26} /></div>
    <span className="mobile-kicker">Remote workbench</span>
    <h2>先连接一台 Worker</h2>
    <p>移动端不运行本地 CLI。Codex、Claude Code 和 Git 全部留在你的远端开发机上。</p>
    <Button className="mobile-cta" onClick={onAdd}><Plus />添加 Worker</Button>
  </section>;
}

function WorkerPicker({ workers, selectedID, health, onSelect, onRefresh }) {
  const selected = workers.find((item) => item.id === selectedID);
  return <section className="mobile-worker-strip">
    <div className="mobile-select-wrap">
      <span className={`mobile-online-dot ${health ? "online" : "offline"}`} />
      <select aria-label="远端 Worker" value={selectedID} onChange={(event) => onSelect(event.target.value)}>
        {workers.map((worker) => <option key={worker.id} value={worker.id}>{workerLabel(worker)}</option>)}
      </select>
      <ChevronDown size={15} aria-hidden="true" />
    </div>
    <div className="mobile-worker-meta">
      {health ? <><Wifi size={13} />{health.latencyMilliseconds}ms · 协议 v{health.health?.protocolVersion}</> : <><WifiOff size={13} />未连接</>}
    </div>
    <button type="button" className="mobile-icon-button" aria-label={`刷新 ${selected ? workerLabel(selected) : "Worker"}`} onClick={onRefresh}><RefreshCw size={16} /></button>
  </section>;
}

function WorkspacePicker({ workspaces, selectedID, snapshot, onSelect }) {
  if (!workspaces.length) return <section className="mobile-warning-card">
    <CircleAlert size={18} /><div><strong>没有已准备的 Workspace</strong><p>先在桌面端或 Worker 主机上准备项目，移动端只使用现有映射。</p></div>
  </section>;
  const selected = workspaces.find((item) => item.id === selectedID);
  return <section className="mobile-workspace-card">
    <span className="mobile-kicker">Workspace</span>
    <div className="mobile-field-select">
      <FolderGit2 size={18} />
      <select aria-label="远端 Workspace" value={selectedID} onChange={(event) => onSelect(event.target.value)}>
        {workspaces.map((workspace) => <option key={workspace.id} value={workspace.id}>{workspaceLabel(workspace)}</option>)}
      </select>
      <ChevronDown size={16} />
    </div>
    <div className="mobile-workspace-meta">
      <span title={selected?.path}>{selected?.path || "—"}</span>
      {snapshot?.head && <b>{snapshot.branch || "detached"} · {shortID(snapshot.head)}</b>}
    </div>
  </section>;
}

function RuntimePicker({ health, value, onChange }) {
  const available = health?.health?.runtimes || {};
  return <div className="mobile-runtime-picker" role="radiogroup" aria-label="Agent 运行时">
    {RUNTIMES.map((runtime) => {
      const enabled = Boolean(available[runtime.id]);
      return <button type="button" role="radio" aria-checked={value === runtime.id} disabled={!enabled} className={value === runtime.id ? "selected" : ""} key={runtime.id} onClick={() => onChange(runtime.id)}>
        <Bot size={15} /><span>{runtime.label}</span>{value === runtime.id && <Check size={13} />}
      </button>;
    })}
  </div>;
}

function PermissionCard({ runID, event, busy, onRespond }) {
  const permission = event.permission;
  if (!permission) return null;
  return <article className="mobile-permission-card">
    <header><CircleAlert size={17} /><div><strong>{permission.displayName || permission.title || permission.toolName || "工具权限"}</strong><small>{permission.description || "Claude 正在等待你的决定"}</small></div></header>
    {permission.input && <pre>{JSON.stringify(permission.input, null, 2)}</pre>}
    <div>
      <Button variant="outline" size="sm" disabled={busy} onClick={() => onRespond(runID, permission.id, "deny")}>拒绝</Button>
      {!permission.suppressAlwaysAllow && permission.suggestions?.length > 0 && <Button variant="outline" size="sm" disabled={busy} onClick={() => onRespond(runID, permission.id, "allow_always")}>始终允许</Button>}
      <Button size="sm" disabled={busy} onClick={() => onRespond(runID, permission.id, "allow_once")}>允许一次</Button>
    </div>
  </article>;
}

function EventTimeline({ run, permissionBusy, onRespond }) {
  const events = run?.events || [];
  if (!run) return null;
  return <section className="mobile-run-card">
    <header>
      <div><span className={`mobile-run-pulse ${run.status}`} /><strong>{runStatusLabel(run.status)}</strong></div>
      <small>{formatTime(run.startedAt)}</small>
    </header>
    <div className="mobile-event-list">
      {events.length === 0 && run.status === "running" && <div className="mobile-event-placeholder"><LoaderCircle className="animate-spin" size={16} />等待 Worker 返回事件…</div>}
      {events.map((event, index) => event.kind === "permission_request" ? <PermissionCard key={`${event.at}-${index}`} runID={run.id} event={event} busy={permissionBusy === event.permission?.id} onRespond={onRespond} /> : <article className={`mobile-event ${event.kind || "message"}`} key={`${event.at}-${index}`}>
        <div><span>{eventLabel(event.kind)}</span><time>{formatTime(event.at)}</time></div>
        {event.text && <pre>{event.text}</pre>}
      </article>)}
    </div>
    {run.error && <div className="mobile-run-error"><CircleAlert size={16} />{run.error}</div>}
    {run.result?.finalMessage && <div className="mobile-run-result"><span className="mobile-kicker">最终回复</span><p>{run.result.finalMessage}</p></div>}
  </section>;
}

function RunScreen({ workers, selectedWorkerID, health, workspaces, workspaceID, snapshot, currentRun, busy, permissionBusy, onSelectWorker, onRefreshWorker, onSelectWorkspace, onStart, onInterrupt, onRespondPermission, onAddWorker }) {
  const [runtime, setRuntime] = useState("codex");
  const [prompt, setPrompt] = useState("");
  const [advanced, setAdvanced] = useState(false);
  const [model, setModel] = useState("");
  const [reasoningEffort, setReasoningEffort] = useState("");
  const available = health?.health?.runtimes || {};
  useEffect(() => {
    if (available[runtime]) return;
    const next = RUNTIMES.find((item) => available[item.id]);
    if (next) setRuntime(next.id);
  }, [available, runtime]);
  const canRun = selectedWorkerID && workspaceID && available[runtime] && prompt.trim() && currentRun?.status !== "running" && !busy;

  if (!workers.length) return <EmptyWorkers onAdd={onAddWorker} />;
  return <div className="mobile-screen-body">
    <WorkerPicker workers={workers} selectedID={selectedWorkerID} health={health} onSelect={onSelectWorker} onRefresh={onRefreshWorker} />
    <WorkspacePicker workspaces={workspaces} selectedID={workspaceID} snapshot={snapshot} onSelect={onSelectWorkspace} />
    <section className="mobile-composer-card">
      <div className="mobile-section-heading"><div><span className="mobile-kicker">New analysis</span><h2>交给远端 Agent</h2></div><span className="mobile-readonly-badge">只读</span></div>
      <RuntimePicker health={health} value={runtime} onChange={setRuntime} />
      <Textarea value={prompt} rows={6} placeholder="让 Agent 分析代码、定位问题，或者给出实现方案…" onChange={(event) => setPrompt(event.target.value)} />
      <button type="button" className="mobile-advanced-toggle" aria-expanded={advanced} onClick={() => setAdvanced((value) => !value)}><Settings2 size={14} />高级选项<ChevronDown className={advanced ? "open" : ""} size={14} /></button>
      {advanced && <div className="mobile-advanced-grid">
        <label><span>模型（可选）</span><Input value={model} autoCapitalize="none" placeholder="跟随 Worker 默认" onChange={(event) => setModel(event.target.value)} /></label>
        <label><span>推理强度（可选）</span><Input value={reasoningEffort} autoCapitalize="none" placeholder="medium / high" onChange={(event) => setReasoningEffort(event.target.value)} /></label>
      </div>}
      <div className="mobile-composer-actions">
        {currentRun?.status === "running" ? <Button variant="outline" className="mobile-stop-button" disabled={busy} onClick={() => onInterrupt(currentRun.id)}><Square size={14} />停止运行</Button> : <Button className="mobile-cta" disabled={!canRun} onClick={async () => {
          const started = await onStart({ workerId: selectedWorkerID, workspaceId: workspaceID, runtime, prompt, model, reasoningEffort });
          if (started) setPrompt("");
        }}>{busy ? <LoaderCircle className="animate-spin" /> : <Play />}开始分析</Button>}
      </div>
      <p className="mobile-readonly-note">移动端暂不接收 Git 补丁，因此只开放不会修改远端工作区的分析任务。</p>
    </section>
    <EventTimeline run={currentRun} permissionBusy={permissionBusy} onRespond={onRespondPermission} />
  </div>;
}

function WorkersScreen({ workers, healthByID, busy, onAdd, onRefresh, onDelete }) {
  return <div className="mobile-screen-body">
    <section className="mobile-page-intro"><span className="mobile-kicker">Connections</span><h2>远端 Worker</h2><p>配对信息只保存在当前设备的应用沙箱内。</p></section>
    {!workers.length ? <EmptyWorkers onAdd={onAdd} /> : <div className="mobile-worker-list">
      {workers.map((worker) => {
        const health = healthByID[worker.id];
        return <article className="mobile-worker-card" key={worker.id}>
          <header><div className="mobile-worker-avatar"><Server size={20} /></div><div><strong>{workerLabel(worker)}</strong><span>{worker.baseUrl}</span></div><span className={`mobile-status-chip ${health ? "online" : "offline"}`}>{health ? "在线" : "离线"}</span></header>
          <div className="mobile-worker-stats">
            <span><Radio size={14} />{health ? `${health.latencyMilliseconds}ms` : "—"}</span>
            <span><Cpu size={14} />{health ? RUNTIMES.filter((item) => health.health?.runtimes?.[item.id]).map((item) => item.label).join(" · ") || "无可用运行时" : "等待检查"}</span>
          </div>
          <footer><Button variant="outline" size="sm" disabled={busy} onClick={() => onRefresh(worker.id)}><RefreshCw />检查</Button><Button variant="ghost" size="icon-sm" aria-label={`删除 ${workerLabel(worker)}`} disabled={busy} onClick={() => onDelete(worker)}><Trash2 /></Button></footer>
        </article>;
      })}
    </div>}
    <Button variant="outline" className="mobile-add-worker" onClick={onAdd}><Plus />添加 Worker</Button>
  </div>;
}

export default function MobileWorkbench() {
  const [tab, setTab] = useState("run");
  const [workers, setWorkers] = useState([]);
  const [selectedWorkerID, setSelectedWorkerID] = useState("");
  const [healthByID, setHealthByID] = useState({});
  const [workspaces, setWorkspaces] = useState([]);
  const [workspaceID, setWorkspaceID] = useState("");
  const [snapshot, setSnapshot] = useState(null);
  const [currentRun, setCurrentRun] = useState(null);
  const [pairOpen, setPairOpen] = useState(false);
  const [busy, setBusy] = useState("");
  const [permissionBusy, setPermissionBusy] = useState("");
  const [notice, setNotice] = useState(null);

  const notify = useCallback((type, message) => {
    setNotice({ type, message });
    window.setTimeout(() => setNotice((current) => current?.message === message ? null : current), 4200);
  }, []);

  const loadWorkers = useCallback(async () => {
    const items = await MobileBinding.ListWorkers();
    setWorkers(items || []);
    setSelectedWorkerID((current) => items?.some((item) => item.id === current) ? current : items?.[0]?.id || "");
    return items || [];
  }, []);

  const refreshWorker = useCallback(async (id = selectedWorkerID, silent = false) => {
    if (!id) return null;
    try {
      const value = await MobileBinding.CheckWorker(id);
      setHealthByID((current) => ({ ...current, [id]: value }));
      return value;
    } catch (error) {
      setHealthByID((current) => { const next = { ...current }; delete next[id]; return next; });
      if (!silent) notify("error", errorMessage(error));
      return null;
    }
  }, [notify, selectedWorkerID]);

  const loadWorkspaces = useCallback(async (workerID) => {
    if (!workerID) { setWorkspaces([]); setWorkspaceID(""); return []; }
    try {
      const items = await MobileBinding.ListWorkspaces(workerID);
      setWorkspaces(items || []);
      setWorkspaceID((current) => items?.some((item) => item.id === current) ? current : items?.[0]?.id || "");
      return items || [];
    } catch (error) {
      setWorkspaces([]); setWorkspaceID(""); notify("error", errorMessage(error)); return [];
    }
  }, [notify]);

  useEffect(() => {
    void (async () => {
      try {
        const [items, runs] = await Promise.all([loadWorkers(), MobileBinding.ListRuns()]);
        const latest = [...(runs || [])].sort((a, b) => String(b.startedAt).localeCompare(String(a.startedAt)))[0];
        if (latest) setCurrentRun(latest);
        if (items[0]) await refreshWorker(items[0].id, true);
      } catch (error) { notify("error", errorMessage(error)); }
    })();
  }, [loadWorkers, notify, refreshWorker]);

  useEffect(() => { if (selectedWorkerID) void loadWorkspaces(selectedWorkerID); }, [loadWorkspaces, selectedWorkerID]);
  useEffect(() => {
    if (!selectedWorkerID || !workspaceID) { setSnapshot(null); return; }
    void MobileBinding.WorkspaceGitStatus(selectedWorkerID, workspaceID).then(setSnapshot).catch((error) => { setSnapshot(null); notify("error", errorMessage(error)); });
  }, [notify, selectedWorkerID, workspaceID]);

  useEffect(() => {
    const off = Events.On(RUN_EVENT, (event) => {
      const frame = event.data;
      if (!frame?.runId) return;
      setCurrentRun((current) => {
        if (!current || current.id !== frame.runId) return current;
        const next = { ...current };
        if (frame.event) next.events = [...(current.events || []), frame.event];
        if (frame.status) next.status = frame.status;
        if (frame.result) next.result = frame.result;
        if (frame.error) next.error = frame.error;
        return next;
      });
      if (frame.status && frame.status !== "running") {
        void MobileBinding.GetRun(frame.runId).then(setCurrentRun).catch(() => {});
      }
    });
    return () => off();
  }, []);

  const pairWorker = async ({ baseURL, code }) => {
    setBusy("pair");
    try {
      const worker = await MobileBinding.PairWorker(baseURL.trim(), code.trim());
      await loadWorkers(); setSelectedWorkerID(worker.id); await refreshWorker(worker.id, true);
      notify("success", `已连接 ${workerLabel(worker)}`); setTab("run"); return true;
    } catch (error) { notify("error", errorMessage(error)); return false; }
    finally { setBusy(""); }
  };

  const startRun = async (input) => {
    setBusy("run");
    try { const run = await MobileBinding.StartRun(input); setCurrentRun(run); return true; }
    catch (error) { notify("error", errorMessage(error)); return false; }
    finally { setBusy(""); }
  };

  const interruptRun = async (runID) => {
    setBusy("interrupt");
    try { await MobileBinding.InterruptRun(runID); notify("info", "已请求 Worker 停止运行"); }
    catch (error) { notify("error", errorMessage(error)); }
    finally { setBusy(""); }
  };

  const respondPermission = async (runID, requestID, decision) => {
    setPermissionBusy(requestID);
    try { await MobileBinding.RespondPermission({ runId: runID, requestId: requestID, decision }); }
    catch (error) { notify("error", errorMessage(error)); }
    finally { setPermissionBusy(""); }
  };

  const deleteWorker = async (worker) => {
    if (!window.confirm(`删除 ${workerLabel(worker)} 的本地配对信息？`)) return;
    setBusy("delete");
    try { await MobileBinding.DeleteWorker(worker.id); setHealthByID((current) => { const next = { ...current }; delete next[worker.id]; return next; }); await loadWorkers(); }
    catch (error) { notify("error", errorMessage(error)); }
    finally { setBusy(""); }
  };

  const selectedHealth = healthByID[selectedWorkerID] || null;
  const title = tab === "run" ? "工作台" : "Worker";
  return <div className="mobile-app-shell">
    <header className="mobile-topbar"><div><span className="mobile-brand-mark">1</span><strong>Oneshot</strong></div><span>{title}</span><button type="button" className="mobile-icon-button" aria-label="更多"><MoreHorizontal size={19} /></button></header>
    <main>
      {tab === "run" ? <RunScreen workers={workers} selectedWorkerID={selectedWorkerID} health={selectedHealth} workspaces={workspaces} workspaceID={workspaceID} snapshot={snapshot} currentRun={currentRun} busy={busy} permissionBusy={permissionBusy} onSelectWorker={(id) => { setSelectedWorkerID(id); void refreshWorker(id, true); }} onRefreshWorker={() => refreshWorker()} onSelectWorkspace={setWorkspaceID} onStart={startRun} onInterrupt={interruptRun} onRespondPermission={respondPermission} onAddWorker={() => setPairOpen(true)} /> : <WorkersScreen workers={workers} healthByID={healthByID} busy={busy} onAdd={() => setPairOpen(true)} onRefresh={refreshWorker} onDelete={deleteWorker} />}
    </main>
    <nav className="mobile-bottom-nav" aria-label="主导航"><button type="button" className={tab === "run" ? "active" : ""} onClick={() => setTab("run")}><Play size={19} /><span>运行</span></button><button type="button" className={tab === "workers" ? "active" : ""} onClick={() => setTab("workers")}><Server size={19} /><span>Worker</span></button></nav>
    <Notice value={notice} onClose={() => setNotice(null)} />
    <PairSheet open={pairOpen} busy={busy === "pair"} onClose={() => setPairOpen(false)} onPair={pairWorker} />
  </div>;
}
