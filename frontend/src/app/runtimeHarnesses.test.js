import assert from "node:assert/strict";
import test from "node:test";
import {
  runtimeHarness,
  runtimeHarnessEnabled,
  runtimeHarnessOptions,
  hasRemoteFSHarness,
  selectRuntimeHarness,
  selectTaskExecutionTarget,
  supportsRuntimeProfile,
  taskExecutionTarget,
  workflowHarnessesEnabled,
} from "./runtimeHarnesses.js";

test("runtime harness metadata exposes the supported task runtimes", () => {
  assert.equal(runtimeHarness("codex").label, "Codex");
  assert.equal(runtimeHarness("codex").supportsSpeed, true);
  assert.equal(runtimeHarness("claude").label, "Claude Code");
  assert.equal(runtimeHarness("claude").supportsSpeed, false);
  assert.equal(runtimeHarness("modu").label, "modu_code");
  assert.equal(runtimeHarness("pi").label, "Pi");
  assert.equal(runtimeHarness("grok").label, "Grok Build");
  assert.equal(runtimeHarness("dsh").label, "DeepSeek Harness");
  assert.equal(supportsRuntimeProfile("codex"), true);
  assert.equal(supportsRuntimeProfile("claude"), true);
  assert.equal(supportsRuntimeProfile("modu"), false);
  // Pi spells reasoning effort --thinking and Grok exposes --reasoning-effort;
  // the DeepSeek Harness headless profile offers no reasoning control at all.
  assert.equal(supportsRuntimeProfile("pi"), true);
  assert.equal(supportsRuntimeProfile("grok"), true);
  assert.equal(supportsRuntimeProfile("dsh"), false);
  // Codex remains the only harness with a speed/processing tier.
  assert.equal(runtimeHarness("pi").supportsSpeed, false);
  assert.equal(runtimeHarness("grok").supportsSpeed, false);
  assert.equal(runtimeHarness("codex").supportsRemoteFs, true);
  assert.equal(runtimeHarness("pi").supportsRemoteFs, false);
});

test("harness preferences filter local and remote choices", () => {
  const runtimes = [
    { id: "codex", available: true, supportsRemoteFs: true, enabled: true, remoteFsEnabled: true },
    { id: "claude", available: true, supportsRemoteFs: true, enabled: false, remoteFsEnabled: true },
    { id: "pi", available: true, supportsRemoteFs: false, enabled: true, remoteFsEnabled: false },
  ];
  const settings = {
    codex: { enabled: true, remoteFsEnabled: true },
    claude: { enabled: false, remoteFsEnabled: true },
    pi: { enabled: true, remoteFsEnabled: false },
  };
  assert.equal(runtimeHarnessEnabled("pi", runtimes, settings), true);
  assert.equal(runtimeHarnessEnabled("pi", runtimes, settings, true), false);
  assert.equal(hasRemoteFSHarness(runtimes, settings), true);
  assert.deepEqual(runtimeHarnessOptions(runtimes, "missing", settings).map((item) => item.value), ["codex", "modu", "pi", "grok", "dsh"]);
  assert.deepEqual(runtimeHarnessOptions(runtimes, "missing", settings, true).map((item) => item.value), ["codex", "modu"]);
  assert.equal(workflowHarnessesEnabled({ steps: [{ runtime: "pi" }] }, runtimes, settings, true), false);
  assert.equal(workflowHarnessesEnabled({ steps: [{ runtime: "codex" }] }, runtimes, settings, true), true);
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
    { value: "pi", label: "Pi" },
    { value: "grok", label: "Grok Build" },
    { value: "dsh", label: "DeepSeek Harness" },
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
