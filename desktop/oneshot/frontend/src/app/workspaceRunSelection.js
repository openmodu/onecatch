export function runPairsContain(runPairs, runID) {
  if (!runID) return false;
  return (runPairs || []).some(([, taskRuns]) => (taskRuns || []).some((run) => run.id === runID));
}
