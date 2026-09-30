export const SIDEBAR_NAVIGATION_ITEMS = ["templates", "skills", "usage", "workflows"];
const DEFAULT_ITEMS = ["templates", "skills"];
const STORAGE_KEY = "onecatch.sidebar.navigation";

export function normalizeSidebarItems(value) {
  if (!Array.isArray(value)) return [...DEFAULT_ITEMS];
  return SIDEBAR_NAVIGATION_ITEMS.filter((item) => value.includes(item));
}

export function readSidebarItems(storage) {
  try {
    const saved = storage?.getItem(STORAGE_KEY);
    return saved === null || saved === undefined ? [...DEFAULT_ITEMS] : normalizeSidebarItems(JSON.parse(saved));
  } catch { return [...DEFAULT_ITEMS]; }
}

export function writeSidebarItems(storage, items) {
  try { storage?.setItem(STORAGE_KEY, JSON.stringify(normalizeSidebarItems(items))); } catch { /* In-memory preferences still work when storage is unavailable. */ }
}
