import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, ChevronRight, File, FileBraces, FileCode, FileCog, FileText, FileTerminal, Folder, FolderOpen, RefreshCw, Save, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Textarea } from "@/components/ui/textarea";
import { WorkspaceBinding } from "../../../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { Action, Kicker } from "../../../ui/primitives.jsx";
import { errorMessage } from "../../format.js";
import {
  FILE_EDITOR_MIN_WIDTH,
  FILE_TREE_DEFAULT_RATIO,
  FILE_TREE_KEYBOARD_STEP,
  FILE_TREE_MIN_WIDTH,
  FILE_TREE_RESIZER_WIDTH,
  clampFileTreeWidth,
  readFileTreeRatio,
  writeFileTreeRatio,
} from "../../fileTreeLayout.js";
import { primaryShortcutLabel } from "../../platform.js";
import { highlightSource } from "../../syntaxHighlight.js";

const DEMO_TREE = {
  "": [
    { name: "frontend", path: "frontend", directory: true },
    { name: "README.md", path: "README.md", directory: false, size: 1184 },
  ],
  frontend: [
    { name: "src", path: "frontend/src", directory: true },
    { name: "package.json", path: "frontend/package.json", directory: false, size: 642 },
  ],
  "frontend/src": [
    { name: "App.jsx", path: "frontend/src/App.jsx", directory: false, size: 1840 },
  ],
};

const DEMO_CONTENT = {
  "README.md": "# OneCatch\n\nLocal-first Agent workflow workbench.\n",
  "frontend/package.json": "{\n  \"name\": \"onecatch-desktop-frontend\",\n  \"private\": true\n}\n",
  "frontend/src/App.jsx": "export default function App() {\n  return <main>OneCatch</main>;\n}\n",
};

function fileVisual(name = "") {
  const lower = name.toLowerCase();
  const extension = lower.includes(".") ? lower.slice(lower.lastIndexOf(".")) : "";
  if ([".json", ".jsonc"].includes(extension)) return { Icon: FileBraces, tone: "text-warning" };
  if ([".c", ".cc", ".cpp", ".cs", ".css", ".go", ".h", ".hpp", ".html", ".java", ".js", ".jsx", ".kt", ".kts", ".py", ".rs", ".scss", ".sql", ".ts", ".tsx", ".xml"].includes(extension)) return { Icon: FileCode, tone: "text-info" };
  if ([".md", ".mdx", ".txt", ".rst"].includes(extension)) return { Icon: FileText, tone: "text-success" };
  if ([".yml", ".yaml", ".toml", ".ini", ".env"].includes(extension) || ["taskfile", "dockerfile", "makefile"].some((item) => lower.startsWith(item))) return { Icon: FileCog, tone: "text-destructive" };
  if ([".sh", ".zsh", ".fish", ".bat", ".ps1"].includes(extension)) return { Icon: FileTerminal, tone: "text-primary" };
  return { Icon: File, tone: "text-muted-foreground" };
}

function FileTreeRows({ directory = "", depth = 0, entriesByDirectory, expanded, loadingDirectories, selectedPath, onToggleDirectory, onSelectFile }) {
  const entries = entriesByDirectory[directory] || [];
  return entries.map((entry) => {
    const open = entry.directory && expanded.has(entry.path);
    const file = fileVisual(entry.name);
    const Icon = entry.directory ? (open ? FolderOpen : Folder) : file.Icon;
    const iconTone = entry.directory ? "text-muted-foreground" : file.tone;
    return <div key={entry.path}>
      <button
        type="button"
        role="treeitem"
        aria-expanded={entry.directory ? open : undefined}
        aria-selected={!entry.directory && selectedPath === entry.path}
        className={`flex h-7 w-full min-w-0 items-center gap-1.5 rounded-md pr-2 text-left text-xs transition-colors hover:bg-accent hover:text-foreground ${selectedPath === entry.path ? "bg-accent text-foreground" : "text-muted-foreground"}`}
        style={{ paddingLeft: `${8 + depth * 14}px` }}
        title={entry.path}
        onClick={() => entry.directory ? onToggleDirectory(entry.path) : onSelectFile(entry.path)}
      >
        {entry.directory ? (open ? <ChevronDown size={13} aria-hidden="true" /> : <ChevronRight size={13} aria-hidden="true" />) : <span className="w-[13px] shrink-0" aria-hidden="true" />}
        <Icon size={14} className={`shrink-0 ${iconTone}`} aria-hidden="true" />
        <span className="min-w-0 flex-1 truncate">{entry.name}</span>
      </button>
      {open && <div role="group">
        {loadingDirectories.has(entry.path) && <p className="m-0 py-1 pr-2 text-[11px] text-muted-foreground" style={{ paddingLeft: `${35 + depth * 14}px` }}>…</p>}
        {!loadingDirectories.has(entry.path) && <FileTreeRows directory={entry.path} depth={depth + 1} entriesByDirectory={entriesByDirectory} expanded={expanded} loadingDirectories={loadingDirectories} selectedPath={selectedPath} onToggleDirectory={onToggleDirectory} onSelectFile={onSelectFile} />}
      </div>}
    </div>;
  });
}

