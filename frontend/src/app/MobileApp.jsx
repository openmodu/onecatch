import { useCallback, useEffect, useMemo, useState } from "react";
import { Events } from "@wailsio/runtime";
import {
  Activity,
  ArrowDownUp,
  ArrowLeft,
  Bot,
  Check,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  Clock3,
  Cloud,
  Cpu,
  Folder,
  FolderGit2,
	GitBranch,
	HardDrive,
  Link2,
  LoaderCircle,
  Menu,
  MessageCircle,
  MoreHorizontal,
  PanelLeftClose,
	Pencil,
  PenLine,
  Plus,
  RefreshCw,
  Search,
  Send,
  Server,
  Settings2,
  Square,
  Trash2,
  Wifi,
  Wrench,
  X,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { MobileBinding } from "../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import MarkdownContent from "./components/MarkdownContent.jsx";
import { errorMessage, formatTime } from "./format.js";
import { applyMobileRunFrame, foldMobileEvents, groupMobileConversations, mergeMobileRun } from "./mobileRuns.js";
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
  if (workspace?.name) return workspace.name;
  const pieces = String(workspace?.path || "").split(/[\\/]/).filter(Boolean);
  return pieces.at(-1) || workspace?.id || "Workspace";
}

function workspaceGitLabel(snapshot) {
  if (!snapshot) return "尚未检查";
  if (!snapshot.isRepo) return "不是 Git 仓库";
  if (snapshot.status || (snapshot.files || []).length) return `${(snapshot.files || []).length || 1} 项未提交变更`;
  return snapshot.branch || snapshot.head?.slice(0, 8) || "Git 工作区";
}

function runStatusLabel(status) {
  return { running: "进行中", succeeded: "已完成", failed: "失败" }[status] || "等待";
}

function eventLabel(kind) {
  return {
    reasoning: "思考过程",
    tool_use: "调用工具",
    tool_result: "工具结果",
    file_change: "文件变化",
    usage: "用量",
    error: "错误",
    started: "已连接 Worker",
  }[kind] || kind;
}

function relativeTime(value) {
  const date = new Date(value);
  const distance = Date.now() - date.getTime();
  if (!Number.isFinite(distance)) return "";
  if (distance < 60_000) return "刚刚";
  if (distance < 3_600_000) return `${Math.floor(distance / 60_000)} 分钟前`;
  if (distance < 86_400_000) return `${Math.floor(distance / 3_600_000)} 小时前`;
  if (distance < 604_800_000) return `${Math.floor(distance / 86_400_000)} 天前`;
  return date.toLocaleDateString("zh-CN", { month: "numeric", day: "numeric" });
}

function Notice({ value, onClose }) {
  if (!value) return null;
  return <div className={`mobile-notice ${value.type || "info"}`} role="status">
    <span>{value.message}</span>
    <button type="button" aria-label="关闭" onClick={onClose}><X size={16} /></button>
  </div>;
}

function StatusDot({ online, running = false }) {
  return <span className={`mobile-status-dot ${online ? "online" : ""} ${running ? "running" : ""}`} aria-hidden="true" />;
}

function PairSheet({ open, busy, initialURL = "https://", onClose, onPair }) {
  const [baseURL, setBaseURL] = useState(initialURL || "https://");
  const [code, setCode] = useState("");
  useEffect(() => { if (open) setBaseURL(initialURL || "https://"); }, [initialURL, open]);
  if (!open) return null;
  return <div className="mobile-sheet-backdrop" role="presentation" onPointerDown={(event) => event.target === event.currentTarget && !busy && onClose()}>
    <section className="mobile-sheet" role="dialog" aria-modal="true" aria-labelledby="pair-title">
      <div className="mobile-sheet-handle" />
      <header><div><small>安全配对</small><h2 id="pair-title">连接远端 Worker</h2></div><button type="button" className="mobile-icon-button" aria-label="关闭" disabled={busy} onClick={onClose}><X /></button></header>
      <p className="mobile-sheet-copy">在远端执行 <code>onecatch-worker --pair</code>。配对码 10 分钟内有效且只能使用一次，失效后重新执行命令即可。</p>
      <form onSubmit={(event) => { event.preventDefault(); void onPair({ baseURL, code }).then((ok) => { if (ok) { setCode(""); onClose(); } }); }}>
        <label><span>Worker 地址</span><Input value={baseURL} inputMode="url" autoCapitalize="none" autoCorrect="off" placeholder="https://192.168.1.20:9231" onChange={(event) => setBaseURL(event.target.value)} /></label>
        <label><span>一次性配对码</span><Input value={code} autoCapitalize="characters" autoCorrect="off" maxLength={16} placeholder="例如 ABCD-EFGH" onChange={(event) => setCode(event.target.value.toUpperCase())} /></label>
		<Button className="mobile-main-action" type="submit" disabled={busy || !baseURL.trim() || !code.trim()}>{busy ? <><LoaderCircle className="animate-spin" />正在连接</> : <><Link2 />连接 Worker</>}</Button>
      </form>
      <p className="mobile-security-note">首次 HTTPS 配对会固定服务器证书指纹，之后会拒绝证书变化。</p>
    </section>
  </div>;
}

function EmptyConnection({ onPair }) {
  return <section className="mobile-empty-state">
    <span className="mobile-empty-symbol"><Cloud /></span>
    <h2>连接你的开发机</h2>
    <p>iPhone 只作为工作台。代码、Git 和 Agent 都在远端 Worker 上运行。</p>
	<Button className="mobile-main-action" onClick={onPair}><Link2 />连接 Worker</Button>
  </section>;
}

