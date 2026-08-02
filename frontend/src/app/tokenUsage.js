export function summarizeTokenUsage(stepRuns = []) {
  const summary = {
    inputTokens: 0,
    cachedInputTokens: 0,
    cacheCreationInputTokens: 0,
    outputTokens: 0,
    reasoningOutputTokens: 0,
  };

  for (const step of stepRuns) {
    summary.inputTokens += Number(step.inputTokens) || 0;
    summary.cachedInputTokens += Number(step.cachedInputTokens) || 0;
    summary.cacheCreationInputTokens += Number(step.cacheCreationInputTokens) || 0;
    summary.outputTokens += Number(step.outputTokens) || 0;
    summary.reasoningOutputTokens += Number(step.reasoningOutputTokens) || 0;
  }

  return summary;
}
