export const COMPACT_LAYOUT_MAX_WIDTH = 1100;
export const COMPACT_LAYOUT_QUERY = `(max-width: ${COMPACT_LAYOUT_MAX_WIDTH}px)`;

// Crossing into the compact layout closes auxiliary panels once. Components
// still own their live state, so users can deliberately reopen either panel
// without fighting a permanently forced media-query override.
export function collapsePanelAtCompact(currentCollapsed, compactViewport) {
  return compactViewport ? true : currentCollapsed;
}