function Header({ title, subtitle, onMenu, onBack, onMore, onNew }) {
  return <header className="mobile-topbar">
    <button type="button" className="mobile-round-button" aria-label={onBack ? "返回" : "打开侧栏"} onClick={onBack || onMenu}>{onBack ? <ArrowLeft /> : <Menu />}</button>
    <div className="mobile-topbar-title"><strong>{title}</strong>{subtitle && <span>{subtitle}</span>}</div>
    <button type="button" className="mobile-round-button" aria-label={onNew ? "新建会话" : "更多"} onClick={onNew || onMore}>{onNew ? <PenLine /> : <MoreHorizontal />}</button>
  </header>;
}

function BottomDock({ query, setQuery, onWorker, onNew, workerOnline }) {
  return <footer className="mobile-bottom-dock">
    <label className="mobile-search-pill"><Search /><input value={query} placeholder="搜索聊天记录" aria-label="搜索聊天记录" onChange={(event) => setQuery(event.target.value)} /></label>
    <button type="button" className="mobile-dark-button" aria-label="Worker 管理" onClick={onWorker}><Activity /><StatusDot online={workerOnline} /></button>
    <button type="button" className="mobile-dark-button" aria-label="新建会话" onClick={onNew}><PenLine /></button>
  </footer>;
}

function ProjectHome({ workspaces, conversations, query, onOpenWorkspace, onNew, onManage }) {
  const normalized = query.trim().toLowerCase();
  const visible = workspaces.filter((workspace) => {
    if (!normalized) return true;
    const sessions = conversations.filter((item) => item.workspaceId === workspace.id);
    return workspaceLabel(workspace).toLowerCase().includes(normalized) || sessions.some((item) => item.title.toLowerCase().includes(normalized));
  });
  return <div className="mobile-page mobile-project-page">
    <h1>项目</h1>
    <div className="mobile-project-list">
      {visible.map((workspace) => {
        const sessions = conversations.filter((item) => item.workspaceId === workspace.id);
        const running = sessions.some((item) => item.status === "running");
        return <div className="mobile-project-row" key={workspace.id}>
          <button type="button" className="mobile-project-link" onClick={() => onOpenWorkspace(workspace.id)}>
            <Folder /><strong>{workspaceLabel(workspace)}</strong><ChevronRight />{running && <StatusDot online running />}
          </button>
          <button type="button" className="mobile-compose-project" aria-label={`在 ${workspaceLabel(workspace)} 新建会话`} onClick={() => onNew(workspace.id)}><PenLine /></button>
        </div>;
      })}
    </div>
	{!visible.length && (workspaces.length ? <div className="mobile-list-empty"><Search /><p>没有匹配的项目或会话</p></div> : <section className="mobile-list-empty"><FolderGit2 /><h2>还没有 Workspace</h2><p>从 Git 仓库克隆，或绑定 Worker 上已有的干净工作区。</p><Button className="mobile-main-action" onClick={onManage}><Plus />创建 Workspace</Button></section>)}
  </div>;
}

function SessionList({ workspace, conversations, query, onOpen, onNew }) {
  const normalized = query.trim().toLowerCase();
  const visible = conversations.filter((item) => item.workspaceId === workspace?.id && (!normalized || item.title.toLowerCase().includes(normalized)));
  return <div className="mobile-page mobile-session-page">
    <div className="mobile-workspace-heading"><FolderGit2 /><div><h1>{workspaceLabel(workspace)}</h1><span>{workspace?.path}</span></div></div>
    <div className="mobile-session-list">
      {visible.map((conversation) => <button type="button" className="mobile-session-row" key={conversation.id} onClick={() => onOpen(conversation.id)}>
        <span className="mobile-session-icon"><MessageCircle /></span>
        <span className="mobile-session-copy"><strong>{conversation.title}</strong><small>{conversation.runtime} · {conversation.runs.length} 轮 · {relativeTime(conversation.startedAt)}</small></span>
        <span className={`mobile-session-status ${conversation.status}`}>{runStatusLabel(conversation.status)}</span><ChevronRight />
      </button>)}
    </div>
	{!visible.length && <section className="mobile-list-empty"><MessageCircle /><h2>还没有会话</h2><p>从一个明确的问题开始，后续可以在同一 session 里继续追问。</p><Button className="mobile-main-action" onClick={onNew}><Plus />新建会话</Button></section>}
  </div>;
}

