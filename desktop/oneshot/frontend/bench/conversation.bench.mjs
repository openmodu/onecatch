// Measures the claim behind the round-cache fix: during streaming, the timeline
// was rebuilt and re-sorted from every runtime event on each ~80ms flush.
// buildRunConversation is called from a useMemo keyed on a signature that
// includes the streaming text length, so the key changes every flush.
import { buildRunConversation } from "../src/app/runConversation.js";

function makeDetail(rounds, eventsPerRound) {
  const stepRuns = [];
  const runtimeEvents = [];
  let seq = 0;
  for (let r = 0; r < rounds; r += 1) {
    const stepRunId = `step_${r}`;
    stepRuns.push({
      id: stepRunId,
      stepId: "s",
      status: r === rounds - 1 ? "running" : "succeeded",
      attempt: 1,
      startedAt: new Date(1700000000000 + r * 60000).toISOString(),
      finishedAt: new Date(1700000000000 + r * 60000 + 30000).toISOString(),
    });
    for (let e = 0; e < eventsPerRound; e += 1) {
      seq += 1;
      const streaming = r === rounds - 1 && e === eventsPerRound - 1;
      runtimeEvents.push({
        stepRunId,
        seq,
        kind: e % 3 === 0 ? "message" : "tool_use",
        streamId: `s${r}_${e}`,
        revision: 1,
        streaming,
        text: `event ${r}/${e} ${"x".repeat(120)}`,
        at: new Date(1700000000000 + r * 60000 + e * 100).toISOString(),
      });
    }
  }
  return {
    run: { id: "run_1", status: "running" },
    task: { id: "t", prompt: "do the thing", createdAt: new Date(1700000000000).toISOString() },
    workflow: { steps: [{ id: "s", name: "step", runtime: "codex" }] },
    stepRuns,
    runtimeEvents,
    events: [],
    instructions: [],
    active: true,
  };
}

// Each flush appends to the streaming event's text and rebuilds.
function simulateFlushes(detail, flushes) {
  const streamingEvent = detail.runtimeEvents[detail.runtimeEvents.length - 1];
  const start = process.hrtime.bigint();
  for (let i = 0; i < flushes; i += 1) {
    streamingEvent.text += "more text ";
    buildRunConversation(detail);
  }
  return Number(process.hrtime.bigint() - start) / 1e6;
}

const flushes = 200;
const shapes = [
  [5, 20],
  [15, 40],
  [30, 60],
  [50, 100],
];

console.log(`buildRunConversation over ${flushes} streaming flushes (80ms cadence)\n`);
console.log("rounds x events |  events | total rebuild | per flush");
console.log("----------------|---------|---------------|----------");
for (const [rounds, per] of shapes) {
  const detail = makeDetail(rounds, per);
  simulateFlushes(makeDetail(rounds, per), 20); // warm up
  const ms = simulateFlushes(detail, flushes);
  console.log(
    `${String(rounds).padStart(6)} x ${String(per).padEnd(7)} | ${String(rounds * per).padStart(7)} | ${ms.toFixed(0).padStart(12)}ms | ${(ms / flushes).toFixed(2).padStart(7)}ms`,
  );
}
