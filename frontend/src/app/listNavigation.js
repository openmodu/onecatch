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

export function workspaceResults(items = [], { selectedID = "", query = "", expanded = false, limit = COMPACT_WORKSPACE_LIMIT } = {}) {
  const sorted = sortWorkspaces(items);
  const needle = query.trim().toLocaleLowerCase();
  if (needle) return sorted.filter((item) => `${item.name || ""}\n${item.path || ""}`.toLocaleLowerCase().includes(needle));
  if (expanded || sorted.length <= limit) return sorted;
  const compact = sorted.slice(0, limit);
  if (selectedID && !compact.some((item) => item.id === selectedID)) {
    const selected = sorted.find((item) => item.id === selectedID);
    if (selected) compact[compact.length - 1] = selected;
  }
  return compact;
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
    merged.push(existing ? preserveEqualValue(existing, item) : item);
  }
  if (merged.length === current.length && merged.every((item, index) => item === current[index])) return current;
  return merged;
}
