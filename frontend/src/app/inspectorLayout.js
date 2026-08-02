export const INSPECTOR_LAYOUT_STORAGE_KEY = "oneshot.layout.task-workbench.v1";
export const INSPECTOR_COMPACT_QUERY = "(max-width: 1100px)";

export function parseInspectorPreference(value) {
  if (!value) return null;
  try {
    const parsed = JSON.parse(value);
    return typeof parsed?.inspectorCollapsed === "boolean" ? parsed.inspectorCollapsed : null;
  } catch {
    return null;
  }
}

export function readInspectorPreference(storage) {
  try {
    return parseInspectorPreference(storage?.getItem(INSPECTOR_LAYOUT_STORAGE_KEY));
  } catch {
    return null;
  }
}

export function writeInspectorPreference(storage, inspectorCollapsed) {
  try {
    storage?.setItem(INSPECTOR_LAYOUT_STORAGE_KEY, JSON.stringify({ inspectorCollapsed }));
    return true;
  } catch {
    return false;
  }
}

// A saved user choice always wins. The compact breakpoint is only the default
// for installations where the user has not made an explicit choice yet.
export function resolveInspectorCollapsed(preference, compactViewport) {
  return typeof preference === "boolean" ? preference : compactViewport;
}
