import test from "node:test";
import assert from "node:assert/strict";
import { isRemoteWorker, selectTaskWorker, workerSupportsHarness } from "./taskWorkers.js";
import { selectTaskExecutionTarget } from "./runtimeHarnesses.js";

test("sandbox selection keeps the prompt and uses remote Codex without local worktree or profile", () => {
  const sandbox = { id: "sandbox", provider: "volcengine-sandbox", enabled: true };
  const next = selectTaskWorker({ prompt: "hello", harness: "claude", model: "local-model", worktree: { mode: "new" } }, sandbox);
  assert.equal(next.prompt, "hello");
  assert.equal(next.harness, "codex");
  assert.equal(next.workerId, "sandbox");
  assert.equal(next.model, "");
  assert.deepEqual(next.worktree, { mode: "project" });
  assert.equal(workerSupportsHarness(sandbox, "claude"), false);
  assert.equal(workerSupportsHarness(sandbox, "codex"), true);
  assert.equal(workerSupportsHarness({ ...sandbox, enabled: false }, "codex"), false);
  assert.equal(workerSupportsHarness(undefined, "codex"), false);
  assert.equal(isRemoteWorker(selectTaskWorker(next, null).workerId), false);
});
test("workflow selection clears the conversation worker because each node supplies its own", () => {
  assert.equal(selectTaskExecutionTarget({workerId:"sandbox"}, "workflow:review").workerId, "");
  assert.equal(isRemoteWorker("local"), false);
});

test("combined Agent menu selects remote targets and clears worker when switching back to local", async () => {
  const { selectConversationTarget, workerLabel } = await import("./taskWorkers.js");
  const worker = { id: "sandbox:one", name: "Volcengine Sandbox · gem", provider: "volcengine-sandbox", enabled: true };
  const form = { workflowId: "single_agent", harness: "modu", prompt: "hello" };
  const remote = selectConversationTarget(form, `remote:${JSON.stringify([worker.id, "codex"])}`, [worker]);
  assert.equal(remote.workerId, worker.id);
  assert.equal(remote.harness, "codex");
  assert.equal(workerLabel(worker), "codex volc_sandbox");
  const local = selectConversationTarget(remote, "agent:modu", [worker]);
  assert.equal(local.workerId, "");
  assert.equal(local.harness, "modu");
  assert.equal(local.prompt, "hello");
  assert.equal(selectConversationTarget(form, 'remote:["missing","codex"]', [worker]), form);
});
