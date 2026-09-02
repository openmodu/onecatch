function validComposerSubmit(event, composing) {
  const nativeEvent = event.nativeEvent || event;
  return event.key === "Enter"
    && !composing
    && !nativeEvent.isComposing
    && nativeEvent.keyCode !== 229;
}

export function composerSubmitMode(event, { running = false, defaultRunningMode = "queue" } = {}, composing = false) {
  if (!validComposerSubmit(event, composing)) return "";
  const invertRunningMode = running && event.shiftKey && (event.metaKey || event.ctrlKey);
  if (event.shiftKey && !invertRunningMode) return "";
  if (!running) return "queue";
  if (!invertRunningMode) return defaultRunningMode;
  return defaultRunningMode === "queue" ? "insert" : "queue";
}

export function shouldSubmitComposer(event, composing = false) {
  return Boolean(composerSubmitMode(event, {}, composing));
}
