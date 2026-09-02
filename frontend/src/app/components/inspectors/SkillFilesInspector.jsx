import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, ChevronRight, File, FileCode2, FileText, Folder, FolderOpen, RefreshCw, RotateCcw, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { SkillBinding } from "../../../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { errorMessage } from "../../format.js";
import { formatSkillBytes, newSkillTemplate } from "../../skills.js";
import { currentSkillFileRequest, currentSkillSelection, publishSkillFileDraft, SKILL_FILE_OPEN_EVENT, SKILL_SELECTED_EVENT, subscribeSkillWorkspace } from "../../skillWorkspace.js";

const DEMO_TREE = {
  "": [
    { name: "release-notes", path: "release-notes", directory: true },
    { name: "code-review", path: "code-review", directory: true },
  ],
  "release-notes": [
    { name: "references", path: "release-notes/references", directory: true },
    { name: "SKILL.md", path: "release-notes/SKILL.md", directory: false },
  ],
  "release-notes/references": [
    { name: "style-guide.md", path: "release-notes/references/style-guide.md", directory: false },
  ],
  "code-review": [
    { name: "scripts", path: "code-review/scripts", directory: true },
    { name: "SKILL.md", path: "code-review/SKILL.md", directory: false },
  ],
  "code-review/scripts": [
    { name: "check.sh", path: "code-review/scripts/check.sh", directory: false },
  ],
};

const DEMO_FILES = {
  "release-notes/references/style-guide.md": "# Style guide\n\nPrefer the imperative mood.\nName the user-visible change, not the commit.\n",
  "code-review/scripts/check.sh": "#!/bin/sh\nset -eu\ngit diff --stat\n",
};

function demoFile(path) {
  if (path.endsWith("/SKILL.md")) {
    const name = path.slice(0, -"/SKILL.md".length);
    return newSkillTemplate(name, `Demo content for ${name}.`);
  }
  return DEMO_FILES[path] || "";
}

function fileVisual(name = "") {
  const lower = name.toLowerCase();
  if (lower.endsWith(".md") || lower.endsWith(".mdx") || lower.endsWith(".txt")) return { Icon: FileText, tone: "text-success" };
  if (/\.(c|cc|cpp|css|go|html|js|jsx|py|rs|sh|ts|tsx)$/.test(lower)) return { Icon: FileCode2, tone: "text-info" };
  return { Icon: File, tone: "text-muted-foreground" };
}

function SkillFileRows({ directory = "", depth = 0, entriesByDirectory, expanded, loadingDirectories, activePath, onToggleDirectory, onOpenFile }) {
  return (entriesByDirectory[directory] || []).map((entry) => {
    const open = entry.directory && expanded.has(entry.path);
    const visual = fileVisual(entry.name);
    const Icon = entry.directory ? (open ? FolderOpen : Folder) : visual.Icon;
    const active = !entry.directory && entry.path === activePath;
    return <div key={entry.path}>
      <button
        type="button"
        role="treeitem"
        aria-expanded={entry.directory ? open : undefined}
        aria-current={active ? "true" : undefined}
        className={`flex h-7 w-full min-w-0 items-center gap-1.5 rounded-md pr-2 text-left text-xs transition-colors ${active ? "bg-accent text-foreground" : "text-muted-foreground hover:bg-accent/60 hover:text-foreground"}`}
        style={{ paddingLeft: `${8 + depth * 14}px` }}
        title={entry.path}
        onClick={() => entry.directory ? onToggleDirectory(entry.path) : onOpenFile(entry.path)}
      >
        {entry.directory ? (open ? <ChevronDown size={13} aria-hidden="true" /> : <ChevronRight size={13} aria-hidden="true" />) : <span className="w-[13px] shrink-0" aria-hidden="true" />}
        <Icon size={14} className={`shrink-0 ${entry.directory ? "text-muted-foreground" : visual.tone}`} aria-hidden="true" />
        <span className="min-w-0 flex-1 truncate">{entry.name}</span>
      </button>
      {open && <div role="group">
        {loadingDirectories.has(entry.path)
          ? <p className="m-0 py-1 pr-2 text-[11px] text-muted-foreground" style={{ paddingLeft: `${35 + depth * 14}px` }}>…</p>
          : <SkillFileRows directory={entry.path} depth={depth + 1} entriesByDirectory={entriesByDirectory} expanded={expanded} loadingDirectories={loadingDirectories} activePath={activePath} onToggleDirectory={onToggleDirectory} onOpenFile={onOpenFile} />}
      </div>}
    </div>;
  });
}

