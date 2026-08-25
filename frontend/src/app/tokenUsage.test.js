import test from "node:test";
import assert from "node:assert/strict";
import { summarizeContextWindow, summarizeTokenUsage } from "./tokenUsage.js";

test("summarizes total and detailed token usage across steps", () => {
  assert.deepEqual(summarizeTokenUsage([
    {
      inputTokens: 1200,
      cachedInputTokens: 900,
      cacheCreationInputTokens: 40,
      outputTokens: 320,
      reasoningOutputTokens: 120,
    },
    {
      inputTokens: 300,
      cachedInputTokens: 200,
      cacheCreationInputTokens: 10,
      outputTokens: 80,
      reasoningOutputTokens: 20,
    },
  ]), {
    inputTokens: 1500,
    cachedInputTokens: 1100,
    cacheCreationInputTokens: 50,
    outputTokens: 400,
    reasoningOutputTokens: 140,
    cacheHitRate: (1100 / 1500) * 100,
  });
});

test("keeps older step data compatible when detailed fields are absent", () => {
  assert.deepEqual(summarizeTokenUsage([{ inputTokens: 42, outputTokens: 7 }]), {
    inputTokens: 42,
    cachedInputTokens: 0,
    cacheCreationInputTokens: 0,
    outputTokens: 7,
    reasoningOutputTokens: 0,
    cacheHitRate: 0,
  });
});

test("normalizes legacy Modu usage that stored only fresh input", () => {
  const summary = summarizeTokenUsage([{
    inputTokens: 36827,
    cachedInputTokens: 128512,
    outputTokens: 1163,
  }]);

  assert.equal(summary.inputTokens, 165339);
  assert.equal(summary.cachedInputTokens, 128512);
  assert.equal(summary.cacheHitRate, (128512 / 165339) * 100);
});

test("context occupancy takes the latest reading, never the sum of the steps", () => {
  // Three steps each ran against the same 200k window. Adding them would claim
  // 330k of occupancy in a context that never held more than 150k.
  const summary = summarizeContextWindow([
    { contextWindow: 200000, contextTokens: 60000 },
    { contextWindow: 200000, contextTokens: 120000 },
    { contextWindow: 200000, contextTokens: 150000 },
  ]);
  assert.equal(summary.window, 200000);
  assert.equal(summary.tokens, 150000);
  assert.equal(summary.known, true);
  assert.equal(Math.round(summary.ratio * 100), 75);
});

test("a window without a sample is not a ratio", () => {
  const summary = summarizeContextWindow([{ contextWindow: 200000, contextTokens: 0 }]);
  assert.equal(summary.known, false);
  assert.equal(summary.ratio, 0);
});

test("a prompt reported over its own window clamps instead of overdrawing", () => {
  const summary = summarizeContextWindow([{ contextWindow: 200000, contextTokens: 214000 }]);
  assert.equal(summary.ratio, 1);
});

test("steps that report no context leave the previous reading standing", () => {
  const summary = summarizeContextWindow([
    { contextWindow: 200000, contextTokens: 90000 },
    { inputTokens: 10 },
  ]);
  assert.equal(summary.window, 200000);
  assert.equal(summary.tokens, 90000);
});
