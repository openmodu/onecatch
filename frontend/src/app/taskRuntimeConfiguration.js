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
