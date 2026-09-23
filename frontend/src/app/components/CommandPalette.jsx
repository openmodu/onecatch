import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { BookOpen, ChartNoAxesCombined, Circle, FileText, Folder, FolderPlus, GitBranch, Search, Settings, SquarePen } from "lucide-react";
import { commandPaletteShortcutIndex, commandPaletteWorkspaceResults, moveCommandPaletteIndex } from "../commandPaletteNavigation.js";
import { paletteCommandResults, readCommandHistory, rememberCommand } from "../paletteCommands.js";
import { primaryShortcutLabel } from "../platform.js";

function PaletteRow({ item, active, onActivate, onActive }) {
  const Icon = item.icon;
  return <button type="button" id={`palette-${item.key}`} role="option" aria-selected={active} tabIndex={-1} className={`command-palette__item ${active ? "active" : ""}`} onPointerMove={onActive} onClick={onActivate}>
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
  onOpenView,
}) {
  const { t } = useTranslation();
  const inputRef = useRef(null);
  const bodyRef = useRef(null);
  const [history] = useState(() => { try { return readCommandHistory(window.localStorage); } catch { return []; } });
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
      meta: workspace.remoteFs ? `${workspace.remoteFs.username ? `${workspace.remoteFs.username}@` : ""}${workspace.remoteFs.host}:${workspace.remoteFs.root}` : workspace.path,
      shortcutLabel,
      shortcutKey: "",
    })), [normalizedQuery, taskItems.length, workspaces]);

  const icons = { "new-task": SquarePen, "add-workspace": FolderPlus, templates: FileText, skills: BookOpen, usage: ChartNoAxesCombined, settings: Settings };
  const commandItems = paletteCommandResults(t, normalizedQuery, history).map((command) => ({
    key: `command:${command.id}`, kind: "command", command: command.id, icon: icons[command.id], label: command.label,
    meta: !normalizedQuery && command.recentIndex >= 0 ? t("palette.recent") : "",
    shortcutLabel: command.shortcut ? primaryShortcutLabel(command.shortcut) : "",
    shortcutKey: command.shortcut?.toLowerCase() || "",
  }));
  const items = normalizedQuery ? [...commandItems, ...taskItems, ...projectItems] : [...taskItems, ...projectItems, ...commandItems];
  const resultCount = items.length;

  useEffect(() => {
    bodyRef.current?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: "nearest" });
  }, [activeIndex]);

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
    else if (item.kind === "command") onOpenView(item.command);
    if (item.kind === "command") { try { rememberCommand(window.localStorage, item.command); } catch { /* Storage may be unavailable. */ } }
    onClose({ restoreFocus: false });
  };
  const handleKeyDown = (event) => {
    event.stopPropagation();
    if (event.nativeEvent?.isComposing) return;
    if (event.key === "Tab") { event.preventDefault(); inputRef.current?.focus(); return; }
    if (event.key === "Escape" || ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k")) {
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
    <div role="group" aria-label={label}>
      {groupItems.map((item, index) => <PaletteRow key={item.key} item={item} active={activeIndex === offset + index} onActive={() => setActiveIndex(offset + index)} onActivate={() => activate(item)} />)}
    </div>
  </section> : null;

  return createPortal(<div className="command-palette-backdrop" onPointerDown={(event) => event.target === event.currentTarget && onClose()}>
    <section className="command-palette" id="global-command-palette" role="dialog" aria-modal="true" aria-busy={loading} aria-label={t("sidebar.commandPalette")} onKeyDown={handleKeyDown}>
      <label className="command-palette__search">
        <span className="command-palette__search-icon" aria-hidden="true"><Search size={16} /></span>
        <input spellCheck={false} autoCorrect="off" autoCapitalize="none" ref={inputRef} autoFocus value={query} role="combobox" aria-expanded="true" aria-controls="command-palette-results" aria-activedescendant={items[activeIndex] ? `palette-${items[activeIndex].key}` : undefined} aria-autocomplete="list" aria-label={t("sidebar.searchTasksCommands")} placeholder={t("sidebar.searchTasksCommands")} onChange={(event) => onQueryChange(event.target.value)} />
        <kbd>Esc</kbd>
      </label>
      <div ref={bodyRef} className="command-palette__body" id="command-palette-results" role="listbox" aria-label={t("sidebar.commandPalette")}>
        {normalizedQuery && renderGroup(t("palette.commands"), commandItems, 0)}
        {loading && !taskItems.length && <div className="command-palette__empty">{t("common.loading")}</div>}
        {renderGroup(t("task.tasks"), taskItems, normalizedQuery ? commandItems.length : 0)}
        {renderGroup(t("sidebar.projects"), projectItems, taskItems.length + (normalizedQuery ? commandItems.length : 0))}
        {!loading && normalizedQuery && resultCount === 0 && <div className="command-palette__empty">{t("sidebar.noSearchResults")}</div>}
        {!normalizedQuery && renderGroup(t("palette.commands"), commandItems, taskItems.length + projectItems.length)}
      </div>
    </section>
  </div>, document.body);
}
