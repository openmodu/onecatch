import { lazy, memo, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Ellipsis, Folder, FolderOpen, Pin, Plus, Search, Trash2 } from "lucide-react";
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
    return <div className={`workspace-row group relative block min-w-0 ${active ? "active" : ""} ${expanded ? "expanded" : ""} ${menuOpen ? "menu-open" : ""}`} key={workspace.id}>
      <button className={`workspace-item grid w-full min-w-0 grid-cols-[20px_minmax(0,1fr)] items-center gap-2 rounded-md py-2 pr-16 pl-2 text-left transition-colors hover:bg-accent ${active ? "bg-accent/70 text-foreground" : "text-muted-foreground"}`} title={workspace.path} aria-expanded={expanded} aria-controls={taskPanelID} onClick={() => toggleProject(workspace)}>
        {expanded ? <FolderOpen size={18} aria-hidden="true" className="text-muted-foreground" /> : <Folder size={18} aria-hidden="true" className="text-muted-foreground" />}
        <span className="min-w-0"><strong className="block truncate text-sm font-medium">{workspace.name}</strong></span>
      </button>
      <div className="workspace-row-actions absolute top-1.5 right-1.5 z-20 hidden h-6 items-center gap-1 group-hover:flex group-focus-within:flex" ref={menuOpen ? projectMenu : null}>
        <Action ref={menuOpen ? projectMenuTrigger : null} size="compact" tone="muted" className="workspace-menu-trigger" aria-label={t("sidebar.projectMenu", { name: workspace.name })} aria-haspopup="menu" aria-expanded={menuOpen} aria-controls={menuID} title={t("sidebar.projectMenu", { name: workspace.name })} onClick={(event) => toggleProjectMenu(event, workspace)}><Ellipsis size={16} aria-hidden="true" /></Action>
        <Action size="compact" tone="primary" className="workspace-new-task" aria-label={t("sidebar.newTaskInProject", { name: workspace.name })} title={t("sidebar.newTaskInProject", { name: workspace.name })} onClick={() => createTaskForWorkspace(workspace)}><Plus size={14} strokeWidth={2.5} aria-hidden="true" /></Action>
        {menuOpen && <div className="workspace-context-menu fixed z-40 grid w-44 gap-0.5 rounded-md border bg-popover p-1 text-popover-foreground shadow-md" id={menuID} role="menu" aria-label={t("sidebar.projectMenu", { name: workspace.name })} style={projectMenuPosition}>
          <button type="button" role="menuitem" className="grid w-full grid-cols-[18px_minmax(0,1fr)] items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground" onClick={() => { onTogglePinned(workspace); setProjectMenuWorkspaceID(""); }}><Pin size={15} aria-hidden="true" /><span>{t(workspace.pinned ? "common.unpin" : "common.pin")}</span></button>
          <button type="button" role="menuitem" className="danger grid w-full grid-cols-[18px_minmax(0,1fr)] items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm text-destructive transition-colors hover:bg-destructive/10" onClick={() => { onRemoveWorkspace(workspace); setProjectMenuWorkspaceID(""); }}><Trash2 size={15} aria-hidden="true" /><span>{t("common.remove")}</span></button>
        </div>}
      </div>
      {expanded && active && <div className="project-task-panel mt-px mr-1 mb-2 ml-[18px]" id={taskPanelID}>
        <div className="project-task-list grid gap-px">
          {visibleTaskEntries.map((entry) => {
            if (entry.kind === "queued") {
              const task = entry.item;
              const selected = selectedQueuedTaskID === task.id;
              return <button type="button" className={`project-task-item grid w-full min-w-0 grid-cols-[10px_minmax(0,1fr)] items-center gap-1 rounded-md bg-transparent py-1.5 pr-2 pl-1 text-left transition-colors hover:bg-accent hover:text-foreground ${selected ? "selected bg-accent/70 text-foreground" : "text-muted-foreground"}`} title={task.title} aria-current={selected ? "page" : undefined} key={entry.key} onClick={() => openQueuedTask(task)}><span className="project-task-marker text-center font-bold text-primary" aria-hidden="true">{selected ? "*" : ""}</span><span className="project-task-title min-w-0 truncate text-[13px] font-medium">{task.title}</span></button>;
            }
            const run = entry.item;
            const title = run.task?.title || run.id;
            const selected = selectedRunID === run.id;
            return <button type="button" className={`project-task-item grid w-full min-w-0 grid-cols-[10px_minmax(0,1fr)] items-center gap-1 rounded-md bg-transparent py-1.5 pr-2 pl-1 text-left transition-colors hover:bg-accent hover:text-foreground ${selected ? "selected bg-accent/70 text-foreground" : "text-muted-foreground"}`} title={title} aria-current={selected ? "page" : undefined} key={entry.key} onClick={() => openRun(run)}><span className="project-task-marker text-center font-bold text-primary" aria-hidden="true">{selected ? "*" : ""}</span><span className="project-task-title min-w-0 truncate text-[13px] font-medium">{title}</span></button>;
          })}
          {!taskEntries.length && !runLoading && <div className="project-task-empty px-2 py-2 text-xs leading-relaxed text-muted-foreground">{taskSearch || taskStatus ? t("task.noMatches") : t("task.empty")}</div>}
          {runLoading && !taskEntries.length && <div className="project-task-empty px-2 py-2 text-xs leading-relaxed text-muted-foreground">{t("task.loading")}</div>}
        </div>
        {!taskListExpanded && taskEntries.length > SIDEBAR_TASK_PREVIEW_LIMIT && <Action size="compact" tone="muted" className="project-task-more" onClick={() => setTaskListExpanded(true)}>{t("sidebar.showTasks", { count: Math.max(0, taskTotal - SIDEBAR_TASK_PREVIEW_LIMIT) })}</Action>}
        {taskListExpanded && taskEntries.length > SIDEBAR_TASK_PREVIEW_LIMIT && <Action size="compact" tone="muted" className="project-task-more" onClick={() => setTaskListExpanded(false)}>{t("sidebar.hideTasks")}</Action>}
        {(taskListExpanded || taskEntries.length <= SIDEBAR_TASK_PREVIEW_LIMIT) && (runLoading || runHasMore) && <Action size="compact" tone="muted" className="project-task-more" disabled={runLoading} onClick={onLoadMoreRuns}>{runLoading ? t("task.loading") : t("task.loadMore", { visible: taskEntries.length, total: taskTotal })}</Action>}
      </div>}
    </div>;
  };

  const widthBounds = typeof window === "undefined" ? sidebarWidthBounds() : sidebarWidthBounds(window.innerWidth);
  return <aside className={`sidebar relative z-30 flex min-h-0 shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground ${resizing ? "resizing" : ""}`} style={{ width: `${width}px` }}>
    <div className="brand grid min-h-[62px] grid-cols-[minmax(0,1fr)_auto] items-center gap-2 border-b pt-3.5 pr-3 pb-3 pl-4"><strong className="block text-[15px] font-bold tracking-[0.14em] text-primary">ONESHOT</strong><Action ref={searchTrigger} size="compact" tone="muted" className={`sidebar-search-trigger ${workspaceSearchOpen ? "active" : ""}`} aria-label={t("sidebar.searchPanel")} aria-haspopup="dialog" aria-expanded={workspaceSearchOpen} aria-controls="global-command-palette" title={`${t("sidebar.searchPanel")} · ⌘K`} onClick={toggleSearch}><Search size={16} aria-hidden="true" /></Action></div>
    <div className="workspace-block flex min-h-0 flex-1 flex-col">
      <div className="project-sections min-h-0 flex-1 overflow-y-auto overscroll-contain px-2 pt-3 pb-3">
        <section className="project-section" aria-labelledby="pinned-project-heading">
          <div className="project-section-heading grid min-h-7 grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-2 px-1 pb-1.5 text-xs font-semibold text-muted-foreground" id="pinned-project-heading"><span>{t("sidebar.pinnedProjects")}</span><small>{pinnedWorkspaces.length}</small></div>
          <div className="workspace-list flex min-h-0 flex-none flex-col gap-0.5">{pinnedWorkspaces.map(renderWorkspace)}{!pinnedWorkspaces.length && <div className="sidebar-empty px-2 py-2 text-xs text-muted-foreground">{t("sidebar.noPinnedProjects")}</div>}</div>
        </section>
        <section className="project-section mt-3 border-t pt-3" aria-labelledby="project-heading">
          <div className="project-section-heading grid min-h-7 grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-2 px-1 pb-1.5 text-xs font-semibold text-muted-foreground" id="project-heading"><span>{t("sidebar.projects")}</span><small>{regularProjectCount}</small><Action size="compact" tone="primary" className="add-workspace" onClick={onAddWorkspace}>{t("sidebar.addProject")}</Action></div>
          <div className={`workspace-list flex min-h-0 flex-none flex-col gap-0.5 ${workspaceExpanded ? "expanded" : ""}`}>{projectWorkspaces.map(renderWorkspace)}{!workspaces.length && <div className="sidebar-empty px-2 py-3 text-xs text-muted-foreground">{t("sidebar.noWorkspaces")}</div>}{!projectWorkspaces.length && pinnedWorkspaces.length > 0 && <div className="sidebar-empty px-2 py-2 text-xs text-muted-foreground">{t("sidebar.allProjectsPinned")}</div>}</div>
          {regularProjectCount > 8 && <Action size="compact" tone="muted" className="workspace-expand" onClick={onToggleExpanded}>{workspaceExpanded ? t("sidebar.collapse") : t("sidebar.allProjects", { count: regularProjectCount })}</Action>}
        </section>
      </div>
    </div>
    <nav className="primary-nav relative mt-auto border-t">
      <div className="secondary-navigation static" ref={secondaryNavigation}>
        {secondaryNavigationOpen && <div className="secondary-navigation-menu absolute right-2 bottom-[calc(100%+6px)] left-2 z-25 grid gap-1 rounded-md border bg-popover p-1.5 text-popover-foreground shadow-md" id="sidebar-secondary-navigation" role="menu" aria-label={t("sidebar.menu")}>
          <button ref={firstSecondaryNavigationItem} role="menuitem" className={`w-full rounded-sm px-2.5 py-2 text-left text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground ${view === "workflows" || editor ? "active bg-accent text-accent-foreground" : "text-muted-foreground"}`} onClick={() => goToSecondaryView("workflows")}><b>{t("sidebar.workflows")}</b></button>
          <button role="menuitem" className={`w-full rounded-sm px-2.5 py-2 text-left text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground ${view === "settings" && !editor ? "active bg-accent text-accent-foreground" : "text-muted-foreground"}`} onClick={() => goToSecondaryView("settings")}><b>{t("sidebar.settings")}</b></button>
        </div>}
        <button ref={secondaryNavigationTrigger} className={`secondary-navigation-trigger flex min-h-[52px] w-full items-center gap-2.5 bg-transparent px-4 text-sm font-medium transition-colors hover:bg-accent ${view === "workflows" || view === "settings" || editor ? "active text-foreground" : "text-muted-foreground"}`} aria-label={t("sidebar.menu")} aria-haspopup="menu" aria-expanded={secondaryNavigationOpen} aria-controls="sidebar-secondary-navigation" onClick={() => setSecondaryNavigationOpen((open) => !open)}><span className="secondary-navigation-marker font-bold text-primary" aria-hidden="true">&gt;</span><b>{t("sidebar.menu")}</b></button>
      </div>
    </nav>
    {workspaceSearchOpen && <Suspense fallback={null}><CommandPalette open query={searchQuery} taskResults={searchTaskItems} loading={searchLoading} workspaces={workspaces} onQueryChange={onSearchQueryChange} onClose={closeSearch} onOpenTask={openTaskFromSearch} onOpenWorkspace={openWorkspaceFromSearch} onNewTask={() => { onGoView("tasks"); onNewTask(); }} onAddWorkspace={onAddWorkspace} onOpenSettings={() => onGoView("settings")} /></Suspense>}
    <div className="sidebar-resizer group absolute top-0 -right-[5px] z-20 h-full w-2.5 cursor-col-resize touch-none select-none" role="separator" aria-label={t("sidebar.resize")} aria-orientation="vertical" aria-valuemin={widthBounds.min} aria-valuemax={widthBounds.max} aria-valuenow={width} tabIndex={0} title={t("sidebar.resizeHint")} onDoubleClick={() => commitWidth(SIDEBAR_DEFAULT_WIDTH)} onKeyDown={resizeWithKeyboard} onPointerDown={startResize}><span aria-hidden="true" className={`absolute inset-y-0 left-1 w-px bg-transparent group-hover:w-0.5 group-hover:bg-ring group-focus-visible:w-0.5 group-focus-visible:bg-ring ${resizing ? "w-0.5 bg-ring" : ""}`} /></div>
  </aside>;
}

export default memo(Sidebar);
