export const FILE_TREE_DEFAULT_RATIO = 1 / 3;
export const FILE_TREE_MIN_WIDTH = 138;
export const FILE_EDITOR_MIN_WIDTH = 220;
export const FILE_TREE_RESIZER_WIDTH = 7;
export const FILE_TREE_KEYBOARD_STEP = 20;
export const FILE_TREE_RATIO_STORAGE_KEY = "onecatch.file-inspector.tree-ratio";

export function clampFileTreeWidth(width, containerWidth) {
  const availableWidth = Number(containerWidth);
  const maximumWidth = Number.isFinite(availableWidth)
    ? Math.max(FILE_TREE_MIN_WIDTH, availableWidth - FILE_EDITOR_MIN_WIDTH - FILE_TREE_RESIZER_WIDTH)
    : Number.POSITIVE_INFINITY;
  const requestedWidth = Number(width);
  const nextWidth = Number.isFinite(requestedWidth) ? requestedWidth : FILE_TREE_MIN_WIDTH;
  return Math.min(maximumWidth, Math.max(FILE_TREE_MIN_WIDTH, nextWidth));
}

export function readFileTreeRatio(storage) {
  try {
    const target = storage ?? globalThis.localStorage;
    const value = Number(target?.getItem(FILE_TREE_RATIO_STORAGE_KEY));
    return Number.isFinite(value) && value > 0 && value < 1 ? value : FILE_TREE_DEFAULT_RATIO;
  } catch {
    return FILE_TREE_DEFAULT_RATIO;
  }
}

export function writeFileTreeRatio(ratio, storage) {
  try {
    const target = storage ?? globalThis.localStorage;
    target?.setItem(FILE_TREE_RATIO_STORAGE_KEY, String(ratio));
  } catch {
    // Resizing still works when storage is unavailable.
  }
}
