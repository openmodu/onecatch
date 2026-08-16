import assert from "node:assert/strict";
import test from "node:test";
import {
  runtimeHarness,
  runtimeHarnessOptions,
  selectRuntimeHarness,
  selectTaskExecutionTarget,
  supportsRuntimeProfile,
  taskExecutionTarget,
} from "./runtimeHarnesses.js";

test("runtime harness metadata exposes the supported task runtimes", () => {
  assert.equal(runtimeHarness("codex").label, "Codex");
  assert.equal(runtimeHarness("codex").supportsSpeed, true);
  assert.equal(runtimeHarness("claude").label, "Claude Code");
  assert.equal(runtimeHarness("claude").supportsSpeed, false);
  assert.equal(runtimeHarness("modu").label, "modu_code");
  assert.equal(supportsRuntimeProfile("codex"), true);
  assert.equal(supportsRuntimeProfile("claude"), true);
  assert.equal(supportsRuntimeProfile("modu"), false);
});

test("runtime harness options preserve choices and flag unavailable binaries", () => {
  const options = runtimeHarnessOptions([
    { id: "codex", available: true },
    { id: "claude", available: false },
  ], "不可用");
  assert.deepEqual(options.map(({ value, label }) => ({ value, label })), [
    { value: "codex", label: "Codex" },
    { value: "claude", label: "Claude Code" },
    { value: "modu", label: "modu_code" },
  ]);
  assert.equal(options[1].disabled, true);
  assert.equal(options[1].meta, "不可用");
  assert.equal(options[2].disabled, false, "an unprobed runtime remains selectable while discovery is loading");
});

test("switching harness clears settings that belong to the previous runtime", () => {
  const current = { harness: "codex", model: "gpt-5", reasoningEffort: "high", serviceTier: "fast", prompt: "keep me" };
  assert.deepEqual(selectRuntimeHarness(current, "claude"), {
    harness: "claude",
    model: "",
    reasoningEffort: "",
    serviceTier: "",
    prompt: "keep me",
  });
  assert.equal(selectRuntimeHarness(current, "codex"), current);
});

test("Agent and workflow are mutually exclusive execution targets", () => {
  const agent = { workflowId: "single_agent", harness: "codex", model: "gpt", reasoningEffort: "high", serviceTier: "fast" };
  assert.equal(taskExecutionTarget(agent), "agent:codex");

  const workflow = selectTaskExecutionTarget(agent, "workflow:implement_review");
  assert.deepEqual(workflow, {
    workflowId: "implement_review",
    harness: "",
    model: "",
    reasoningEffort: "",
    serviceTier: "",
  });
  assert.equal(taskExecutionTarget(workflow), "workflow:implement_review");

  const claude = selectTaskExecutionTarget(workflow, "agent:claude");
  assert.equal(claude.workflowId, "single_agent");
  assert.equal(claude.harness, "claude");
});
