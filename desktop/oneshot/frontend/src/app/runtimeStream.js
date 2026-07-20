function streamKey(value) {
  return `${value.stepRunId || ""}\u0000${value.streamId || ""}`;
}

function atomicKey(value) {
  return `${value.stepRunId || ""}\u0000${value.seq || 0}\u0000${value.kind || ""}`;
}

function frameEvent(frame) {
  return {
    stepRunId: frame.stepRunId || "",
    seq: Number(frame.seq || 0),
    kind: frame.kind || "",
    streamId: frame.streamId || "",
    revision: Number(frame.revision || 0),
    streaming: frame.phase !== "end",
    text: frame.phase === "start" ? "" : String(frame.text || ""),
    failed: Boolean(frame.failed),
    ...(frame.permission ? { permission: frame.permission } : {}),
    ...(frame.permissionDecision ? { permissionDecision: frame.permissionDecision } : {}),
    at: frame.at || "",
  };
}

// Applies best-effort Wails frames to the durable RunDetail returned by GetRun.
// Revisions make snapshot replay and duplicate transport delivery idempotent.
export function applyRuntimeFrames(detail, incoming) {
  if (!detail || !Array.isArray(incoming) || incoming.length === 0) return detail;
  const events = [...(detail.runtimeEvents || [])];
  const streamIndexes = new Map();
  const atomics = new Set();
  events.forEach((event, index) => {
    if (event.streamId) streamIndexes.set(streamKey(event), index);
    else atomics.add(atomicKey(event));
  });

  let changed = false;
  for (const frame of incoming) {
    if (!frame || (frame.runId && frame.runId !== detail.run?.id)) continue;
    if (!frame.streamId) {
      const key = atomicKey(frame);
      if (atomics.has(key)) continue;
      atomics.add(key);
      events.push(frameEvent({ ...frame, phase: "end" }));
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

  return changed ? { ...detail, runtimeEvents: events } : detail;
}

export function applyRuntimeFrame(detail, frame) {
  return applyRuntimeFrames(detail, [frame]);
}
