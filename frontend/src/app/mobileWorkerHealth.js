// Health responses update worker metadata. Only a change in the IDs being
// watched should restart polling, not a fresh object from the native bridge.
export function workerHealthPollKey(workers, selectedWorkerID, pollEveryWorker) {
  if (!selectedWorkerID) return "[]";
  const ids = [selectedWorkerID, ...(pollEveryWorker ? workers.map((worker) => worker.id) : [])];
  return JSON.stringify([...new Set(ids.filter(Boolean))].sort());
}

// Manual refresh, foreground events and the timer share an in-flight request
// per computer. Different computers can still be checked independently.
export function createWorkerHealthLoader(check, publish) {
  const pending = new Map();
  return (id) => {
    if (pending.has(id)) return pending.get(id);
    const request = Promise.resolve().then(() => check(id)).then((value) => {
      publish(id, value);
      return value;
    }).finally(() => pending.delete(id));
    pending.set(id, request);
    return request;
  };
}
