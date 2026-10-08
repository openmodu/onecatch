// A model request belongs to one harness selection. Cancelling on selection
// changes prevents late successes and failures from replacing the current menu.
export function loadTaskRuntimeConfiguration(harness, inspect, publish, formatError) {
  let active = true;
  publish({ harness, loading: true, data: null, error: "" });
  Promise.resolve().then(() => inspect(harness)).then(
    (data) => { if (active) publish({ harness, loading: false, data, error: "" }); },
    (error) => { if (active) publish({ harness, loading: false, data: null, error: formatError(error) }); },
  );
  return () => { active = false; };
}

// Re-read when returning from the CLI or account settings. Each refresh cancels
// publication from the previous request, including failures arriving late.
export function watchRuntimeConfiguration(harness, inspect, publish, formatError, target = window) {
  let cancel;
  const refresh = () => {
    cancel?.();
    cancel = loadTaskRuntimeConfiguration(harness, inspect, publish, formatError);
  };
  refresh();
  target.addEventListener("focus", refresh);
  return () => {
    cancel?.();
    target.removeEventListener("focus", refresh);
  };
}
