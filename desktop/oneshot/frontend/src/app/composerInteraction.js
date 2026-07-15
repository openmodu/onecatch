export function preserveComposerFocus(event) {
  if (event.button !== 0) return false;
  const button = event.target?.closest?.("button:not(:disabled)");
  if (!button || !event.currentTarget?.contains?.(button)) return false;
  event.preventDefault();
  return true;
}
