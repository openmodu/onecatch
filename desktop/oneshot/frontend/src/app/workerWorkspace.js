function dirty(snapshot) {
  return Boolean(snapshot?.status?.trim() || snapshot?.files?.length);
}

export function assessWorkerWorkspace(local, remote) {
  if (!local?.isRepo) return { code: "localGitRequired", ready: false };
  if (!remote?.isRepo) return { code: "remoteGitRequired", ready: false };
  if (!local.head) return { code: "localCommitRequired", ready: false };
  if (!remote.head) return { code: "remoteCommitRequired", ready: false };
  const localDirty = dirty(local);
  const remoteDirty = dirty(remote);
  if (localDirty && remoteDirty) return { code: "bothDirty", ready: false };
  if (localDirty) return { code: "localDirty", ready: false };
  if (remoteDirty) return { code: "remoteDirty", ready: false };
  if (local.head !== remote.head) return { code: "revisionMismatch", ready: false };
  return { code: "ready", ready: true };
}

export function classifyWorkerPreflightError(error) {
  const message = String(error?.message || error || "");
  if (message.includes("worker_workspace_unmapped")) return { code: "workspaceUnmapped", message: "" };
  if (message.includes("worker_unavailable")) return { code: "workerUnavailable", message: "" };
  return { code: "error", message };
}

function shellQuote(value) {
  return `'${String(value).replaceAll("'", "'\"'\"'")}'`;
}

export function defaultRemoteWorkspacePath(workspace) {
  const name = String(workspace?.path || workspace?.name || "project").split(/[\\/]/).filter(Boolean).pop() || "project";
  return `/absolute/path/to/${name}`;
}

export function buildWorkerCommand({ workspace, workerID, remotePath }) {
  if (!workspace?.id) return "";
  const mapping = `${workspace.id}=${remotePath || defaultRemoteWorkspacePath(workspace)}`;
  return [
    "ONESHOT_WORKER_TOKEN='<shared-token>' oneshot-worker \\",
    "  --listen 0.0.0.0:9231 \\",
    `  --id ${shellQuote(workerID || "mac-mini")} \\`,
    `  --workspace ${shellQuote(mapping)} \\`,
    "  --tls-cert '<server.pem>' \\",
    "  --tls-key '<server-key.pem>'",
  ].join("\n");
}