function WorkspaceManagerPage({ workspaces, statusByID, managementSupported, busy, onOpen, onCreate, onEdit, onRefresh }) {
  if (!managementSupported) return <section className="mobile-page mobile-workspace-manager"><div className="mobile-list-empty"><CircleAlert /><h2>当前 Worker 不支持管理</h2><p>请更新并重启远端 Worker；已有 Workspace 仍然可以正常打开。</p></div></section>;
  return <section className="mobile-page mobile-workspace-manager">
    <header className="mobile-section-heading"><div><h1>Workspace</h1><p>管理当前 Worker 上的代码工作区</p></div><Button size="sm" onClick={onCreate}><Plus />新建</Button></header>
    <div className="mobile-workspace-manage-list">
      {workspaces.map((workspace) => {
        const state = statusByID[workspace.id];
        const snapshot = state?.snapshot;
        const dirty = Boolean(snapshot && (snapshot.status || (snapshot.files || []).length));
        return <article className="mobile-workspace-manage-card" key={workspace.id}>
          <header><span className="mobile-workspace-manage-icon"><FolderGit2 /></span><div><strong>{workspaceLabel(workspace)}</strong><small>{workspace.id}</small></div><span className={`mobile-git-state ${dirty ? "dirty" : snapshot?.isRepo ? "clean" : ""}`}>{state?.loading ? "检查中" : state?.error ? "不可用" : workspaceGitLabel(snapshot)}</span></header>
          <div className="mobile-workspace-manage-meta"><span><HardDrive />{workspace.path}</span>{workspace.remoteUrl && <span><GitBranch />{workspace.remoteUrl}</span>}</div>
          <footer><Button variant="outline" size="sm" disabled={Boolean(busy)} onClick={() => onOpen(workspace.id)}>打开</Button><Button variant="outline" size="icon-sm" aria-label={`刷新 ${workspaceLabel(workspace)}`} disabled={Boolean(busy) || state?.loading} onClick={() => onRefresh(workspace)}><RefreshCw className={state?.loading ? "animate-spin" : ""} /></Button><Button variant="ghost" size="icon-sm" aria-label={`编辑 ${workspaceLabel(workspace)}`} disabled={Boolean(busy)} onClick={() => onEdit(workspace)}><Pencil /></Button></footer>
        </article>;
      })}
    </div>
	{!workspaces.length && <div className="mobile-list-empty"><FolderGit2 /><h2>创建第一个 Workspace</h2><p>Worker 可以克隆 Git 仓库，也可以安全绑定已有的干净目录。</p><Button className="mobile-main-action" onClick={onCreate}><Plus />创建 Workspace</Button></div>}
  </section>;
}

function WorkspaceEditorSheet({ workspace, busy, onClose, onSave, onDelete }) {
  const [name, setName] = useState("");
  const [id, setID] = useState("");
  const [source, setSource] = useState("clone");
  const [remoteURL, setRemoteURL] = useState("");
  const [revision, setRevision] = useState("HEAD");
  const [path, setPath] = useState("");
  const [deleteFiles, setDeleteFiles] = useState(false);
  useEffect(() => {
    if (workspace === undefined) return;
	const nextSource = workspace && !workspace.managed ? "existing" : "clone";
    setName(workspace?.name || workspace?.id || "");
    setID(workspace?.id || "");
	setSource(nextSource);
    setRemoteURL(workspace?.remoteUrl || "");
	setRevision(nextSource === "clone" ? workspace?.revision || "HEAD" : "");
    setPath(workspace?.path || "");
    setDeleteFiles(false);
  }, [workspace]);
  if (workspace === undefined) return null;
  const editing = Boolean(workspace);
  const canSave = name.trim() && id.trim() && (source === "clone" ? remoteURL.trim() : path.trim());
  return <div className="mobile-sheet-backdrop" onPointerDown={(event) => event.target === event.currentTarget && !busy && onClose()}>
    <section className="mobile-sheet mobile-workspace-editor" role="dialog" aria-modal="true" aria-labelledby="workspace-editor-title">
      <div className="mobile-sheet-handle" />
      <header><div><small>Remote Workspace</small><h2 id="workspace-editor-title">{editing ? "编辑 Workspace" : "新建 Workspace"}</h2></div><button type="button" className="mobile-icon-button" aria-label="关闭" disabled={Boolean(busy)} onClick={onClose}><X /></button></header>
      <form onSubmit={(event) => { event.preventDefault(); void onSave({ id: id.trim(), name: name.trim(), path: source === "existing" ? path.trim() : editing && !workspace.managed ? workspace.path : "", remoteUrl: source === "clone" ? remoteURL.trim() : "", revision: revision.trim() }); }}>
        <label><span>显示名称</span><Input value={name} placeholder="例如 OneCatch" onChange={(event) => setName(event.target.value)} /></label>
        <label><span>Workspace ID</span><Input value={id} disabled={editing} autoCapitalize="none" autoCorrect="off" placeholder="onecatch" pattern="[A-Za-z0-9_-]{1,128}" onChange={(event) => setID(event.target.value.replace(/[^A-Za-z0-9_-]/g, ""))} /></label>
		<div className="mobile-workspace-source" role="radiogroup" aria-label="Workspace 来源"><button type="button" className={source === "clone" ? "selected" : ""} role="radio" aria-checked={source === "clone"} disabled={editing} onClick={() => { setSource("clone"); setRevision((value) => value || "HEAD"); }}><Cloud />克隆 Git 仓库</button><button type="button" className={source === "existing" ? "selected" : ""} role="radio" aria-checked={source === "existing"} disabled={editing} onClick={() => { setSource("existing"); setRevision(""); }}><HardDrive />绑定已有目录</button></div>
        {source === "clone" ? <label><span>Git 地址</span><Input value={remoteURL} inputMode="url" autoCapitalize="none" autoCorrect="off" placeholder="git@github.com:org/repo.git" onChange={(event) => setRemoteURL(event.target.value)} /></label> : <label><span>Worker 绝对路径</span><Input value={path} autoCapitalize="none" autoCorrect="off" placeholder="/Users/me/Code/project" onChange={(event) => setPath(event.target.value)} /></label>}
		<label><span>分支、标签或 Commit</span><Input value={revision} autoCapitalize="none" autoCorrect="off" placeholder={source === "clone" ? "HEAD" : "留空保持当前提交"} onChange={(event) => setRevision(event.target.value)} /></label>
        <p className="mobile-context-note">创建和切换前会检查 Git 状态；存在未提交变更时 Worker 会拒绝操作。</p>
		<Button className="mobile-main-action" type="submit" disabled={Boolean(busy) || !canSave}>{busy === "workspace-save" ? <><LoaderCircle className="animate-spin" />正在保存</> : "保存 Workspace"}</Button>
      </form>
      {editing && <div className="mobile-workspace-danger-zone">
        {workspace.managed && <button type="button" className={`mobile-delete-toggle ${deleteFiles ? "selected" : ""}`} role="checkbox" aria-checked={deleteFiles} onClick={() => setDeleteFiles((value) => !value)}><span>{deleteFiles && <Check />}</span><div><strong>同时删除远端克隆</strong><small>仅允许删除由 Worker 创建且没有未提交变更的副本</small></div></button>}
        <Button variant="destructive" disabled={Boolean(busy)} onClick={() => onDelete(workspace, deleteFiles)}>{busy === "workspace-delete" ? <LoaderCircle className="animate-spin" /> : <Trash2 />}{deleteFiles ? "删除 Workspace 和文件" : "移除 Workspace"}</Button>
      </div>}
    </section>
  </div>;
}

