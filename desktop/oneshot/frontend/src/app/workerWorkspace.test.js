import test from "node:test";
import assert from "node:assert/strict";
import { assessWorkerWorkspace, buildWorkerCommand, classifyWorkerPreflightError, defaultRemoteWorkspacePath } from "./workerWorkspace.js";

const clean = { isRepo: true, head: "abc123", branch: "main", status: "", files: [] };

test("worker workspace preflight requires clean clones at the same revision", () => {
  assert.deepEqual(assessWorkerWorkspace(clean, clean), { code: "ready", ready: true });
  assert.equal(assessWorkerWorkspace({ ...clean, status: " M app.go" }, clean).code, "localDirty");
  assert.equal(assessWorkerWorkspace(clean, { ...clean, files: [{ path: "app.go" }] }).code, "remoteDirty");
  assert.equal(assessWorkerWorkspace({ ...clean, status: " M a" }, { ...clean, status: "?? b" }).code, "bothDirty");
  assert.equal(assessWorkerWorkspace(clean, { ...clean, head: "def456" }).code, "revisionMismatch");
  assert.equal(assessWorkerWorkspace({ isRepo: false }, clean).code, "localGitRequired");
  assert.equal(assessWorkerWorkspace(clean, { isRepo: false }).code, "remoteGitRequired");
});

test("worker command contains the exact coordinator workspace mapping", () => {
  const workspace = { id: "oneshot-a1b2c3", path: "/Users/me/Code/oneshot" };
  assert.equal(defaultRemoteWorkspacePath(workspace), "/absolute/path/to/oneshot");
  const command = buildWorkerCommand({ workspace, workerID: "build-mac", remotePath: "/srv/oneshot" });
  assert.match(command, /--id 'build-mac'/);
  assert.match(command, /--workspace 'oneshot-a1b2c3=\/srv\/oneshot'/);
  assert.match(command, /--tls-cert '<server\.pem>'/);
});

test("worker command safely quotes mapping values", () => {
  const command = buildWorkerCommand({ workspace: { id: "project-id" }, workerID: "builder's mac", remotePath: "/srv/team's project" });
  assert.match(command, /builder'"'"'s mac/);
  assert.match(command, /team'"'"'s project/);
});

test("worker preflight turns common transport errors into actionable states", () => {
  assert.deepEqual(classifyWorkerPreflightError(new Error("worker_workspace_unmapped: workspace is not mapped")), { code: "workspaceUnmapped", message: "" });
  assert.deepEqual(classifyWorkerPreflightError("worker_unavailable: connection refused"), { code: "workerUnavailable", message: "" });
  assert.deepEqual(classifyWorkerPreflightError("worker_tls_invalid: bad certificate"), { code: "error", message: "worker_tls_invalid: bad certificate" });
});
