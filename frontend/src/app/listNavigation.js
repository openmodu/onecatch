export const COMPACT_WORKSPACE_LIMIT = 8;

function timestamp(value) {
  const parsed = new Date(value || 0).getTime();
  return Number.isNaN(parsed) ? 0 : parsed;
}

export function sortWorkspaces(items = []) {
  return [...items].sort((a, b) => {
    const recent = timestamp(b.lastOpenedAt) - timestamp(a.lastOpenedAt);
    return recent || String(a.name || "").localeCompare(String(b.name || ""));
  });
}

export function workspaceResults(items = [], { query = "", expanded = false, limit = COMPACT_WORKSPACE_LIMIT } = {}) {
  // Keep the order established when the sidebar was loaded. Opening a
  // workspace updates lastOpenedAt, but navigation must not make its row jump.
  const sorted = [...items];
  const needle = query.trim().toLocaleLowerCase();
  if (needle) return sorted.filter((item) => `${item.name || ""}\n${item.path || ""}`.toLocaleLowerCase().includes(needle));
  if (expanded || sorted.length <= limit) return sorted;
  return sorted.slice(0, limit);
}

export function workspaceSections(items = [], { selectedID = "", query = "", expanded = false, limit = COMPACT_WORKSPACE_LIMIT } = {}) {
  const sorted = sortWorkspaces(items);
  const needle = query.trim().toLocaleLowerCase();
  const filtered = needle ? sorted.filter((item) => `${item.name || ""}\n${item.path || ""}`.toLocaleLowerCase().includes(needle)) : sorted;
  const pinned = filtered.filter((item) => item.pinned);
  const projects = filtered.filter((item) => !item.pinned);
  if (needle || expanded || projects.length <= limit) return { pinned, projects };
  const compact = projects.slice(0, limit);
  if (selectedID && !compact.some((item) => item.id === selectedID)) {
    const selected = projects.find((item) => item.id === selectedID);
    if (selected) compact[compact.length - 1] = selected;
  }
  return { pinned, projects: compact };
}

export function preserveEqualValue(current, next) {
  if (Object.is(current, next)) return current;
  try {
    return JSON.stringify(current) === JSON.stringify(next) ? current : next;
  } catch {
    return next;
  }
}

// Reuse the previous object when a reload returned the same thing, decided from
// the few fields that actually move rather than by serialising both sides.
//
// preserveEqualValue answers the same question by stringifying, which is fine
// for a small row but not for a run detail: that object carries the whole
// transcript, so the comparison alone walked hundreds of kilobytes on every
// window focus, to conclude nothing had changed.
export function preserveByFingerprint(current, next, fingerprint) {
  if (Object.is(current, next)) return current;
  if (!current || !next) return next;
  try {
    return fingerprint(current) === fingerprint(next) ? current : next;
  } catch {
    return next;
  }
}

// What a row in the run list can change by. Runs are revisioned, so the
// revision plus the timestamps covers every mutation the list renders.
export function runItemFingerprint(item = {}) {
  return [
    item.id, item.revision, item.status, item.updatedAt, item.finishedAt,
    item.task?.title, item.task?.status, item.task?.updatedAt, item.task?.pinned,
  ].join("|");
}

// What a run detail can change by. Step runs and the transcript are summarised
// by their tail rather than their contents: a transcript only ever grows, and a
// live stream announces itself by bumping the last entry's revision and length.
export function runDetailFingerprint(detail = {}) {
  const steps = detail.stepRuns || [];
  const events = detail.runtimeEvents || [];
  const last = events[events.length - 1];
  return [
    detail.run?.id, detail.run?.revision, detail.run?.status,
    detail.active ? 1 : 0, detail.lastError || "",
    steps.length, steps[steps.length - 1]?.status, steps[steps.length - 1]?.finishedAt,
    events.length, detail.runtimeEventsTotal,
    last?.seq, last?.revision, last?.text?.length, last?.streaming ? 1 : 0,
    (detail.instructions || []).length, (detail.events || []).length,
  ].join("|");
}

const runID = (item) => item?.id || item?.run?.id;

// Dedupes by run id while reusing the previous object for any row whose content
// did not change, so memoized rows can skip re-rendering during background
// polls. Returns the current array unchanged when nothing at all moved.
export function mergeRunItems(current = [], incoming = [], reset = false) {
  const previous = new Map(current.map((item) => [runID(item), item]));
  const source = reset ? incoming : [...current, ...incoming];
  const seen = new Set();
  const merged = [];
  for (const item of source) {
    const id = runID(item);
    if (!id || seen.has(id)) continue;
    seen.add(id);
    const existing = previous.get(id);
    merged.push(existing ? preserveByFingerprint(existing, item, runItemFingerprint) : item);
  }
  if (merged.length === current.length && merged.every((item, index) => item === current[index])) return current;
  return merged;
}