function PermissionCard({ runID, event, busy, onRespond }) {
  const permission = event.permission;
  if (!permission) return null;
  return <article className="mobile-permission-card">
    <header><CircleAlert /><div><strong>{permission.displayName || permission.title || permission.toolName || "工具权限"}</strong><small>{permission.description || "Agent 正在等待你的决定"}</small></div></header>
    {permission.input && <pre>{JSON.stringify(permission.input, null, 2)}</pre>}
    <footer><Button variant="outline" size="sm" disabled={busy} onClick={() => onRespond(runID, permission.id, "deny")}>拒绝</Button><Button size="sm" disabled={busy} onClick={() => onRespond(runID, permission.id, "allow_once")}>允许一次</Button></footer>
  </article>;
}

function AgentEvent({ run, event, index, permissionBusy, onRespond }) {
  if (event.kind === "permission_request") return <PermissionCard runID={run.id} event={event} busy={permissionBusy === event.permission?.id} onRespond={onRespond} />;
  if (event.kind === "message") return event.text ? <div className="mobile-assistant-message"><span className="mobile-agent-mark"><Bot /></span><MarkdownContent content={event.text} streaming={Boolean(event.streaming || (run.status === "running" && index === run.events.length - 1))} /></div> : null;
  if (!event.text && !event.kind) return null;
  return <details className={`mobile-event-detail ${event.kind === "error" ? "error" : ""}`}>
    <summary><span>{event.kind?.startsWith("tool") ? <Wrench /> : <Clock3 />}{eventLabel(event.kind)}</span><ChevronDown /></summary>
    {event.text && <pre>{event.text}</pre>}
  </details>;
}

function ConversationView({ conversation, workspace, snapshot, prompt, setPrompt, busy, permissionBusy, runtime, onOpenContext, onStart, onInterrupt, onRespond }) {
  const runs = conversation?.runs || [];
  const running = runs.find((item) => item.status === "running");
  const latest = runs.at(-1);
  const canSend = prompt.trim() && !running && !busy && snapshot?.isRepo && !snapshot?.status && !(snapshot?.files || []).length;
  return <div className="mobile-conversation-shell">
    <main className={`mobile-transcript ${runs.length ? "has-messages" : "empty"}`}>
      {!runs.length && <section className="mobile-chat-empty"><span className="mobile-chat-mark">1</span><h1>想让远端 Agent 做什么？</h1><p>当前工作区：{workspaceLabel(workspace)}</p></section>}
      {runs.map((run) => {
        const visibleEvents = foldMobileEvents(run.events || []);
        return <section className="mobile-turn" key={run.id}>
        <div className="mobile-user-message"><MarkdownContent content={run.prompt} /></div>
        <div className="mobile-turn-meta"><span>{String(run.runtime || "agent")}</span><time>{formatTime(run.startedAt)}</time></div>
        {visibleEvents.map((event, index) => <AgentEvent key={`${event.at || index}-${index}`} run={{ ...run, events: visibleEvents }} event={event} index={index} permissionBusy={permissionBusy} onRespond={onRespond} />)}
        {run.status === "running" && !visibleEvents.some((event) => event.text) && <div className="mobile-thinking"><LoaderCircle className="animate-spin" />正在连接远端 Agent…</div>}
        {run.error && <div className="mobile-run-error"><CircleAlert />{run.error}</div>}
        {run.result?.finalMessage && !visibleEvents.some((event) => event.kind === "message" && event.text === run.result.finalMessage) && <div className="mobile-assistant-message final"><span className="mobile-agent-mark"><Bot /></span><MarkdownContent content={run.result.finalMessage} /></div>}
      </section>;
      })}
    </main>
    <footer className="mobile-composer-wrap">
      {snapshot && (!snapshot.isRepo || snapshot.status || (snapshot.files || []).length) && <div className="mobile-workspace-alert"><CircleAlert />远端工作区需要保持干净才能开始只读任务</div>}
      <div className="mobile-composer">
        <Textarea value={prompt} rows={1} placeholder={runs.length ? "继续追问…" : "给 Agent 发送任务"} onChange={(event) => setPrompt(event.target.value)} />
        <div className="mobile-composer-footer"><button type="button" className="mobile-context-button" onClick={onOpenContext}><Plus /><span>{workspaceLabel(workspace)}</span><b>{runtime}</b></button>
          {running ? <button type="button" className="mobile-send-button stop" aria-label="停止" disabled={busy} onClick={() => onInterrupt(running.id)}><Square /></button> : <button type="button" className="mobile-send-button" aria-label="发送" disabled={!canSend} onClick={() => onStart({ resumeSessionId: latest?.result?.sessionId || "" })}>{busy ? <LoaderCircle className="animate-spin" /> : <Send />}</button>}
        </div>
      </div>
      <p>Agent 在远端以只读模式运行</p>
    </footer>
  </div>;
}

