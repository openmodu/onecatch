import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { CircleCheckBig, FileDiff, FilePlus2, Folder, LoaderCircle, RefreshCw, Sparkles, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Events } from "@wailsio/runtime";
import { GitBinding, RuntimeBinding, WorkspaceBinding } from "../../../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { Button } from "@/components/ui/button";
import { buildGitReview } from "../../gitReview.js";
import { runtimesChangedEvent } from "../../auxiliaryWindowEvents.js";
import { errorMessage } from "../../format.js";
import { runtimeHarnesses } from "../../runtimeHarnesses.js";
import HarnessSelector from "../HarnessSelector.jsx";

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

const DEMO_AGENT_REVIEW = {
  runtime: "codex",
  summary: "发现一处会改变既有交互行为的问题。",
  findings: [{
    priority: 2,
    title: "保留非单 Agent 工作流的发送标签",
    body: "标签现在无条件变为“发送消息”，会让工作流任务失去原有的继续语义。应只在 directAgent 为 true 时切换。",
    file: "frontend/src/app/components/Composer.jsx",
    startLine: 45,
    endLine: 45,
  }],
  truncated: false,
};

const statusCode = (file) => `${file?.status?.index || " "}${file?.status?.worktree || " "}`;
const fileName = (path = "") => path.split("/").pop() || path;
const directoryName = (path = "") => path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : ".";

function scopeLabel(scope, t) {
  if (scope === "staged") return t("review.staged");
  if (scope === "untracked") return t("review.untracked");
  return t("review.worktree");
}

function DiffLine({ line, filePath, findingPriority, registerLine }) {
  const marker = line.kind === "add" ? "+" : line.kind === "delete" ? "−" : " ";
  const lineNumber = line.newNumber ?? line.oldNumber;
  return <div
    className={`review-diff-line ${line.kind}${findingPriority !== undefined ? ` review-finding-line priority-${findingPriority}` : ""}`}
    ref={(node) => registerLine?.(filePath, lineNumber, node)}
  >
    <span className="review-line-number">{line.oldNumber ?? ""}</span>
    <span className="review-line-number">{line.newNumber ?? ""}</span>
    <span className="review-line-marker" aria-hidden="true">{marker}</span>
    <code>{line.text || " "}</code>
  </div>;
}

