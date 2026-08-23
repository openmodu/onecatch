export function runtimeResumeCommand(runtime, sessionID) {
  // This command can be written directly into the embedded shell. Runtime IDs
  // are normally UUID-like; reject shell metacharacters instead of attempting
  // incompatible quoting rules for POSIX shells and PowerShell.
  if (!sessionID || !/^[A-Za-z0-9._:/-]+$/.test(sessionID)) return "";
  if (runtime === "codex") return `codex resume ${sessionID}`;
  if (runtime === "claude") return `claude --resume ${sessionID}`;
  if (runtime === "modu") return `modu_code --resume ${sessionID}`;
  if (runtime === "pi") return `pi --session ${sessionID}`;
  if (runtime === "grok") return `grok --resume ${sessionID}`;
  return "";
}

export function runtimeSessionEntries(detail, translate = (_key, options) => options.defaultValue) {
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
      idLabel: step.runtime === "codex" ? translate("inspector.threadID", { defaultValue: "thread id" }) : translate("inspector.sessionID", { defaultValue: "session id" }),
      command: runtimeResumeCommand(step.runtime, sessionID),
    }];
  });
}
