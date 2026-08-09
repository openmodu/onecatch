import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Clipboard } from "@wailsio/runtime";
import { GitBinding, WorkerBinding } from "../../../bindings/github.com/openmodu/oneshot/internal/transport/wails/index.js";
import { Badge } from "@/components/ui/badge";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { SettingsButton, SettingsField, SettingsKicker, SettingsSection } from "./settings/SettingsControls.jsx";
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
  return <div className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] gap-x-3 gap-y-1 rounded-lg bg-background/75 p-3">
    <span className="col-span-full text-[10px] font-medium tracking-wide text-muted-foreground uppercase">{label}</span>
    <strong className="truncate text-sm text-foreground">{snapshot?.branch || "—"}</strong>
    <code className="select-text text-xs text-info">{snapshot?.head ? shortID(snapshot.head) : "—"}</code>
    <small className="col-span-full text-xs text-muted-foreground">{snapshot?.status?.trim() || snapshot?.files?.length ? t("worker.preflightChanges", { count: snapshot?.files?.length || 1 }) : t("worker.preflightClean")}</small>
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

  return <div>
    <SettingsSection title={t("worker.title")} description={t("worker.description")} aside={<SettingsButton tone="primary" onClick={() => openWorker(null)}>{t("worker.register")}</SettingsButton>} contentClassName="p-4">
      <div className="grid gap-3">{workers.map((worker) => <Card className="gap-4 py-4 shadow-none" key={worker.id}>
        <CardHeader className="gap-1 px-4">
          <CardTitle className="text-sm">{worker.name}</CardTitle>
          <CardDescription className="text-xs">{worker.id}</CardDescription>
          <CardAction><Badge variant="outline" className={worker.enabled ? "border-success/35 bg-success/10 text-success" : "border-border bg-muted text-muted-foreground"}>{worker.enabled ? t("common.enabled") : t("common.disabled")}</Badge></CardAction>
        </CardHeader>
        <CardContent className="px-4">
          <code className="block select-text truncate rounded-md bg-muted px-2.5 py-2 text-xs text-muted-foreground">{worker.baseUrl}</code>
          <div className="mt-3 flex flex-wrap gap-2 text-xs text-muted-foreground">{health[worker.id]?.checking && !health[worker.id]?.ok ? t("worker.checking") : health[worker.id]?.ok ? <><span className="text-success">protocol v{health[worker.id].protocolVersion || 1}</span><span className="text-success">{health[worker.id].latencyMilliseconds ?? 0} ms</span>{Object.entries(health[worker.id].runtimes || {}).map(([runtime, ok]) => <span key={runtime} className={ok ? "text-success" : "text-destructive"}>{runtime}</span>)}</> : health[worker.id]?.error || t("worker.notChecked")}</div>
          {workspace && <section className={`mt-4 rounded-lg bg-muted/65 p-3 ${preflight[worker.id]?.ready ? "text-success" : preflight[worker.id] ? "text-warning" : "text-muted-foreground"}`}>
            <div className="flex items-center justify-between gap-4"><span className="min-w-0"><SettingsKicker>{t("worker.preflight")}</SettingsKicker><strong className="ml-2 text-sm text-foreground">{workspace.name}</strong></span><div className="flex shrink-0 gap-2"><SettingsButton compact tone="primary" disabled={preflight[worker.id]?.preparing} onClick={() => prepareWorkspace(worker)}>{preflight[worker.id]?.preparing ? t("worker.workspacePreparing") : t("worker.workspacePrepare")}</SettingsButton><SettingsButton compact tone="muted" disabled={preflight[worker.id]?.checking || preflight[worker.id]?.preparing} onClick={() => runPreflight(worker)}>{preflight[worker.id]?.checking ? t("worker.preflightChecking") : t("worker.preflightRun")}</SettingsButton></div></div>
            {preflight[worker.id]?.local && <div className="mt-3 grid grid-cols-2 gap-2"><SnapshotSummary label={t("worker.preflightLocal")} snapshot={preflight[worker.id].local} t={t} /><SnapshotSummary label={t("worker.preflightRemote")} snapshot={preflight[worker.id].remote} t={t} /></div>}
            {preflight[worker.id]?.mapping?.path && <div className="mt-3 flex gap-2 text-xs text-muted-foreground"><span>{t("worker.remoteMappedPath")}</span><code className="min-w-0 select-text truncate text-info">{preflight[worker.id].mapping.path}</code></div>}
            {preflight[worker.id] && !preflight[worker.id].checking && !preflight[worker.id].preparing && <p className="mt-3 mb-0 text-xs">{preflight[worker.id].error || preflightCopy[preflight[worker.id].code]}</p>}
            {!preflight[worker.id] && <p className="mt-3 mb-0 text-xs">{t("worker.preflightDescription")}</p>}
          </section>}
          <div className="mt-4 flex flex-wrap gap-2"><SettingsButton tone="muted" onClick={() => checkWorker(worker)}>{t("worker.health")}</SettingsButton><SettingsButton tone="muted" onClick={() => openWorker(worker)}>{t("common.edit")}</SettingsButton><SettingsButton tone="danger" onClick={() => deleteWorker(worker.id)}>{t("common.delete")}</SettingsButton></div>
        </CardContent>
      </Card>)}{!workers.length && <div className="rounded-lg bg-muted p-6 text-center"><h4 className="m-0 text-sm text-foreground">{t("worker.empty")}</h4><p className="mt-1 mb-0 text-xs text-muted-foreground">{t("worker.emptyDescription")}</p></div>}</div>
    </SettingsSection>
    <SettingsSection title={t("worker.command")} description={t("worker.commandDescription")} contentClassName="p-4">
      <SettingsField label={t("worker.commandWorkerID")}><Input value={workerID} onChange={(event) => setWorkerID(event.target.value)} placeholder="mac-mini" /></SettingsField>
      <pre className="mt-4 select-text overflow-x-auto rounded-lg bg-muted p-4 text-xs text-foreground"><code>{command}</code></pre>
      <div className="mt-4 flex items-center justify-between gap-4"><p className="m-0 text-xs text-muted-foreground">{t("worker.networkWarning")}</p><SettingsButton tone={copied ? "cyan" : "primary"} onClick={copyCommand}>{copied ? t("worker.commandCopiedShort") : t("worker.commandCopy")}</SettingsButton></div>
    </SettingsSection>
  </div>;
}