export default function ReviewPanel({ mode, workspaceID, preferredHarness = "codex", active, onClose, notify }) {
  const { t, i18n } = useTranslation();
  const [review, setReview] = useState({ files: [], additions: 0, deletions: 0 });
  const [isRepository, setIsRepository] = useState(null);
  const [loading, setLoading] = useState(false);
  const [selectedPath, setSelectedPath] = useState("");
  const [runtimes, setRuntimes] = useState([]);
  const [reviewProfile, setReviewProfile] = useState({ harness: preferredHarness || "codex" });
  const [agentReview, setAgentReview] = useState(null);
  const [reviewing, setReviewing] = useState(false);
  const fileRefs = useRef(new Map());
  const lineRefs = useRef(new Map());
  const diffScroll = useRef(null);

  useEffect(() => {
    setReviewProfile({ harness: preferredHarness || "codex" });
    setAgentReview(null);
  }, [preferredHarness, workspaceID]);

  useEffect(() => {
    if (mode === "demo") {
      setRuntimes(runtimeHarnesses.map((runtime) => ({ id: runtime.id, name: runtime.label, available: true })));
      return;
    }
    let cancelled = false;
    RuntimeBinding.ListRuntimes()
      .then((items) => { if (!cancelled) setRuntimes(items || []); })
      .catch(() => { if (!cancelled) setRuntimes([]); });
    return () => { cancelled = true; };
  }, [mode]);

  useEffect(() => {
    if (mode !== "wails") return undefined;
    return Events.On(runtimesChangedEvent, (event) => {
      if (Array.isArray(event?.data)) setRuntimes(event.data);
    });
  }, [mode]);

  useEffect(() => {
    if (!runtimes.length || runtimes.some((runtime) => runtime.id === reviewProfile.harness && runtime.enabled !== false)) return;
    const next = runtimes.find((runtime) => runtime.enabled !== false && runtime.available !== false)
      || runtimes.find((runtime) => runtime.enabled !== false);
    setReviewProfile((current) => ({ ...current, harness: next?.id || "" }));
    setAgentReview(null);
  }, [reviewProfile.harness, runtimes]);

  const load = useCallback(async () => {
    if (!workspaceID || !active) return;
    setLoading(true);
    setAgentReview(null);
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

  const runAgentReview = useCallback(async () => {
    if (!workspaceID || reviewing || !review.files.length) return;
    setReviewing(true);
    try {
      const result = mode === "demo"
        ? { ...DEMO_AGENT_REVIEW, runtime: reviewProfile.harness }
        : await GitBinding.ReviewChanges({ workspaceId: workspaceID, runtime: reviewProfile.harness, language: i18n.resolvedLanguage || i18n.language || "en" });
      setAgentReview(result);
    } catch (error) {
      notify?.("error", errorMessage(error));
    } finally {
      setReviewing(false);
    }
  }, [i18n.language, i18n.resolvedLanguage, mode, notify, review.files.length, reviewProfile.harness, reviewing, workspaceID]);

  const groups = useMemo(() => {
    const value = new Map();
    review.files.forEach((file) => {
      const directory = directoryName(file.path);
      value.set(directory, [...(value.get(directory) || []), file]);
    });
    return [...value.entries()];
  }, [review.files]);

  const findingLines = useMemo(() => {
    const lines = new Map();
    (agentReview?.findings || []).forEach((finding) => {
      for (let line = finding.startLine; line <= finding.endLine; line += 1) {
        const key = `${finding.file}:${line}`;
        const current = lines.get(key);
        if (current === undefined || finding.priority < current) lines.set(key, finding.priority);
      }
    });
    return lines;
  }, [agentReview?.findings]);

  const registerLine = useCallback((path, line, node) => {
    if (!path || !line) return;
    const key = `${path}:${line}`;
    if (node) lineRefs.current.set(key, node);
    else lineRefs.current.delete(key);
  }, []);

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

  const selectFinding = (finding) => {
    setSelectedPath(finding.file);
    const scroller = diffScroll.current;
    const node = lineRefs.current.get(`${finding.file}:${finding.startLine}`) || fileRefs.current.get(finding.file);
    if (!node || !scroller) return;
    const top = scroller.scrollTop + node.getBoundingClientRect().top - scroller.getBoundingClientRect().top - 36;
    scroller.scrollTo({ top: Math.max(0, top), behavior: "smooth" });
  };

  const runtimeStatus = runtimes.find((runtime) => runtime.id === reviewProfile.harness);
  const reviewDisabled = reviewing || loading || isRepository === false || !review.files.length || !runtimeStatus || runtimeStatus.enabled === false || runtimeStatus.available === false;
  const changeReviewProfile = (update) => {
    setReviewProfile((current) => typeof update === "function" ? update(current) : update);
    setAgentReview(null);
  };

  return <section className="review-panel" aria-label={t("review.title")}>
    <header className="review-toolbar">
      <div>
        <strong>{t("review.title")}</strong>
        <span>{t("review.filesChanged", { count: review.files.length })}</span>
      </div>
      <HarnessSelector value={reviewProfile} onChange={changeReviewProfile} runtimes={runtimes} readOnly={reviewing} agentLabel className="review-agent-selector" menuSide="bottom" />
      <Button type="button" size="sm" className="review-agent-run" disabled={reviewDisabled} onClick={runAgentReview} aria-label={t("review.runAgent")} title={t("review.runAgent")}>
        {reviewing ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : <Sparkles aria-hidden="true" />}
        {/* Hidden below the panel's narrow breakpoint, where the icon and the
            button's accessible name carry it on their own. */}
        <span className="review-agent-run-label">{reviewing ? t("review.reviewing") : t("review.runAgent")}</span>
      </Button>
      <div className="review-stats" aria-label={t("review.summary")}><b>+{review.additions}</b><em>−{review.deletions}</em></div>
      <Button type="button" variant="ghost" size="icon-sm" aria-label={t("common.refresh")} title={t("common.refresh")} disabled={loading} onClick={load}>{loading ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : <RefreshCw aria-hidden="true" />}</Button>
      <Button type="button" variant="ghost" size="icon-sm" aria-label={t("common.close")} title={t("common.close")} onClick={onClose}><X aria-hidden="true" /></Button>
    </header>
    <div className={`review-layout${agentReview || reviewing ? " has-agent-review" : ""}`}>
      <div className="review-diff-scroll" ref={diffScroll}>
        {loading && !review.files.length ? <div className="review-empty"><LoaderCircle className="animate-spin" aria-hidden="true" />{t("common.loading")}</div>
          : review.files.length ? review.files.map((file) => <article className="review-file" key={file.path} ref={(node) => { if (node) fileRefs.current.set(file.path, node); else fileRefs.current.delete(file.path); }}>
            <header>
              <FileDiff size={15} aria-hidden="true" />
              <strong title={file.path}>{file.path}</strong>
              <span className="review-file-stats"><b>+{file.additions}</b><em>−{file.deletions}</em></span>
            </header>
            {file.hunks.length ? file.hunks.map((hunk, index) => <section className="review-hunk" key={`${file.path}-${hunk.scope}-${index}`}>
              <div className="review-hunk-header"><span>{scopeLabel(hunk.scope, t)}</span><code>{hunk.header}</code></div>
              {hunk.lines.map((line, lineIndex) => <DiffLine line={line} filePath={file.path} findingPriority={findingLines.get(`${file.path}:${line.newNumber ?? line.oldNumber}`)} registerLine={registerLine} key={`${index}-${lineIndex}`} />)}
            </section>) : <p className="review-no-preview">{t("review.previewUnavailable")}</p>}
          </article>) : <div className="review-empty"><FileDiff aria-hidden="true" />{t(isRepository === false ? "review.notGit" : "review.noChanges")}</div>}
      </div>
      <aside className={`review-files${agentReview || reviewing ? " has-agent-review" : ""}`} aria-label={t("review.changedFiles")}>
        {(agentReview || reviewing) && <section className="review-findings" aria-label={t("review.agentFindings")}>
          <div className="review-findings-heading"><span>{t("review.agentFindings")}</span><strong>{agentReview?.findings?.length || 0}</strong></div>
          {reviewing ? <div className="review-findings-empty"><LoaderCircle className="animate-spin" aria-hidden="true" />{t("review.reviewing")}</div>
            : <div className="review-findings-list">
              {agentReview?.summary && <p className="review-agent-summary">{agentReview.summary}</p>}
              {agentReview?.truncated && <p className="review-agent-partial">{t("review.partialInput")}</p>}
              {agentReview?.findings?.length ? agentReview.findings.map((finding, index) => <button type="button" className={`review-finding priority-${finding.priority}`} onClick={() => selectFinding(finding)} key={`${finding.file}:${finding.startLine}:${index}`}>
                <span className="review-finding-priority">P{finding.priority}</span>
                <strong>{finding.title}</strong>
                <small>{finding.file}:{finding.startLine}{finding.endLine > finding.startLine ? `–${finding.endLine}` : ""}</small>
                <p>{finding.body}</p>
              </button>) : <div className="review-findings-empty success"><CircleCheckBig aria-hidden="true" />{t("review.noFindings")}</div>}
            </div>}
        </section>}
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
