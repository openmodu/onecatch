export function resumeControlState(detail, busy, resumeRequested = false) {
  const paused = detail?.run?.status === "paused";
  const restoring = paused && (busy === "resume" || resumeRequested);
  const waitingForStop = paused && Boolean(detail?.active) && !restoring;

  return {
    disabled: restoring || waitingForStop,
    label: restoring ? "恢复中" : waitingForStop ? "等待停止" : "恢复运行",
  };
}

export function markResumeAccepted(detail, runID) {
  if (!detail || detail.run?.id !== runID) return detail;
  return { ...detail, active: true };
}
