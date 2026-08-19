import assert from "node:assert/strict";
import test from "node:test";
import {
  claudeModelDisplayLabel,
  codexEffortValues,
  codexServiceTierValues,
  defaultClaudeModel,
  groupedClaudeModels,
  selectedCodexModel,
} from "./codexRuntimeOptions.js";

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

test("Claude models follow the reference hierarchy without inventing unavailable models", () => {
  const models = [
    { model: "fable", displayName: "Fable", alias: true },
    { model: "opus", displayName: "Opus", alias: true },
    { model: "sonnet", displayName: "Sonnet", alias: true },
    { model: "claude-opus-4-8", displayName: "claude-opus-4-8", alias: false },
  ];
  assert.equal(defaultClaudeModel(models), "opus");
  assert.equal(claudeModelDisplayLabel(models[0]), "Fable 5");
  assert.equal(claudeModelDisplayLabel(models[3]), "Opus 4.8");
  assert.deepEqual(groupedClaudeModels(models).primary.map((model) => model.model), ["fable", "opus", "sonnet"]);
  assert.deepEqual(groupedClaudeModels(models).more.map((model) => model.model), ["claude-opus-4-8"]);
  assert.deepEqual(groupedClaudeModels([...models, { model: "claude-fable-5", displayName: "claude-fable-5", alias: false }]).more.map((model) => model.model), ["claude-opus-4-8", "claude-fable-5"]);
});
