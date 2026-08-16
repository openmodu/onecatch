const TERMINAL_STEP_STATUSES = new Set(["succeeded", "failed", "interrupted"]);

function timestamp(value) {
  const parsed = new Date(value || 0).getTime();
  return Number.isFinite(parsed) ? parsed : 0;
}

export function latestStepRunsByStep(stepRuns = []) {
  const latest = new Map();
  for (const [index, stepRun] of stepRuns.entries()) {
    const previous = latest.get(stepRun.stepId);
    const candidateRank = [Number(stepRun.attempt) || 0, timestamp(stepRun.finishedAt || stepRun.startedAt), index];
    const previousRank = previous?.rank || [-1, -1, -1];
    const isLater = candidateRank[0] > previousRank[0]
      || (candidateRank[0] === previousRank[0] && candidateRank[1] > previousRank[1])
      || (candidateRank[0] === previousRank[0] && candidateRank[1] === previousRank[1] && candidateRank[2] > previousRank[2]);
    if (isLater) latest.set(stepRun.stepId, { stepRun, rank: candidateRank });
  }
  return new Map([...latest].map(([stepID, entry]) => [stepID, entry.stepRun]));
}

export function currentWorkflowStepID(run = {}, workflow = {}, stepRuns = []) {
  if (workflow.mode !== "dag" && (run.status === "ready" || run.status === "running")) return run.currentStepId || "";

  const preferredNodeStatuses = run.status === "running" ? new Set(["running"])
    : run.status === "paused" ? new Set(["paused", "running"])
      : run.status === "failed" ? new Set(["failed", "paused", "running"])
        : new Set();
  const nodeCandidates = workflow.mode === "dag" ? Object.entries(run.nodes || {})
    .filter(([, node]) => preferredNodeStatuses.has(node?.status))
    .sort((left, right) => timestamp(right[1]?.finishedAt || right[1]?.startedAt) - timestamp(left[1]?.finishedAt || left[1]?.startedAt)) : [];
  if (nodeCandidates.length > 0) return nodeCandidates[0][0];

  const latestAttempt = stepRuns
    .map((stepRun, index) => ({ stepRun, index, at: timestamp(stepRun.finishedAt || stepRun.startedAt) }))
    .sort((left, right) => right.at - left.at || right.index - left.index)[0]?.stepRun;
  return latestAttempt?.stepId || run.currentStepId || "";
}

// Run, node and attempt snapshots can be persisted a few milliseconds apart.
// Reconcile them conservatively so a terminal attempt never appears to still
// be running and a paused/cancelled run does not retain a stale active node.
export function effectiveWorkflowStepStatus(run = {}, stepID, node, stepRun) {
  const current = stepID === run.currentStepId;
  const nodeStatus = node?.status || "";
  const attemptStatus = stepRun?.status || "";

  if (current && run.status === "paused" && run.pauseReason === "interrupted") return "interrupted";
  if (TERMINAL_STEP_STATUSES.has(attemptStatus)
    && (!nodeStatus || nodeStatus === "pending" || nodeStatus === "running")) {
    return attemptStatus;
  }
  if (!current) return nodeStatus || attemptStatus || "pending";

  if (run.status === "paused") {
    return nodeStatus === "running" || nodeStatus === "pending" || !nodeStatus ? "paused" : nodeStatus;
  }
  if (run.status === "cancelled") return "cancelled";
  if (run.status === "failed" && (!nodeStatus || nodeStatus === "pending" || nodeStatus === "running")) return "failed";
  if (run.status === "running" && (!nodeStatus || nodeStatus === "pending") && attemptStatus !== "succeeded") return "running";
  if (run.status === "completed" && !nodeStatus && !attemptStatus) return "completed";
  return nodeStatus || attemptStatus || "pending";
}

export function summarizeAgentDuration(stepRuns = [], now = Date.now()) {
  return stepRuns.reduce((total, stepRun) => {
    const persisted = Math.max(0, Number(stepRun.durationMs) || 0);
    if (persisted > 0 || stepRun.status !== "running") return total + persisted;
    const startedAt = timestamp(stepRun.startedAt);
    return total + (startedAt > 0 ? Math.max(0, now - startedAt) : 0);
  }, 0);
}
