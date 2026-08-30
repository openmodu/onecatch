export const TRAY_ACTION_EVENT = "onecatch:tray-navigate";

export function normalizeTrayNavigation(value) {
  if (value?.action === "new") return { action: "new" };
  if (value?.action !== "open") return null;
  const workspaceId = String(value.workspaceId || "").trim();
  const taskId = String(value.taskId || "").trim();
  const runId = String(value.runId || "").trim();
  if (!workspaceId || !taskId) return null;
  return { action: "open", workspaceId, taskId, runId };
}

export function newestTaskRun(runs = []) {
  return [...runs].sort((left, right) => {
    const byActivity = String(right?.updatedAt || "").localeCompare(String(left?.updatedAt || ""));
    if (byActivity) return byActivity;
    return String(left?.id || "").localeCompare(String(right?.id || ""));
  })[0] || null;
}
