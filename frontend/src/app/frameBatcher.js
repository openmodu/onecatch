// Coalesce any number of runtime events into one React update per display
// frame. Keeping the scheduler injectable makes the behavior deterministic in
// tests and gives older webviews a timer fallback at the call site.
export function createFrameBatcher(flush, scheduleFrame, cancelFrame, fallback = null) {
  let frameHandle = null;
  let timerHandle = null;
  const cancel = () => {
    if (frameHandle !== null) cancelFrame(frameHandle);
    if (timerHandle !== null) fallback.cancel(timerHandle);
    frameHandle = timerHandle = null;
  };
  const deliver = () => {
    if (frameHandle === null && timerHandle === null) return;
    cancel();
    flush();
  };

  return {
    schedule() {
      if (frameHandle !== null) return;
      frameHandle = scheduleFrame(deliver);
      if (fallback) timerHandle = fallback.schedule(deliver);
    },

    cancel,
  };
}