function Sidebar({ open, workspaces, conversations, selectedConversationID, health, onClose, onHome, onWorkspace, onConversation, onNew, onWorkers }) {
  if (!open) return null;
  return <div className="mobile-drawer-backdrop" onPointerDown={(event) => event.target === event.currentTarget && onClose()}>
    <aside className="mobile-drawer" aria-label="工作区与会话">
      <header><div><span className="mobile-brand-mark">1</span><strong>OneCatch</strong></div><button type="button" className="mobile-icon-button" aria-label="关闭侧栏" onClick={onClose}><PanelLeftClose /></button></header>
      <button type="button" className="mobile-new-session" onClick={() => { onNew(); onClose(); }}><PenLine />新建会话</button>
      <button type="button" className="mobile-drawer-home" onClick={() => { onHome(); onClose(); }}><Folder />全部项目<ChevronRight /></button>
      <div className="mobile-drawer-scroll">
        {workspaces.map((workspace) => {
          const sessions = conversations.filter((item) => item.workspaceId === workspace.id);
          return <section className="mobile-drawer-group" key={workspace.id}>
            <button type="button" className="mobile-drawer-workspace" onClick={() => { onWorkspace(workspace.id); onClose(); }}><FolderGit2 /><strong>{workspaceLabel(workspace)}</strong><span>{sessions.length}</span></button>
            {sessions.slice(0, 8).map((conversation) => <button type="button" className={conversation.id === selectedConversationID ? "selected" : ""} key={conversation.id} onClick={() => { onConversation(conversation.id); onClose(); }}><span className="mobile-drawer-session-title">{conversation.title}</span>{conversation.status === "running" && <StatusDot online running />}</button>)}
          </section>;
        })}
      </div>
      <button type="button" className="mobile-drawer-worker" onClick={() => { onWorkers(); onClose(); }}><span className="mobile-worker-glyph"><Server /></span><span><strong>{health?.worker?.name || "远端 Worker"}</strong><small>{health ? `在线 · ${health.latencyMilliseconds}ms` : "未连接"}</small></span><StatusDot online={Boolean(health)} /><ChevronRight /></button>
    </aside>
  </div>;
}

function MoreMenu({ open, sortMode, health, onSort, onWorkspaces, onWorkers, onPair, onSettings, onClose }) {
  if (!open) return null;
  return <div className="mobile-popover-backdrop" onPointerDown={(event) => event.target === event.currentTarget && onClose()}>
    <section className="mobile-more-menu">
      <small>整理</small>
      <button type="button" onClick={() => { onSort("project"); onClose(); }}>{sortMode === "project" ? <Check /> : <span />}<Folder />按项目</button>
      <button type="button" onClick={() => { onSort("recent"); onClose(); }}>{sortMode === "recent" ? <Check /> : <span />}<ArrowDownUp />按时间倒序排列</button>
      <hr />
      <small>管理</small>
	  <button type="button" onClick={() => { onWorkspaces(); onClose(); }}><span /><FolderGit2 />Workspace 管理</button>
      <button type="button" onClick={() => { onWorkers(); onClose(); }}><span /><Cloud />Worker 管理</button>
      <button type="button" onClick={() => { onPair(); onClose(); }}><span /><Link2 />添加连接</button>
      <button type="button" onClick={() => { onSettings(); onClose(); }}><span /><Settings2 />运行设置</button>
      <hr />
      <div className="mobile-menu-status"><small>当前连接</small><strong><StatusDot online={Boolean(health)} />{health?.worker?.name || "远端 Worker"}</strong><span>{health ? `${health.latencyMilliseconds}ms · ${RUNTIMES.filter((item) => health.health?.runtimes?.[item.id]).map((item) => item.label).join(" · ") || "无可用运行时"}` : "离线"}</span></div>
    </section>
  </div>;
}

function ContextSheet({ open, workers, selectedWorkerID, workspaces, workspaceID, health, runtime, runtimeLocked, model, reasoningEffort, onClose, onSelectWorker, onSelectWorkspace, onRuntime, onModel, onReasoning }) {
  if (!open) return null;
  const available = health?.health?.runtimes || {};
  return <div className="mobile-sheet-backdrop" onPointerDown={(event) => event.target === event.currentTarget && onClose()}>
    <section className="mobile-sheet mobile-context-sheet">
      <div className="mobile-sheet-handle" />
      <header><div><small>会话上下文</small><h2>远端运行设置</h2></div><button type="button" className="mobile-icon-button" aria-label="关闭" onClick={onClose}><X /></button></header>
      <label><span>Worker</span><select value={selectedWorkerID} onChange={(event) => onSelectWorker(event.target.value)}>{workers.map((worker) => <option key={worker.id} value={worker.id}>{workerLabel(worker)}</option>)}</select></label>
      <label><span>工作区</span><select value={workspaceID} onChange={(event) => onSelectWorkspace(event.target.value)}>{workspaces.map((workspace) => <option key={workspace.id} value={workspace.id}>{workspaceLabel(workspace)}</option>)}</select></label>
      <div className="mobile-runtime-picker">{RUNTIMES.map((item) => <button type="button" className={runtime === item.id ? "selected" : ""} disabled={!available[item.id] || (runtimeLocked && runtime !== item.id)} key={item.id} onClick={() => onRuntime(item.id)}><Bot />{item.label}{runtime === item.id && <Check />}</button>)}</div>
      {runtimeLocked && <p className="mobile-context-note">已有 session 会保持原运行时；切换工作区或新建会话后可以重新选择。</p>}
      <label><span>模型（可选）</span><Input value={model} placeholder="跟随 Worker 默认" autoCapitalize="none" onChange={(event) => onModel(event.target.value)} /></label>
      <label><span>推理强度（可选）</span><Input value={reasoningEffort} placeholder="medium / high" autoCapitalize="none" onChange={(event) => onReasoning(event.target.value)} /></label>
	  <Button className="mobile-main-action" onClick={onClose}>完成</Button>
    </section>
  </div>;
}

