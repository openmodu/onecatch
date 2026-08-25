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

// Detachment has no viewport-derived default: a fresh installation always
// starts docked, so a missing or malformed record simply means "not detached".
export function parseInspectorDetached(value) {
  return parseLayout(value).inspectorDetached === true;
}

export function readInspectorDetached(storage) {
  try {
    return parseInspectorDetached(storage?.getItem(INSPECTOR_LAYOUT_STORAGE_KEY));
  } catch {
    return false;
  }
}

// The record is merged rather than replaced on write. Only detachment is
// stored today, but a stale inspectorCollapsed key from an older build may
// still be in the record and merging leaves it undisturbed rather than
// rewriting storage on every float.
function writeLayout(storage, patch) {
  try {
    const current = parseLayout(storage?.getItem(INSPECTOR_LAYOUT_STORAGE_KEY));
    storage?.setItem(INSPECTOR_LAYOUT_STORAGE_KEY, JSON.stringify({ ...current, ...patch }));
    return true;
  } catch {
    return false;
  }
}

export function writeInspectorDetached(storage, inspectorDetached) {
  return writeLayout(storage, { inspectorDetached });
}

// The status panel starts closed until the user explicitly opens it. Compact
// viewport transitions are handled separately so they can close both side
// panels once without preventing a deliberate reopen at the minimum width.
// The inspector starts collapsed and is not restored from storage, so a
// non-boolean here means "nothing has been chosen yet this session".
export function resolveInspectorCollapsed(preference) {
  return typeof preference === "boolean" ? preference : true;
}
