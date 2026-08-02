import test from "node:test";
import assert from "node:assert/strict";
import { assessWorkerWorkspace, buildWorkerCommand, classifyWorkerPreflightError } from "./workerWorkspace.js";

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

test("worker command enables one-time pairing without exposing a token", () => {
  const command = buildWorkerCommand({ workerID: "build-mac" });
  assert.match(command, /--id 'build-mac'/);
  assert.match(command, /--install-service/);
  assert.match(command, /--pair/);
  assert.doesNotMatch(command, /TOKEN|--workspace/);
  assert.match(command, /--tls-cert '<server\.pem>'/);
});

test("worker command safely quotes the worker id", () => {
  const command = buildWorkerCommand({ workerID: "builder's mac" });
  assert.match(command, /builder'"'"'s mac/);
});

test("worker preflight turns common transport errors into actionable states", () => {
  assert.deepEqual(classifyWorkerPreflightError(new Error("worker_workspace_unmapped: workspace is not mapped")), { code: "workspaceUnmapped", message: "" });
  assert.deepEqual(classifyWorkerPreflightError("worker_workspace_management_unsupported"), { code: "workspaceManagementUnsupported", message: "" });
  assert.deepEqual(classifyWorkerPreflightError("worker_workspace_clone_failed: auth required"), { code: "workspaceCloneFailed", message: "" });
  assert.deepEqual(classifyWorkerPreflightError("worker_unavailable: connection refused"), { code: "workerUnavailable", message: "" });
  assert.deepEqual(classifyWorkerPreflightError("worker_tls_invalid: bad certificate"), { code: "error", message: "worker_tls_invalid: bad certificate" });
});
