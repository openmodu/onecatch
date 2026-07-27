import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Clipboard } from "@wailsio/runtime";
import { GitBinding, WorkerBinding } from "../../../bindings/github.com/openmodu/oneshot/desktop/oneshot/bindings/index.js";
import { Action, Field, Kicker, SettingsModule, StatusBadge } from "../../ui/primitives.jsx";
import { errorMessage, shortID } from "../format.js";
import { assessWorkerWorkspace, buildWorkerCommand, classifyWorkerPreflightError, defaultRemoteWorkspacePath } from "../workerWorkspace.js";

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
  const [remotePath, setRemotePath] = useState(() => defaultRemoteWorkspacePath(workspace));
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    setPreflight({});
    setRemotePath(defaultRemoteWorkspacePath(workspace));
  }, [workspace?.id]);
  useEffect(() => {
    if (!copied) return undefined;
    const timer = window.setTimeout(() => setCopied(false), 1800);
    return () => window.clearTimeout(timer);
  }, [copied]);

  const command = useMemo(() => buildWorkerCommand({ workspace, workerID, remotePath }), [remotePath, workerID, workspace]);
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
        <div className="worker-health">{health[worker.id]?.checking ? t("worker.checking") : health[worker.id]?.ok ? <><span className="ok"><i />protocol v{health[worker.id].protocolVersion || 1}</span>{Object.entries(health[worker.id].runtimes || {}).map(([runtime, ok]) => <span key={runtime} className={ok ? "ok" : "missing"}><i />{runtime}</span>)}</> : health[worker.id]?.error || t("worker.notChecked")}</div>
        {workspace && <section className={`worker-preflight ${preflight[worker.id]?.ready ? "ready" : preflight[worker.id] ? "blocked" : ""}`}>
          <div className="worker-preflight-head"><span><Kicker>{t("worker.preflight")}</Kicker><strong>{workspace.name}</strong></span><Action size="compact" tone="muted" disabled={preflight[worker.id]?.checking} onClick={() => runPreflight(worker)}>{preflight[worker.id]?.checking ? t("worker.preflightChecking") : t("worker.preflightRun")}</Action></div>
          {preflight[worker.id]?.local && <div className="worker-preflight-pair"><SnapshotSummary label={t("worker.preflightLocal")} snapshot={preflight[worker.id].local} t={t} /><SnapshotSummary label={t("worker.preflightRemote")} snapshot={preflight[worker.id].remote} t={t} /></div>}
          {preflight[worker.id] && !preflight[worker.id].checking && <p>{preflight[worker.id].error || preflightCopy[preflight[worker.id].code]}</p>}
          {!preflight[worker.id] && <p>{t("worker.preflightDescription")}</p>}
        </section>}
        <div className="worker-actions"><Action onClick={() => checkWorker(worker)}>{t("worker.health")}</Action><Action onClick={() => openWorker(worker)}>{t("common.edit")}</Action><Action tone="danger" onClick={() => deleteWorker(worker.id)}>{t("common.delete")}</Action></div>
      </article>)}{!workers.length && <div className="empty-state"><h4>{t("worker.empty")}</h4><p>{t("worker.emptyDescription")}</p></div>}</div>
    </SettingsModule>
    <SettingsModule title={t("worker.command")} description={workspace ? t("worker.commandDescription", { name: workspace.name }) : t("worker.commandNoWorkspace")} bodyClassName="worker-command">
      {workspace ? <>
        <div className="worker-command-fields">
          <Field label={t("worker.commandWorkerID")}><input value={workerID} onChange={(event) => setWorkerID(event.target.value)} placeholder="mac-mini" /></Field>
          <Field label={t("worker.commandRemotePath")}><input value={remotePath} onChange={(event) => setRemotePath(event.target.value)} placeholder="/absolute/path/to/clone" /></Field>
        </div>
        <div className="worker-command-mapping"><span>{t("worker.commandWorkspaceID")}</span><code>{workspace.id}</code></div>
        <pre><code>{command}</code></pre>
        <div className="worker-command-actions"><p>{t("worker.networkWarning")}</p><Action tone={copied ? "cyan" : "primary"} onClick={copyCommand}>{copied ? t("worker.commandCopiedShort") : t("worker.commandCopy")}</Action></div>
      </> : <p>{t("worker.commandNoWorkspace")}</p>}
    </SettingsModule>
  </div>;
}
