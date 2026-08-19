import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronDown, ChevronLeft, ChevronRight, Columns2, Maximize2, Minimize2, Plus, Rows2, Search, SquareTerminal, Trash2, X } from "lucide-react";
import { Browser, Clipboard, Events } from "@wailsio/runtime";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { TerminalBinding } from "../../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { errorMessage } from "../format.js";
import { decodeTerminalData, encodeTerminalData } from "../terminalCodec.js";
import { normalizeTerminalPreferences, resolveTerminalTheme } from "../terminalThemes.js";
import { paneIDs, paneNode, removePane, splitPane, updateSplitRatio } from "../terminalLayout.js";
import TerminalPane from "./TerminalPane.jsx";
import TerminalSplitLayout from "./TerminalSplitLayout.jsx";

const OUTPUT_EVENT = "onecatch:terminal-output";
const EXIT_EVENT = "onecatch:terminal-exit";
const DEFAULT_HEIGHT = 286;
const MIN_HEIGHT = 190;
const MAX_HEIGHT = 560;
const SEARCH_DECORATIONS = { matchBackground: "#d9b86c", matchOverviewRuler: "#9a762d", activeMatchBackground: "#e58c55", activeMatchColorOverviewRuler: "#d76f32" };

let nextTabID = 1;
let nextSplitID = 1;
const shortWorkspace = (path = "") => path.split(/[\\/]/).filter(Boolean).pop() || path;

