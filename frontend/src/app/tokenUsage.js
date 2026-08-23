export function summarizeTokenUsage(stepRuns = []) {
  const summary = {
    inputTokens: 0,
    cachedInputTokens: 0,
    cacheCreationInputTokens: 0,
    outputTokens: 0,
    reasoningOutputTokens: 0,
  };

  for (const step of stepRuns) {
    const inputTokens = Number(step.inputTokens) || 0;
    const cachedInputTokens = Number(step.cachedInputTokens) || 0;
    const cacheCreationInputTokens = Number(step.cacheCreationInputTokens) || 0;
    const detailedCacheTokens = cachedInputTokens + cacheCreationInputTokens;

    // InputTokens normally includes both cache subsets. Older Modu runs saved
    // only fresh input, which is recognizable when the subsets exceed the
    // alleged total. Repair those records while leaving normalized runs alone.
    summary.inputTokens += detailedCacheTokens > inputTokens
      ? inputTokens + detailedCacheTokens
      : inputTokens;
    summary.cachedInputTokens += cachedInputTokens;
    summary.cacheCreationInputTokens += cacheCreationInputTokens;
    summary.outputTokens += Number(step.outputTokens) || 0;
    summary.reasoningOutputTokens += Number(step.reasoningOutputTokens) || 0;
  }

  summary.cacheHitRate = summary.inputTokens > 0
    ? (summary.cachedInputTokens / summary.inputTokens) * 100
    : 0;

  return summary;
}
