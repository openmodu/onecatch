import { memo } from "react";

// Its props (workspaces, runtimes, view, query state + stable callbacks) don't
// change during run polling, so memo lets it skip the poll-driven re-renders.
function Sidebar({
  workspaces,
  workspaceID,
  visibleWorkspaces,
  workspaceQuery,
  workspaceExpanded,
  workspaceSearchOpen,
  runtimes,
  view,
  editor,
  onToggleSearch,
  onQueryChange,
  onSelectWorkspace,
  onTogglePinned,
  onRemoveWorkspace,
  onToggleExpanded,
  onAddWorkspace,
  onGoView,
}) {
  return <aside className="sidebar">
    <div className="brand"><strong>ONESHOT</strong><span>// personal workspace</span></div>
    <div className="workspace-block">
      <div className="workspace-heading"><div className="sidebar-section-label">cwd <span>{workspaces.length}</span></div><button type="button" className={`workspace-search-toggle ${workspaceSearchOpen ? "active" : ""}`} aria-label="搜索工作目录" title="搜索工作目录" onClick={onToggleSearch}>[ / ]</button></div>
      {workspaceSearchOpen && <div className="workspace-search"><span>/</span><input autoFocus aria-label="搜索工作目录" value={workspaceQuery} onChange={(event) => onQueryChange(event.target.value)} placeholder="名称或路径" /><button type="button" aria-label="清空搜索" onClick={() => onQueryChange("")}>[ x ]</button></div>}
      <div className={`workspace-list ${workspaceExpanded || workspaceQuery ? "expanded" : ""}`}>
        {visibleWorkspaces.map((workspace) => <div className={`workspace-row ${workspace.id === workspaceID ? "active" : ""}`} key={workspace.id}><button className="workspace-item" onClick={() => onSelectWorkspace(workspace.id)}><span><strong>{workspace.name}</strong><small>{workspace.path}</small></span></button><button type="button" className={`workspace-pin ${workspace.pinned ? "pinned" : ""}`} aria-label={`${workspace.pinned ? "取消置顶" : "置顶"}${workspace.name}`} aria-pressed={Boolean(workspace.pinned)} title={workspace.pinned ? "取消置顶" : "置顶"} onClick={() => onTogglePinned(workspace)}>{workspace.pinned ? "*" : "."}</button><button type="button" className="workspace-remove" aria-label={`移除${workspace.name}`} title="从列表移除" onClick={() => onRemoveWorkspace(workspace)}>-</button></div>)}
        {!workspaces.length && <div className="sidebar-empty">还没有工作目录</div>}
        {workspaces.length > 0 && !visibleWorkspaces.length && <div className="sidebar-empty">没有匹配的工作目录</div>}
      </div>
      {!workspaceQuery && workspaces.length > 8 && <button type="button" className="workspace-expand" onClick={onToggleExpanded}>[ {workspaceExpanded ? "收起" : `全部 CWD · ${workspaces.length}`} ]</button>}
      <button className="add-workspace" onClick={onAddWorkspace}>[ + 加入工作目录 ]</button>
    </div>
    <nav className="primary-nav">
      <button className={view === "tasks" && !editor ? "active" : ""} onClick={() => onGoView("tasks")}><span>&gt;</span><b>任务与运行</b><small>runs</small></button>
      <button className={view === "workflows" || editor ? "active" : ""} onClick={() => onGoView("workflows")}><span>%</span><b>Workflow</b><small>flows</small></button>
      <button className={view === "settings" && !editor ? "active" : ""} onClick={() => onGoView("settings")}><span>#</span><b>设置</b><small>prefs</small></button>
    </nav>
    <div className="runtime-panel">
      <div className="sidebar-section-label">runtime</div>
      {runtimes.map((runtime) => <div className="runtime-row" key={runtime.id}><span className={`runtime-dot ${runtime.available ? "online" : "offline"}`} /><strong>{runtime.id}</strong><small>{runtime.available ? String(runtime.version || "ready").replace(`${runtime.id} `, "") : "missing"}</small></div>)}
      <div className="storage-note"><span>数据仅保存在本机</span><code>~/.oneshot/</code></div>
    </div>
  </aside>;
}

export default memo(Sidebar);
