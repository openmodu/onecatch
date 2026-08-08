import { memo, useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { GitBinding, WorkerBinding } from "../../../../bindings/github.com/openmodu/oneshot/internal/transport/wails/index.js";
import { Action, Kicker, TUISelect } from "../../../ui/primitives.jsx";
import { errorMessage, shortID } from "../../format.js";

const FILE_TONE = {
  added: "text-success",
  deleted: "text-destructive",
  renamed: "text-info",
  modified: "text-warning",
};

function fileState(file, t) {
  const code = `${file.index || " "}${file.worktree || " "}`;
  if (code.includes("?")) return { label: t("inspector.fileAdded"), tone: "added" };
  if (code.includes("D")) return { label: t("inspector.fileDeleted"), tone: "deleted" };
  if (code.includes("R")) return { label: t("inspector.fileRenamed"), tone: "renamed" };
  if (code.includes("A")) return { label: t("inspector.fileAdded"), tone: "added" };
  return { label: t("inspector.fileModified"), tone: "modified" };
}

// Branch operations are available for the local workspace only. The remote
// source is an operational view of that clone, so its branches remain read-only.
function GitInspector({ mode, workspaceID, runWorkerID = "", notify }) {
  const { t } = useTranslation();
  const [snapshot, setSnapshot] = useState(null);
  const [refreshing, setRefreshing] = useState(false);
  const [workers, setWorkers] = useState([]);
  const [branches, setBranches] = useState([]);
  const [branchBusy, setBranchBusy] = useState(false);
  const [newBranch, setNewBranch] = useState("");
  // Which machine's git state to show: "local" or a remote worker id.
  const [source, setSource] = useState("local");

  useEffect(() => {
    if (mode !== "wails") return;
    WorkerBinding.ListWorkers()
      .then((list) => setWorkers((list || []).filter((entry) => entry.enabled)))
      .catch(() => setWorkers([]));
  }, [mode]);

  // Follow the run: default to the worker its latest step ran on, but only when
  // that worker is registered here (so we never query one we cannot reach) and
  // fall back to local otherwise. The user can still override with the selector.
  useEffect(() => {
    setSource(runWorkerID && workers.some((entry) => entry.id === runWorkerID) ? runWorkerID : "local");
  }, [runWorkerID, workspaceID, workers]);

  useEffect(() => { setNewBranch(""); }, [source, workspaceID]);

  const load = useCallback(async () => {
    if (!workspaceID) {
      setSnapshot(null);
      return;
    }
    setRefreshing(true);
    try {
      if (mode === "demo") {
        setSnapshot({
          isRepo: true,
          branch: "feature/workbench",
          head: "019f5ed",
          ahead: 0,
          behind: 0,
          files: [{ path: "src/app/App.jsx", index: " ", worktree: "M" }],
        });
        setBranches([
          { name: "feature/workbench", current: true, upstream: "origin/feature/workbench" },
          { name: "main", current: false, upstream: "origin/main" },
        ]);
        return;
      }
      const nextSnapshot = source === "local"
        ? await GitBinding.Status(workspaceID)
        : await WorkerBinding.WorkerGitStatus(source, workspaceID);
      setSnapshot(nextSnapshot);
      setBranches(source === "local" && nextSnapshot?.isRepo ? (await GitBinding.ListBranches(workspaceID) || []) : []);
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setRefreshing(false);
    }
  }, [mode, notify, source, workspaceID]);

  useEffect(() => { load(); }, [load]);

  const sourceOptions = useMemo(
    () => [{ value: "local", label: t("inspector.sourceLocal") }, ...workers.map((entry) => ({ value: entry.id, label: entry.name }))],
    [workers, t],
  );

  const branchOptions = useMemo(
    () => {
      const items = [...branches];
      if (snapshot?.branch && !items.some((branch) => branch.name === snapshot.branch)) items.push({ name: snapshot.branch, current: true, upstream: "" });
      if (!snapshot?.branch) return [{ value: "", label: t("inspector.detachedHead"), meta: snapshot?.head ? shortID(snapshot.head) : "", disabled: true }];
      return items
        .sort((left, right) => Number(right.current) - Number(left.current) || left.name.localeCompare(right.name))
        .map((branch) => ({ value: branch.name, label: branch.name, meta: branch.upstream || "" }));
    },
    [branches, snapshot?.branch, snapshot?.head, t],
  );

  const switchBranch = useCallback(async (name) => {
    if (!workspaceID || !name || name === snapshot?.branch || source !== "local") return;
    setBranchBusy(true);
    try {
      if (mode === "demo") {
        setSnapshot((current) => ({ ...current, branch: name }));
        setBranches((items) => items.map((branch) => ({ ...branch, current: branch.name === name })));
      } else {
        setSnapshot(await GitBinding.SwitchBranch(workspaceID, name));
        setBranches(await GitBinding.ListBranches(workspaceID) || []);
      }
      notify("success", t("inspector.branchSwitched", { name }));
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBranchBusy(false);
    }
  }, [mode, notify, snapshot?.branch, source, t, workspaceID]);

  const createBranch = useCallback(async (event) => {
    event.preventDefault();
    const name = newBranch.trim();
    if (!name) {
      notify("error", t("inspector.branchNameRequired"));
      return;
    }
    setBranchBusy(true);
    try {
      if (mode === "demo") {
        if (branches.some((branch) => branch.name === name)) throw new Error(t("inspector.branchExists", { name }));
        setSnapshot((current) => ({ ...current, branch: name }));
        setBranches((items) => [...items.map((branch) => ({ ...branch, current: false })), { name, current: true, upstream: "" }]);
      } else {
        setSnapshot(await GitBinding.CreateBranch(workspaceID, name));
        setBranches(await GitBinding.ListBranches(workspaceID) || []);
      }
      setNewBranch("");
      notify("success", t("inspector.branchCreated", { name }));
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setBranchBusy(false);
    }
  }, [branches, mode, newBranch, notify, t, workspaceID]);

  if (snapshot && !snapshot.isRepo) return <p className="m-0 px-4 py-5 text-xs leading-relaxed text-muted-foreground">{t("inspector.notGit")}</p>;

  const files = snapshot?.files || [];
  // grid-cols-[minmax(0,1fr)] rather than a bare grid: auto-sized tracks let a
  // long branch name or remote ref push the whole panel past its 310px column.
  return <div className="grid grid-cols-[minmax(0,1fr)] gap-3.5 p-3.5">
    {workers.length > 0 && <div><TUISelect ariaLabel={t("inspector.source")} value={source} onChange={setSource} options={sourceOptions} /></div>}
    <div className="flex items-start justify-between gap-2.5">
      <div className="min-w-0">
        <Kicker className="block">{t("inspector.repository")}</Kicker>
        <strong className="mt-1 block truncate text-sm font-semibold text-foreground">{snapshot?.branch || t("common.loading")}</strong>
        <small className="mt-0.5 block font-mono text-[11px] text-muted-foreground">{snapshot?.head ? shortID(snapshot.head) : ""}</small>
      </div>
      <Action size="compact" tone="muted" className="shrink-0" disabled={refreshing} aria-label={t("inspector.refreshGit")} onClick={load}>{refreshing ? t("common.refreshing") : t("common.refresh")}</Action>
    </div>
    <div className="flex items-center gap-3 rounded-md border bg-muted/50 px-2.5 py-2 text-[11px]" aria-label={t("inspector.gitSync")}>
      <span className="font-mono text-muted-foreground">↑ {snapshot?.ahead || 0}</span>
      <span className="font-mono text-muted-foreground">↓ {snapshot?.behind || 0}</span>
      <strong className={`ml-auto font-medium ${files.length ? "text-warning" : "text-success"}`}>{files.length ? t("inspector.changesCount", { count: files.length }) : t("inspector.clean")}</strong>
    </div>
    {source === "local" && snapshot?.isRepo && <section className="grid grid-cols-[minmax(0,1fr)] gap-2">
      <Kicker>{t("inspector.branches")}</Kicker>
      <TUISelect ariaLabel={t("inspector.branchSelect")} value={snapshot?.branch || ""} onChange={switchBranch} options={branchOptions} disabled={branchBusy || !branchOptions.length} />
      <form className="flex gap-1.5" onSubmit={createBranch}>
        <input className="h-8 min-w-0 flex-1 rounded-md border border-input bg-transparent px-2.5 text-xs shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:opacity-50" value={newBranch} onChange={(event) => setNewBranch(event.target.value)} disabled={branchBusy} aria-label={t("inspector.branchName")} placeholder={t("inspector.branchCreatePlaceholder")} />
        <Action type="submit" size="compact" disabled={branchBusy || !newBranch.trim()}>{branchBusy ? t("common.loading") : t("inspector.branchCreate")}</Action>
      </form>
    </section>}
    {source !== "local" && <p className="m-0 text-[11px] leading-relaxed text-muted-foreground">{t("inspector.remoteBranchesReadOnly")}</p>}
    {files.length > 0 && <section className="grid gap-1">
      <Kicker className="mb-1">{t("inspector.changes")}</Kicker>
      {files.map((file) => {
        const state = fileState(file, t);
        const tone = FILE_TONE[state.tone] || "text-muted-foreground";
        return <div className="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-2 py-0.5" key={file.path}>
          <b className={`font-mono text-[11px] font-bold ${tone}`}>{state.label}</b>
          <span className="truncate font-mono text-[11px] text-muted-foreground" title={file.path}>{file.path}</span>
        </div>;
      })}
    </section>}
    {snapshot && !files.length && <p className="m-0 text-[11px] text-muted-foreground">{t("inspector.noChanges")}</p>}
  </div>;
}

export default memo(GitInspector);
