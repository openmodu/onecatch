import { desktopPlatform } from "./platform.js";

export const TERMINAL_SELECTION_DRAG_THRESHOLD = 6;
export const TERMINAL_SELECTION_DURATION_THRESHOLD = 180;

export function isTerminalCopyShortcut(event, navigatorValue = globalThis.navigator) {
  if (event?.type !== "keydown" || event.key?.toLowerCase() !== "c" || event.altKey) return false;
  if (desktopPlatform(navigatorValue) === "macos") return event.metaKey && !event.ctrlKey;
  return event.ctrlKey && event.shiftKey && !event.metaKey;
}

export function isAccidentalTerminalSelectionDrag(start, end, distanceThreshold = TERMINAL_SELECTION_DRAG_THRESHOLD, durationThreshold = TERMINAL_SELECTION_DURATION_THRESHOLD) {
  if (!start || !end) return false;
  const deltaX = end.clientX - start.clientX;
  const deltaY = end.clientY - start.clientY;
  const distanceIsTiny = deltaX * deltaX + deltaY * deltaY < distanceThreshold * distanceThreshold;
  const durationIsTiny = Number.isFinite(start.timeStamp) && Number.isFinite(end.timeStamp) && end.timeStamp - start.timeStamp < durationThreshold;
  return distanceIsTiny || durationIsTiny;
}

export function hasLostTerminalSelectionMouseUp(selectionActive, event) {
  return Boolean(selectionActive && event && (event.buttons & 1) === 0);
}

export function shouldStartTerminalSelection(start, event) {
  return Boolean(start && event && (event.buttons & 1) !== 0 && !isAccidentalTerminalSelectionDrag(start, event));
}