const TerminalDock = forwardRef(function TerminalDock({ mode, workspace, preferences, notify, onVisibilityChange }, ref) {
  const { t } = useTranslation();
  const config = normalizeTerminalPreferences(preferences);
  const [open, setOpen] = useState(false);
  const [height, setHeight] = useState(DEFAULT_HEIGHT);
  const [maximized, setMaximized] = useState(false);
  const [tabs, setTabs] = useState([]);
  const [activeID, setActiveID] = useState("");
  const [focusedID, setFocusedID] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchFound, setSearchFound] = useState(true);
  const runtimeRef = useRef(new Map());
  const pendingRef = useRef(new Map());
  const resizeTimersRef = useRef(new Map());
  const cancelledTabsRef = useRef(new Set());
  const tabsRef = useRef(tabs);
  const activeRef = useRef(activeID);
  const focusedRef = useRef(focusedID);
  const resizeRef = useRef(null);
  tabsRef.current = tabs;
  activeRef.current = activeID;
  focusedRef.current = focusedID;

  const updateTab = useCallback((id, change) => setTabs((items) => items.map((item) => item.id === id ? { ...item, ...change } : item)), []);
  const writeTab = useCallback((id, data) => {
    const tab = tabsRef.current.find((item) => item.id === id);
    if (!tab?.session?.id || !tab.running || mode !== "wails") return;
    void TerminalBinding.WriteTerminal(tab.session.id, encodeTerminalData(data)).catch((error) => notify?.("error", errorMessage(error)));
  }, [mode, notify]);

  const registerRuntime = useCallback((id, runtime) => {
    if (!runtime) { runtimeRef.current.delete(id); return; }
    runtimeRef.current.set(id, runtime);
    const buffered = pendingRef.current.get(id) || [];
    buffered.forEach((data) => runtime.terminal.write(data));
    pendingRef.current.delete(id);
  }, []);

  const resizePTY = useCallback((id, rows, cols) => {
    const tab = tabsRef.current.find((item) => item.id === id);
    if (!tab?.session?.id || mode !== "wails") return;
    window.clearTimeout(resizeTimersRef.current.get(id));
    resizeTimersRef.current.set(id, window.setTimeout(() => {
      void TerminalBinding.ResizeTerminal(tab.session.id, rows, cols).catch(() => {});
    }, 140));
  }, [mode]);

  const focusActive = useCallback(() => {
    const runtime = runtimeRef.current.get(focusedRef.current || activeRef.current);
    requestAnimationFrame(() => { runtime?.fit.fit(); runtime?.terminal.focus(); });
  }, []);

  const createTab = useCallback(async (command = "", splitParentID = "", requestedID = "") => {
    setOpen(true);
    if (mode !== "wails") { notify?.("error", t("terminal.desktopOnly")); return null; }
    if (!workspace?.path) { notify?.("error", t("terminal.workspaceRequired")); return null; }
    const id = requestedID || `terminal-${nextTabID++}`;
    const tab = { id, session: null, workspace: workspace.path, title: shortWorkspace(workspace.path), running: false, starting: true, exitCode: null, splitParentID };
    setTabs((items) => [...items, tab]);
    if (!splitParentID) setActiveID(id);
    setFocusedID(id);
    setSearchOpen(false);
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    try {
      const runtime = runtimeRef.current.get(id);
      const session = await TerminalBinding.CreateTerminal({ workspace: workspace.path, shell: config.shell, arguments: config.arguments, rows: runtime?.terminal.rows || 24, cols: runtime?.terminal.cols || 80 });
      if (cancelledTabsRef.current.has(id)) {
        await TerminalBinding.CloseTerminal(session.id).catch(() => {});
        return null;
      }
      updateTab(id, { session, title: session.shell || tab.title, running: true, starting: false });
      if (command) await TerminalBinding.WriteTerminal(session.id, encodeTerminalData(`${command}\r`));
      requestAnimationFrame(() => { runtime?.fit.fit(); runtime?.terminal.focus(); });
      return session;
    } catch (error) {
      if (!cancelledTabsRef.current.has(id)) {
        updateTab(id, { starting: false, running: false, exitCode: -1 });
        notify?.("error", errorMessage(error));
      }
      return null;
    }
  }, [config.arguments, config.shell, mode, notify, t, updateTab, workspace?.path]);

  const toggleDock = useCallback(() => {
    if (open) {
      setMaximized(false);
      setOpen(false);
      return;
    }
    if (tabsRef.current.length) {
      setOpen(true);
      focusActive();
      return;
    }
    void createTab();
  }, [createTab, focusActive, open]);

  useImperativeHandle(ref, () => ({ open: createTab, newTab: createTab, toggle: toggleDock }), [createTab, toggleDock]);

  useEffect(() => { onVisibilityChange?.(open); }, [onVisibilityChange, open]);
  useEffect(() => () => onVisibilityChange?.(false), [onVisibilityChange]);

  useEffect(() => {
    const offOutput = Events.On(OUTPUT_EVENT, (event) => {
      const frame = event.data;
      const tab = tabsRef.current.find((item) => item.session?.id === frame?.sessionId);
      if (!tab || !frame?.data) return;
      const data = decodeTerminalData(frame.data);
      const runtime = runtimeRef.current.get(tab.id);
      if (runtime) runtime.terminal.write(data);
      else pendingRef.current.set(tab.id, [...(pendingRef.current.get(tab.id) || []), data]);
    });
    const offExit = Events.On(EXIT_EVENT, (event) => {
      const frame = event.data;
      const tab = tabsRef.current.find((item) => item.session?.id === frame?.sessionId);
      if (!tab) return;
      updateTab(tab.id, { running: false, starting: false, exitCode: frame.exitCode });
      const message = frame.error ? `\r\n\x1b[31m${t("terminal.processError", { error: frame.error })}\x1b[0m\r\n` : `\r\n\x1b[90m${t("terminal.processExited", { code: frame.exitCode })}\x1b[0m\r\n`;
      runtimeRef.current.get(tab.id)?.terminal.write(message);
    });
    return () => { offOutput(); offExit(); };
  }, [t, updateTab]);

  useEffect(() => {
    const refreshThemes = () => tabsRef.current.forEach((tab) => { const runtime = runtimeRef.current.get(tab.id); if (runtime) runtime.terminal.options.theme = resolveTerminalTheme(config.theme); });
    refreshThemes();
    if (config.theme !== "system") return undefined;
    const observer = new MutationObserver(refreshThemes);
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme", "data-accent"] });
    return () => observer.disconnect();
  }, [config.theme]);

  useEffect(() => {
    const shortcut = (event) => {
      if (!(event.ctrlKey || event.metaKey) || event.code !== "Backquote") return;
      event.preventDefault();
      toggleDock();
    };
    window.addEventListener("keydown", shortcut);
    return () => window.removeEventListener("keydown", shortcut);
  }, [toggleDock]);

  useEffect(() => () => resizeTimersRef.current.forEach((timer) => window.clearTimeout(timer)), []);
  useEffect(() => () => {
    tabsRef.current.forEach((tab) => {
      cancelledTabsRef.current.add(tab.id);
      if (tab.session?.id && tab.running) void TerminalBinding.CloseTerminal(tab.session.id).catch(() => {});
    });
  }, []);

  const selectTab = (id) => { setActiveID(id); setFocusedID(id); setSearchOpen(false); requestAnimationFrame(() => { const runtime = runtimeRef.current.get(id); runtime?.fit.fit(); runtime?.terminal.focus(); }); };
  const closeTab = async (id) => {
    const current = tabsRef.current;
    const roots = current.filter((item) => !item.splitParentID);
    const index = roots.findIndex((item) => item.id === id);
    const tab = roots[index];
    if (!tab) return;
    const closing = current.filter((item) => item.id === id || item.splitParentID === id);
    closing.forEach((item) => {
      cancelledTabsRef.current.add(item.id);
      pendingRef.current.delete(item.id);
      if (item.session?.id && item.running) void TerminalBinding.CloseTerminal(item.session.id).catch(() => {});
    });
    const next = current.filter((item) => item.id !== id && item.splitParentID !== id);
    setTabs(next);
    if (activeRef.current === id) {
      const nextRoots = next.filter((item) => !item.splitParentID);
      const nextID = nextRoots[Math.min(index, nextRoots.length - 1)]?.id || "";
      setActiveID(nextID);
      setFocusedID(nextID);
    }
  };
  const splitActive = async (direction) => {
    const root = tabsRef.current.find((item) => item.id === activeRef.current);
    if (!root) return;
    const layout = root.splitLayout || paneNode(root.id);
    const targetID = paneIDs(layout).includes(focusedRef.current) ? focusedRef.current : paneIDs(layout)[0];
    if (!targetID) return;
    const paneID = `terminal-${nextTabID++}`;
    const nextLayout = splitPane(layout, targetID, paneID, direction, `split-${nextSplitID++}`);
    updateTab(root.id, { splitLayout: nextLayout });
    await createTab("", root.id, paneID);
  };
  const closePane = (paneID) => {
    const rootID = activeRef.current;
    const root = tabsRef.current.find((item) => item.id === rootID);
    if (!root) return;
    const layout = root.splitLayout || paneNode(rootID);
    const ids = paneIDs(layout);
    if (!ids.includes(paneID)) return;
    if (ids.length === 1) {
      void closeTab(rootID);
      return;
    }
    const pane = tabsRef.current.find((item) => item.id === paneID);
    cancelledTabsRef.current.add(paneID);
    pendingRef.current.delete(paneID);
    if (pane?.session?.id && pane.running) void TerminalBinding.CloseTerminal(pane.session.id).catch(() => {});
    const nextLayout = removePane(layout, paneID);
    setTabs((items) => items
      .filter((item) => item.id !== paneID || item.id === rootID)
      .map((item) => item.id === rootID ? { ...item, ...(paneID === rootID ? { session: null, running: false, isGroupOnly: true } : {}), splitLayout: nextLayout } : item));
    const nextFocusedID = paneIDs(nextLayout)[0] || rootID;
    setFocusedID(nextFocusedID);
    requestAnimationFrame(() => runtimeRef.current.get(nextFocusedID)?.fit.fit());
  };
  const terminate = () => { const tab = tabsRef.current.find((item) => item.id === (focusedRef.current || activeRef.current)); if (tab?.session?.id && tab.running) void TerminalBinding.CloseTerminal(tab.session.id).catch((error) => notify?.("error", errorMessage(error))); };
  const copy = useCallback(async () => { const text = runtimeRef.current.get(focusedRef.current || activeRef.current)?.terminal.getSelection(); if (text) await Clipboard.SetText(text); }, []);
  const openSearch = useCallback(() => { setSearchOpen(true); requestAnimationFrame(() => document.querySelector(".terminal-search-input")?.focus()); }, []);
  const search = (direction = "next", query = searchQuery) => {
    const addon = runtimeRef.current.get(focusedRef.current || activeRef.current)?.search;
    const found = query ? addon?.[direction === "previous" ? "findPrevious" : "findNext"](query, { incremental: direction === "next", decorations: SEARCH_DECORATIONS }) : true;
    setSearchFound(Boolean(found));
  };
  const closeSearch = () => { runtimeRef.current.get(focusedRef.current || activeRef.current)?.search.clearDecorations(); setSearchOpen(false); focusActive(); };
  const openLink = useCallback((uri) => { try { const url = new URL(uri); if (url.protocol === "http:" || url.protocol === "https:") void Browser.OpenURL(url); } catch { /* Ignore malformed terminal output. */ } }, []);
  const activatePane = useCallback((id) => { focusedRef.current = id; setFocusedID(id); }, []);
  const resizeSplit = useCallback((rootID, splitID, ratio) => {
    setTabs((items) => items.map((item) => item.id === rootID ? { ...item, splitLayout: updateSplitRatio(item.splitLayout || paneNode(rootID), splitID, ratio) } : item));
  }, []);

  const beginResize = (event) => { if (!open) return; event.preventDefault(); event.currentTarget.setPointerCapture(event.pointerId); resizeRef.current = { pointerID: event.pointerId, startY: event.clientY, startHeight: height }; };
  const moveResize = (event) => { const value = resizeRef.current; if (!value || value.pointerID !== event.pointerId) return; const max = Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, window.innerHeight - 180)); setHeight(Math.max(MIN_HEIGHT, Math.min(max, value.startHeight + value.startY - event.clientY))); };
  const endResize = (event) => { if (resizeRef.current?.pointerID === event.pointerId) { resizeRef.current = null; event.currentTarget.releasePointerCapture?.(event.pointerId); } };
  const resizeWithKeyboard = (event) => { if (!["ArrowUp", "ArrowDown"].includes(event.key)) return; event.preventDefault(); setHeight((value) => Math.max(MIN_HEIGHT, Math.min(MAX_HEIGHT, value + (event.key === "ArrowUp" ? 20 : -20)))); };
  const rootTabs = tabs.filter((item) => !item.splitParentID);
  const activeTab = tabs.find((item) => item.id === activeID);
  const focusedTab = tabs.find((item) => item.id === focusedID) || activeTab;
  const activeLayout = activeTab?.splitLayout || (activeTab ? paneNode(activeTab.id) : null);
  const visiblePaneIDs = paneIDs(activeLayout);
  const hasSplit = visiblePaneIDs.length > 1;
  const anyPaneRunning = tabs.some((item) => visiblePaneIDs.includes(item.id) && item.running);
  const statusText = focusedTab?.starting ? t("terminal.starting") : focusedTab?.running ? t("terminal.running") : focusedTab?.exitCode != null ? t("terminal.exited", { code: focusedTab.exitCode }) : t("terminal.ready");

  return <section className={`terminal-dock no-drag shrink-0 ${open ? "is-open" : "is-hidden"} ${maximized ? "is-maximized" : ""}`} style={open && !maximized ? { height: `${height}px` } : undefined} aria-label={t("terminal.title")} aria-hidden={!open}>
    <span className="terminal-resize" role="separator" aria-label={t("terminal.resize")} aria-orientation="horizontal" tabIndex={open ? 0 : -1} onPointerDown={beginResize} onPointerMove={moveResize} onPointerUp={endResize} onPointerCancel={endResize} onKeyDown={resizeWithKeyboard} />
    <header className="terminal-toolbar select-none">
      <button type="button" className="terminal-title" aria-expanded="true" onClick={toggleDock}><SquareTerminal size={14} strokeWidth={2.2} aria-hidden="true" /><strong>{t("terminal.title")}</strong><span className={`terminal-status-dot ${anyPaneRunning ? "running" : ""}`} aria-hidden="true" /></button>
      <div className="terminal-tabs" role="tablist" aria-label={t("terminal.tabs")}>{rootTabs.map((tab) => <div className={`terminal-tab-shell ${tab.id === activeID ? "active" : ""}`} key={tab.id}><button type="button" role="tab" aria-selected={tab.id === activeID} className="terminal-tab" onClick={() => selectTab(tab.id)} title={tab.workspace}><span>{tab.title}</span>{tab.starting && <i aria-hidden="true" />}{!tab.running && !tab.starting && tab.exitCode != null && <em>{tab.exitCode}</em>}</button><button type="button" className="terminal-tab-close" aria-label={`${t("terminal.closeTab")} · ${tab.title}`} onClick={() => void closeTab(tab.id)}><X size={11} /></button></div>)}</div>
      <button type="button" className="terminal-tool" aria-label={t("terminal.newTab")} title={t("terminal.newTab")} onClick={() => void createTab()}><Plus size={14} aria-hidden="true" /></button>
      <span className="terminal-state">{statusText}</span>
      <button type="button" className="terminal-tool" disabled={!focusedTab} aria-label={t("terminal.search")} title={t("terminal.search")} onClick={openSearch}><Search size={14} aria-hidden="true" /></button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild><button type="button" className="terminal-tool" disabled={!activeTab} aria-label={t("terminal.split")} title={t("terminal.split")}><Columns2 size={14} aria-hidden="true" /></button></DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-40">
          <DropdownMenuItem onSelect={() => void splitActive("vertical")}><Columns2 size={14} aria-hidden="true" />{t("terminal.splitVertical")}</DropdownMenuItem>
          <DropdownMenuItem onSelect={() => void splitActive("horizontal")}><Rows2 size={14} aria-hidden="true" />{t("terminal.splitHorizontal")}</DropdownMenuItem>
          {hasSplit && <><DropdownMenuSeparator /><DropdownMenuItem onSelect={() => closePane(focusedRef.current)}><X size={14} aria-hidden="true" />{t("terminal.closeSplit")}</DropdownMenuItem></>}
        </DropdownMenuContent>
      </DropdownMenu>
      <button type="button" className="terminal-tool" disabled={!focusedTab?.running} aria-label={t("terminal.terminate")} title={t("terminal.terminate")} onClick={terminate}><Trash2 size={14} aria-hidden="true" /></button>
      <button type="button" className="terminal-tool" aria-label={maximized ? t("terminal.restore") : t("terminal.maximize")} aria-pressed={maximized} title={maximized ? t("terminal.restore") : t("terminal.maximize")} onClick={() => setMaximized((value) => !value)}>{maximized ? <Minimize2 size={14} aria-hidden="true" /> : <Maximize2 size={14} aria-hidden="true" />}</button>
      <button type="button" className="terminal-tool" aria-label={t("terminal.collapse")} title={t("terminal.collapse")} onClick={toggleDock}><ChevronDown size={15} /></button>
    </header>
    <div className="terminal-viewport" aria-hidden={!open}>
      <div className="terminal-pane-layout">
        {rootTabs.map((rootTab) => {
          const layout = rootTab.splitLayout || paneNode(rootTab.id);
          const layoutHasSplit = paneIDs(layout).length > 1;
          const active = rootTab.id === activeID;
          return <div className={`terminal-tab-layout ${active ? "active" : ""}`} key={rootTab.id} aria-hidden={!active}>
            <TerminalSplitLayout node={layout} onRatioChange={(splitID, ratio) => resizeSplit(rootTab.id, splitID, ratio)} resizeLabel={t("terminal.resizeSplit")} renderPane={(paneID) => {
              const pane = tabs.find((item) => item.id === paneID);
              if (!pane) return null;
              return <TerminalPane key={pane.id} tabID={pane.id} active={active} theme={config.theme} onReady={registerRuntime} onData={writeTab} onResize={resizePTY} onCopy={copy} onSearch={openSearch} onOpenLink={openLink} onActivate={activatePane} onClose={closePane} closeLabel={t("terminal.closePane")} canClose={layoutHasSplit} />;
            }} />
          </div>;
        })}
      </div>
      {!tabs.length && <button type="button" className="terminal-empty" onClick={() => void createTab()}><Plus size={14} aria-hidden="true" />{t("terminal.start")}</button>}
      {searchOpen && <form className="terminal-search" onSubmit={(event) => { event.preventDefault(); search(); }}><Search size={13} aria-hidden="true" /><input className="terminal-search-input" value={searchQuery} placeholder={t("terminal.searchPlaceholder")} aria-label={t("terminal.search")} onChange={(event) => { setSearchQuery(event.target.value); search("next", event.target.value); }} onKeyDown={(event) => { if (event.key === "Escape") { event.preventDefault(); closeSearch(); } }} /><span className={searchFound ? "" : "not-found"}>{searchQuery && !searchFound ? t("terminal.noMatches") : ""}</span><button type="button" aria-label={t("terminal.previousMatch")} onClick={() => search("previous")}><ChevronLeft size={13} /></button><button type="button" aria-label={t("terminal.nextMatch")} onClick={() => search("next")}><ChevronRight size={13} /></button><button type="button" aria-label={t("common.close")} onClick={closeSearch}><X size={13} /></button></form>}
    </div>
  </section>;
});

export default TerminalDock;
