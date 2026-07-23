// Collapses every task and run in the active workspace into one glanceable
// standby signal. Run statuses are already pushed live (runstate), so this is a
// pure derivation the lock screen renders and the completion watcher observes —
// no extra polling, no backend call.
//
// Only work that is still in flight counts. A completed/failed/cancelled run is
// history: surfacing it forever would keep the standby permanently "busy". The
// three live states are:
//   - running  : an agent is actively working
//   - queued   : a task waiting its turn in the workspace FIFO
//   - paused   : a run stopped for you (interrupted, or waiting on input)

export const LOCK_PHASE = { working: "working", waiting: "waiting", done: "done" };

function runTitle(run) {
  return run.task?.title || run.title || run.id || "";
}

export function buildLockSignal(tasks = [], runs = []) {
  let running = 0;
  let paused = 0;
  const activeRuns = [];
  const waitingRuns = [];
  for (const run of runs) {
    if (run.status === "running") {
      running += 1;
      activeRuns.push({ id: run.id, title: runTitle(run), status: "running" });
    } else if (run.status === "paused") {
      paused += 1;
      waitingRuns.push({ id: run.id, title: runTitle(run), status: "paused" });
    }
  }
  const queuedTasks = tasks.filter((task) => task.status === "queued");
  const queued = queuedTasks.length;
  for (const task of queuedTasks) activeRuns.push({ id: task.id, title: task.title || task.id, status: "queued" });

  const active = running + queued;
  const phase = paused > 0 ? LOCK_PHASE.waiting : active > 0 ? LOCK_PHASE.working : LOCK_PHASE.done;
  return {
    running,
    queued,
    paused,
    active,
    phase,
    // Runs to list on the standby screen: in-flight first, then the ones
    // blocked on you. Deliberately excludes finished work.
    items: [...activeRuns, ...waitingRuns],
  };
}

// completionEdge compares the previous and next signals and reports the single
// transition worth a system notification, or "" for none. Kept separate from
// buildLockSignal so it can be unit-tested without timers or React.
//   - "done"    : the last in-flight run finished — active fell to 0 with none
//                 left waiting. This is the "your tasks are done" ping.
//   - "waiting" : a run newly needs you (paused count rose). Just as worth
//                 surfacing while you are away from the screen.
export function completionEdge(previous, next) {
  if (!previous) return "";
  if (previous.active > 0 && next.active === 0 && next.paused === 0) return LOCK_PHASE.done;
  if (next.paused > previous.paused) return LOCK_PHASE.waiting;
  return "";
}
