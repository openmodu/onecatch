export const SIDEBAR_TASK_PREVIEW_LIMIT = 3;

function searchText(value) {
  return String(value || "").toLocaleLowerCase();
}

function matchesQuery(values, query) {
  const needle = searchText(query).trim();
  return !needle || searchText(values.join("\n")).includes(needle);
}

export function buildSidebarTaskEntries(tasks = [], runs = [], { query = "", status = "" } = {}) {
  const queued = tasks
    .filter((task) => task.status === "queued")
    .sort((left, right) => new Date(left.queue?.enqueuedAt || left.createdAt) - new Date(right.queue?.enqueuedAt || right.createdAt));
  const queuedEntries = (!status || status === "queued" ? queued : [])
    .filter((task) => matchesQuery([task.id, task.title, task.prompt], query))
    .map((task) => ({ kind: "queued", key: `task:${task.id}`, item: task, queuePosition: queued.findIndex((candidate) => candidate.id === task.id) + 1 }));
  const runEntries = (status === "queued" ? [] : runs)
    .filter((run) => (!status || run.status === status) && matchesQuery([run.id, run.task?.title, ...Object.values(run.sessions || {})], query))
    .map((run) => ({ kind: "run", key: `run:${run.id}`, item: run }));
  return [...queuedEntries, ...runEntries];
}

export function visibleSidebarTaskEntries(entries = [], expanded = false, limit = SIDEBAR_TASK_PREVIEW_LIMIT) {
  return expanded ? entries : entries.slice(0, limit);
}
