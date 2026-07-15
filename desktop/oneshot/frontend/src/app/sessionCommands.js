export function runtimeResumeCommand(runtime, sessionID) {
  if (!sessionID) return "";
  if (runtime === "codex") return `codex resume ${sessionID}`;
  if (runtime === "claude") return `claude --resume ${sessionID}`;
  if (runtime === "modu") return `modu_code --resume ${sessionID}`;
  return "";
}

export function runtimeSessionEntries(detail) {
  const workflowSteps = detail?.workflow?.steps || [];
  const runSessions = detail?.run?.sessions || {};
  const latestByStep = new Map();
  for (const stepRun of detail?.stepRuns || []) {
    const sessionID = stepRun.sessionIdAfter || stepRun.sessionIdBefore;
    if (sessionID) latestByStep.set(stepRun.stepId, sessionID);
  }
  return workflowSteps.flatMap((step) => {
    const sessionID = runSessions[step.id] || latestByStep.get(step.id);
    if (!sessionID) return [];
    return [{
      stepID: step.id,
      stepName: step.name || step.id,
      runtime: step.runtime,
      sessionID,
      idLabel: step.runtime === "codex" ? "thread id" : "session id",
      command: runtimeResumeCommand(step.runtime, sessionID),
    }];
  });
}
