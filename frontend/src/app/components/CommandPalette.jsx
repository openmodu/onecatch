import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { Circle, Folder, FolderPlus, GitBranch, Search, Settings, SquarePen } from "lucide-react";
import { commandPaletteShortcutIndex, commandPaletteWorkspaceResults, moveCommandPaletteIndex } from "../commandPaletteNavigation.js";

function PaletteRow({ item, active, onActivate, onActive }) {
  const Icon = item.icon;
  return <button type="button" role="option" aria-selected={active} tabIndex={-1} className={`command-palette__item ${active ? "active" : ""}`} onPointerMove={onActive} onClick={onActivate}>
    <span className="command-palette__icon" aria-hidden="true"><Icon size={15} strokeWidth={item.iconStrokeWidth || 2} /></span>
    <span className="command-palette__copy"><strong>{item.label}</strong>{item.description && <small>{item.description}</small>}</span>
    {item.meta && <span className="command-palette__meta" title={item.meta}>{item.meta}</span>}
    {item.shortcutLabel && <kbd>{item.shortcutLabel}</kbd>}
  </button>;
}

export default function CommandPalette({
  open,
  query,
  taskResults,
  loading,
  workspaces,
  onQueryChange,
  onClose,
  onOpenTask,
  onOpenWorkspace,
  onNewTask,
  onAddWorkspace,
  onOpenSettings,
}) {
  const { t } = useTranslation();
  const inputRef = useRef(null);
  const [activeIndex, setActiveIndex] = useState(0);
  const normalizedQuery = query.trim().toLocaleLowerCase();

  const taskItems = useMemo(() => taskResults.slice(0, 9).map((result, index) => ({
    key: `task:${result.task.id}`,
    kind: "task",
    result,
    icon: result.task.status === "queued" ? Circle : GitBranch,
    label: result.task.title,
    meta: result.workspace.name,
    shortcutLabel: `⌘${index + 1}`,
    shortcutKey: "",
  })), [taskResults]);

  const projectItems = useMemo(() => commandPaletteWorkspaceResults(workspaces, normalizedQuery, taskItems.length)
    .map(({ workspace, shortcutLabel }) => ({
      key: `workspace:${workspace.id}`,
      kind: "workspace",
      workspace,
      icon: Folder,
      label: workspace.name,
      meta: workspace.path,
      shortcutLabel,
      shortcutKey: "",
    })), [normalizedQuery, taskItems.length, workspaces]);

  const commandItems = [
    { key: "command:new-task", kind: "command", command: "new-task", icon: SquarePen, label: t("task.newTask"), shortcutLabel: "⌘N", shortcutKey: "n" },
    { key: "command:add-workspace", kind: "command", command: "add-workspace", icon: FolderPlus, label: t("sidebar.openFolder"), shortcutLabel: "⌘O", shortcutKey: "o" },
    { key: "command:settings", kind: "command", command: "settings", icon: Settings, label: t("sidebar.settings"), shortcutLabel: "⌘,", shortcutKey: "," },
  ];
  const items = [...taskItems, ...projectItems, ...commandItems];
  const resultCount = taskItems.length + projectItems.length;

  useEffect(() => {
    if (!open) return undefined;
    setActiveIndex(0);
    requestAnimationFrame(() => inputRef.current?.focus());
    return undefined;
  }, [open]);

  useEffect(() => {
    if (!open) return;
    setActiveIndex(0);
  }, [normalizedQuery, open]);

  useEffect(() => {
    if (!open) return;
    setActiveIndex((current) => Math.min(Math.max(0, current), Math.max(0, items.length - 1)));
  }, [items.length, open]);

  if (!open) return null;

  const activate = (item) => {
    if (!item) return;
    if (item.kind === "task") onOpenTask(item.result);
    else if (item.kind === "workspace") onOpenWorkspace(item.workspace);
    else if (item.command === "new-task") onNewTask();
    else if (item.command === "add-workspace") onAddWorkspace();
    else if (item.command === "settings") onOpenSettings();
    onClose({ restoreFocus: false });
  };
  const handleKeyDown = (event) => {
    if (event.nativeEvent?.isComposing) return;
    if (event.key === "Escape") {
      event.preventDefault();
      onClose();
      return;
    }
    if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
      event.preventDefault();
      setActiveIndex((current) => moveCommandPaletteIndex(current, items.length, event.key));
      return;
    }
    if (event.key === "Enter") {
      event.preventDefault();
      activate(items[activeIndex]);
      return;
    }
    if (event.metaKey || event.ctrlKey) {
      const shortcutIndex = commandPaletteShortcutIndex(items, event.key);
      if (shortcutIndex >= 0) {
        event.preventDefault();
        activate(items[shortcutIndex]);
      }
    }
  };
  const renderGroup = (label, groupItems, offset) => groupItems.length ? <section className="command-palette__group" aria-label={label}>
    <div className="command-palette__group-title">{label}</div>
    <div role="listbox" aria-label={label}>
      {groupItems.map((item, index) => <PaletteRow key={item.key} item={item} active={activeIndex === offset + index} onActive={() => setActiveIndex(offset + index)} onActivate={() => activate(item)} />)}
    </div>
  </section> : null;

  return createPortal(<div className="command-palette-backdrop" onPointerDown={(event) => event.target === event.currentTarget && onClose()}>
    <section className="command-palette" id="global-command-palette" role="dialog" aria-modal="true" aria-busy={loading} aria-label={t("sidebar.commandPalette")} onKeyDown={handleKeyDown}>
      <label className="command-palette__search">
        <span className="command-palette__search-icon" aria-hidden="true"><Search size={16} /></span>
        <input ref={inputRef} autoFocus value={query} aria-label={t("sidebar.searchTasksCommands")} placeholder={t("sidebar.searchTasksCommands")} onChange={(event) => onQueryChange(event.target.value)} />
        <kbd>Esc</kbd>
      </label>
      <div className="command-palette__body">
        {loading && !taskItems.length && <div className="command-palette__empty">{t("common.loading")}</div>}
        {renderGroup(t("task.tasks"), taskItems, 0)}
        {renderGroup(t("sidebar.projects"), projectItems, taskItems.length)}
        {!loading && normalizedQuery && resultCount === 0 && <div className="command-palette__empty">{t("sidebar.noSearchResults")}</div>}
        {renderGroup(t("sidebar.recommended"), commandItems, resultCount)}
      </div>
    </section>
  </div>, document.body);
}
