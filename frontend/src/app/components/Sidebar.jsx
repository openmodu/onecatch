import { lazy, memo, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Events } from "@wailsio/runtime";
import { Boxes, Ellipsis, Folder, FolderOpen, Languages, Menu, Palette, PanelLeftClose, PanelLeftOpen, Pencil, Pin, Plus, Search, Settings2, SunMoon, Trash2, Workflow } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Action } from "../../ui/primitives.jsx";
import { APPEARANCE_CHANGED_EVENT, accentThemes, readAppearance, saveAppearance } from "../appearance.js";
import { LANGUAGE_CHANGED_EVENT, normalizeLanguage } from "../../i18n.js";
import {
  SIDEBAR_DEFAULT_WIDTH,
  SIDEBAR_KEYBOARD_STEP,
  clampSidebarWidth,
  readSidebarWidth,
  sidebarWidthBounds,
  writeSidebarWidth,
} from "../sidebarLayout.js";
import { SIDEBAR_TASK_PREVIEW_LIMIT, buildSidebarTaskEntries, visibleSidebarTaskEntries } from "../sidebarNavigation.js";
import { desktopPlatform, primaryShortcutLabel } from "../platform.js";

const CommandPalette = lazy(() => import("./CommandPalette.jsx"));
const SIDEBAR_COLLAPSED_STORAGE_KEY = "onecatch.sidebar.collapsed";
const SIDEBAR_PEEK_WIDTH = 216;

function initialSidebarWidth() {
  if (typeof window === "undefined") return SIDEBAR_DEFAULT_WIDTH;
  return readSidebarWidth(window.localStorage, window.innerWidth) ?? clampSidebarWidth(SIDEBAR_DEFAULT_WIDTH, window.innerWidth);
}

function initialSidebarCollapsed() {
  if (typeof window === "undefined") return false;
  try { return window.localStorage.getItem(SIDEBAR_COLLAPSED_STORAGE_KEY) === "true"; } catch { return false; }
}

function disarmMacSidebarDoubleClick(event) {
  if (desktopPlatform() !== "macos" || event.button !== 0 || event.detail !== 2) return;
  const dragHandle = event.currentTarget;
  // Wails handles draggable-region double clicks on window capture before
  // React sees the dblclick event. Disarm only the second press so the first
  // press can still become a normal window drag.
  dragHandle.style.setProperty("--wails-draggable", "no-drag");
  window.setTimeout(() => dragHandle.style.removeProperty("--wails-draggable"), 500);
}

function finishMacSidebarDoubleClick(event) {
  if (desktopPlatform() !== "macos") return;
  event.preventDefault();
  event.currentTarget.style.removeProperty("--wails-draggable");
}

