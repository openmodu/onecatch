import assert from "node:assert/strict";
import test from "node:test";
import {
  claudeModelDisplayLabel,
  codexEffortValues,
  codexServiceTierValues,
  defaultClaudeModel,
  groupedClaudeModels,
  runtimeDefaultEffort,
  runtimeEffortValues,
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

test("Pi runtime options use harness-wide thinking levels", () => {
  const pi = {
    efforts: ["off", "minimal", "low", "medium", "high", "xhigh"],
    models: [
      { model: "deepseek/deepseek-v4-flash", displayName: "deepseek-v4-flash" },
      { model: "deepseek/deepseek-v4-pro", displayName: "deepseek-v4-pro" },
    ],
  };
  assert.deepEqual(runtimeEffortValues(pi, "deepseek/deepseek-v4-flash"), pi.efforts);
  assert.deepEqual(runtimeEffortValues(pi, "deepseek/deepseek-v4-pro", "high"), pi.efforts);
  assert.equal(runtimeDefaultEffort(pi, "deepseek/deepseek-v4-flash"), "");
});

test("generic harness models override harness-wide reasoning levels", () => {
  const grok = {
    efforts: ["low", "medium", "high", "xhigh"],
    models: [
      { model: "grok-4.5", efforts: ["low", "medium", "high"], defaultEffort: "medium" },
      { model: "grok-4.6", efforts: ["low", "medium", "high", "xhigh"], defaultEffort: "high" },
    ],
  };
  assert.deepEqual(runtimeEffortValues(grok, "grok-4.5"), ["low", "medium", "high"]);
  assert.deepEqual(runtimeEffortValues(grok, "grok-4.6"), ["low", "medium", "high", "xhigh"]);
  assert.equal(runtimeDefaultEffort(grok, "grok-4.5"), "medium");
});

test("Claude models follow the reference hierarchy without inventing unavailable models", () => {
  const models = [
    { model: "fable", displayName: "Fable", alias: true },
    { model: "opus", displayName: "Opus", alias: true },
    { model: "sonnet", displayName: "Sonnet", alias: true },
    { model: "claude-opus-4-8", displayName: "claude-opus-4-8", alias: false },
  ];
  assert.equal(defaultClaudeModel(models), "");
  assert.equal(defaultClaudeModel(models, "sonnet"), "sonnet");
  assert.equal(defaultClaudeModel([{ model: "detected", isDefault: true }]), "detected");
  assert.equal(claudeModelDisplayLabel(models[0]), "Fable");
  assert.equal(claudeModelDisplayLabel(models[3]), "Opus 4.8");
  assert.deepEqual(groupedClaudeModels(models).primary.map((model) => model.model), ["fable", "opus", "sonnet"]);
  assert.deepEqual(groupedClaudeModels(models).more.map((model) => model.model), ["claude-opus-4-8"]);
  assert.deepEqual(groupedClaudeModels([...models, { model: "claude-fable-5", displayName: "claude-fable-5", alias: false }]).more.map((model) => model.model), ["claude-opus-4-8", "claude-fable-5"]);
});

test("model labels follow runtime metadata without pinning alias versions", () => {
  assert.equal(claudeModelDisplayLabel({ model: "opus", alias: true, displayName: "Opus 6" }), "Opus 6");
  assert.equal(claudeModelDisplayLabel({ model: "opus", alias: true }), "opus");
  assert.equal(claudeModelDisplayLabel({ model: "claude-opus-6", displayName: "Provider model" }), "Provider model");
});

test("models absent from the catalog retain the configured identity", () => {
  assert.equal(selectedCodexModel(configuration, "custom-model").model, "custom-model");
  assert.equal(selectedCodexModel({ ...configuration, model: "configured-model" }).model, "configured-model");
  assert.equal(selectedCodexModel(undefined, "saved-model").model, "saved-model");
  assert.equal(selectedCodexModel({ models: configuration.models }).model, "gpt-sol");
  assert.equal(selectedCodexModel(undefined), null);
});

test("Pi model lists do not imply a selected default model", () => {
  const pi = {
    models: [
      { model: "provider/first", displayName: "First" },
      { model: "provider/selected", displayName: "Selected" },
    ],
  };
  assert.equal(selectedCodexModel(pi), null);
  assert.equal(selectedCodexModel(pi, "provider/selected").displayName, "Selected");
  assert.equal(selectedCodexModel(pi, "provider/custom").model, "provider/custom");
});

test("Claude third-party defaults and mapped aliases use discovered metadata", () => {
  const models = [
    { model: "opus", displayName: "Opus · provider/reasoner", alias: true },
    { model: "provider/reasoner", displayName: "Gateway Reasoner", alias: false },
  ];
  assert.equal(defaultClaudeModel(models, "provider/reasoner"), "provider/reasoner");
  assert.equal(claudeModelDisplayLabel(models[0]), "Opus · provider/reasoner");
  assert.equal(claudeModelDisplayLabel(models[1]), "Gateway Reasoner");
  assert.deepEqual(groupedClaudeModels(models).more, [models[1]]);
});
