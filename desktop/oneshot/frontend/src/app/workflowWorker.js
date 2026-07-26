export function isRemoteWorker(workerId) {
  return Boolean(workerId && workerId !== "local");
}

export function assignWorkflowWorker(step, workerId) {
  const next = { ...step, workerId: workerId || "local" };
  if (isRemoteWorker(next.workerId)) next.sandbox = "read-only";
  return next;
}
