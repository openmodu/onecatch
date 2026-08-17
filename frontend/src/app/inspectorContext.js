// The inspector can run inside the workbench or in its own native window. When
// it is detached the two documents share nothing, so the main window publishes
// the small slice of state the inspector actually renders and the detached
// window subscribes to it.
//
// Deliberately excluded: `runtimeEvents`. That is the unbounded transcript, it
// grows by the frame while an agent streams, and no inspector tab reads it.
// Keeping it out is what makes one event per real state change affordable.
export const INSPECTOR_CONTEXT_EVENT = "onecatch:inspector-context";
export const INSPECTOR_REQUEST_EVENT = "onecatch:inspector-request";
export const INSPECTOR_ACTION_EVENT = "onecatch:inspector-action";
export const INSPECTOR_WINDOW_EVENT = "onecatch:inspector-window";

// Read-only remote steps may inspect worker-local state, so follow that clone.
// Writable remote steps synchronize their patch back and clean the worker;
// those must default to local so the Git panel shows the delivered changes.
export function activeWorkerID(runDetail) {
  const steps = runDetail?.workflow?.steps || [];
  const stepRuns = runDetail?.stepRuns || [];
  const stepID = stepRuns[stepRuns.length - 1]?.stepId || runDetail?.run?.currentStepId;
  const step = steps.find((item) => item.id === stepID);
  return step?.sandbox === "read-only" && step.workerId && step.workerId !== "local" ? step.workerId : "";
}

export function sortQueuedTasks(tasks = []) {
  return tasks
    .filter((task) => task.status === "queued")
    .sort((left, right) => new Date(left.queue?.enqueuedAt || left.createdAt) - new Date(right.queue?.enqueuedAt || right.createdAt));
}

// 1-based, matching what the inspector prints; 0 means "not in the queue".
export function queuedTaskPosition(tasks, taskID) {
  if (!taskID) return 0;
  return sortQueuedTasks(tasks).findIndex((task) => task.id === taskID) + 1;
}

function inspectorDetail(runDetail) {
  if (!runDetail) return null;
  return {
    run: runDetail.run || null,
    task: runDetail.task || null,
    workflow: runDetail.workflow || null,
    stepRuns: runDetail.stepRuns || [],
    events: runDetail.events || [],
    instructions: runDetail.instructions || [],
    active: Boolean(runDetail.active),
    lastError: runDetail.lastError || "",
  };
}

export function buildInspectorContext({ mode, workspaceID, runDetail, tasks = [], selectedQueuedTaskID = "", draft = false } = {}) {
  const queuedTask = selectedQueuedTaskID ? tasks.find((task) => task.id === selectedQueuedTaskID) || null : null;
  return {
    mode: mode || "",
    workspaceID: workspaceID || "",
    runWorkerID: draft ? "" : activeWorkerID(runDetail),
    draft: Boolean(draft),
    detail: draft ? null : inspectorDetail(runDetail),
    queuedTask: draft ? null : queuedTask,
    queuePosition: draft ? 0 : queuedTaskPosition(tasks, selectedQueuedTaskID),
  };
}

// Cheap fingerprint of everything the inspector tabs read, so the 80ms
// transcript cadence does not push an identical context across the window
// boundary dozens of times a second.
export function inspectorContextSignature(context) {
  const detail = context?.detail;
  const stepRuns = detail?.stepRuns || [];
  const events = detail?.events || [];
  return [
    context?.mode || "",
    context?.workspaceID || "",
    context?.runWorkerID || "",
    context?.draft ? 1 : 0,
    context?.queuedTask?.id || "",
    context?.queuedTask?.status || "",
    context?.queuePosition || 0,
    detail?.run?.id || "",
    detail?.run?.status || "",
    detail?.run?.revision || 0,
    detail?.run?.pauseReason || "",
    detail?.run?.currentStepId || "",
    detail?.run?.updatedAt || "",
    detail?.active ? 1 : 0,
    detail?.task?.id || "",
    detail?.task?.title || "",
    detail?.workflow?.id || "",
    stepRuns.length,
    stepRuns.map((step) => [step.stepId, step.attempt || 0, step.status, step.startedAt || "", step.finishedAt || "", step.inputTokens || 0, step.outputTokens || 0].join(":")).join(","),
    events.length,
    events[events.length - 1]?.seq || 0,
    (detail?.instructions || []).map((instruction) => `${instruction.id}:${instruction.status}`).join(","),
    JSON.stringify(detail?.lastError || ""),
  ].join("|");
}
