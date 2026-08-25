function streamKey(value) {
  return `${value.stepRunId || ""}\u0000${value.streamId || ""}`;
}

function atomicKey(value) {
  return `${value.stepRunId || ""}\u0000${value.seq || 0}\u0000${value.kind || ""}`;
}

function isStreamEvent(value) {
  return Boolean(value.streamId && (value.phase || value.streaming || Number(value.revision || 0) > 0));
}

function frameEvent(frame) {
  return {
    stepRunId: frame.stepRunId || "",
    seq: Number(frame.seq || 0),
    kind: frame.kind || "",
    streamId: frame.streamId || "",
    revision: Number(frame.revision || 0),
    streaming: Boolean(frame.phase && frame.phase !== "end"),
    text: frame.phase === "start" ? "" : String(frame.text || ""),
    failed: Boolean(frame.failed),
    ...(frame.permission ? { permission: frame.permission } : {}),
    ...(frame.permissionDecision ? { permissionDecision: frame.permissionDecision } : {}),
    at: frame.at || "",
  };
}

function applyUsageFrame(stepRuns, frame) {
  if (frame.kind !== "usage" || !frame.usage || !frame.stepRunId) return stepRuns;
  const index = stepRuns.findIndex((step) => step.id === frame.stepRunId);
  if (index < 0) return stepRuns;
  const current = stepRuns[index];
  const fields = ["inputTokens", "cachedInputTokens", "cacheCreationInputTokens", "outputTokens", "reasoningOutputTokens"];
  const next = { ...current };
  let changed = false;
  for (const field of fields) {
    const value = Number(frame.usage[field]) || 0;
    if (value > (Number(current[field]) || 0)) {
      next[field] = value;
      changed = true;
    }
  }
  // Occupancy deliberately skips the take-the-larger rule the counters above
  // use for out-of-order frames. Those only grow, so a smaller value is stale;
  // the context window empties on compaction, so a smaller value is the news.
  if (frame.context) {
    const window = Number(frame.context.window) || 0;
    const tokens = Number(frame.context.tokens) || 0;
    if (window > 0 && window !== current.contextWindow) {
      next.contextWindow = window;
      changed = true;
    }
    if (tokens > 0 && tokens !== current.contextTokens) {
      next.contextTokens = tokens;
      changed = true;
    }
  }
  if (!changed) return stepRuns;
  const result = [...stepRuns];
  result[index] = next;
  return result;
}

function preserveLiveUsage(currentSteps, incomingSteps) {
  const currentByID = new Map(currentSteps.map((step) => [step.id, step]));
  const fields = ["inputTokens", "cachedInputTokens", "cacheCreationInputTokens", "outputTokens", "reasoningOutputTokens"];
  return incomingSteps.map((step) => {
    const current = currentByID.get(step.id);
    if (!current) return step;
    let next = step;
    for (const field of fields) {
      if ((Number(current[field]) || 0) > (Number(next[field]) || 0)) {
        if (next === step) next = { ...step };
        next[field] = current[field];
      }
    }
    return next;
  });
}

// Applies best-effort Wails frames to the durable RunDetail returned by GetRun.
// Revisions make snapshot replay and duplicate transport delivery idempotent.
export function applyRuntimeFrames(detail, incoming) {
  if (!detail || !Array.isArray(incoming) || incoming.length === 0) return detail;
  const events = [...(detail.runtimeEvents || [])];
  const streamIndexes = new Map();
  const atomics = new Set();
  events.forEach((event, index) => {
    if (isStreamEvent(event)) streamIndexes.set(streamKey(event), index);
    else atomics.add(atomicKey(event));
  });

  let changed = false;
  let stepRuns = detail.stepRuns || [];
  for (const frame of incoming) {
    if (!frame || (frame.runId && frame.runId !== detail.run?.id)) continue;
    stepRuns = applyUsageFrame(stepRuns, frame);
    if (!isStreamEvent(frame)) {
      const key = atomicKey(frame);
      if (atomics.has(key)) continue;
      atomics.add(key);
      events.push(frameEvent(frame));
      changed = true;
      continue;
    }

    const key = streamKey(frame);
    const index = streamIndexes.get(key);
    const current = index === undefined ? null : events[index];
    const revision = Number(frame.revision || 0);
    const currentRevision = Number(current?.revision || 0);

    if (current && revision <= currentRevision) {
      // Normally end gets a newer revision. Accepting an equal-revision end
      // also keeps recovery compatible with older emitters.
      if (frame.phase !== "end" || !current.streaming) continue;
    }

    let next;
    if (!current) {
      next = frameEvent(frame);
    } else if (frame.phase === "delta") {
      next = {
        ...current,
        kind: frame.kind || current.kind,
        revision,
        streaming: true,
        text: `${current.text || ""}${frame.text || ""}`,
        failed: Boolean(current.failed || frame.failed),
        at: frame.at || current.at,
      };
    } else if (frame.phase === "start") {
      next = { ...frameEvent(frame), text: "", streaming: true };
    } else {
      next = {
        ...current,
        ...frameEvent(frame),
        streaming: frame.phase !== "end",
      };
    }

    if (index === undefined) {
      streamIndexes.set(key, events.length);
      events.push(next);
    } else {
      events[index] = next;
    }
    changed = true;
  }

  const usageChanged = stepRuns !== (detail.stepRuns || []);
  return changed || usageChanged ? { ...detail, runtimeEvents: events, stepRuns } : detail;
}

export function applyRuntimeFrame(detail, frame) {
  return applyRuntimeFrames(detail, [frame]);
}

// Merges a pushed run-state view (the bounded half: run + step runs +
// instructions + active) into the detail loaded by GetRun, preserving the
// unbounded transcript (runtimeEvents/events), which arrives separately as
// runstream frames. Returns the same reference when the view targets a
// different run or nothing changed, so an idle push causes no re-render.
export function applyRunState(detail, view) {
  if (!detail || !view || view.runId !== detail.run?.id) return detail;
  const nextRun = view.run || detail.run;
  const nextStepRuns = preserveLiveUsage(detail.stepRuns || [], view.stepRuns || detail.stepRuns || []);
  const nextInstructions = view.instructions || detail.instructions || [];
  const nextActive = Boolean(view.active);
  // A pushed run revision can lag a value the frontend just wrote optimistically
  // (or one an in-flight GetRun already returned). Never move the run backwards.
  if (Number(nextRun.revision || 0) < Number(detail.run?.revision || 0)) return detail;
  return { ...detail, run: nextRun, stepRuns: nextStepRuns, instructions: nextInstructions, active: nextActive };
}
