export const INSPECTOR_LAYOUT_STORAGE_KEY = "onecatch.layout.task-workbench.v1";

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

// The status panel starts closed until the user explicitly opens it. Compact
// viewport transitions are handled separately so they can close both side
// panels once without preventing a deliberate reopen at the minimum width.
export function resolveInspectorCollapsed(preference) {
  return typeof preference === "boolean" ? preference : true;
}
