import { lazy, memo, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { DotsThree, FolderOpen, FolderSimple, GearSix, GitBranch, Info, List, MagnifyingGlass, Plus, PushPin, Trash } from "@phosphor-icons/react";
import { Action } from "../../ui/primitives.jsx";
import {
  SIDEBAR_DEFAULT_WIDTH,
  SIDEBAR_KEYBOARD_STEP,
  clampSidebarWidth,
  readSidebarWidth,
  sidebarWidthBounds,
  writeSidebarWidth,
} from "../sidebarLayout.js";
import { SIDEBAR_TASK_PREVIEW_LIMIT, buildSidebarTaskEntries, visibleSidebarTaskEntries } from "../sidebarNavigation.js";

const CommandPalette = lazy(() => import("./CommandPalette.jsx"));

function initialSidebarWidth() {
  if (typeof window === "undefined") return SIDEBAR_DEFAULT_WIDTH;
  return readSidebarWidth(window.localStorage, window.innerWidth) ?? clampSidebarWidth(SIDEBAR_DEFAULT_WIDTH, window.innerWidth);
}

// Its workspace and task data only change when the active project changes or a
// run is updated; memo keeps unrelated settings/editor renders out of the rail.
function Sidebar({
  workspaces,
  workspaceID,
  pinnedWorkspaces,
  projectWorkspaces,
  searchQuery,
  searchTaskItems,
  searchLoading,
  workspaceExpanded,
  workspaceSearchOpen,
  tasks,
  runs,
  selectedRunID,
  selectedQueuedTaskID,
  runLoading,
  runTotal,
  runHasMore,
  taskSearch,
  taskStatus,
  view,
  editor,
  onToggleSearch,
  onClearSearch,
  onSearchQueryChange,
  onSelectWorkspace,
  onTogglePinned,
  onRemoveWorkspace,
  onToggleExpanded,
  onAddWorkspace,
  onNewTask,
  onLoadMoreRuns,
  onSelectRun,
  onSelectQueued,
  onGoView,
}) {
  const { t } = useTranslation();
  const [width, setWidth] = useState(initialSidebarWidth);
  const [resizing, setResizing] = useState(false);
  const [expandedWorkspaceID, setExpandedWorkspaceID] = useState(workspaceID);
  const [taskListExpanded, setTaskListExpanded] = useState(false);
  const [projectMenuWorkspaceID, setProjectMenuWorkspaceID] = useState("");
  const [projectMenuPosition, setProjectMenuPosition] = useState({ left: 0, top: 0 });
  const [pendingSearchTask, setPendingSearchTask] = useState(null);
  const [secondaryNavigationOpen, setSecondaryNavigationOpen] = useState(false);
  const drag = useRef(null);
  const searchTrigger = useRef(null);
  const projectMenu = useRef(null);
  const projectMenuTrigger = useRef(null);
  const secondaryNavigation = useRef(null);
  const secondaryNavigationTrigger = useRef(null);
  const firstSecondaryNavigationItem = useRef(null);

  const taskEntries = useMemo(() => buildSidebarTaskEntries(tasks, runs, { query: taskSearch, status: taskStatus }), [runs, taskSearch, taskStatus, tasks]);
  const visibleTaskEntries = visibleSidebarTaskEntries(taskEntries, taskListExpanded);
  const queuedEntryCount = taskEntries.filter((entry) => entry.kind === "queued").length;
  const taskTotal = Math.max(taskEntries.length, queuedEntryCount + (taskStatus === "queued" ? 0 : runTotal));
  const regularProjectCount = workspaces.filter((workspace) => !workspace.pinned).length;
  const closeSearch = useCallback(() => {
    onClearSearch();
    if (workspaceSearchOpen) onToggleSearch();
    requestAnimationFrame(() => searchTrigger.current?.focus());
  }, [onClearSearch, onToggleSearch, workspaceSearchOpen]);
  const openSearch = useCallback(() => {
    if (workspaceSearchOpen) return;
    onClearSearch();
    onToggleSearch();
  }, [onClearSearch, onToggleSearch, workspaceSearchOpen]);
  const toggleSearch = () => workspaceSearchOpen ? closeSearch() : openSearch();

  useEffect(() => {
    const fitViewport = () => setWidth((current) => clampSidebarWidth(current, window.innerWidth));
    window.addEventListener("resize", fitViewport);
    return () => {
      window.removeEventListener("resize", fitViewport);
      drag.current?.cleanup?.();
    };
  }, []);

  useEffect(() => {
    setExpandedWorkspaceID(workspaceID);
    setTaskListExpanded(false);
  }, [workspaceID]);

  useEffect(() => setTaskListExpanded(false), [taskSearch, taskStatus]);

  useEffect(() => {
    if (!pendingSearchTask || pendingSearchTask.workspace.id !== workspaceID) return;
    setExpandedWorkspaceID(workspaceID);
    setTaskListExpanded(false);
    if (pendingSearchTask.latestRun) onSelectRun(pendingSearchTask.latestRun);
    else onSelectQueued(pendingSearchTask.task);
    setPendingSearchTask(null);
  }, [onSelectQueued, onSelectRun, pendingSearchTask, workspaceID]);

  useEffect(() => {
    if (!projectMenuWorkspaceID) return undefined;
    const closeOutside = (event) => {
      if (!projectMenu.current?.contains(event.target)) setProjectMenuWorkspaceID("");
    };
    const closeWithKeyboard = (event) => {
      if (event.key !== "Escape") return;
      setProjectMenuWorkspaceID("");
      projectMenuTrigger.current?.focus();
    };
    window.addEventListener("pointerdown", closeOutside);
    window.addEventListener("keydown", closeWithKeyboard);
    return () => {
      window.removeEventListener("pointerdown", closeOutside);
      window.removeEventListener("keydown", closeWithKeyboard);
    };
  }, [projectMenuWorkspaceID]);

  useEffect(() => {
    const openWithKeyboard = (event) => {
      if (!(event.metaKey || event.ctrlKey) || event.key.toLocaleLowerCase() !== "k") return;
      event.preventDefault();
      if (workspaceSearchOpen) closeSearch(); else openSearch();
    };
    window.addEventListener("keydown", openWithKeyboard);
    return () => window.removeEventListener("keydown", openWithKeyboard);
  }, [closeSearch, openSearch, workspaceSearchOpen]);

  useEffect(() => {
    if (!secondaryNavigationOpen) return undefined;
    firstSecondaryNavigationItem.current?.focus();
    const closeOutside = (event) => {
      if (!secondaryNavigation.current?.contains(event.target)) setSecondaryNavigationOpen(false);
    };
    const closeWithKeyboard = (event) => {
      if (event.key !== "Escape") return;
      setSecondaryNavigationOpen(false);
      secondaryNavigationTrigger.current?.focus();
    };
    window.addEventListener("pointerdown", closeOutside);
    window.addEventListener("keydown", closeWithKeyboard);
    return () => {
      window.removeEventListener("pointerdown", closeOutside);
      window.removeEventListener("keydown", closeWithKeyboard);
    };
  }, [secondaryNavigationOpen]);

  const commitWidth = (next) => {
    const fitted = clampSidebarWidth(next, window.innerWidth);
    setWidth(fitted);
    writeSidebarWidth(window.localStorage, fitted);
  };
  const goToSecondaryView = (nextView) => {
    setSecondaryNavigationOpen(false);
    onGoView(nextView);
  };
  const toggleProject = (workspace) => {
    setProjectMenuWorkspaceID("");
    const opening = expandedWorkspaceID !== workspace.id;
    setExpandedWorkspaceID(opening ? workspace.id : "");
    setTaskListExpanded(false);
    if (opening) {
      // The sidebar is the only nav now, so opening a project must also return
      // to the tasks view from wherever the menu last took us (workflows/settings).
      onGoView("tasks");
      if (workspace.id !== workspaceID) onSelectWorkspace(workspace.id);
    }
  };
  const openRun = (run) => {
    onGoView("tasks");
    onSelectRun(run);
  };
  const openQueuedTask = (task) => {
    onGoView("tasks");
    onSelectQueued(task);
  };
  const openWorkspaceFromSearch = (workspace) => {
    setExpandedWorkspaceID(workspace.id);
    setTaskListExpanded(false);
    onGoView("tasks");
    if (workspace.id !== workspaceID) onSelectWorkspace(workspace.id);
  };
  const openTaskFromSearch = (item) => {
    onGoView("tasks");
    setExpandedWorkspaceID(item.workspace.id);
    setTaskListExpanded(false);
    if (item.workspace.id !== workspaceID) {
      setPendingSearchTask(item);
      onSelectWorkspace(item.workspace.id);
      return;
    }
    if (item.latestRun) onSelectRun(item.latestRun);
    else onSelectQueued(item.task);
  };
  const createTaskForWorkspace = (workspace) => {
    setProjectMenuWorkspaceID("");
    onGoView("tasks");
    if (workspace.id !== workspaceID) onSelectWorkspace(workspace.id);
    onNewTask();
  };
  const toggleProjectMenu = (event, workspace) => {
    if (projectMenuWorkspaceID === workspace.id) {
      setProjectMenuWorkspaceID("");
      return;
    }
    const trigger = event.currentTarget.getBoundingClientRect();
    const menuWidth = 176;
    const menuHeight = 82;
    setProjectMenuPosition({
      left: Math.min(trigger.right + 4, window.innerWidth - menuWidth - 8),
      top: Math.min(trigger.top, window.innerHeight - menuHeight - 8),
    });
    setProjectMenuWorkspaceID(workspace.id);
  };
  const startResize = (event) => {
    event.preventDefault();
    drag.current?.cleanup?.();
    const pointerID = event.pointerId;
    const startX = event.clientX;
    const startWidth = width;
    const move = (moveEvent) => {
      if (moveEvent.pointerId !== pointerID) return;
      setWidth(clampSidebarWidth(startWidth + moveEvent.clientX - startX, window.innerWidth));
    };
    const finish = (finishEvent) => {
      if (finishEvent.pointerId !== pointerID) return;
      const next = startWidth + finishEvent.clientX - startX;
      drag.current?.cleanup?.();
      drag.current = null;
      setResizing(false);
      commitWidth(next);
    };
    const cleanup = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", finish);
      window.removeEventListener("pointercancel", finish);
    };
    drag.current = { pointerID, startX, startWidth, cleanup };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", finish);
    window.addEventListener("pointercancel", finish);
    setResizing(true);
  };
  const resizeWithKeyboard = (event) => {
    const bounds = sidebarWidthBounds(window.innerWidth);
    const next = event.key === "ArrowLeft" ? width - SIDEBAR_KEYBOARD_STEP
      : event.key === "ArrowRight" ? width + SIDEBAR_KEYBOARD_STEP
        : event.key === "Home" ? bounds.min
          : event.key === "End" ? bounds.max
            : null;
    if (next === null) return;
    event.preventDefault();
    commitWidth(next);
  };

  const renderWorkspace = (workspace) => {
    const active = workspace.id === workspaceID;
    const expanded = expandedWorkspaceID === workspace.id;
    const menuOpen = projectMenuWorkspaceID === workspace.id;
    const taskPanelID = `workspace-tasks-${encodeURIComponent(workspace.id)}`;
    const menuID = `workspace-menu-${encodeURIComponent(workspace.id)}`;
    return <div className={`workspace-row ${active ? "active" : ""} ${expanded ? "expanded" : ""} ${menuOpen ? "menu-open" : ""}`} key={workspace.id}>
      <button className="workspace-item" title={workspace.path} aria-expanded={expanded} aria-controls={taskPanelID} onClick={() => toggleProject(workspace)}>
        {expanded ? <FolderOpen size={20} weight="regular" aria-hidden="true" /> : <FolderSimple size={20} weight="regular" aria-hidden="true" />}
        <span className="workspace-copy"><strong>{workspace.name}</strong></span>
      </button>
      <div className="workspace-row-actions" ref={menuOpen ? projectMenu : null}>
        <Action ref={menuOpen ? projectMenuTrigger : null} size="compact" tone="muted" className="workspace-menu-trigger" aria-label={t("sidebar.projectMenu", { name: workspace.name })} aria-haspopup="menu" aria-expanded={menuOpen} aria-controls={menuID} title={t("sidebar.projectMenu", { name: workspace.name })} onClick={(event) => toggleProjectMenu(event, workspace)}><DotsThree size={16} weight="regular" aria-hidden="true" /></Action>
        <Action size="compact" tone="primary" className="workspace-new-task" aria-label={t("sidebar.newTaskInProject", { name: workspace.name })} title={t("sidebar.newTaskInProject", { name: workspace.name })} onClick={() => createTaskForWorkspace(workspace)}><Plus size={14} weight="bold" aria-hidden="true" /></Action>
        {menuOpen && <div className="workspace-context-menu" id={menuID} role="menu" aria-label={t("sidebar.projectMenu", { name: workspace.name })} style={projectMenuPosition}>
          <button type="button" role="menuitem" onClick={() => { onTogglePinned(workspace); setProjectMenuWorkspaceID(""); }}><PushPin size={18} aria-hidden="true" /><span>{t(workspace.pinned ? "common.unpin" : "common.pin")}</span></button>
          <button type="button" role="menuitem" className="danger" onClick={() => { onRemoveWorkspace(workspace); setProjectMenuWorkspaceID(""); }}><Trash size={18} aria-hidden="true" /><span>{t("common.remove")}</span></button>
        </div>}
      </div>
      {expanded && active && <div className="project-task-panel" id={taskPanelID}>
        <div className="project-task-list">
          {visibleTaskEntries.map((entry) => {
            if (entry.kind === "queued") {
              const task = entry.item;
              const selected = selectedQueuedTaskID === task.id;
              return <button type="button" className={`project-task-item ${selected ? "selected" : ""}`} title={task.title} key={entry.key} onClick={() => openQueuedTask(task)}><span className="project-task-title">{task.title}</span>{selected && <Info size={16} weight="bold" aria-hidden="true" />}</button>;
            }
            const run = entry.item;
            const title = run.task?.title || run.id;
            const selected = selectedRunID === run.id;
            return <button type="button" className={`project-task-item ${selected ? "selected" : ""}`} title={title} key={entry.key} onClick={() => openRun(run)}><span className="project-task-title">{title}</span>{selected && <Info size={16} weight="bold" aria-hidden="true" />}</button>;
          })}
          {!taskEntries.length && !runLoading && <div className="project-task-empty">{taskSearch || taskStatus ? t("task.noMatches") : t("task.empty")}</div>}
          {runLoading && !taskEntries.length && <div className="project-task-empty">{t("task.loading")}</div>}
        </div>
        {!taskListExpanded && taskEntries.length > SIDEBAR_TASK_PREVIEW_LIMIT && <Action size="compact" tone="muted" className="project-task-more" onClick={() => setTaskListExpanded(true)}>{t("sidebar.showTasks", { count: Math.max(0, taskTotal - SIDEBAR_TASK_PREVIEW_LIMIT) })}</Action>}
        {taskListExpanded && taskEntries.length > SIDEBAR_TASK_PREVIEW_LIMIT && <Action size="compact" tone="muted" className="project-task-more" onClick={() => setTaskListExpanded(false)}>{t("sidebar.hideTasks")}</Action>}
        {(taskListExpanded || taskEntries.length <= SIDEBAR_TASK_PREVIEW_LIMIT) && (runLoading || runHasMore) && <Action size="compact" tone="muted" className="project-task-more" disabled={runLoading} onClick={onLoadMoreRuns}>{runLoading ? t("task.loading") : t("task.loadMore", { visible: taskEntries.length, total: taskTotal })}</Action>}
      </div>}
    </div>;
  };

  const widthBounds = typeof window === "undefined" ? sidebarWidthBounds() : sidebarWidthBounds(window.innerWidth);
  return <aside className={`sidebar ${resizing ? "resizing" : ""}`} style={{ width: `${width}px` }}>
    <div className="brand"><strong>ONESHOT</strong><Action ref={searchTrigger} size="compact" tone="muted" className={`sidebar-search-trigger ${workspaceSearchOpen ? "active" : ""}`} aria-label={t("sidebar.searchPanel")} aria-haspopup="dialog" aria-expanded={workspaceSearchOpen} aria-controls="global-command-palette" title={`${t("sidebar.searchPanel")} · ⌘K`} onClick={toggleSearch}><MagnifyingGlass size={16} weight="regular" aria-hidden="true" /></Action></div>
    <div className="workspace-block">
      <div className="project-sections">
        <section className="project-section pinned-project-section" aria-labelledby="pinned-project-heading">
          <div className="project-section-heading" id="pinned-project-heading"><span>{t("sidebar.pinnedProjects")}</span><small>{pinnedWorkspaces.length}</small></div>
          <div className="workspace-list">{pinnedWorkspaces.map(renderWorkspace)}{!pinnedWorkspaces.length && <div className="sidebar-empty compact">{t("sidebar.noPinnedProjects")}</div>}</div>
        </section>
        <section className="project-section" aria-labelledby="project-heading">
          <div className="project-section-heading" id="project-heading"><span>{t("sidebar.projects")}</span><small>{regularProjectCount}</small><Action size="compact" tone="primary" className="add-workspace" onClick={onAddWorkspace}>{t("sidebar.addProject")}</Action></div>
          <div className={`workspace-list ${workspaceExpanded ? "expanded" : ""}`}>{projectWorkspaces.map(renderWorkspace)}{!workspaces.length && <div className="sidebar-empty">{t("sidebar.noWorkspaces")}</div>}{!projectWorkspaces.length && pinnedWorkspaces.length > 0 && <div className="sidebar-empty compact">{t("sidebar.allProjectsPinned")}</div>}</div>
          {regularProjectCount > 8 && <Action size="compact" tone="muted" className="workspace-expand" onClick={onToggleExpanded}>{workspaceExpanded ? t("sidebar.collapse") : t("sidebar.allProjects", { count: regularProjectCount })}</Action>}
        </section>
      </div>
    </div>
    <nav className="primary-nav">
      <div className="secondary-navigation" ref={secondaryNavigation}>
        {secondaryNavigationOpen && <div className="secondary-navigation-menu" id="sidebar-secondary-navigation" role="menu" aria-label={t("sidebar.menu")}>
          <button ref={firstSecondaryNavigationItem} role="menuitem" className={view === "workflows" || editor ? "active" : ""} onClick={() => goToSecondaryView("workflows")}><GitBranch size={17} aria-hidden="true" /><b>{t("sidebar.workflows")}</b></button>
          <button role="menuitem" className={view === "settings" && !editor ? "active" : ""} onClick={() => goToSecondaryView("settings")}><GearSix size={17} aria-hidden="true" /><b>{t("sidebar.settings")}</b></button>
        </div>}
        <button ref={secondaryNavigationTrigger} className={`secondary-navigation-trigger ${view === "workflows" || view === "settings" || editor ? "active" : ""}`} aria-label={t("sidebar.menu")} aria-haspopup="menu" aria-expanded={secondaryNavigationOpen} aria-controls="sidebar-secondary-navigation" onClick={() => setSecondaryNavigationOpen((open) => !open)}><List size={17} aria-hidden="true" /><b>{t("sidebar.menu")}</b></button>
      </div>
    </nav>
    {workspaceSearchOpen && <Suspense fallback={null}><CommandPalette open query={searchQuery} taskResults={searchTaskItems} loading={searchLoading} workspaces={workspaces} onQueryChange={onSearchQueryChange} onClose={closeSearch} onOpenTask={openTaskFromSearch} onOpenWorkspace={openWorkspaceFromSearch} onNewTask={() => { onGoView("tasks"); onNewTask(); }} onAddWorkspace={onAddWorkspace} onOpenSettings={() => onGoView("settings")} /></Suspense>}
    <div className="sidebar-resizer" role="separator" aria-label={t("sidebar.resize")} aria-orientation="vertical" aria-valuemin={widthBounds.min} aria-valuemax={widthBounds.max} aria-valuenow={width} tabIndex={0} title={t("sidebar.resizeHint")} onDoubleClick={() => commitWidth(SIDEBAR_DEFAULT_WIDTH)} onKeyDown={resizeWithKeyboard} onPointerDown={startResize}><span aria-hidden="true" /></div>
  </aside>;
}

export default memo(Sidebar);
