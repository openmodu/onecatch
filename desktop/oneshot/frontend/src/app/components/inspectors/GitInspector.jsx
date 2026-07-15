import { memo, useCallback, useEffect, useState } from "react";
import { GitBinding } from "../../../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";
import { Action, Kicker } from "../../../ui/primitives.jsx";
import { errorMessage, shortID } from "../../format.js";

function fileState(file) {
  const code = `${file.index || " "}${file.worktree || " "}`;
  if (code.includes("?")) return { label: "新增", tone: "added" };
  if (code.includes("D")) return { label: "删除", tone: "deleted" };
  if (code.includes("R")) return { label: "重命名", tone: "renamed" };
  if (code.includes("A")) return { label: "新增", tone: "added" };
  return { label: "修改", tone: "modified" };
}

// Git is intentionally read-only here. The inspector answers one question:
// what changed in this workspace? Mutating operations stay in the user's Git
// client or terminal, where staging and partial commits remain explicit.
function GitInspector({ mode, workspaceID, notify }) {
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

  if (snapshot && !snapshot.isRepo) return <p className="inspector-placeholder">当前 Workspace 不是 Git 仓库。</p>;

  const files = snapshot?.files || [];
  return <div className="git-inspector">
    <div className="git-head">
      <div><Kicker>repository</Kicker><strong>{snapshot?.branch || "读取中…"}</strong><small>{snapshot?.head ? shortID(snapshot.head) : ""}</small></div>
      <Action bracket={false} tone="muted" className="git-refresh" disabled={refreshing} aria-label="刷新 Git 状态" onClick={load}>{refreshing ? "刷新中…" : "刷新"}</Action>
    </div>
    <div className="git-sync" aria-label="Git 同步状态">
      <span>↑ {snapshot?.ahead || 0}</span>
      <span>↓ {snapshot?.behind || 0}</span>
      <strong>{files.length ? `${files.length} 个变更` : "工作区干净"}</strong>
    </div>
    {files.length > 0 && <section className="git-change-list">
      <Kicker>changes</Kicker>
      {files.map((file) => { const state = fileState(file); return <div className="git-change-row" key={file.path}><b className={state.tone}>{state.label}</b><span title={file.path}>{file.path}</span></div>; })}
    </section>}
    {snapshot && !files.length && <p className="git-clean-copy">没有未提交的文件变更。</p>}
  </div>;
}

export default memo(GitInspector);
