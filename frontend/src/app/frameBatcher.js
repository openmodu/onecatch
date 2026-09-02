// Coalesce any number of runtime events into one React update per display
// frame. Keeping the scheduler injectable makes the behavior deterministic in
// tests and gives older webviews a timer fallback at the call site.
export function createFrameBatcher(flush, scheduleFrame, cancelFrame) {
  let frameHandle = null;

  return {
    schedule() {
      if (frameHandle !== null) return;
      frameHandle = scheduleFrame(() => {
        frameHandle = null;
        flush();
      });
    },

    cancel() {
      if (frameHandle === null) return;
      cancelFrame(frameHandle);
      frameHandle = null;
    },
  };
}
