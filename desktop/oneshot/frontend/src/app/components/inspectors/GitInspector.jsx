import { memo, useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { GitBinding } from "../../../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";
import { Action, Kicker } from "../../../ui/primitives.jsx";
import { errorMessage, shortID } from "../../format.js";

function fileState(file, t) {
  const code = `${file.index || " "}${file.worktree || " "}`;
  if (code.includes("?")) return { label: t("inspector.fileAdded"), tone: "added" };
  if (code.includes("D")) return { label: t("inspector.fileDeleted"), tone: "deleted" };
  if (code.includes("R")) return { label: t("inspector.fileRenamed"), tone: "renamed" };
  if (code.includes("A")) return { label: t("inspector.fileAdded"), tone: "added" };
  return { label: t("inspector.fileModified"), tone: "modified" };
}

// Git is intentionally read-only here. The inspector answers one question:
// what changed in this workspace? Mutating operations stay in the user's Git
// client or terminal, where staging and partial commits remain explicit.
function GitInspector({ mode, workspaceID, notify }) {
  const { t } = useTranslation();
  const [snapshot, setSnapshot] = useState(null);
  const [refreshing, setRefreshing] = useState(false);

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
        return;
      }
      setSnapshot(await GitBinding.Status(workspaceID));
    } catch (error) {
      notify("error", errorMessage(error));
    } finally {
      setRefreshing(false);
    }
  }, [mode, notify, workspaceID]);

  useEffect(() => { load(); }, [load]);

  if (snapshot && !snapshot.isRepo) return <p className="inspector-placeholder">{t("inspector.notGit")}</p>;

  const files = snapshot?.files || [];
  return <div className="git-inspector">
    <div className="git-head">
      <div><Kicker>{t("inspector.repository")}</Kicker><strong>{snapshot?.branch || t("common.loading")}</strong><small>{snapshot?.head ? shortID(snapshot.head) : ""}</small></div>
      <Action bracket={false} tone="muted" className="git-refresh" disabled={refreshing} aria-label={t("inspector.refreshGit")} onClick={load}>{refreshing ? t("common.refreshing") : t("common.refresh")}</Action>
    </div>
    <div className="git-sync" aria-label={t("inspector.gitSync")}>
      <span>↑ {snapshot?.ahead || 0}</span>
      <span>↓ {snapshot?.behind || 0}</span>
      <strong>{files.length ? t("inspector.changesCount", { count: files.length }) : t("inspector.clean")}</strong>
    </div>
    {files.length > 0 && <section className="git-change-list">
      <Kicker>{t("inspector.changes")}</Kicker>
      {files.map((file) => { const state = fileState(file, t); return <div className="git-change-row" key={file.path}><b className={state.tone}>{state.label}</b><span title={file.path}>{file.path}</span></div>; })}
    </section>}
    {snapshot && !files.length && <p className="git-clean-copy">{t("inspector.noChanges")}</p>}
  </div>;
}

export default memo(GitInspector);
