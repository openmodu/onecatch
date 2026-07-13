export const COMPACT_WORKSPACE_LIMIT = 8;
export const RUN_ROW_HEIGHT = 64;
export const RUN_GROUP_HEIGHT = 30;

function timestamp(value) {
  const parsed = new Date(value || 0).getTime();
  return Number.isNaN(parsed) ? 0 : parsed;
}

export function sortWorkspaces(items = []) {
  return [...items].sort((a, b) => {
    if (Boolean(a.pinned) !== Boolean(b.pinned)) return a.pinned ? -1 : 1;
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

export function mergeRunItems(current = [], incoming = [], reset = false) {
  const source = reset ? incoming : [...current, ...incoming];
  const seen = new Set();
  return source.filter((item) => {
    const id = item?.id || item?.run?.id;
    if (!id || seen.has(id)) return false;
    seen.add(id);
    return true;
  });
}

function dayKey(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "unknown";
  return `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;
}

export function buildRunRows(items = [], now = new Date()) {
  const today = dayKey(now);
  const yesterdayDate = new Date(now);
  yesterdayDate.setDate(yesterdayDate.getDate() - 1);
  const yesterday = dayKey(yesterdayDate);
  const groups = [];
  let lastGroup = "";
  for (const item of items) {
    const key = dayKey(item.updatedAt);
    const group = key === today ? "today" : key === yesterday ? "yesterday" : "earlier";
    const label = group === "today" ? "今天" : group === "yesterday" ? "昨天" : "更早";
    if (lastGroup !== group) {
      groups.push({ type: "group", key: `group-${group}`, group, label, height: RUN_GROUP_HEIGHT });
      lastGroup = group;
    }
    groups.push({ type: "run", key: item.id, item, height: RUN_ROW_HEIGHT });
  }
  return groups;
}

export function virtualRunWindow(rows = [], scrollTop = 0, viewportHeight = 0, overscan = 160) {
  const start = Math.max(0, scrollTop - overscan);
  const end = scrollTop + viewportHeight + overscan;
  let top = 0;
  const visible = [];
  for (const row of rows) {
    const bottom = top + row.height;
    if (bottom >= start && top <= end) visible.push({ ...row, top });
    top = bottom;
  }
  return { visible, totalHeight: top };
}
