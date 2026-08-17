export const INSPECTOR_LAYOUT_STORAGE_KEY = "onecatch.layout.task-workbench.v1";
export const INSPECTOR_COMPACT_QUERY = "(max-width: 1100px)";

function parseLayout(value) {
  if (!value) return {};
  try {
    const parsed = JSON.parse(value);
    return parsed && typeof parsed === "object" ? parsed : {};
  } catch {
    return {};
  }
}

export function parseInspectorPreference(value) {
  const collapsed = parseLayout(value).inspectorCollapsed;
  return typeof collapsed === "boolean" ? collapsed : null;
}

// Detachment has no viewport-derived default: a fresh installation always
// starts docked, so a missing or malformed record simply means "not detached".
export function parseInspectorDetached(value) {
  return parseLayout(value).inspectorDetached === true;
}

export function readInspectorPreference(storage) {
  try {
    return parseInspectorPreference(storage?.getItem(INSPECTOR_LAYOUT_STORAGE_KEY));
  } catch {
    return null;
  }
}

export function readInspectorDetached(storage) {
  try {
    return parseInspectorDetached(storage?.getItem(INSPECTOR_LAYOUT_STORAGE_KEY));
  } catch {
    return false;
  }
}

// Both flags share one record, so every writer merges instead of replacing:
// floating the inspector must not quietly discard the collapse preference the
// user will need again once it is docked.
function writeLayout(storage, patch) {
  try {
    const current = parseLayout(storage?.getItem(INSPECTOR_LAYOUT_STORAGE_KEY));
    storage?.setItem(INSPECTOR_LAYOUT_STORAGE_KEY, JSON.stringify({ ...current, ...patch }));
    return true;
  } catch {
    return false;
  }
}

export function writeInspectorPreference(storage, inspectorCollapsed) {
  return writeLayout(storage, { inspectorCollapsed });
}

export function writeInspectorDetached(storage, inspectorDetached) {
  return writeLayout(storage, { inspectorDetached });
}

// A saved user choice always wins. The compact breakpoint is only the default
// for installations where the user has not made an explicit choice yet.
export function resolveInspectorCollapsed(preference, compactViewport) {
  return typeof preference === "boolean" ? preference : compactViewport;
}
