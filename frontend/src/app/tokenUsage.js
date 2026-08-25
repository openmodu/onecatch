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

// Context occupancy is not summable. Each step run reports how full the window
// was during that step; the window held one prompt at a time, so adding the
// steps up would invent an occupancy no model ever saw. The figure worth
// showing is the most recent step that reported one.
export function summarizeContextWindow(stepRuns = []) {
  let window = 0;
  let tokens = 0;
  for (const step of stepRuns) {
    const stepWindow = Number(step.contextWindow) || 0;
    const stepTokens = Number(step.contextTokens) || 0;
    // A step that reported neither leaves the last known reading standing
    // rather than blanking a gauge mid-run.
    if (stepWindow > 0) window = stepWindow;
    if (stepTokens > 0) tokens = stepTokens;
  }
  // A window with no sample, or a sample with no window, is not a ratio.
  const known = window > 0 && tokens > 0;
  return {
    window,
    tokens,
    known,
    // Clamped: a harness that compacts late can report a prompt fractionally
    // over its own advertised window, and an arc past 100% reads as a bug.
    ratio: known ? Math.min(tokens / window, 1) : 0,
  };
}
