import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Clipboard } from "@wailsio/runtime";
import { GitBinding, WorkerBinding } from "../../../bindings/github.com/openmodu/oneshot/internal/desktop/bindings/index.js";
import { Action, Field, Kicker, SettingsModule, StatusBadge } from "../../ui/primitives.jsx";
import { errorMessage, shortID } from "../format.js";
import { assessWorkerWorkspace, buildWorkerCommand, classifyWorkerPreflightError } from "../workerWorkspace.js";

async function copyText(value) {
  try {
    await Clipboard.SetText(value);
  } catch (wailsError) {
    if (!navigator.clipboard?.writeText) throw wailsError;
    await navigator.clipboard.writeText(value);
  }
}

function SnapshotSummary({ label, snapshot, t }) {
  return <div className="worker-preflight-snapshot">
    <span>{label}</span>
    <strong>{snapshot?.branch || "—"}</strong>
    <code>{snapshot?.head ? shortID(snapshot.head) : "—"}</code>
    <small>{snapshot?.status?.trim() || snapshot?.files?.length ? t("worker.preflightChanges", { count: snapshot?.files?.length || 1 }) : t("worker.preflightClean")}</small>
  </div>;
}

export default function WorkerPage({ mode, workspace, workers, health, checkWorker, deleteWorker, openWorker, notify }) {
  const { t } = useTranslation();
  const [preflight, setPreflight] = useState({});
  const [workerID, setWorkerID] = useState("mac-mini");
  const [copied, setCopied] = useState(false);
  const checkWorkerRef = useRef(checkWorker);
  checkWorkerRef.current = checkWorker;
  const workerPollKey = workers.filter((worker) => worker.enabled).map((worker) => worker.id).sort().join("\x00");
  useEffect(() => {
    setPreflight({});
  }, [workspace?.id]);
  useEffect(() => {
    if (!copied) return undefined;
    const timer = window.setTimeout(() => setCopied(false), 1800);
    return () => window.clearTimeout(timer);
  }, [copied]);
  useEffect(() => {
    const enabledWorkers = workers.filter((worker) => worker.enabled);
    const poll = () => enabledWorkers.forEach((worker) => { void checkWorkerRef.current(worker); });
    poll();
    const timer = window.setInterval(poll, 15000);
    return () => window.clearInterval(timer);
  }, [workerPollKey]);

  const command = useMemo(() => buildWorkerCommand({ workerID }), [workerID]);
  const preflightCopy = {
    ready: t("worker.preflightReady"),
    localGitRequired: t("worker.preflightLocalGitRequired"),
    remoteGitRequired: t("worker.preflightRemoteGitRequired"),
    localCommitRequired: t("worker.preflightLocalCommitRequired"),
    remoteCommitRequired: t("worker.preflightRemoteCommitRequired"),
    localDirty: t("worker.preflightLocalDirty"),
    remoteDirty: t("worker.preflightRemoteDirty"),
    bothDirty: t("worker.preflightBothDirty"),
    revisionMismatch: t("worker.preflightRevisionMismatch"),
    workspaceUnmapped: t("worker.preflightWorkspaceUnmapped"),
    workspaceManagementUnsupported: t("worker.workspaceManagementUnsupported"),
    workspaceRemoteMissing: t("worker.workspaceRemoteMissing"),
    workspaceRemoteMismatch: t("worker.workspaceRemoteMismatch"),
    workspaceCloneFailed: t("worker.workspaceCloneFailed"),
    workspaceFetchFailed: t("worker.workspaceFetchFailed"),
    workspaceRevisionMissing: t("worker.workspaceRevisionMissing"),
    workerUnavailable: t("worker.preflightWorkerUnavailable"),
  };

  const runPreflight = async (worker) => {
    if (!workspace?.id) return;
    setPreflight((current) => ({ ...current, [worker.id]: { checking: true } }));
    try {
      const [local, remote] = mode === "demo"
        ? [
          { isRepo: true, branch: "main", head: "a31f821", status: "", files: [] },
          { isRepo: true, branch: "main", head: "a31f821", status: "", files: [] },
        ]
        : await Promise.all([GitBinding.Status(workspace.id), WorkerBinding.WorkerGitStatus(worker.id, workspace.id)]);
      const assessment = assessWorkerWorkspace(local, remote);
      setPreflight((current) => ({ ...current, [worker.id]: { ...assessment, local, remote } }));
    } catch (error) {
      const classified = classifyWorkerPreflightError(error);
      setPreflight((current) => ({ ...current, [worker.id]: { code: classified.code, ready: false, error: classified.message ? errorMessage(classified.message) : "" } }));
    }
  };

  const prepareWorkspace = async (worker) => {
    if (!workspace?.id) return;
    setPreflight((current) => ({ ...current, [worker.id]: { ...current[worker.id], preparing: true } }));
    try {
      const result = mode === "demo"
        ? {
          mapping: { id: workspace.id, path: `~/.oneshot-worker/projects/${workspace.id}` },
          local: { isRepo: true, branch: "main", head: "a31f821", status: "", files: [] },
          remote: { isRepo: true, branch: "main", head: "a31f821", status: "", files: [] },
        }
        : await WorkerBinding.PrepareWorkerWorkspace(worker.id, workspace.id);
      const assessment = assessWorkerWorkspace(result.local, result.remote);
      setPreflight((current) => ({ ...current, [worker.id]: { ...assessment, ...result, preparing: false } }));
      notify?.("success", t("worker.workspacePrepared", { name: worker.name }));
    } catch (error) {
      const classified = classifyWorkerPreflightError(error);
      setPreflight((current) => ({ ...current, [worker.id]: { code: classified.code, ready: false, preparing: false, error: classified.message ? errorMessage(classified.message) : "" } }));
      notify?.("error", t("worker.workspacePrepareFailed", { error: errorMessage(error) }));
    }
  };

  const copyCommand = async () => {
    try {
      await copyText(command);
      setCopied(true);
      notify?.("success", t("worker.commandCopied"));
    } catch (error) {
      notify?.("error", t("worker.commandCopyFailed", { error: errorMessage(error) }));
    }
  };

  return <div className="worker-page">
    <SettingsModule title={t("worker.title")} description={t("worker.description")} aside={<Action tone="primary" onClick={() => openWorker(null)}>{t("worker.register")}</Action>} bodyClassName="worker-list-body">
      <div className="worker-grid">{workers.map((worker) => <article className="worker-card" key={worker.id}>
        <div className="worker-card-head"><div className="worker-machine"><span><h4>{worker.name}</h4><small>{worker.id}</small></span></div><StatusBadge status={worker.enabled ? "completed" : "cancelled"} className="status-pill">{worker.enabled ? t("common.enabled") : t("common.disabled")}</StatusBadge></div>
        <code>{worker.baseUrl}</code>
        <div className="worker-health">{health[worker.id]?.checking && !health[worker.id]?.ok ? t("worker.checking") : health[worker.id]?.ok ? <><span className="ok"><i />protocol v{health[worker.id].protocolVersion || 1}</span><span className="ok"><i />{health[worker.id].latencyMilliseconds ?? 0} ms</span>{Object.entries(health[worker.id].runtimes || {}).map(([runtime, ok]) => <span key={runtime} className={ok ? "ok" : "missing"}><i />{runtime}</span>)}</> : health[worker.id]?.error || t("worker.notChecked")}</div>
        {workspace && <section className={`worker-preflight ${preflight[worker.id]?.ready ? "ready" : preflight[worker.id] ? "blocked" : ""}`}>
          <div className="worker-preflight-head"><span><Kicker>{t("worker.preflight")}</Kicker><strong>{workspace.name}</strong></span><div className="worker-preflight-actions"><Action size="compact" tone="primary" disabled={preflight[worker.id]?.preparing} onClick={() => prepareWorkspace(worker)}>{preflight[worker.id]?.preparing ? t("worker.workspacePreparing") : t("worker.workspacePrepare")}</Action><Action size="compact" tone="muted" disabled={preflight[worker.id]?.checking || preflight[worker.id]?.preparing} onClick={() => runPreflight(worker)}>{preflight[worker.id]?.checking ? t("worker.preflightChecking") : t("worker.preflightRun")}</Action></div></div>
          {preflight[worker.id]?.local && <div className="worker-preflight-pair"><SnapshotSummary label={t("worker.preflightLocal")} snapshot={preflight[worker.id].local} t={t} /><SnapshotSummary label={t("worker.preflightRemote")} snapshot={preflight[worker.id].remote} t={t} /></div>}
          {preflight[worker.id]?.mapping?.path && <div className="worker-preflight-path"><span>{t("worker.remoteMappedPath")}</span><code>{preflight[worker.id].mapping.path}</code></div>}
          {preflight[worker.id] && !preflight[worker.id].checking && !preflight[worker.id].preparing && <p>{preflight[worker.id].error || preflightCopy[preflight[worker.id].code]}</p>}
          {!preflight[worker.id] && <p>{t("worker.preflightDescription")}</p>}
        </section>}
        <div className="worker-actions"><Action onClick={() => checkWorker(worker)}>{t("worker.health")}</Action><Action onClick={() => openWorker(worker)}>{t("common.edit")}</Action><Action tone="danger" onClick={() => deleteWorker(worker.id)}>{t("common.delete")}</Action></div>
      </article>)}{!workers.length && <div className="empty-state"><h4>{t("worker.empty")}</h4><p>{t("worker.emptyDescription")}</p></div>}</div>
    </SettingsModule>
    <SettingsModule title={t("worker.command")} description={t("worker.commandDescription")} bodyClassName="worker-command">
      <div className="worker-command-fields">
        <Field label={t("worker.commandWorkerID")}><input value={workerID} onChange={(event) => setWorkerID(event.target.value)} placeholder="mac-mini" /></Field>
      </div>
      <pre><code>{command}</code></pre>
      <div className="worker-command-actions"><p>{t("worker.networkWarning")}</p><Action tone={copied ? "cyan" : "primary"} onClick={copyCommand}>{copied ? t("worker.commandCopiedShort") : t("worker.commandCopy")}</Action></div>
    </SettingsModule>
  </div>;
}
