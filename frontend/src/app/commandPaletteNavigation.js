export function moveCommandPaletteIndex(current, total, key) {
  if (total <= 0) return -1;
  if (key === "Home") return 0;
  if (key === "End") return total - 1;
  if (key === "ArrowDown") return current < 0 ? 0 : (current + 1) % total;
  if (key === "ArrowUp") return current < 0 ? total - 1 : (current - 1 + total) % total;
  return current;
}

export function commandPaletteShortcutIndex(items, key) {
  const normalized = String(key || "").toLocaleLowerCase();
  if (/^[1-9]$/.test(normalized)) {
    return items.findIndex((item) => item.shortcutLabel === `⌘${normalized}`);
  }
  return items.findIndex((item) => item.shortcutKey === normalized);
}

export function commandPaletteWorkspaceResults(workspaces, query, taskCount, limit = 4) {
  const normalized = String(query || "").trim().toLocaleLowerCase();
  if (!normalized) return [];
  return workspaces
    .filter((workspace) => `${workspace.name}\n${workspace.path}\n${workspace.remoteFs?.username || ""}\n${workspace.remoteFs?.host || ""}`.toLocaleLowerCase().includes(normalized))
    .slice(0, limit)
    .map((workspace, index) => ({
      workspace,
      shortcutLabel: taskCount + index < 9 ? `⌘${taskCount + index + 1}` : "",
    }));
}
