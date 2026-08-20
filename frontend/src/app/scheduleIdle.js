// Defer work until the window has nothing better to do, with a timeout so it
// still runs on a busy machine. Used for the reference data a window renders
// without: loading it eagerly puts it in front of the first paint, which is the
// one moment the user is actually waiting.
//
// Returns a cancel function, so an unmounting effect can drop the work entirely.
export function scheduleIdle(callback, timeout = 600) {
  if (typeof window === "undefined") return () => {};
  if (typeof window.requestIdleCallback === "function") {
    const id = window.requestIdleCallback(callback, { timeout });
    return () => window.cancelIdleCallback(id);
  }
  const id = window.setTimeout(callback, 0);
  return () => window.clearTimeout(id);
}