function FileInspector({ mode, workspaceID, active = true, notify, onDirtyChange }) {
  const { t } = useTranslation();
  const [entriesByDirectory, setEntriesByDirectory] = useState({});
  const [expanded, setExpanded] = useState(() => new Set());
  const [loadingDirectories, setLoadingDirectories] = useState(() => new Set());
  const [treeError, setTreeError] = useState("");
  const [openFiles, setOpenFiles] = useState([]);
  const [activePath, setActivePath] = useState("");
  const [loadingFile, setLoadingFile] = useState(false);
  const [fileError, setFileError] = useState("");
  const [saving, setSaving] = useState(false);
  const [currentLine, setCurrentLine] = useState(1);
  const initialTreeRatio = useRef(readFileTreeRatio());
  const [treeWidth, setTreeWidth] = useState(() => Math.round(960 * initialTreeRatio.current));
  const [treeResizing, setTreeResizing] = useState(false);
  const [inspectorWidth, setInspectorWidth] = useState(0);
  const workspaceRef = useRef(workspaceID);
  const openRequestRef = useRef(0);
  const inspectorRef = useRef(null);
  const editorRef = useRef(null);
  const lineNumbersRef = useRef(null);
  const highlightedCodeRef = useRef(null);
  const fileViewStateRef = useRef(new Map());
  const treeWidthRef = useRef(treeWidth);
  const treeRatioRef = useRef(initialTreeRatio.current);
  const treeResizeRef = useRef(null);
  workspaceRef.current = workspaceID;
  const document = useMemo(() => openFiles.find((file) => file.path === activePath) || null, [activePath, openFiles]);
  const draft = document?.draft || "";
  const dirty = Boolean(document && draft !== document.content);
  const hasDirtyFiles = useMemo(() => openFiles.some((file) => file.draft !== file.content), [openFiles]);

  useEffect(() => onDirtyChange?.(hasDirtyFiles), [hasDirtyFiles, onDirtyChange]);
  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange]);

  const listDirectory = useCallback(async (directory = "") => {
    if (!workspaceID) return;
    const requestWorkspaceID = workspaceID;
    setLoadingDirectories((current) => new Set(current).add(directory));
    setTreeError("");
    try {
      const entries = mode === "demo" ? (DEMO_TREE[directory] || []) : await WorkspaceBinding.ListWorkspaceFiles(workspaceID, directory);
      if (workspaceRef.current !== requestWorkspaceID) return;
      setEntriesByDirectory((current) => ({ ...current, [directory]: entries || [] }));
    } catch (error) {
      if (workspaceRef.current === requestWorkspaceID) setTreeError(errorMessage(error));
    } finally {
      if (workspaceRef.current === requestWorkspaceID) setLoadingDirectories((current) => {
        const next = new Set(current);
        next.delete(directory);
        return next;
      });
    }
  }, [mode, workspaceID]);

  useEffect(() => {
    setEntriesByDirectory({});
    setExpanded(new Set());
    setOpenFiles([]);
    setActivePath("");
    setCurrentLine(1);
    setFileError("");
    setTreeError("");
    openRequestRef.current += 1;
    fileViewStateRef.current.clear();
    if (workspaceID) void listDirectory("");
  }, [listDirectory, workspaceID]);

  const refreshTree = () => {
    setEntriesByDirectory({});
    setExpanded(new Set());
    void listDirectory("");
  };

  const toggleDirectory = async (path) => {
    if (expanded.has(path)) {
      setExpanded((current) => {
        const next = new Set(current);
        next.delete(path);
        return next;
      });
      return;
    }
    setExpanded((current) => new Set(current).add(path));
    if (!Object.hasOwn(entriesByDirectory, path)) await listDirectory(path);
  };

  const restoreFileView = useCallback((path) => {
    requestAnimationFrame(() => {
      const view = fileViewStateRef.current.get(path) || { line: 1, scrollTop: 0, scrollLeft: 0 };
      setCurrentLine(view.line);
      if (editorRef.current) {
        editorRef.current.scrollTop = view.scrollTop;
        editorRef.current.scrollLeft = view.scrollLeft;
      }
      if (lineNumbersRef.current) lineNumbersRef.current.style.transform = `translateY(${-view.scrollTop}px)`;
      if (highlightedCodeRef.current) highlightedCodeRef.current.style.transform = `translate(${-view.scrollLeft}px, ${-view.scrollTop}px)`;
    });
  }, []);

  const activateFile = useCallback((path) => {
    setActivePath(path);
    setFileError("");
    restoreFileView(path);
  }, [restoreFileView]);

  const readFile = useCallback(async (path, { discard = false } = {}) => {
    if (!path || !workspaceID) return;
    const existing = openFiles.find((file) => file.path === path);
    if (existing && !discard) {
      activateFile(path);
      return;
    }
    if (existing && discard && existing.draft !== existing.content && !globalThis.confirm(t("files.discardConfirm"))) return;
    const requestWorkspaceID = workspaceID;
    const requestID = openRequestRef.current + 1;
    openRequestRef.current = requestID;
    setLoadingFile(true);
    setFileError("");
    try {
      const next = mode === "demo"
        ? { path, content: DEMO_CONTENT[path] || "", hash: `demo-${path}`, size: (DEMO_CONTENT[path] || "").length, modifiedAt: new Date().toISOString() }
        : await WorkspaceBinding.ReadWorkspaceFile(workspaceID, path);
      if (workspaceRef.current !== requestWorkspaceID) return;
      const opened = { ...next, draft: next.content };
      setOpenFiles((current) => current.some((file) => file.path === path)
        ? current.map((file) => file.path === path ? opened : file)
        : [...current, opened]);
      fileViewStateRef.current.set(path, { line: 1, scrollTop: 0, scrollLeft: 0 });
      if (openRequestRef.current === requestID) activateFile(path);
    } catch (error) {
      if (workspaceRef.current === requestWorkspaceID && openRequestRef.current === requestID) setFileError(errorMessage(error));
    } finally {
      if (workspaceRef.current === requestWorkspaceID && openRequestRef.current === requestID) setLoadingFile(false);
    }
  }, [activateFile, mode, openFiles, t, workspaceID]);

  const closeFile = (event, path) => {
    event.stopPropagation();
    const closingIndex = openFiles.findIndex((file) => file.path === path);
    if (closingIndex < 0) return;
    const closing = openFiles[closingIndex];
    if (closing.draft !== closing.content && !globalThis.confirm(t("files.closeUnsavedConfirm", { name: path.slice(path.lastIndexOf("/") + 1) }))) return;
    const nextFiles = openFiles.filter((file) => file.path !== path);
    setOpenFiles(nextFiles);
    fileViewStateRef.current.delete(path);
    if (activePath === path) {
      const next = nextFiles[Math.min(closingIndex, nextFiles.length - 1)];
      setActivePath(next?.path || "");
      if (next) restoreFileView(next.path);
      else setCurrentLine(1);
    }
  };

  const reloadFile = () => {
    if (!document?.path) return;
    void readFile(document.path, { discard: true });
  };

  const saveFile = useCallback(async () => {
    if (!document || !dirty || saving) return;
    setSaving(true);
    setFileError("");
    try {
      const saved = mode === "demo"
        ? { ...document, content: draft, hash: `demo-${Date.now()}`, size: new Blob([draft]).size, modifiedAt: new Date().toISOString() }
        : await WorkspaceBinding.WriteWorkspaceFile({ workspaceId: workspaceID, path: document.path, content: draft, expectedHash: document.hash });
      setOpenFiles((current) => current.map((file) => file.path === saved.path ? { ...saved, draft: saved.content } : file));
      setEntriesByDirectory((current) => {
        const parent = saved.path.includes("/") ? saved.path.slice(0, saved.path.lastIndexOf("/")) : "";
        if (!current[parent]) return current;
        return { ...current, [parent]: current[parent].map((entry) => entry.path === saved.path ? { ...entry, size: saved.size, modifiedAt: saved.modifiedAt } : entry) };
      });
      notify?.("success", t("files.saved"));
    } catch (error) {
      const message = errorMessage(error);
      setFileError(message);
      notify?.("error", message);
    } finally {
      setSaving(false);
    }
  }, [dirty, document, draft, mode, notify, saving, t, workspaceID]);

  useEffect(() => {
    if (!active) return undefined;
    const onKeyDown = (event) => {
      if (!(event.metaKey || event.ctrlKey) || event.key.toLowerCase() !== "s") return;
      event.preventDefault();
      void saveFile();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [active, saveFile]);

  useEffect(() => {
    const inspector = inspectorRef.current;
    if (!inspector) return undefined;
    const fitTreeToInspector = () => {
      const width = inspector.getBoundingClientRect().width;
      if (!width) return;
      const nextWidth = clampFileTreeWidth(width * treeRatioRef.current, width);
      setInspectorWidth(width);
      treeWidthRef.current = nextWidth;
      setTreeWidth(nextWidth);
    };
    fitTreeToInspector();
    if (typeof ResizeObserver === "undefined") return undefined;
    const observer = new ResizeObserver(fitTreeToInspector);
    observer.observe(inspector);
    return () => observer.disconnect();
  }, [workspaceID]);

  const resizeTreeTo = (requestedWidth, { persist = true } = {}) => {
    const width = inspectorRef.current?.getBoundingClientRect().width || inspectorWidth;
    const nextWidth = clampFileTreeWidth(requestedWidth, width);
    treeWidthRef.current = nextWidth;
    setTreeWidth(nextWidth);
    if (width > 0) {
      treeRatioRef.current = nextWidth / width;
      if (persist) writeFileTreeRatio(treeRatioRef.current);
    }
  };

  const beginTreeResize = (event) => {
    if (event.button !== 0) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    treeResizeRef.current = { pointerId: event.pointerId, startX: event.clientX, startWidth: treeWidthRef.current };
    setTreeResizing(true);
  };

  const continueTreeResize = (event) => {
    const resize = treeResizeRef.current;
    if (!resize || resize.pointerId !== event.pointerId) return;
    resizeTreeTo(resize.startWidth + resize.startX - event.clientX, { persist: false });
  };

  const finishTreeResize = (event) => {
    const resize = treeResizeRef.current;
    if (!resize || resize.pointerId !== event.pointerId) return;
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    treeResizeRef.current = null;
    setTreeResizing(false);
    writeFileTreeRatio(treeRatioRef.current);
  };

  const resetTreeWidth = () => {
    const width = inspectorRef.current?.getBoundingClientRect().width || inspectorWidth;
    treeRatioRef.current = FILE_TREE_DEFAULT_RATIO;
    resizeTreeTo(width * FILE_TREE_DEFAULT_RATIO);
  };

  const resizeTreeWithKeyboard = (event) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight" && event.key !== "Home") return;
    event.preventDefault();
    if (event.key === "Home") {
      resetTreeWidth();
      return;
    }
    resizeTreeTo(treeWidthRef.current + (event.key === "ArrowLeft" ? FILE_TREE_KEYBOARD_STEP : -FILE_TREE_KEYBOARD_STEP));
  };

  const rootEntries = useMemo(() => entriesByDirectory[""] || [], [entriesByDirectory]);
  const lineNumbers = useMemo(() => Array.from({ length: draft.split("\n").length }, (_, index) => index + 1), [draft]);
  const highlighted = useMemo(() => highlightSource(draft, document?.path), [document?.path, draft]);
  const updateDraft = (value) => setOpenFiles((current) => current.map((file) => file.path === activePath ? { ...file, draft: value } : file));
  const updateCurrentLine = (element) => {
    const line = element.value.slice(0, element.selectionStart).split("\n").length;
    setCurrentLine(line);
    if (activePath) fileViewStateRef.current.set(activePath, { ...fileViewStateRef.current.get(activePath), line });
  };
  const syncEditorScroll = (event) => {
    const { scrollLeft, scrollTop } = event.currentTarget;
    if (lineNumbersRef.current) lineNumbersRef.current.style.transform = `translateY(${-scrollTop}px)`;
    if (highlightedCodeRef.current) highlightedCodeRef.current.style.transform = `translate(${-scrollLeft}px, ${-scrollTop}px)`;
    if (activePath) fileViewStateRef.current.set(activePath, { ...fileViewStateRef.current.get(activePath), line: currentLine, scrollLeft, scrollTop });
  };
  if (!workspaceID) return <p className="m-0 px-4 py-5 text-xs leading-relaxed text-muted-foreground">{t("files.selectWorkspace")}</p>;

  return <div
    ref={inspectorRef}
    className="file-inspector grid h-full min-h-0 grid-rows-[minmax(0,1fr)] overflow-hidden bg-background/65"
    style={{ gridTemplateColumns: `minmax(0, 1fr) ${FILE_TREE_RESIZER_WIDTH}px ${treeWidth}px` }}
  >
    <section className="flex min-h-0 min-w-0 flex-col overflow-hidden">
      <div className="flex min-h-9 shrink-0 items-stretch overflow-hidden bg-muted/20">
        <div className="flex min-w-0 flex-1 items-stretch overflow-x-auto" role="tablist" aria-label={t("files.openFiles")}>
          {openFiles.map((file) => {
            const visual = fileVisual(file.path);
            const name = file.path.slice(file.path.lastIndexOf("/") + 1);
            const fileDirty = file.draft !== file.content;
            return <div className={`group flex min-w-0 max-w-48 shrink-0 items-center rounded-t-md px-1 ${file.path === activePath ? "bg-background text-foreground" : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"}`} key={file.path}>
              <button type="button" role="tab" aria-selected={file.path === activePath} title={file.path} className="flex h-full min-w-0 items-center gap-1.5 bg-transparent px-1.5 text-[11px] outline-none" onClick={() => activateFile(file.path)}>
                <visual.Icon size={12} className={`shrink-0 ${visual.tone}`} aria-hidden="true" />
                <span className="truncate">{name}</span>
                {fileDirty && <span className="size-1.5 shrink-0 rounded-full bg-warning" title={t("files.unsaved")} aria-label={t("files.unsaved")} />}
              </button>
              <button type="button" className="grid size-5 shrink-0 place-items-center rounded text-muted-foreground opacity-60 outline-none hover:bg-accent hover:text-foreground group-hover:opacity-100 focus-visible:opacity-100" aria-label={t("files.closeFile", { name })} title={t("files.closeFile", { name })} onClick={(event) => closeFile(event, file.path)}><X size={11} aria-hidden="true" /></button>
            </div>;
          })}
          {!openFiles.length && <span className="self-center px-2.5 text-[11px] text-muted-foreground">{t("files.noFile")}</span>}
        </div>
        <div className="flex shrink-0 items-center gap-1 px-1.5">
          {document && <Action size="compact" tone="muted" disabled={loadingFile || saving} onClick={reloadFile}>{t("common.refresh")}</Action>}
          {document && <Action size="compact" tone="primary" disabled={!dirty || saving} onClick={() => void saveFile()}><Save size={12} aria-hidden="true" />{saving ? t("common.saving") : t("common.save")}</Action>}
        </div>
      </div>
      {loadingFile && !document ? <p className="m-0 px-3 py-4 text-xs text-muted-foreground">{t("common.loading")}</p> : document ? <>
        <div className="grid min-h-0 flex-1 grid-cols-[42px_minmax(0,1fr)] overflow-hidden bg-background/65">
          <div className="relative min-h-0 overflow-hidden bg-muted/35 text-right font-mono text-[12px] leading-5 text-muted-foreground/75" aria-hidden="true">
            <div ref={lineNumbersRef} className="py-3 pr-2 will-change-transform">{lineNumbers.map((line) => <span className={`block h-5 tabular-nums ${line === currentLine ? "font-semibold text-primary" : ""}`} key={line}>{line}</span>)}</div>
          </div>
          <div className="file-editor-stage relative min-h-0 min-w-0 overflow-hidden">
            <pre className="file-editor-highlight pointer-events-none absolute inset-0 m-0 overflow-hidden bg-transparent" aria-hidden="true"><code ref={highlightedCodeRef} className={`language-${highlighted.language}`} dangerouslySetInnerHTML={{ __html: `${highlighted.html}${draft.endsWith("\n") ? " " : ""}` }} /></pre>
            <Textarea
              ref={editorRef}
              className="file-editor-textarea absolute inset-0 h-full min-h-0 w-full resize-none overflow-auto rounded-none border-0 bg-transparent px-3 py-3 font-mono text-[12px] leading-5 whitespace-pre field-sizing-fixed"
              aria-label={t("files.editor", { path: document.path })}
              spellCheck="false"
              wrap="off"
              value={draft}
              onChange={(event) => { updateDraft(event.target.value); updateCurrentLine(event.target); }}
              onClick={(event) => updateCurrentLine(event.currentTarget)}
              onKeyUp={(event) => updateCurrentLine(event.currentTarget)}
              onSelect={(event) => updateCurrentLine(event.currentTarget)}
              onScroll={syncEditorScroll}
            />
          </div>
        </div>
        <div className="flex shrink-0 items-center justify-between px-2.5 py-1.5 text-[10px] text-muted-foreground"><span>{highlighted.language === "plain" ? t("files.plainText") : highlighted.language}</span><span>{t("files.saveHint", { shortcut: primaryShortcutLabel("S") })}</span></div>
      </> : <p className="m-0 px-3 py-4 text-xs leading-relaxed text-muted-foreground">{t("files.openHint")}</p>}
      {fileError && <p className="m-0 shrink-0 bg-destructive/8 px-2.5 py-2 text-[11px] leading-relaxed text-destructive">{fileError}</p>}
    </section>

    <div
      role="separator"
      aria-label={t("files.resizeTree")}
      aria-orientation="vertical"
      aria-valuemin={FILE_TREE_MIN_WIDTH}
      aria-valuemax={Math.max(FILE_TREE_MIN_WIDTH, Math.round(inspectorWidth - FILE_EDITOR_MIN_WIDTH - FILE_TREE_RESIZER_WIDTH))}
      aria-valuenow={Math.round(treeWidth)}
      tabIndex="0"
      title={t("files.resizeTreeHint")}
      className={`relative z-10 h-full cursor-col-resize touch-none bg-transparent outline-none after:pointer-events-none after:absolute after:inset-y-0 after:left-1/2 after:w-px after:-translate-x-1/2 after:bg-border/60 after:transition-colors hover:after:bg-primary focus-visible:after:bg-primary ${treeResizing ? "after:bg-primary" : ""}`}
      onPointerDown={beginTreeResize}
      onPointerMove={continueTreeResize}
      onPointerUp={finishTreeResize}
      onPointerCancel={finishTreeResize}
      onDoubleClick={resetTreeWidth}
      onKeyDown={resizeTreeWithKeyboard}
    />

    <section className="flex min-h-0 min-w-0 flex-col overflow-hidden bg-muted/15">
      <div className="flex shrink-0 items-center gap-2 px-2.5 py-2">
        <Kicker>{t("files.workspaceFiles")}</Kicker>
        <Action size="compact" tone="muted" className="ml-auto px-1.5" aria-label={t("files.refresh")} title={t("files.refresh")} disabled={loadingDirectories.has("")} onClick={refreshTree}><RefreshCw size={12} className={loadingDirectories.has("") ? "animate-spin" : ""} aria-hidden="true" /></Action>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-1.5 pb-2" role="tree" aria-label={t("files.workspaceFiles")}>
        <FileTreeRows entriesByDirectory={entriesByDirectory} expanded={expanded} loadingDirectories={loadingDirectories} selectedPath={document?.path || ""} onToggleDirectory={toggleDirectory} onSelectFile={readFile} />
        {!loadingDirectories.has("") && !treeError && rootEntries.length === 0 && <p className="m-0 px-2 py-3 text-xs text-muted-foreground">{t("files.empty")}</p>}
        {treeError && <p className="m-0 px-2 py-3 text-xs leading-relaxed text-destructive">{treeError}</p>}
      </div>
    </section>
  </div>;
}

export default memo(FileInspector);
