export const REVIEW_INSPECTOR_MIN_WIDTH = 620;
export const REVIEW_INSPECTOR_MAX_WIDTH = 960;

export function preferredReviewInspectorWidth(workbenchWidth) {
  const width = Number(workbenchWidth);
  if (!Number.isFinite(width) || width <= 0) return REVIEW_INSPECTOR_MIN_WIDTH;

  return Math.min(
    REVIEW_INSPECTOR_MAX_WIDTH,
    Math.max(REVIEW_INSPECTOR_MIN_WIDTH, Math.round(width / 2)),
  );
}
