import test from "node:test";
import assert from "node:assert/strict";
import { summarizeTokenUsage } from "./tokenUsage.js";

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
