import { memo, useCallback, useEffect, useState } from "react";
import { GitBinding } from "../../../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";
import { Action, Kicker, TUISelect } from "../../../ui/primitives.jsx";
import { errorMessage, shortID } from "../../format.js";

// Props are stable across run polling; memo keeps its own git state from being
// thrown away by parent re-renders.
function GitInspector({ mode, workspaceID, runtimes, notify }) {
  const [snapshot, setSnapshot] = useState(null);
  const [diff, setDiff] = useState("");
  const [staged, setStaged] = useState(false);
  const [message, setMessage] = useState("");
  const [runtime, setRuntime] = useState(runtimes.find((item) => item.available)?.id || "codex");
  const [push, setPush] = useState(false);
  const [gitBusy, setGitBusy] = useState("");
  const load = useCallback(async (nextStaged = staged) => {
    if (!workspaceID) return;
    try {
      if (mode === "demo") { setSnapshot({ isRepo: true, branch: "feature/workbench", head: "019f5ed", ahead: 0, behind: 0, files: [{ path: "src/app/App.jsx", index: " ", worktree: "M" }], diffStat: "1 file changed" }); setDiff("diff --git a/src/app/App.jsx b/src/app/App.jsx\n+三栏任务工作台"); return; }
      const [statusValue, diffValue] = await Promise.all([GitBinding.Status(workspaceID), GitBinding.Diff(workspaceID, nextStaged)]);
      setSnapshot(statusValue); setDiff(diffValue || "");
    } catch (error) { notify("error", errorMessage(error)); }
  }, [mode, notify, staged, workspaceID]);
  useEffect(() => { load(staged); }, [load, staged]);
  const stageAll = async () => { setGitBusy("stage"); try { if (mode !== "demo") await GitBinding.StageAll(workspaceID); setStaged(true); await load(true); notify("success", "所有变更已暂存"); } catch (error) { notify("error", errorMessage(error)); } finally { setGitBusy(""); } };
  const generate = async () => { setGitBusy("generate"); try { const value = mode === "demo" ? "feat: add task workbench" : await GitBinding.GenerateCommitMessage(workspaceID, runtime); setMessage(value); } catch (error) { notify("error", errorMessage(error)); } finally { setGitBusy(""); } };
  const commit = async () => { if (!message.trim()) { notify("error", "请填写提交信息"); return; } setGitBusy("commit"); try { const result = mode === "demo" ? { commitHash: "019f5ed", pushed: push } : await GitBinding.CommitAndPush({ workspaceId: workspaceID, message: message.trim(), remote: "", push }); notify("success", result.pushed ? `已提交并推送 ${shortID(result.commitHash)}` : `已提交 ${shortID(result.commitHash)}`); setMessage(""); setStaged(false); await load(false); } catch (error) { notify("error", errorMessage(error)); } finally { setGitBusy(""); } };
  if (snapshot && !snapshot.isRepo) return <p className="inspector-placeholder">当前 Workspace 不是 Git 仓库。</p>;
  return <div className="git-inspector"><div className="git-head"><div><Kicker>repository</Kicker><strong>{snapshot?.branch || "读取中…"}</strong><small>{snapshot?.head || ""}</small></div><button type="button" onClick={() => load(staged)}>[ 刷新 ]</button></div><div className="git-sync"><span>↑ {snapshot?.ahead || 0}</span><span>↓ {snapshot?.behind || 0}</span><span>{snapshot?.files?.length || 0} files</span></div><div className="git-files">{snapshot?.files?.map((file) => <div key={file.path}><b>{`${file.index || " "}${file.worktree || " "}`}</b><span title={file.path}>{file.path}</span></div>)}{snapshot && !snapshot.files?.length && <p>工作区干净。</p>}</div><div className="git-diff-tabs"><button type="button" className={!staged ? "active" : ""} onClick={() => setStaged(false)}>未暂存</button><button type="button" className={staged ? "active" : ""} onClick={() => setStaged(true)}>已暂存</button><button type="button" disabled={gitBusy === "stage" || !snapshot?.files?.length} onClick={stageAll}>全部暂存</button></div><pre className="git-diff">{diff || "没有可显示的 Diff。"}</pre><div className="commit-box"><div><TUISelect ariaLabel="生成提交信息的 Runtime" value={runtime} onChange={setRuntime} options={runtimes.filter((item) => item.available).map((item) => ({ value: item.id, label: item.name || item.id }))} /><button type="button" disabled={gitBusy === "generate" || !snapshot?.files?.length} onClick={generate}>{gitBusy === "generate" ? "生成中…" : "生成提交信息"}</button></div><textarea value={message} onChange={(event) => setMessage(event.target.value)} placeholder="feat: describe the change" /><label><input type="checkbox" checked={push} onChange={(event) => setPush(event.target.checked)} /> commit 后推送当前分支</label><Action tone="primary" disabled={gitBusy === "commit" || !message.trim()} onClick={commit}>{gitBusy === "commit" ? "提交中…" : push ? "Commit & Push" : "Commit"}</Action></div></div>;
}

export default memo(GitInspector);