// Its workspace and task data only change when the active project changes or a
// run is updated; memo keeps unrelated settings/editor renders out of the rail.
function Sidebar({
  workspaces,
  workspaceID,
  projectWorkspaces,
  searchQuery,
  searchTaskItems,
  searchLoading,
  workspaceExpanded,
  workspaceSearchOpen,
  tasks,
  pinnedTasks,
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
  onToggleTaskPinned,
  onDeleteTask,
  onRenameTask,
  onRemoveWorkspace,
  onToggleExpanded,
  onAddWorkspace,
  onNewTask,
  onLoadMoreRuns,
  onSelectRun,
  onSelectQueued,
  onGoView,
  onCollapsedChange,
}) {
  const { t, i18n } = useTranslation();
  const [width, setWidth] = useState(initialSidebarWidth);
  const [resizing, setResizing] = useState(false);
  const [appearance, setAppearance] = useState(readAppearance);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(initialSidebarCollapsed);
  const [sidebarPeeked, setSidebarPeeked] = useState(false);
  const [expandedWorkspaceID, setExpandedWorkspaceID] = useState(workspaceID);
  const [taskListExpanded, setTaskListExpanded] = useState(false);
  const [pendingSearchTask, setPendingSearchTask] = useState(null);
  const drag = useRef(null);
  const sidebarToggleRef = useRef(null);
  const searchTrigger = useRef(null);
  const sidebarPeekTimer = useRef(null);
  const sidebarPeekBlocked = useRef(false);
  const sidebarMenuOpen = useRef(false);
  const sidebarVisible = !sidebarCollapsed || sidebarPeeked;
  const sidebarDisplayWidth = sidebarCollapsed ? Math.min(width, SIDEBAR_PEEK_WIDTH) : width;

  const taskEntries = useMemo(() => buildSidebarTaskEntries(tasks, runs, { query: taskSearch, status: taskStatus }).filter((entry) => !(entry.kind === "run" ? entry.item.task : entry.item)?.pinned), [runs, taskSearch, taskStatus, tasks]);
  const visibleTaskEntries = visibleSidebarTaskEntries(taskEntries, taskListExpanded);
  const compactTaskEntryCount = visibleSidebarTaskEntries(taskEntries).length;
  const hiddenTaskCount = Math.max(0, taskEntries.length - compactTaskEntryCount);
  const queuedEntryCount = taskEntries.filter((entry) => entry.kind === "queued").length;
  const taskTotal = Math.max(taskEntries.length, queuedEntryCount + (taskStatus === "queued" ? 0 : runTotal));
  const regularProjectCount = workspaces.length;
  const closeSearch = useCallback(({ restoreFocus = true } = {}) => {
    onClearSearch();
    if (workspaceSearchOpen) onToggleSearch();
    if (sidebarCollapsed) {
      sidebarPeekBlocked.current = restoreFocus;
      setSidebarPeeked(false);
    }
    requestAnimationFrame(() => {
      if (!restoreFocus) {
        document.activeElement?.blur?.();
        return;
      }
      if (sidebarCollapsed) sidebarToggleRef.current?.focus({ preventScroll: true });
      else searchTrigger.current?.focus({ preventScroll: true });
    });
  }, [onClearSearch, onToggleSearch, sidebarCollapsed, workspaceSearchOpen]);
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
      window.clearTimeout(sidebarPeekTimer.current);
    };
  }, []);

  useEffect(() => Events.On(APPEARANCE_CHANGED_EVENT, (event) => setAppearance(event?.data || readAppearance())), []);

  useEffect(() => {
    // On macOS the WebView is transparent over a native NSVisualEffectView.
    // Browser previews simply have no such handler and keep the CSS fallback.
    const nativeSidebar = globalThis.webkit?.messageHandlers?.onecatchSidebar;
    if (!nativeSidebar) return;
    document.documentElement.dataset.nativeSidebarMaterial = "true";
    nativeSidebar.postMessage({
      width: sidebarVisible ? sidebarDisplayWidth : 0,
      compact: sidebarCollapsed && sidebarVisible,
    });
  }, [sidebarCollapsed, sidebarDisplayWidth, sidebarVisible]);

  useEffect(() => {
    setExpandedWorkspaceID(workspaceID);
    setTaskListExpanded(false);
  }, [workspaceID]);

  useEffect(() => setTaskListExpanded(false), [taskSearch, taskStatus]);

  useEffect(() => {
    onCollapsedChange?.(sidebarCollapsed);
  }, [onCollapsedChange, sidebarCollapsed]);

  useEffect(() => {
    if (!pendingSearchTask || pendingSearchTask.workspace.id !== workspaceID) return;
    setExpandedWorkspaceID(workspaceID);
    setTaskListExpanded(false);
    if (pendingSearchTask.latestRun) onSelectRun(pendingSearchTask.latestRun);
    else onSelectQueued(pendingSearchTask.task);
    setPendingSearchTask(null);
  }, [onSelectQueued, onSelectRun, pendingSearchTask, workspaceID]);

  useEffect(() => {
    const openWithKeyboard = (event) => {
      if (!(event.metaKey || event.ctrlKey) || event.key.toLocaleLowerCase() !== "k") return;
      event.preventDefault();
      if (workspaceSearchOpen) closeSearch(); else openSearch();
    };
    window.addEventListener("keydown", openWithKeyboard);
    return () => window.removeEventListener("keydown", openWithKeyboard);
  }, [closeSearch, openSearch, workspaceSearchOpen]);

  const commitWidth = (next) => {
    const fitted = clampSidebarWidth(next, window.innerWidth);
    setWidth(fitted);
    writeSidebarWidth(window.localStorage, fitted);
  };
  const updateLanguage = (language) => {
    const next = normalizeLanguage(language);
    void i18n.changeLanguage(next);
    void Events.Emit(LANGUAGE_CHANGED_EVENT, next);
  };
  const updateAppearance = (change) => {
    const next = saveAppearance({ ...appearance, ...change });
    setAppearance(next);
    void Events.Emit(APPEARANCE_CHANGED_EVENT, next);
  };
  const displayedTheme = appearance.theme === "dark" ? "dark" : appearance.theme === "light" ? "light" : globalThis.matchMedia?.("(prefers-color-scheme: dark)")?.matches ? "dark" : "light";
  const cancelSidebarHide = () => window.clearTimeout(sidebarPeekTimer.current);
  const revealSidebar = () => {
    cancelSidebarHide();
    if (sidebarCollapsed && !sidebarPeekBlocked.current) setSidebarPeeked(true);
  };
  const scheduleSidebarHide = () => {
    cancelSidebarHide();
    sidebarPeekTimer.current = window.setTimeout(() => {
      if (!sidebarMenuOpen.current) setSidebarPeeked(false);
    }, 120);
  };
  const toggleSidebar = () => {
    cancelSidebarHide();
    setSidebarPeeked(false);
    setSidebarCollapsed((current) => {
      const next = !current;
      // A pointer click leaves the button hovered and focused. Do not turn
      // that same click into an immediate peek; require a real leave/re-enter.
      sidebarPeekBlocked.current = next;
      try { window.localStorage.setItem(SIDEBAR_COLLAPSED_STORAGE_KEY, String(next)); } catch { /* best effort */ }
      return next;
    });
  };
  const releaseSidebarPeekBlock = () => {
    sidebarPeekBlocked.current = false;
    scheduleSidebarHide();
  };
  const goToSecondaryView = (nextView) => onGoView(nextView);
  const toggleProject = (workspace) => {
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
  const openPinnedTask = (task) => {
    const workspace = workspaces.find((item) => item.id === task.workspaceId);
    if (workspace && workspace.id !== workspaceID) {
      setPendingSearchTask({ workspace, task, latestRun: null });
      onSelectWorkspace(workspace.id);
      return;
    }
    openQueuedTask(task);
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
    onGoView("tasks");
    if (workspace.id !== workspaceID) onSelectWorkspace(workspace.id);
    onNewTask();
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

  const renderTaskActions = (task) => <div className="task-row-actions absolute top-1 right-1 flex h-6 items-center gap-0.5">
    <Action size="compact" tone="muted" className={`size-6 border-0 bg-transparent p-0 shadow-none hover:bg-background/70 ${task.pinned ? "text-foreground opacity-70" : "pointer-events-none opacity-0 group-hover/task:pointer-events-auto group-hover/task:opacity-100 group-focus-within/task:pointer-events-auto group-focus-within/task:opacity-100"}`} aria-label={t(task.pinned ? "common.unpin" : "common.pin")} title={t(task.pinned ? "common.unpin" : "common.pin")} onClick={() => onToggleTaskPinned(task)}><Pin size={13} className={task.pinned ? "fill-current" : ""} aria-hidden="true" /></Action>
    <Action size="compact" tone="muted" className="pointer-events-none size-6 border-0 bg-transparent p-0 text-muted-foreground opacity-0 shadow-none hover:bg-background/70 hover:text-foreground group-hover/task:pointer-events-auto group-hover/task:opacity-100 group-focus-within/task:pointer-events-auto group-focus-within/task:opacity-100" aria-label={t("task.rename")} title={t("task.rename")} onClick={() => onRenameTask(task)}><Pencil size={13} strokeWidth={2} aria-hidden="true" /></Action>
    <Action size="compact" tone="muted" className="pointer-events-none size-6 border-0 bg-transparent p-0 text-muted-foreground opacity-0 shadow-none hover:bg-destructive/10 hover:text-destructive group-hover/task:pointer-events-auto group-hover/task:opacity-100 group-focus-within/task:pointer-events-auto group-focus-within/task:opacity-100" aria-label={t("app.deleteTask")} title={t("app.deleteTask")} onClick={() => onDeleteTask(task)}><Trash2 size={13} aria-hidden="true" /></Action>
  </div>;

  const renderPinnedTask = (task) => {
    const selectedRun = runs.find((run) => run.id === selectedRunID);
    const selected = selectedQueuedTaskID === task.id || selectedRun?.task?.id === task.id;
    return <div className={`group/task relative w-full min-w-0 max-w-full overflow-hidden rounded-lg ${selected ? "bg-accent" : ""}`} key={task.id}><button type="button" className={`flex h-8 w-full min-w-0 max-w-full items-center rounded-lg bg-transparent py-0 pr-20 pl-2 text-left transition-colors hover:bg-accent/60 hover:text-foreground ${selected ? "text-foreground" : "text-muted-foreground"}`} title={task.title} aria-current={selected ? "page" : undefined} onClick={() => openPinnedTask(task)}><span className={`block min-w-0 flex-1 truncate text-[13px] ${selected ? "font-medium" : "font-normal"}`}>{task.title}</span></button>{renderTaskActions(task)}</div>;
  };

  const renderWorkspace = (workspace) => {
    const active = workspace.id === workspaceID;
    const expanded = expandedWorkspaceID === workspace.id;
    const taskPanelID = `workspace-tasks-${encodeURIComponent(workspace.id)}`;
    return <div className={`workspace-row group relative block w-full min-w-0 max-w-full overflow-hidden ${active ? "active" : ""} ${expanded ? "expanded" : ""}`} key={workspace.id}>
      <button className={`workspace-item grid h-8 w-full min-w-0 grid-cols-[16px_minmax(0,1fr)] items-center gap-2 rounded-lg py-0 pr-14 pl-2 text-left transition-colors hover:bg-accent/70 hover:text-foreground ${active ? "text-foreground" : "text-muted-foreground"}`} title={workspace.path} aria-expanded={expanded} aria-controls={taskPanelID} onClick={() => toggleProject(workspace)}>
        {expanded ? <FolderOpen size={16} strokeWidth={2} aria-hidden="true" className="text-muted-foreground" /> : <Folder size={16} strokeWidth={2} aria-hidden="true" className="text-muted-foreground" />}
        <span className="min-w-0"><strong className="block truncate text-[13px] font-medium leading-none">{workspace.name}</strong></span>
      </button>
      {/* Hidden by opacity rather than `display: none`, for two reasons:
          display:none drops the buttons out of the tab order, so they were
          unreachable by keyboard and group-focus-within could never fire; and
          it un-focuses the trigger the moment the menu closes, leaving Radix
          nowhere to restore focus to. has-[[data-state=open]] keeps the row
          revealed while the menu is up — Radix stamps that on the trigger. */}
      <div className="workspace-row-actions pointer-events-none absolute top-1 right-1 z-20 flex h-6 items-center gap-0.5 opacity-0 transition-opacity group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100 has-[[data-state=open]]:pointer-events-auto has-[[data-state=open]]:opacity-100">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Action size="compact" tone="muted" className="workspace-menu-trigger size-6 border-0 bg-transparent p-0 shadow-none hover:bg-accent" aria-label={t("sidebar.projectMenu", { name: workspace.name })} title={t("sidebar.projectMenu", { name: workspace.name })}><Ellipsis size={14} aria-hidden="true" /></Action>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" side="right" className="w-44">
            <DropdownMenuItem variant="destructive" onSelect={() => onRemoveWorkspace(workspace)}>
              <Trash2 size={15} aria-hidden="true" />{t("common.remove")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <Action size="compact" tone="muted" className="workspace-new-task size-6 border-0 bg-transparent p-0 shadow-none hover:bg-accent" aria-label={t("sidebar.newTaskInProject", { name: workspace.name })} title={t("sidebar.newTaskInProject", { name: workspace.name })} onClick={() => createTaskForWorkspace(workspace)}><Plus size={14} strokeWidth={2} aria-hidden="true" /></Action>
      </div>
      {expanded && active && <div className="project-task-panel mb-1" id={taskPanelID}>
        <div className="project-task-list grid gap-px">
          {visibleTaskEntries.map((entry) => {
            if (entry.kind === "queued" || entry.kind === "pinned") {
              const task = entry.item;
              const selected = selectedQueuedTaskID === task.id;
              return <div className={`group/task relative w-full min-w-0 max-w-full overflow-hidden rounded-lg ${selected ? "bg-accent" : ""}`} key={entry.key}><button type="button" className={`project-task-item flex h-8 w-full min-w-0 max-w-full items-center rounded-lg bg-transparent py-0 pr-20 pl-8 text-left transition-colors hover:bg-accent/60 hover:text-foreground ${selected ? "selected text-foreground" : "text-muted-foreground"}`} title={task.title} aria-current={selected ? "page" : undefined} onClick={() => openQueuedTask(task)}><span className={`project-task-title block min-w-0 flex-1 truncate text-[13px] ${selected ? "font-medium" : "font-normal"}`}>{task.title}</span></button>{renderTaskActions(task)}</div>;
            }
            const run = entry.item;
            const task = run.task;
            const title = run.task?.title || run.id;
            const selected = selectedRunID === run.id;
            return <div className={`group/task relative w-full min-w-0 max-w-full overflow-hidden rounded-lg ${selected ? "bg-accent" : ""}`} key={entry.key}><button type="button" className={`project-task-item flex h-8 w-full min-w-0 max-w-full items-center rounded-lg bg-transparent py-0 pr-20 pl-8 text-left transition-colors hover:bg-accent/60 hover:text-foreground ${selected ? "selected text-foreground" : "text-muted-foreground"}`} title={title} aria-current={selected ? "page" : undefined} onClick={() => openRun(run)}><span className={`project-task-title block min-w-0 flex-1 truncate text-[13px] ${selected ? "font-medium" : "font-normal"}`}>{title}</span></button>{task && renderTaskActions(task)}</div>;
          })}
          {!taskEntries.length && !runLoading && <div className="project-task-empty px-2 py-2 text-xs leading-relaxed text-muted-foreground">{taskSearch || taskStatus ? t("task.noMatches") : t("task.empty")}</div>}
          {runLoading && !taskEntries.length && <div className="project-task-empty px-2 py-2 text-xs leading-relaxed text-muted-foreground">{t("task.loading")}</div>}
        </div>
        {!taskListExpanded && hiddenTaskCount > 0 && <Action size="compact" tone="muted" className="project-task-more h-8 w-full justify-start border-0 bg-transparent pr-2 pl-8 text-[13px] font-normal text-muted-foreground/70 shadow-none hover:bg-transparent hover:text-foreground" onClick={() => setTaskListExpanded(true)}>{t("sidebar.showTasks", { count: hiddenTaskCount })}</Action>}
        {taskListExpanded && hiddenTaskCount > 0 && <Action size="compact" tone="muted" className="project-task-more h-8 w-full justify-start border-0 bg-transparent pr-2 pl-8 text-[13px] font-normal text-muted-foreground/70 shadow-none hover:bg-transparent hover:text-foreground" onClick={() => setTaskListExpanded(false)}>{t("sidebar.hideTasks")}</Action>}
        {(taskListExpanded || hiddenTaskCount === 0) && (runLoading || runHasMore) && <Action size="compact" tone="muted" className="project-task-more h-8 w-full justify-start border-0 bg-transparent pr-2 pl-8 text-[13px] font-normal text-muted-foreground/70 shadow-none hover:bg-transparent hover:text-foreground" disabled={runLoading} onClick={onLoadMoreRuns}>{runLoading ? t("task.loading") : t("task.loadMore", { visible: taskEntries.length, total: taskTotal })}</Action>}
      </div>}
    </div>;
  };

  const widthBounds = typeof window === "undefined" ? sidebarWidthBounds() : sidebarWidthBounds(window.innerWidth);
  return <div className={`sidebar-shell relative z-30 h-full min-h-0 shrink-0 ${sidebarCollapsed ? "is-collapsed" : ""}`} style={{ width: sidebarCollapsed ? 0 : `${width}px` }}>
    <button ref={sidebarToggleRef} type="button" className={`sidebar-visibility-toggle no-drag fixed top-3 left-[92px] z-50 grid place-items-center text-muted-foreground transition-colors hover:text-foreground focus-visible:text-foreground focus-visible:outline-none ${sidebarCollapsed ? "text-foreground" : ""}`} aria-label={sidebarCollapsed ? t("sidebar.expandPanel") : t("sidebar.collapsePanel")} aria-expanded={!sidebarCollapsed} aria-controls="app-sidebar-content" title={sidebarCollapsed ? t("sidebar.expandPanel") : t("sidebar.collapsePanel")} onClick={toggleSidebar} onPointerEnter={revealSidebar} onPointerLeave={releaseSidebarPeekBlock} onFocus={revealSidebar} onBlur={releaseSidebarPeekBlock}>{sidebarCollapsed ? <PanelLeftOpen size={16} aria-hidden="true" /> : <PanelLeftClose size={16} aria-hidden="true" />}</button>
    <aside id="app-sidebar-content" data-visible={sidebarVisible ? "true" : "false"} className={`sidebar z-30 flex min-h-0 shrink-0 select-none flex-col text-sidebar-foreground [clip-path:inset(8px_4px_8px_8px_round_16px)] ${resizing ? "resizing" : ""} ${sidebarCollapsed ? "absolute top-0 left-0 h-[min(560px,calc(100vh-16px))] transition-[opacity,transform] duration-150" : "relative h-full"} ${sidebarVisible ? "translate-x-0 opacity-100" : "pointer-events-none invisible -translate-x-2 opacity-0"}`} style={{ width: `${sidebarDisplayWidth}px` }} aria-label={t("app.windowAria")} aria-hidden={!sidebarVisible} onPointerEnter={revealSidebar} onPointerLeave={scheduleSidebarHide}>
    {/* Traffic-light gutter. The window hides its titlebar and insets the
        lights, so the rail has to reserve this strip itself — and it doubles
        as the window's drag handle, which is why it is empty. */}
    <div className="drag-region h-[52px] shrink-0 cursor-default" aria-hidden="true" onMouseDown={disarmMacSidebarDoubleClick} onDoubleClick={finishMacSidebarDoubleClick} />
    <div className="brand grid h-[46px] shrink-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 pr-4 pl-7"><strong className="block text-[15px] font-semibold tracking-tight text-foreground">OneCatch</strong><div className="flex items-center gap-0.5"><Action ref={searchTrigger} size="compact" tone="muted" className={`sidebar-search-trigger size-7 border-0 bg-transparent p-0 shadow-none hover:bg-accent ${workspaceSearchOpen ? "active bg-accent text-foreground" : ""}`} aria-label={t("sidebar.searchPanel")} aria-haspopup="dialog" aria-expanded={workspaceSearchOpen} aria-controls="global-command-palette" title={`${t("sidebar.searchPanel")} · ⌘K`} onClick={toggleSearch}><Search size={15} strokeWidth={2} aria-hidden="true" /></Action><Action size="compact" tone="muted" className="add-workspace size-7 border-0 bg-transparent p-0 shadow-none hover:bg-accent hover:text-foreground" aria-label={t("sidebar.addProject")} title={t("sidebar.addProject")} onClick={onAddWorkspace}><Plus size={15} strokeWidth={2} aria-hidden="true" /></Action></div></div>
    <div className="workspace-block flex min-h-0 flex-1 flex-col">
      <div className="project-sections min-h-0 min-w-0 max-w-full flex-1 overflow-x-hidden overflow-y-auto overscroll-contain px-3 pt-2 pb-3">
        {pinnedTasks.length > 0 && <section className="project-section mb-3 min-w-0 max-w-full" aria-labelledby="pinned-task-heading">
          <div className="flex h-7 items-center px-2 text-[11px] font-bold text-foreground" id="pinned-task-heading">{t("sidebar.pinnedTasks")}</div>
          <div className="flex flex-col">{pinnedTasks.map(renderPinnedTask)}</div>
        </section>}
        <section className="project-section min-w-0 max-w-full" aria-labelledby="project-heading">
          <div className="flex h-7 items-center px-2 text-[11px] font-bold text-foreground" id="project-heading">{t("sidebar.projects")}</div>
          <div className={`workspace-list flex min-h-0 min-w-0 max-w-full flex-none flex-col ${workspaceExpanded ? "expanded" : ""}`}>{projectWorkspaces.map(renderWorkspace)}{!workspaces.length && <div className="sidebar-empty px-2 py-3 text-xs text-muted-foreground">{t("sidebar.noWorkspaces")}</div>}</div>
          {regularProjectCount > 8 && <Action size="compact" tone="muted" className="workspace-expand h-8 w-full justify-start border-0 bg-transparent pr-2 pl-8 text-[13px] font-normal text-muted-foreground/70 shadow-none hover:bg-transparent hover:text-foreground" onClick={onToggleExpanded}>{workspaceExpanded ? t("sidebar.collapse") : t("sidebar.allProjects", { count: regularProjectCount })}</Action>}
        </section>
      </div>
    </div>
    <nav className="primary-nav relative mt-auto pb-1">
      {/* side="top" + sideOffset opens the menu upward and full-rail wide,
          matching where the footer sits. */}
      <DropdownMenu onOpenChange={(open) => { sidebarMenuOpen.current = open; if (open) revealSidebar(); else scheduleSidebarHide(); }}>
        <DropdownMenuTrigger asChild>
          <button className={`secondary-navigation-trigger grid min-h-[52px] w-full grid-cols-[20px_minmax(0,1fr)] items-center gap-2 border-0 bg-transparent pr-4 pl-7 text-left text-sm font-medium shadow-none transition-colors hover:bg-accent focus-visible:bg-accent focus-visible:outline-none focus-visible:ring-0 ${view === "workflows" || view === "settings" || editor ? "active text-foreground" : "text-muted-foreground"}`} aria-label={t("sidebar.menu")}>
            <Menu size={16} aria-hidden="true" />
            <b className="font-medium">{t("sidebar.menu")}</b>
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent side="top" align="start" alignOffset={8} sideOffset={6} collisionPadding={12} className="secondary-navigation-menu w-[calc(var(--radix-dropdown-menu-trigger-width)-20px)] p-1.5">
          <DropdownMenuItem className={`min-h-8 rounded-md ${view === "settings" && !editor ? "active bg-accent text-accent-foreground" : ""}`} onSelect={() => goToSecondaryView("settings")}>
            <Settings2 aria-hidden="true" />
            <span>{t("sidebar.settings")}</span>
            <DropdownMenuShortcut>{primaryShortcutLabel(",")}</DropdownMenuShortcut>
          </DropdownMenuItem>
          <DropdownMenuSub>
            <DropdownMenuSubTrigger className="min-h-8 rounded-md">
              <Languages aria-hidden="true" />
              <span>{t("settings.language")}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent sideOffset={7} className="min-w-44 p-1.5">
              <DropdownMenuRadioGroup value={normalizeLanguage(i18n.resolvedLanguage)} onValueChange={updateLanguage}>
                <DropdownMenuRadioItem className="min-h-8 rounded-md" value="zh-CN">{t("language.chinese")}</DropdownMenuRadioItem>
                <DropdownMenuRadioItem className="min-h-8 rounded-md" value="en">{t("language.english")}</DropdownMenuRadioItem>
              </DropdownMenuRadioGroup>
            </DropdownMenuSubContent>
          </DropdownMenuSub>
          <DropdownMenuSub>
            <DropdownMenuSubTrigger className="min-h-8 rounded-md">
              <Palette aria-hidden="true" />
              <span>{t("settings.themeColor")}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent sideOffset={7} className="min-w-44 p-1.5">
              <DropdownMenuRadioGroup value={appearance.accent} onValueChange={(accent) => updateAppearance({ accent })}>
                {accentThemes.map((accent) => <DropdownMenuRadioItem className="min-h-8 rounded-md" value={accent} key={accent}><i className={`menu-accent-swatch accent-${accent}`} aria-hidden="true" />{t(`settings.themeColor.${accent}`)}</DropdownMenuRadioItem>)}
              </DropdownMenuRadioGroup>
            </DropdownMenuSubContent>
          </DropdownMenuSub>
          <DropdownMenuSub>
            <DropdownMenuSubTrigger className="min-h-8 rounded-md">
              <SunMoon aria-hidden="true" />
              <span>{t("settings.colorMode")}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent sideOffset={7} className="min-w-36 p-1.5">
              <DropdownMenuRadioGroup value={displayedTheme} onValueChange={(theme) => updateAppearance({ theme })}>
                <DropdownMenuRadioItem className="min-h-8 rounded-md" value="light">{t("settings.colorMode.light")}</DropdownMenuRadioItem>
                <DropdownMenuRadioItem className="min-h-8 rounded-md" value="dark">{t("settings.colorMode.dark")}</DropdownMenuRadioItem>
              </DropdownMenuRadioGroup>
            </DropdownMenuSubContent>
          </DropdownMenuSub>
          <DropdownMenuSeparator />
          <DropdownMenuItem className={`min-h-8 rounded-md ${view === "whiteboard" ? "active bg-accent text-accent-foreground" : ""}`} onSelect={() => goToSecondaryView("whiteboard")}>
            <Boxes aria-hidden="true" />
            <span>{t("sidebar.whiteboard")}</span>
          </DropdownMenuItem>
          <DropdownMenuItem className={`min-h-8 rounded-md ${view === "workflows" || editor ? "active bg-accent text-accent-foreground" : ""}`} onSelect={() => goToSecondaryView("workflows")}>
            <Workflow aria-hidden="true" />
            <span>{t("sidebar.workflows")}</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </nav>
    {workspaceSearchOpen && <Suspense fallback={null}><CommandPalette open query={searchQuery} taskResults={searchTaskItems} loading={searchLoading} workspaces={workspaces} onQueryChange={onSearchQueryChange} onClose={closeSearch} onOpenTask={openTaskFromSearch} onOpenWorkspace={openWorkspaceFromSearch} onNewTask={() => { onGoView("tasks"); onNewTask(); }} onAddWorkspace={onAddWorkspace} onOpenSettings={() => onGoView("settings")} /></Suspense>}
    {!sidebarCollapsed && <div className="sidebar-resizer group absolute top-0 -right-[5px] z-20 h-full w-2.5 cursor-col-resize touch-none select-none" role="separator" aria-label={t("sidebar.resize")} aria-orientation="vertical" aria-valuemin={widthBounds.min} aria-valuemax={widthBounds.max} aria-valuenow={width} tabIndex={0} title={t("sidebar.resizeHint")} onDoubleClick={() => commitWidth(SIDEBAR_DEFAULT_WIDTH)} onKeyDown={resizeWithKeyboard} onPointerDown={startResize}><span aria-hidden="true" className={`absolute inset-y-0 left-1 w-px bg-transparent group-hover:w-0.5 group-hover:bg-ring group-focus-visible:w-0.5 group-focus-visible:bg-ring ${resizing ? "w-0.5 bg-ring" : ""}`} /></div>}
    </aside>
  </div>;
}

export default memo(Sidebar);