function WorkersSheet({ open, workers, healthByID, busy, onClose, onPair, onRefresh, onDelete }) {
  if (!open) return null;
  return <div className="mobile-sheet-backdrop" onPointerDown={(event) => event.target === event.currentTarget && onClose()}>
    <section className="mobile-sheet mobile-workers-sheet">
      <div className="mobile-sheet-handle" />
      <header><div><small>Connections</small><h2>远端 Worker</h2></div><button type="button" className="mobile-icon-button" aria-label="关闭" onClick={onClose}><X /></button></header>
      <div className="mobile-worker-list">{workers.map((worker) => {
        const health = healthByID[worker.id];
        return <article className="mobile-worker-card" key={worker.id}>
          <div className="mobile-worker-card-main"><span className="mobile-worker-glyph"><Server /></span><span><strong>{workerLabel(worker)}</strong><small>{worker.baseUrl}</small></span><span className={`mobile-worker-state ${health ? "online" : ""}`}>{health ? "在线" : "离线"}</span></div>
          <div className="mobile-worker-card-meta"><span><Wifi />{health ? `${health.latencyMilliseconds}ms` : "未连接"}</span><span><Cpu />{health ? RUNTIMES.filter((item) => health.health?.runtimes?.[item.id]).map((item) => item.label).join(" · ") : "—"}</span></div>
          <footer><Button variant="outline" size="sm" disabled={busy} onClick={() => onRefresh(worker.id)}><RefreshCw />检查</Button><Button variant="outline" size="sm" disabled={busy} onClick={() => onPair(worker)}><Link2 />重新配对</Button><Button variant="ghost" size="icon-sm" aria-label={`删除 ${workerLabel(worker)}`} disabled={busy} onClick={() => onDelete(worker)}><Trash2 /></Button></footer>
        </article>;
      })}</div>
	  <Button className="mobile-main-action" onClick={() => onPair(null)}><Plus />添加 Worker</Button>
    </section>
  </div>;
}

