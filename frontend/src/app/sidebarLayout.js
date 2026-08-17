export const SIDEBAR_WIDTH_STORAGE_KEY = "onecatch.layout.sidebar.v1";
export const SIDEBAR_DEFAULT_WIDTH = 216;
export const SIDEBAR_MIN_WIDTH = 180;
export const SIDEBAR_MAX_WIDTH = 480;
export const SIDEBAR_MIN_CONTENT_WIDTH = 560;
export const SIDEBAR_KEYBOARD_STEP = 12;

export function sidebarWidthBounds(viewportWidth) {
  const available = Number.isFinite(viewportWidth) ? viewportWidth - SIDEBAR_MIN_CONTENT_WIDTH : SIDEBAR_MAX_WIDTH;
  return {
    min: SIDEBAR_MIN_WIDTH,
    max: Math.max(SIDEBAR_MIN_WIDTH, Math.min(SIDEBAR_MAX_WIDTH, available)),
  };
}

export function clampSidebarWidth(width, viewportWidth) {
  const bounds = sidebarWidthBounds(viewportWidth);
  const numeric = Number(width);
  if (!Number.isFinite(numeric)) return Math.min(SIDEBAR_DEFAULT_WIDTH, bounds.max);
  return Math.round(Math.min(bounds.max, Math.max(bounds.min, numeric)));
}

export function parseSidebarWidth(value, viewportWidth) {
  if (!value) return null;
  try {
    const parsed = JSON.parse(value);
    if (!Number.isFinite(parsed?.width)) return null;
    return clampSidebarWidth(parsed.width, viewportWidth);
  } catch {
    return null;
  }
}

export function readSidebarWidth(storage, viewportWidth) {
  try {
    return parseSidebarWidth(storage?.getItem(SIDEBAR_WIDTH_STORAGE_KEY), viewportWidth);
  } catch {
    return null;
  }
}

export function writeSidebarWidth(storage, width) {
  try {
    storage?.setItem(SIDEBAR_WIDTH_STORAGE_KEY, JSON.stringify({ width }));
    return true;
  } catch {
    return false;
  }
}
