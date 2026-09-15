function timestamp(value) {
  const parsed = Date.parse(value || "");
  return Number.isFinite(parsed) ? Math.max(0, parsed) : 0;
}

export function activityTime(item = {}) {
  let latest = Math.max(...[item.lastActiveAt, item.updatedAt, item.finishedAt, item.completedAt,
    item.startedAt, item.createdAt, item.lastOpenedAt].map(timestamp));
  for (const event of item.events || []) latest = Math.max(latest, timestamp(event.at));
  for (const instruction of item.queued || []) latest = Math.max(latest, timestamp(instruction.createdAt));
  return latest;
}

export function compareActivity(left, right) {
  return activityTime(right) - activityTime(left) || String(left.id || "").localeCompare(String(right.id || ""));
}

export function withWorkspaceActivity(workspaces, activities = []) {
  const latest = new Map();
  for (const item of activities) {
    const id = item.workspaceId || item.task?.workspaceId;
    latest.set(id, Math.max(latest.get(id) || 0, activityTime(item)));
  }
  return workspaces.map((workspace) => {
    const time = Math.max(activityTime(workspace), latest.get(workspace.id) || 0);
    return time ? { ...workspace, lastActiveAt: new Date(time).toISOString() } : workspace;
  });
}
