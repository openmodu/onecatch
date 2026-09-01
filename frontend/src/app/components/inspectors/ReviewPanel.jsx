import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { FileDiff, FilePlus2, Folder, LoaderCircle, RefreshCw, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { GitBinding, WorkspaceBinding } from "../../../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { Button } from "@/components/ui/button";
import { buildGitReview } from "../../gitReview.js";
import { errorMessage } from "../../format.js";

const DEMO_DIFF = `diff --git a/frontend/src/app/components/Composer.jsx b/frontend/src/app/components/Composer.jsx
index 2cc67d1..b5aa194 100644
--- a/frontend/src/app/components/Composer.jsx
+++ b/frontend/src/app/components/Composer.jsx
@@ -42,3 +42,4 @@ export default function Composer() {
   const canSend = Boolean(draft.trim());
-  const label = "继续";
+  const label = "发送消息";
+  const directAgent = workflowId === "single_agent";
   return <div>{label}</div>;
diff --git a/frontend/src/index.css b/frontend/src/index.css
index 88231a0..1be5412 100644
--- a/frontend/src/index.css
+++ b/frontend/src/index.css
@@ -1234,3 +1234,4 @@
 .conversation-message-actions {
-  height: 24px;
+  height: 27px;
+  padding-top: 3px;
 }
`;

const statusCode = (file) => `${file?.status?.index || " "}${file?.status?.worktree || " "}`;
const fileName = (path = "") => path.split("/").pop() || path;
const directoryName = (path = "") => path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : ".";

function scopeLabel(scope, t) {
  if (scope === "staged") return t("review.staged");
  if (scope === "untracked") return t("review.untracked");
  return t("review.worktree");
}

function DiffLine({ line }) {
  const marker = line.kind === "add" ? "+" : line.kind === "delete" ? "−" : " ";
  return <div className={`review-diff-line ${line.kind}`}>
    <span className="review-line-number">{line.oldNumber ?? ""}</span>
    <span className="review-line-number">{line.newNumber ?? ""}</span>
    <span className="review-line-marker" aria-hidden="true">{marker}</span>
    <code>{line.text || " "}</code>
  </div>;
}

export default function ReviewPanel({ mode, workspaceID, active, onClose, notify }) {
  const { t } = useTranslation();
  const [review, setReview] = useState({ files: [], additions: 0, deletions: 0 });
  const [isRepository, setIsRepository] = useState(null);
  const [loading, setLoading] = useState(false);
  const [selectedPath, setSelectedPath] = useState("");
  const fileRefs = useRef(new Map());
  const diffScroll = useRef(null);

  const load = useCallback(async () => {
    if (!workspaceID || !active) return;
    setLoading(true);
    try {
      if (mode === "demo") {
        const next = buildGitReview({ files: [
          { path: "frontend/src/app/components/Composer.jsx", index: " ", worktree: "M" },
          { path: "frontend/src/index.css", index: " ", worktree: "M" },
        ] }, "", DEMO_DIFF);
        setIsRepository(true);
        setReview(next);
        setSelectedPath((current) => current || next.files[0]?.path || "");
        return;
      }
      const snapshot = await GitBinding.Status(workspaceID);
      if (!snapshot?.isRepo) {
        setIsRepository(false);
        setReview({ files: [], additions: 0, deletions: 0 });
        setSelectedPath("");
        return;
      }
      setIsRepository(true);
      const [stagedDiff, worktreeDiff] = await Promise.all([
        GitBinding.Diff(workspaceID, true),
        GitBinding.Diff(workspaceID, false),
      ]);
      const untrackedFiles = (snapshot.files || []).filter((file) => `${file.index || " "}${file.worktree || " "}`.includes("?"));
      const documents = await Promise.all(untrackedFiles.map(async (file) => {
        try {
          const document = await WorkspaceBinding.ReadWorkspaceFile(workspaceID, file.path);
          return [file.path, document.content];
        } catch {
          return [file.path, undefined];
        }
      }));
      const untracked = Object.fromEntries(documents.filter(([, content]) => content !== undefined));
      const next = buildGitReview(snapshot, stagedDiff, worktreeDiff, untracked);
      setReview(next);
      setSelectedPath((current) => next.files.some((file) => file.path === current) ? current : next.files[0]?.path || "");
    } catch (error) {
      notify?.("error", errorMessage(error));
    } finally {
      setLoading(false);
    }
  }, [active, mode, notify, workspaceID]);

  useEffect(() => { void load(); }, [load]);

  const groups = useMemo(() => {
    const value = new Map();
    review.files.forEach((file) => {
      const directory = directoryName(file.path);
      value.set(directory, [...(value.get(directory) || []), file]);
    });
    return [...value.entries()];
  }, [review.files]);

  const selectFile = (path) => {
    setSelectedPath(path);
    const node = fileRefs.current.get(path);
    const scroller = diffScroll.current;
    if (!node || !scroller) return;
    // Scrolling the diff column itself keeps the surrounding workbench still,
    // which scrollIntoView cannot promise once an ancestor can scroll too.
    const top = scroller.scrollTop + node.getBoundingClientRect().top - scroller.getBoundingClientRect().top;
    scroller.scrollTo({ top, behavior: "smooth" });
  };

  return <section className="review-panel" aria-label={t("review.title")}>
    <header className="review-toolbar">
      <div>
        <strong>{t("review.title")}</strong>
        <span>{t("review.filesChanged", { count: review.files.length })}</span>
      </div>
      <div className="review-stats" aria-label={t("review.summary")}><b>+{review.additions}</b><em>−{review.deletions}</em></div>
      <Button type="button" variant="ghost" size="icon-sm" aria-label={t("common.refresh")} title={t("common.refresh")} disabled={loading} onClick={load}>{loading ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : <RefreshCw aria-hidden="true" />}</Button>
      <Button type="button" variant="ghost" size="icon-sm" aria-label={t("common.close")} title={t("common.close")} onClick={onClose}><X aria-hidden="true" /></Button>
    </header>
    <div className="review-layout">
      <div className="review-diff-scroll" ref={diffScroll}>
        {loading && !review.files.length ? <div className="review-empty"><LoaderCircle className="animate-spin" aria-hidden="true" />{t("common.loading")}</div>
          : review.files.length ? <div className="review-diff-canvas">{review.files.map((file) => <article className="review-file" key={file.path} ref={(node) => { if (node) fileRefs.current.set(file.path, node); else fileRefs.current.delete(file.path); }}>
            <header>
              <FileDiff size={15} aria-hidden="true" />
              <strong title={file.path}>{file.path}</strong>
              <span className="review-file-stats"><b>+{file.additions}</b><em>−{file.deletions}</em></span>
            </header>
            {file.hunks.length ? file.hunks.map((hunk, index) => <section className="review-hunk" key={`${file.path}-${hunk.scope}-${index}`}>
              <div className="review-hunk-header"><span>{scopeLabel(hunk.scope, t)}</span><code>{hunk.header}</code></div>
              {hunk.lines.map((line, lineIndex) => <DiffLine line={line} key={`${index}-${lineIndex}`} />)}
            </section>) : <p className="review-no-preview">{t("review.previewUnavailable")}</p>}
          </article>)}</div> : <div className="review-empty"><FileDiff aria-hidden="true" />{t(isRepository === false ? "review.notGit" : "review.noChanges")}</div>}
      </div>
      <aside className="review-files" aria-label={t("review.changedFiles")}>
        <div className="review-files-heading"><span>{t("review.changedFiles")}</span><strong>{review.files.length}</strong></div>
        <div className="review-file-tree">{groups.map(([directory, files]) => <section key={directory}>
          <div className="review-directory"><Folder size={14} aria-hidden="true" /><span title={directory}>{directory}</span></div>
          {files.map((file) => <button type="button" className={selectedPath === file.path ? "active" : ""} title={file.path} onClick={() => selectFile(file.path)} key={file.path}>
            {statusCode(file).includes("?") ? <FilePlus2 size={14} aria-hidden="true" /> : <FileDiff size={14} aria-hidden="true" />}
            <span>{fileName(file.path)}</span>
            <b>+{file.additions}</b><em>−{file.deletions}</em>
          </button>)}
        </section>)}</div>
      </aside>
    </div>
  </section>;
}
