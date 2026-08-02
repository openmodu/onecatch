import assert from "node:assert/strict";
import test from "node:test";
import { codexEffortValues, codexServiceTierValues, selectedCodexModel } from "./codexRuntimeOptions.js";

const configuration = {
  model: "gpt-sol",
  reasoningEffort: "high",
  serviceTier: "priority",
  models: [
    { id: "gpt-sol", model: "gpt-sol", isDefault: true, reasoningEfforts: ["low", "high"], serviceTiers: [{ id: "priority" }] },
    { id: "gpt-luna", model: "gpt-luna", reasoningEfforts: ["low", "medium"], serviceTiers: [] },
  ],
};

test("Codex runtime options follow detected defaults", () => {
  assert.equal(selectedCodexModel(configuration)?.model, "gpt-sol");
  assert.deepEqual(codexEffortValues(configuration), ["low", "high"]);
  assert.deepEqual(codexServiceTierValues(configuration), ["standard", "priority"]);
});

test("Codex runtime options follow an explicit model and preserve saved custom values", () => {
  assert.equal(selectedCodexModel(configuration, "gpt-luna")?.model, "gpt-luna");
  assert.deepEqual(codexEffortValues(configuration, "gpt-luna", "xhigh"), ["low", "medium", "xhigh"]);
  assert.deepEqual(codexServiceTierValues(configuration, "gpt-luna", "fast"), ["standard", "fast"]);
});
