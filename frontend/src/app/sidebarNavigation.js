export const SIDEBAR_TASK_PREVIEW_LIMIT = 3;

function searchText(value) {
  return String(value || "").toLocaleLowerCase();
}

function matchesQuery(values, query) {
  const needle = searchText(query).trim();
  return !needle || searchText(values.join("\n")).includes(needle);
}

function entryTask(entry) {
  return entry?.kind === "run" ? entry.item?.task : entry?.item;
}

export function buildSidebarTaskEntries(tasks = [], runs = [], { query = "", status = "" } = {}) {
  const pinnedTaskIDs = new Set(tasks.filter((task) => task.pinned).map((task) => task.id));
  const queued = tasks
    .filter((task) => task.status === "queued")
    .sort((left, right) => new Date(left.queue?.enqueuedAt || left.createdAt) - new Date(right.queue?.enqueuedAt || right.createdAt));
  const queuedEntries = (!status || status === "queued" ? queued : [])
    .filter((task) => matchesQuery([task.id, task.title, task.prompt], query))
    .map((task) => ({ kind: "queued", key: `task:${task.id}`, item: task, queuePosition: queued.findIndex((candidate) => candidate.id === task.id) + 1 }));
  const seenPinnedRunTasks = new Set();
  const runEntries = (status === "queued" ? [] : runs)
    .filter((run) => (!status || run.status === status) && matchesQuery([run.id, run.task?.title, ...Object.values(run.sessions || {})], query))
    .map((run) => {
      const taskID = run.task?.id;
      return pinnedTaskIDs.has(taskID) ? { ...run, task: { ...run.task, pinned: true } } : run;
    })
    .filter((run) => {
      const taskID = run.task?.id;
      if (!pinnedTaskIDs.has(taskID)) return true;
      if (seenPinnedRunTasks.has(taskID)) return false;
      seenPinnedRunTasks.add(taskID);
      return true;
    })
    .map((run) => ({ kind: "run", key: `run:${run.id}`, item: run }));
  const representedTaskIDs = new Set([
    ...queuedEntries.map((entry) => entry.item.id),
    ...runEntries.map((entry) => entry.item.task?.id).filter(Boolean),
  ]);
  const pinnedEntries = tasks
    .filter((task) => task.pinned && !representedTaskIDs.has(task.id))
    .filter((task) => (!status || task.status === status) && matchesQuery([task.id, task.title, task.prompt], query))
    .map((task) => ({ kind: "pinned", key: `pinned:${task.id}`, item: task }));
  return [...pinnedEntries, ...queuedEntries, ...runEntries].sort((left, right) => {
    const leftTask = entryTask(left);
    const rightTask = entryTask(right);
    if (Boolean(leftTask?.pinned) !== Boolean(rightTask?.pinned)) return leftTask?.pinned ? -1 : 1;
    return 0;
  });
}

export function visibleSidebarTaskEntries(entries = [], expanded = false, limit = SIDEBAR_TASK_PREVIEW_LIMIT) {
  if (expanded) return entries;
  const pinned = entries.filter((entry) => entryTask(entry)?.pinned);
  const regular = entries.filter((entry) => !entryTask(entry)?.pinned);
  return [...pinned, ...regular.slice(0, Math.max(0, limit - pinned.length))];
}
