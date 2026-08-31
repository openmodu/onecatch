import { memo, useCallback, useEffect, useRef, useState } from "react";
import { ChevronDown, ChevronRight, File, FileCode2, FileText, Folder, FolderOpen, RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { SkillBinding } from "../../../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { errorMessage } from "../../format.js";

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

function fileVisual(name = "") {
  const lower = name.toLowerCase();
  if (lower.endsWith(".md") || lower.endsWith(".mdx") || lower.endsWith(".txt")) return { Icon: FileText, tone: "text-success" };
  if (/\.(c|cc|cpp|css|go|html|js|jsx|py|rs|sh|ts|tsx)$/.test(lower)) return { Icon: FileCode2, tone: "text-info" };
  return { Icon: File, tone: "text-muted-foreground" };
}

function SkillFileRows({ directory = "", depth = 0, entriesByDirectory, expanded, loadingDirectories, onToggleDirectory }) {
  return (entriesByDirectory[directory] || []).map((entry) => {
    const open = entry.directory && expanded.has(entry.path);
    const visual = fileVisual(entry.name);
    const Icon = entry.directory ? (open ? FolderOpen : Folder) : visual.Icon;
    return <div key={entry.path}>
      <button
        type="button"
        role="treeitem"
        aria-expanded={entry.directory ? open : undefined}
        className="flex h-7 w-full min-w-0 items-center gap-1.5 rounded-md pr-2 text-left text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        style={{ paddingLeft: `${8 + depth * 14}px` }}
        title={entry.path}
        onClick={() => entry.directory && onToggleDirectory(entry.path)}
      >
        {entry.directory ? (open ? <ChevronDown size={13} aria-hidden="true" /> : <ChevronRight size={13} aria-hidden="true" />) : <span className="w-[13px] shrink-0" aria-hidden="true" />}
        <Icon size={14} className={`shrink-0 ${entry.directory ? "text-muted-foreground" : visual.tone}`} aria-hidden="true" />
        <span className="min-w-0 flex-1 truncate">{entry.name}</span>
      </button>
      {open && <div role="group">
        {loadingDirectories.has(entry.path)
          ? <p className="m-0 py-1 pr-2 text-[11px] text-muted-foreground" style={{ paddingLeft: `${35 + depth * 14}px` }}>…</p>
          : <SkillFileRows directory={entry.path} depth={depth + 1} entriesByDirectory={entriesByDirectory} expanded={expanded} loadingDirectories={loadingDirectories} onToggleDirectory={onToggleDirectory} />}
      </div>}
    </div>;
  });
}

function SkillFilesInspector({ mode }) {
  const { t } = useTranslation();
  const [entriesByDirectory, setEntriesByDirectory] = useState({});
  const [expanded, setExpanded] = useState(() => new Set());
  const [loadingDirectories, setLoadingDirectories] = useState(() => new Set());
  const [error, setError] = useState("");
  const generationRef = useRef(0);

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
    setExpanded(new Set());
    setLoadingDirectories(new Set());
    void listDirectory("", generation);
  }, [listDirectory]);

  useEffect(() => {
    refresh();
    return () => { generationRef.current += 1; };
  }, [refresh]);

  const toggleDirectory = useCallback(async (path) => {
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
  }, [entriesByDirectory, expanded, listDirectory]);

  const rootEntries = entriesByDirectory[""] || [];
  const rootLoading = loadingDirectories.has("");

  return <div className="flex h-full min-h-0 flex-col">
    <div className="flex h-11 shrink-0 items-center gap-2 border-b border-border/70 px-3">
      <Folder size={14} className="shrink-0 text-muted-foreground" aria-hidden="true" />
      <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-muted-foreground">~/.onecatch/skills</span>
      <button type="button" className="grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground" aria-label={t("common.refresh")} title={t("common.refresh")} onClick={refresh}>
        <RefreshCw size={14} className={rootLoading ? "animate-spin" : ""} aria-hidden="true" />
      </button>
    </div>
    <div className="min-h-0 flex-1 overflow-y-auto px-2 py-2">
      {error && <p className="m-1 rounded-md bg-destructive/10 px-2 py-1.5 text-xs text-destructive">{error}</p>}
      {!error && rootLoading && <p className="m-1 text-xs text-muted-foreground">{t("common.loading")}</p>}
      {!error && !rootLoading && rootEntries.length === 0 && <p className="m-1 text-xs text-muted-foreground">{t("skill.filesEmpty")}</p>}
      {!error && rootEntries.length > 0 && <div role="tree" aria-label={t("skill.files")}><SkillFileRows entriesByDirectory={entriesByDirectory} expanded={expanded} loadingDirectories={loadingDirectories} onToggleDirectory={toggleDirectory} /></div>}
    </div>
  </div>;
}

export default memo(SkillFilesInspector);
