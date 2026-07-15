import { memo } from "react";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
  return <aside className="sidebar">
    <div className="brand"><strong>ONESHOT</strong><span>{t("sidebar.personalWorkspace")}</span></div>
    <div className="workspace-block">
      <div className="workspace-heading"><div className="sidebar-section-label">{t("sidebar.workspaces")} <span>{workspaces.length}</span></div><button type="button" className={`workspace-search-toggle ${workspaceSearchOpen ? "active" : ""}`} aria-label={t("sidebar.searchWorkspace")} title={t("sidebar.searchWorkspace")} onClick={onToggleSearch}>[ / ]</button></div>
      {workspaceSearchOpen && <div className="workspace-search"><span>/</span><input autoFocus aria-label={t("sidebar.searchWorkspace")} value={workspaceQuery} onChange={(event) => onQueryChange(event.target.value)} placeholder={t("sidebar.searchPlaceholder")} /><button type="button" aria-label={t("sidebar.clearSearch")} onClick={() => onQueryChange("")}>[ x ]</button></div>}
      <div className={`workspace-list ${workspaceExpanded || workspaceQuery ? "expanded" : ""}`}>
        {visibleWorkspaces.map((workspace) => <div className={`workspace-row ${workspace.id === workspaceID ? "active" : ""}`} key={workspace.id}><button className="workspace-item" onClick={() => onSelectWorkspace(workspace.id)}><span><strong>{workspace.name}</strong><small>{workspace.path}</small></span></button><button type="button" className={`workspace-pin ${workspace.pinned ? "pinned" : ""}`} aria-label={t(workspace.pinned ? "sidebar.unpin" : "sidebar.pin", { name: workspace.name })} aria-pressed={Boolean(workspace.pinned)} title={t(workspace.pinned ? "sidebar.unpin" : "sidebar.pin", { name: workspace.name })} onClick={() => onTogglePinned(workspace)}>{workspace.pinned ? "*" : "."}</button><button type="button" className="workspace-remove" aria-label={t("sidebar.removeWorkspace", { name: workspace.name })} title={t("sidebar.removeFromList")} onClick={() => onRemoveWorkspace(workspace)}>-</button></div>)}
        {!workspaces.length && <div className="sidebar-empty">{t("sidebar.noWorkspaces")}</div>}
        {workspaces.length > 0 && !visibleWorkspaces.length && <div className="sidebar-empty">{t("sidebar.noMatches")}</div>}
      </div>
      {!workspaceQuery && workspaces.length > 8 && <button type="button" className="workspace-expand" onClick={onToggleExpanded}>[ {workspaceExpanded ? t("sidebar.collapse") : t("sidebar.allWorkspaces", { count: workspaces.length })} ]</button>}
      <button className="add-workspace" onClick={onAddWorkspace}>[ + {t("sidebar.addWorkspace")} ]</button>
    </div>
    <nav className="primary-nav">
      <button className={view === "tasks" && !editor ? "active" : ""} onClick={() => onGoView("tasks")}><span>&gt;</span><b>{t("sidebar.tasks")}</b><small>{t("sidebar.tasksMeta")}</small></button>
      <button className={view === "workflows" || editor ? "active" : ""} onClick={() => onGoView("workflows")}><span>%</span><b>{t("sidebar.workflows")}</b><small>{t("sidebar.workflowsMeta")}</small></button>
      <button className={view === "settings" && !editor ? "active" : ""} onClick={() => onGoView("settings")}><span>#</span><b>{t("sidebar.settings")}</b><small>{t("sidebar.settingsMeta")}</small></button>
    </nav>
    <div className="runtime-panel">
      <div className="sidebar-section-label">{t("sidebar.runtimes")}</div>
      {runtimes.map((runtime) => <div className="runtime-row" key={runtime.id}><span className={`runtime-dot ${runtime.available ? "online" : "offline"}`} /><strong>{runtime.id}</strong><small>{runtime.available ? String(runtime.version || t("common.ready")).replace(`${runtime.id} `, "") : t("common.missing")}</small></div>)}
      <div className="storage-note"><span>{t("sidebar.dataLocal")}</span><code>~/.oneshot/</code></div>
    </div>
  </aside>;
}

export default memo(Sidebar);
