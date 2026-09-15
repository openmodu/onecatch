const SELECTED_WORKER_KEY = "onecatch.mobile.selectedWorkerId";

export function readSelectedWorker(storage) {
  try { return (storage ?? globalThis.localStorage)?.getItem(SELECTED_WORKER_KEY) || ""; }
  catch { return ""; }
}

export function saveSelectedWorker(id, storage) {
  try {
    const target = storage ?? globalThis.localStorage;
    if (id) target?.setItem(SELECTED_WORKER_KEY, id);
    else target?.removeItem(SELECTED_WORKER_KEY);
  } catch { /* Storage failure must not prevent switching computers. */ }
}

export function resolveSelectedWorker(workers = [], selected = "") {
  // Being offline does not invalidate the user's selection. Only removal
  // from the paired-computer list should cause a fallback.
  return workers.some((worker) => worker.id === selected) ? selected : workers[0]?.id || "";
}
