// A start request counts as active before the host returns its run ID.
export function mobileComposerAction({ prompt, running, pending, sharedRuns, busy, ready }) {
  const active = Boolean(running || pending);
  const queueable = active && sharedRuns;
  if (active && (!prompt.trim() || !queueable)) {
    return { mode: "running", disabled: false, interruptible: Boolean(running) && !busy };
  }
  return {
    mode: queueable ? "queue" : "send",
    disabled: !prompt.trim() || Boolean(busy && busy !== "run") || (!queueable && (!ready || Boolean(pending))),
  };
}

// Serialize host queue writes so rapid sends retain their order, including
// sends made before StartRun returns. A failed request must not stall the next.
export function enqueueMobileFollowUp(previous, target, text, send) {
  return previous.catch(() => {}).then(async () => {
    const run = await target;
    if (!run) throw new Error("上一条消息未能启动，请重新发送");
    return { runID: run.id, queued: await send(run.id, text) };
  });
}
