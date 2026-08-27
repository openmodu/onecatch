export const NEW_TASK_TEXTAREA_MIN_HEIGHT = 56;
export const WORKBENCH_TEXTAREA_MIN_HEIGHT = 76;

export function autosizeComposerTextarea(element, minHeight) {
  if (!element) return;
  const maxHeight = minHeight * 3;
  element.style.height = "auto";
  const contentHeight = element.scrollHeight;
  element.style.height = `${Math.min(Math.max(contentHeight, minHeight), maxHeight)}px`;
  element.style.overflowY = contentHeight > maxHeight ? "auto" : "hidden";
}
