import { worktreeSelectionMode } from "../worktreeContext.js";
import { useEffect, useState } from "react";
import { Check, ChevronDown, Circle, Folder, GitBranch, GitFork, HardDrive, LoaderCircle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { GitBinding } from "../../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { StatusBadge } from "../../ui/primitives.jsx";

function workspaceLocation(workspace) {
  return workspace?.remoteFs
    ? `${workspace.remoteFs.username ? `${workspace.remoteFs.username}@` : ""}${workspace.remoteFs.host}:${workspace.remoteFs.root}`
    : workspace?.path || "";
}

function demoWorktrees(workspace) {
  const projectId = workspace.projectId || workspace.id;
  const root = workspace.worktree?.commonDir?.replace(/\/.git$/, "") || workspace.path;
  return [
    { contextId: `worktree:${projectId}:main`, root, path: root, branch: "main", main: true, current: true },
    { contextId: `worktree:${projectId}:login`, root: `${root}-login`, path: `${root}-login`, branch: "feat/login", commonDir: `${root}/.git` },
    { contextId: `worktree:${projectId}:review`, root: `${root}-review`, path: `${root}-review`, branch: "fix/sidebar", commonDir: `${root}/.git` },
    { contextId: `worktree:${projectId}:old`, path: `${root}-old`, branch: "chore/cleanup", unavailable: true, reason: "directory_missing" },
  ];
}

export default function WorkspaceComposerMeta({ mode, workspace, onEdit, onSelectWorktree, selection, disabled = false }) {
  const { t } = useTranslation();
  const [state, setState] = useState({ id: "", snapshot: null, error: false });
  const [open, setOpen] = useState(false);
  const [listing, setListing] = useState({ id: "", items: [], loading: false, error: false });
  const id = workspace?.id;
  const projectId = workspace?.projectId || id;
  const binding = workspace?.worktree;
  const selectionMode = worktreeSelectionMode(workspace, selection);
  const pendingNew = Boolean(onSelectWorktree && selectionMode === "new");
  useEffect(() => {
    if (!id || mode === "loading") return;
    let disposed = false;
    let inFlight = false;
    const refresh = async () => {
      if (inFlight) return;
      inFlight = true;
      try {
        const snapshot = mode === "demo"
          ? { isRepo: true, branch: binding?.branch || "main", head: "a81c79e", files: binding ? [] : [{ path: "src/app.jsx" }, { path: "src/styles.css" }, { path: "README.md" }] }
          : await GitBinding.Status(id);
        if (!disposed) setState({ id, snapshot, error: false });
      } catch { if (!disposed) setState({ id, snapshot: null, error: true }); }
      finally { inFlight = false; }
    };
    void refresh();
    const onFocus = () => { void refresh(); };
    const timer = window.setInterval(() => { if (!document.hidden) void refresh(); }, 10000);
    window.addEventListener("focus", onFocus);
    return () => { disposed = true; clearInterval(timer); window.removeEventListener("focus", onFocus); };
  }, [id, mode, binding?.branch]);

  useEffect(() => {
    if (!open || !projectId || workspace?.remoteFs) return;
    let disposed = false;
    setListing({ id: projectId, items: [], loading: true, error: false });
    const promise = mode === "demo" ? Promise.resolve(demoWorktrees(workspace)) : GitBinding.ListWorktrees(projectId);
    promise.then((items) => { if (!disposed) setListing({ id: projectId, items, loading: false, error: false }); })
      .catch(() => { if (!disposed) setListing({ id: projectId, items: [], loading: false, error: true }); });
    return () => { disposed = true; };
  }, [open, projectId, mode, workspace?.remoteFs]);

  if (!workspace) return null;
  const location = workspaceLocation(workspace);
  const snapshot = state.id === id ? state.snapshot : null;
  const failed = state.id === id && state.error;
  const branch = snapshot?.branch || (snapshot?.head ? snapshot.head.slice(0, 7) : t("worktree.unborn"));
  const changed = snapshot?.files?.length || 0;
  const items = listing.id === projectId ? listing.items : [];
  const label = pendingNew ? t("worktree.new") : binding ? (binding.main ? t("worktree.main") : binding.root?.split(/[\\/]/).pop() || t("worktree.linked")) : t("worktree.current");
  return <div className="composer-workspace-meta">
    <button type="button" className="composer-workspace-button" aria-label={`${t("workspace.editProject")} ${workspace.name}`} title={`${t("workspace.editProject")} · ${location}`} onClick={onEdit}>
      <Folder size={14} strokeWidth={2.1} aria-hidden="true" /><span>{workspace.name}</span>
    </button>
    <StatusBadge status={mode === "wails" ? "good" : "warn"} className="composer-workspace-location" title={location}>
      <HardDrive size={12} strokeWidth={2.2} aria-hidden="true" />
      {mode === "wails" ? t(workspace.remoteFs ? "workspace.remoteFS" : "common.local") : t("common.preview")}
    </StatusBadge>
    {failed ? <span className="composer-git-error" role="status">{t("worktree.statusUnavailable")}</span> : (snapshot?.isRepo || (onSelectWorktree && workspace.autoWorktree)) && <>
      <span className="composer-meta-divider" aria-hidden="true" />
      <span className="composer-git-branch" title={`${branch} · ${location}`}><GitBranch size={13} aria-hidden="true" /><span>{pendingNew ? t("worktree.fromBranch", { branch }) : branch}</span></span>
      {!workspace.remoteFs && (onSelectWorktree ? <DropdownMenu open={open} onOpenChange={setOpen}>
        <DropdownMenuTrigger asChild><button type="button" disabled={disabled} className={`composer-worktree-trigger ${binding ? "is-linked" : ""}`} aria-label={t("worktree.choose")} title={location}><GitFork size={13} aria-hidden="true" /><span>{label}</span><ChevronDown size={11} aria-hidden="true" /></button></DropdownMenuTrigger>
        <DropdownMenuContent align="start" side="top" sideOffset={10} className="worktree-menu">
          <DropdownMenuLabel>{t("worktree.choose")}</DropdownMenuLabel>
          <p className="worktree-menu-hint">{t("worktree.hint")}</p>
          <DropdownMenuItem className="worktree-menu-row" onSelect={() => onSelectWorktree(null)}><GitFork size={15} /><span className="worktree-menu-copy"><strong>{t("worktree.followProject")}</strong><small>{t(workspace.autoWorktree ? "worktree.defaultNew" : "worktree.projectDirectory")}</small></span>{!selection && <Check size={14} />}</DropdownMenuItem>
          <DropdownMenuItem className="worktree-menu-row" disabled={!snapshot?.head} onSelect={() => onSelectWorktree({ mode: "new" })}><GitFork size={15} /><span className="worktree-menu-copy"><strong>{t("worktree.new")}</strong><small>{t("worktree.newDescription")}</small></span>{selection?.mode === "new" && <Check size={14} />}</DropdownMenuItem>
          <DropdownMenuItem className="worktree-menu-row" onSelect={() => onSelectWorktree({ mode: "project" })}><Folder size={15} /><span className="worktree-menu-copy"><strong>{t("worktree.current")}</strong><small>{t("worktree.thisSession")}</small></span>{selection?.mode === "project" && <Check size={14} />}</DropdownMenuItem>
          <DropdownMenuItem className="worktree-menu-row" onSelect={onEdit}><Folder size={15} /><span className="worktree-menu-copy"><strong>{t("worktree.projectSettings")}</strong><small>{t(workspace.autoWorktree ? "worktree.defaultNew" : "worktree.defaultProject")}</small></span></DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuLabel className="worktree-menu-section">{t("worktree.existing")}</DropdownMenuLabel>
          {listing.loading && <div className="worktree-menu-message"><LoaderCircle className="animate-spin" size={14} />{t("common.loading")}</div>}
          {listing.error && <div className="worktree-menu-message" role="alert">{t("worktree.listUnavailable")}</div>}
          {items.map((item) => <DropdownMenuItem key={item.contextId} disabled={item.unavailable} className="worktree-menu-row" onSelect={() => onSelectWorktree(item)} title={item.path}>
            <GitFork size={15} /><span className="worktree-menu-copy"><strong>{item.branch || item.head?.slice(0, 7) || t("worktree.unborn")}{item.main && <em>{t("worktree.main")}</em>}{item.unavailable && <em>{t("worktree.missing")}</em>}</strong><small>{item.path}</small></span>{binding?.contextId === item.contextId && <Check size={14} />}
          </DropdownMenuItem>)}
          <DropdownMenuSeparator /><p className="worktree-menu-footnote">{t("worktree.creationHint")}</p>
        </DropdownMenuContent>
      </DropdownMenu> : binding && <span className="composer-worktree-fixed" title={`${t("worktree.fixed")} · ${location}`}><GitFork size={13} aria-hidden="true" />{label}</span>)}
      {!pendingNew && <span className={`composer-git-changes ${changed ? "has-changes" : ""}`} title={t("worktree.changesHint")}><Circle size={6} fill="currentColor" aria-hidden="true" />{changed ? t("worktree.changed", { count: changed }) : t("worktree.clean")}</span>}
    </>}
  </div>;
}
