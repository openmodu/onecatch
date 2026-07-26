import { memo, useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { GitBinding, WorkerBinding } from "../../../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";
import { Action, Kicker, TUISelect } from "../../../ui/primitives.jsx";
import { errorMessage, shortID } from "../../format.js";

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

  if (snapshot && !snapshot.isRepo) return <p className="inspector-placeholder">{t("inspector.notGit")}</p>;

  const files = snapshot?.files || [];
  return <div className="git-inspector">
    {workers.length > 0 && <div className="git-source"><TUISelect ariaLabel={t("inspector.source")} value={source} onChange={setSource} options={sourceOptions} /></div>}
    <div className="git-head">
      <div><Kicker>{t("inspector.repository")}</Kicker><strong>{snapshot?.branch || t("common.loading")}</strong><small>{snapshot?.head ? shortID(snapshot.head) : ""}</small></div>
      <Action tone="muted" className="git-refresh" disabled={refreshing} aria-label={t("inspector.refreshGit")} onClick={load}>{refreshing ? t("common.refreshing") : t("common.refresh")}</Action>
    </div>
    <div className="git-sync" aria-label={t("inspector.gitSync")}>
      <span>↑ {snapshot?.ahead || 0}</span>
      <span>↓ {snapshot?.behind || 0}</span>
      <strong>{files.length ? t("inspector.changesCount", { count: files.length }) : t("inspector.clean")}</strong>
    </div>
    {source === "local" && snapshot?.isRepo && <section className="git-branches">
      <Kicker>{t("inspector.branches")}</Kicker>
      <TUISelect ariaLabel={t("inspector.branchSelect")} value={snapshot?.branch || ""} onChange={switchBranch} options={branchOptions} disabled={branchBusy || !branchOptions.length} />
      <form className="git-branch-create" onSubmit={createBranch}>
        <input value={newBranch} onChange={(event) => setNewBranch(event.target.value)} disabled={branchBusy} aria-label={t("inspector.branchName")} placeholder={t("inspector.branchCreatePlaceholder")} />
        <Action type="submit" size="compact" disabled={branchBusy || !newBranch.trim()}>{branchBusy ? t("common.loading") : t("inspector.branchCreate")}</Action>
      </form>
    </section>}
    {source !== "local" && <p className="git-branch-readonly">{t("inspector.remoteBranchesReadOnly")}</p>}
    {files.length > 0 && <section className="git-change-list">
      <Kicker>{t("inspector.changes")}</Kicker>
      {files.map((file) => { const state = fileState(file, t); return <div className="git-change-row" key={file.path}><b className={state.tone}>{state.label}</b><span title={file.path}>{file.path}</span></div>; })}
    </section>}
    {snapshot && !files.length && <p className="git-clean-copy">{t("inspector.noChanges")}</p>}
  </div>;
}

export default memo(GitInspector);