function SkillFilesInspector({ mode, notify, onDirtyChange }) {
  const { t } = useTranslation();
  const [entriesByDirectory, setEntriesByDirectory] = useState({});
  const [expanded, setExpanded] = useState(() => new Set());
  const [loadingDirectories, setLoadingDirectories] = useState(() => new Set());
  const [error, setError] = useState("");
  const [file, setFile] = useState(null);
  const [draft, setDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const generationRef = useRef(0);
  const handledRequestRef = useRef(0);
  const dirty = Boolean(file && draft !== file.content);

  const listDirectory = useCallback(async (directory = "", generation = generationRef.current) => {
    setLoadingDirectories((current) => new Set(current).add(directory));
    setError("");
    try {
      const entries = mode === "demo" ? (DEMO_TREE[directory] || []) : await SkillBinding.ListFiles(directory);
      if (generationRef.current !== generation) return;
      setEntriesByDirectory((current) => ({ ...current, [directory]: entries || [] }));
    } catch (loadError) {
      if (generationRef.current === generation) setError(errorMessage(loadError));
    } finally {
      if (generationRef.current === generation) setLoadingDirectories((current) => {
        const next = new Set(current);
        next.delete(directory);
        return next;
      });
    }
  }, [mode]);

  const refresh = useCallback(() => {
    generationRef.current += 1;
    const generation = generationRef.current;
    setEntriesByDirectory({});
    setLoadingDirectories(new Set());
    void listDirectory("", generation);
  }, [listDirectory]);

  useEffect(() => {
    refresh();
    return () => { generationRef.current += 1; };
  }, [refresh]);

  // A dirty buffer here is the same hazard as a dirty task file: the workbench
  // guards its close paths on it.
  useEffect(() => { onDirtyChange?.(dirty); }, [dirty, onDirtyChange]);

  const openFile = useCallback(async (path) => {
    if (!path) return;
    try {
      const value = mode === "demo" ? { path, name: path.split("/").pop(), content: demoFile(path), sizeBytes: demoFile(path).length } : await SkillBinding.ReadFile(path);
      setFile(value);
      setDraft(value.content || "");
      setError("");
    } catch (loadError) {
      notify?.("error", errorMessage(loadError));
    }
  }, [mode, notify]);

  // Selecting a skill in the library expands its directory here, so the files
  // that belong to what is on screen are one click away rather than three.
  const revealSkill = useCallback((name) => {
    if (!name) return;
    setExpanded((current) => current.has(name) ? current : new Set(current).add(name));
    setEntriesByDirectory((current) => {
      if (Object.hasOwn(current, name)) return current;
      void listDirectory(name);
      return current;
    });
  }, [listDirectory]);

  useEffect(() => {
    revealSkill(currentSkillSelection().name);
    return subscribeSkillWorkspace(SKILL_SELECTED_EVENT, (event) => revealSkill(event.detail?.name));
  }, [revealSkill]);

  useEffect(() => {
    const handle = ({ path, token }) => {
      if (!path || token <= handledRequestRef.current) return;
      handledRequestRef.current = token;
      const directory = path.slice(0, path.lastIndexOf("/"));
      if (directory) revealSkill(directory);
      void openFile(path);
    };
    handle(currentSkillFileRequest());
    return subscribeSkillWorkspace(SKILL_FILE_OPEN_EVENT, (event) => handle(event.detail || {}));
  }, [openFile, revealSkill]);

  const editDraft = (value) => {
    setDraft(value);
    publishSkillFileDraft({ path: file?.path, content: value });
  };

  const saveFile = async () => {
    if (!file || !dirty) return;
    setSaving(true);
    try {
      const saved = mode === "demo" ? { ...file, content: draft, sizeBytes: draft.length } : await SkillBinding.WriteFile({ path: file.path, content: draft });
      setFile(saved);
      setDraft(saved.content);
      publishSkillFileDraft({ path: saved.path, content: saved.content, saved: true });
      notify?.("success", t("skill.fileSaved", { name: saved.name }));
    } catch (saveError) {
      notify?.("error", errorMessage(saveError));
    } finally {
      setSaving(false);
    }
  };

  const closeFile = () => {
    if (file) publishSkillFileDraft({ path: file.path, content: file.content, saved: true });
    setFile(null);
    setDraft("");
  };

  const revertFile = () => {
    if (!file) return;
    setDraft(file.content);
    publishSkillFileDraft({ path: file.path, content: file.content });
  };

  const rootEntries = entriesByDirectory[""] || [];
  const rootLoading = loadingDirectories.has("");
  const treeClassName = useMemo(() => file ? "max-h-[36%] shrink-0 overflow-y-auto border-b border-border/60 px-2 py-2" : "min-h-0 flex-1 overflow-y-auto px-2 py-2", [file]);

  return <div className="flex h-full min-h-0 flex-col">
    <div className="flex h-11 shrink-0 items-center gap-2 border-b border-border/70 px-3">
      <Folder size={14} className="shrink-0 text-muted-foreground" aria-hidden="true" />
      <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-muted-foreground">~/.onecatch/skills</span>
      <button type="button" className="grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground" aria-label={t("common.refresh")} title={t("common.refresh")} onClick={refresh}>
        <RefreshCw size={14} className={rootLoading ? "animate-spin" : ""} aria-hidden="true" />
      </button>
    </div>

    <div className={treeClassName}>
      {error && <p className="m-1 rounded-md bg-destructive/10 px-2 py-1.5 text-xs text-destructive">{error}</p>}
      {!error && rootLoading && <p className="m-1 text-xs text-muted-foreground">{t("common.loading")}</p>}
      {!error && !rootLoading && rootEntries.length === 0 && <p className="m-1 text-xs text-muted-foreground">{t("skill.filesEmpty")}</p>}
      {!error && rootEntries.length > 0 && <div role="tree" aria-label={t("skill.files")}><SkillFileRows entriesByDirectory={entriesByDirectory} expanded={expanded} loadingDirectories={loadingDirectories} activePath={file?.path || ""} onToggleDirectory={(path) => {
        setExpanded((current) => {
          const next = new Set(current);
          if (next.has(path)) next.delete(path); else next.add(path);
          return next;
        });
        if (!Object.hasOwn(entriesByDirectory, path)) void listDirectory(path);
      }} onOpenFile={openFile} /></div>}
    </div>

    {file ? <section className="flex min-h-0 flex-1 flex-col" aria-label={t("skill.fileEditor")}>
      <div className="flex h-10 shrink-0 items-center gap-1.5 px-3">
        <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-foreground" title={file.path}>{file.name}</span>
        {dirty && <i className="size-1.5 shrink-0 rounded-full bg-primary" aria-label={t("skill.unsaved")} />}
        <button type="button" className="grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-40" disabled={!dirty || saving} aria-label={t("skill.revertFile")} title={t("skill.revertFile")} onClick={revertFile}><RotateCcw size={14} aria-hidden="true" /></button>
        <button type="button" className="h-7 shrink-0 rounded-md bg-primary px-2.5 text-[11px] font-medium text-primary-foreground transition-opacity hover:opacity-90 disabled:bg-transparent disabled:text-muted-foreground disabled:opacity-100" disabled={!dirty || saving} onClick={saveFile}>{saving ? t("common.saving") : t("common.save")}</button>
        <button type="button" className="grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground" aria-label={t("common.close")} title={t("common.close")} onClick={closeFile}><X size={14} aria-hidden="true" /></button>
      </div>
      <textarea
        className="min-h-0 flex-1 resize-none bg-transparent px-3 pb-2 font-mono text-[11.5px] leading-[1.7] text-foreground outline-none"
        spellCheck="false"
        aria-label={file.path}
        value={draft}
        onChange={(event) => editDraft(event.target.value)}
      />
      <p className="shrink-0 px-3 pb-2 text-[10px] text-muted-foreground">{formatSkillBytes(draft.length)} · {t("skill.livePreviewHint")}</p>
    </section> : <p className="shrink-0 px-3 pb-3 text-[11px] leading-relaxed text-muted-foreground">{t("skill.selectFileHint")}</p>}
  </div>;
}

export default memo(SkillFilesInspector);