export default function MobileWorkbench() {
  const [view, setView] = useState("projects");
  const [workers, setWorkers] = useState([]);
  const [selectedWorkerID, setSelectedWorkerID] = useState("");
  const [healthByID, setHealthByID] = useState({});
  const [workspaces, setWorkspaces] = useState([]);
  const [workspaceID, setWorkspaceID] = useState("");
	const [workspaceStatusByID, setWorkspaceStatusByID] = useState({});
	const [workspaceEditor, setWorkspaceEditor] = useState(undefined);
  const [snapshot, setSnapshot] = useState(null);
  const [runs, setRuns] = useState([]);
  const [selectedConversationID, setSelectedConversationID] = useState("");
  const [prompt, setPrompt] = useState("");
  const [runtime, setRuntime] = useState("codex");
  const [model, setModel] = useState("");
  const [reasoningEffort, setReasoningEffort] = useState("");
  const [query, setQuery] = useState("");
  const [sortMode, setSortMode] = useState("project");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [pairTarget, setPairTarget] = useState(undefined);
  const [contextOpen, setContextOpen] = useState(false);
  const [workersOpen, setWorkersOpen] = useState(false);
  const [busy, setBusy] = useState("");
  const [permissionBusy, setPermissionBusy] = useState("");
  const [notice, setNotice] = useState(null);

  const conversations = useMemo(() => groupMobileConversations(runs), [runs]);
  const orderedWorkspaces = useMemo(() => {
    if (sortMode === "project") return [...workspaces].sort((left, right) => workspaceLabel(left).localeCompare(workspaceLabel(right), "zh-CN"));
    const latest = (id) => conversations.find((item) => item.workspaceId === id)?.startedAt || "";
    return [...workspaces].sort((left, right) => String(latest(right.id)).localeCompare(String(latest(left.id))));
  }, [conversations, sortMode, workspaces]);
  const selectedConversation = conversations.find((item) => item.id === selectedConversationID) || null;
  const selectedWorkspace = workspaces.find((item) => item.id === workspaceID) || null;
  const selectedHealth = healthByID[selectedWorkerID] || null;
	const workspaceManagementSupported = selectedHealth ? Boolean(selectedHealth.health?.capabilities?.workspaceManagement) : true;

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

	const refreshWorkspace = useCallback(async (workspace) => {
	  const id = typeof workspace === "string" ? workspace : workspace?.id;
	  if (!selectedWorkerID || !id) return null;
	  setWorkspaceStatusByID((current) => ({ ...current, [id]: { ...current[id], loading: true, error: "" } }));
	  try {
	    const value = await MobileBinding.WorkspaceGitStatus(selectedWorkerID, id);
	    setWorkspaceStatusByID((current) => ({ ...current, [id]: { loading: false, snapshot: value, error: "" } }));
	    if (id === workspaceID) setSnapshot(value);
	    return value;
	  } catch (error) {
	    const message = errorMessage(error);
	    setWorkspaceStatusByID((current) => ({ ...current, [id]: { loading: false, snapshot: null, error: message } }));
	    return null;
	  }
	}, [selectedWorkerID, workspaceID]);

  useEffect(() => {
    void (async () => {
      try {
        const [items, runItems] = await Promise.all([loadWorkers(), MobileBinding.ListRuns()]);
        setRuns(runItems || []);
        if (items[0]) await refreshWorker(items[0].id, true);
      } catch (error) { notify("error", errorMessage(error)); }
    })();
  }, [loadWorkers, notify, refreshWorker]);

  useEffect(() => { if (selectedWorkerID) void loadWorkspaces(selectedWorkerID); }, [loadWorkspaces, selectedWorkerID]);
	useEffect(() => {
	  if (view !== "workspaces") return;
	  for (const workspace of workspaces) void refreshWorkspace(workspace);
	}, [refreshWorkspace, view, workspaces]);
  useEffect(() => {
    if (!selectedWorkerID || !workspaceID) { setSnapshot(null); return; }
    setSnapshot(null);
    void MobileBinding.WorkspaceGitStatus(selectedWorkerID, workspaceID).then(setSnapshot).catch((error) => { setSnapshot(null); notify("error", errorMessage(error)); });
  }, [notify, selectedWorkerID, workspaceID]);
  useEffect(() => {
    const available = selectedHealth?.health?.runtimes || {};
    if (available[runtime]) return;
    const next = RUNTIMES.find((item) => available[item.id]);
    if (next) setRuntime(next.id);
  }, [runtime, selectedHealth]);
  useEffect(() => {
    const off = Events.On(RUN_EVENT, (event) => {
      const frame = event.data;
      if (!frame?.runId) return;
      setRuns((items) => items.map((run) => applyMobileRunFrame(run, frame)));
      if (frame.status && frame.status !== "running") void MobileBinding.GetRun(frame.runId).then((run) => setRuns((items) => mergeMobileRun(items, run))).catch(() => {});
    });
    return () => off();
  }, []);

  const selectWorkspace = (id, nextView = "sessions") => { setWorkspaceID(id); setSelectedConversationID(""); setView(nextView); setQuery(""); };
  const openConversation = (id) => {
    const conversation = conversations.find((item) => item.id === id);
    if (conversation) { setWorkspaceID(conversation.workspaceId); setRuntime(conversation.runtime || "codex"); }
    setSelectedConversationID(id); setView("conversation"); setQuery("");
  };
  const newConversation = (id = workspaceID) => {
    const nextID = id || workspaces[0]?.id || "";
	if (!nextID) { setView("workspaces"); setWorkspaceEditor(null); return; }
    setWorkspaceID(nextID); setSelectedConversationID(""); setPrompt(""); setView("conversation"); setQuery("");
  };

  const pairWorker = async ({ baseURL, code }) => {
    setBusy("pair");
    try {
      const worker = await MobileBinding.PairWorker(baseURL.trim(), code.trim());
      await loadWorkers(); setSelectedWorkerID(worker.id); await refreshWorker(worker.id, true); setPairTarget(undefined);
      notify("success", `已连接 ${workerLabel(worker)}`); return true;
    } catch (error) { notify("error", errorMessage(error)); return false; }
    finally { setBusy(""); }
  };

  const startRun = async ({ resumeSessionId = "" } = {}) => {
    setBusy("run");
    try {
      const run = await MobileBinding.StartRun({
        workerId: selectedWorkerID, workspaceId: workspaceID, conversationId: selectedConversationID,
        runtime, prompt: prompt.trim(), model: model.trim(), reasoningEffort: reasoningEffort.trim(), resumeSessionId,
      });
      setRuns((items) => mergeMobileRun(items, run)); setSelectedConversationID(run.conversationId || run.id); setPrompt(""); return true;
    } catch (error) { notify("error", errorMessage(error)); return false; }
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

	const saveWorkspace = async (input) => {
	  setBusy("workspace-save");
	  try {
	    const result = await MobileBinding.PrepareWorkspace(selectedWorkerID, input.id, {
	      name: input.name, path: input.path, remoteUrl: input.remoteUrl, revision: input.revision,
	    });
	    const items = await loadWorkspaces(selectedWorkerID);
	    setWorkspaceID((current) => current || result.mapping?.id || items[0]?.id || "");
	    if (result.mapping?.id) setWorkspaceStatusByID((current) => ({ ...current, [result.mapping.id]: { loading: false, snapshot: result.git, error: "" } }));
	    setWorkspaceEditor(undefined);
	    notify("success", `${workspaceLabel(result.mapping)} 已保存`);
	    return true;
	  } catch (error) { notify("error", errorMessage(error)); return false; }
	  finally { setBusy(""); }
	};

	const deleteWorkspace = async (workspace, deleteFiles) => {
	  const action = deleteFiles ? "删除远端克隆及其文件" : "移除 Workspace 映射";
	  if (!window.confirm(`${action}“${workspaceLabel(workspace)}”？${deleteFiles ? " 此操作不可恢复。" : " 代码文件会保留在 Worker。"}`)) return;
	  setBusy("workspace-delete");
	  try {
	    await MobileBinding.RemoveWorkspace(selectedWorkerID, workspace.id, deleteFiles);
	    setWorkspaceStatusByID((current) => { const next = { ...current }; delete next[workspace.id]; return next; });
	    await loadWorkspaces(selectedWorkerID);
	    setWorkspaceEditor(undefined);
	    notify("success", deleteFiles ? "Workspace 和远端克隆已删除" : "Workspace 映射已移除");
	  } catch (error) { notify("error", errorMessage(error)); }
	  finally { setBusy(""); }
	};

	const openWorkspaceManager = () => { setView("workspaces"); setQuery(""); void loadWorkspaces(selectedWorkerID); };

	const title = view === "projects" ? "远程" : view === "workspaces" ? "Workspace" : view === "sessions" ? workspaceLabel(selectedWorkspace) : selectedConversation?.title || "新会话";
	const subtitle = view === "projects" || view === "workspaces" ? <><StatusDot online={Boolean(selectedHealth)} />{selectedHealth?.worker?.name || workerLabel(workers.find((item) => item.id === selectedWorkerID))}</> : view === "conversation" ? `${workspaceLabel(selectedWorkspace)} · ${runtime}` : `${conversations.filter((item) => item.workspaceId === workspaceID).length} 个会话`;
	const goBack = view === "conversation" ? () => setView("sessions") : view === "sessions" || view === "workspaces" ? () => setView("projects") : null;

  return <div className="mobile-app-shell">
    <Header title={title} subtitle={subtitle} onMenu={() => setDrawerOpen(true)} onBack={goBack} onMore={() => setMenuOpen(true)} onNew={view === "conversation" ? () => newConversation() : null} />
	{!workers.length ? <main className="mobile-main"><EmptyConnection onPair={() => setPairTarget(null)} /></main> : view === "projects" ? <main className="mobile-main"><ProjectHome workspaces={orderedWorkspaces} conversations={conversations} query={query} onOpenWorkspace={selectWorkspace} onNew={newConversation} onManage={() => { setView("workspaces"); setWorkspaceEditor(null); }} /></main> : view === "workspaces" ? <main className="mobile-main"><WorkspaceManagerPage workspaces={orderedWorkspaces} statusByID={workspaceStatusByID} managementSupported={workspaceManagementSupported} busy={busy} onOpen={selectWorkspace} onCreate={() => setWorkspaceEditor(null)} onEdit={setWorkspaceEditor} onRefresh={refreshWorkspace} /></main> : view === "sessions" ? <main className="mobile-main"><SessionList workspace={selectedWorkspace} conversations={conversations} query={query} onOpen={openConversation} onNew={() => newConversation()} /></main> : <ConversationView conversation={selectedConversation} workspace={selectedWorkspace} snapshot={snapshot} prompt={prompt} setPrompt={setPrompt} busy={busy} permissionBusy={permissionBusy} runtime={runtime} onOpenContext={() => setContextOpen(true)} onStart={startRun} onInterrupt={interruptRun} onRespond={respondPermission} />}
	{workers.length > 0 && view !== "conversation" && view !== "workspaces" && <BottomDock query={query} setQuery={setQuery} workerOnline={Boolean(selectedHealth)} onWorker={() => setWorkersOpen(true)} onNew={() => newConversation()} />}
    <Sidebar open={drawerOpen} workspaces={orderedWorkspaces} conversations={conversations} selectedConversationID={selectedConversationID} health={selectedHealth} onClose={() => setDrawerOpen(false)} onHome={() => setView("projects")} onWorkspace={selectWorkspace} onConversation={openConversation} onNew={() => newConversation()} onWorkers={() => setWorkersOpen(true)} />
	<MoreMenu open={menuOpen} sortMode={sortMode} health={selectedHealth} onSort={setSortMode} onWorkspaces={openWorkspaceManager} onWorkers={() => setWorkersOpen(true)} onPair={() => setPairTarget(null)} onSettings={() => setContextOpen(true)} onClose={() => setMenuOpen(false)} />
    <ContextSheet open={contextOpen} workers={workers} selectedWorkerID={selectedWorkerID} workspaces={workspaces} workspaceID={workspaceID} health={selectedHealth} runtime={runtime} runtimeLocked={Boolean(selectedConversationID)} model={model} reasoningEffort={reasoningEffort} onClose={() => setContextOpen(false)} onSelectWorker={(id) => { setSelectedWorkerID(id); setSelectedConversationID(""); setView("conversation"); void refreshWorker(id, true); }} onSelectWorkspace={(id) => selectWorkspace(id, "conversation")} onRuntime={setRuntime} onModel={setModel} onReasoning={setReasoningEffort} />
    <WorkersSheet open={workersOpen} workers={workers} healthByID={healthByID} busy={Boolean(busy)} onClose={() => setWorkersOpen(false)} onPair={(worker) => { setWorkersOpen(false); setPairTarget(worker || null); }} onRefresh={refreshWorker} onDelete={deleteWorker} />
	<PairSheet open={pairTarget !== undefined} busy={busy === "pair"} initialURL={pairTarget?.baseUrl || "https://"} onClose={() => setPairTarget(undefined)} onPair={pairWorker} />
	<WorkspaceEditorSheet workspace={workspaceEditor} busy={busy} onClose={() => setWorkspaceEditor(undefined)} onSave={saveWorkspace} onDelete={deleteWorkspace} />
    <Notice value={notice} onClose={() => setNotice(null)} />
  </div>;
}
