const MAX_VISIBLE_EVENTS = 2000;

export function mobileRunTitle(prompt, maximum = 48) {
  const value = String(prompt || "").trim().split(/\r?\n/, 1)[0].replace(/\s+/g, " ");
  if (!value) return "未命名任务";
  return value.length > maximum ? `${value.slice(0, Math.max(1, maximum - 1)).trimEnd()}…` : value;
}

export function sortMobileRuns(items = []) {
  return [...items].sort((left, right) => String(right.startedAt || "").localeCompare(String(left.startedAt || "")));
}

export function mergeMobileRun(items = [], run) {
  if (!run?.id) return sortMobileRuns(items);
  const next = [run, ...items.filter((item) => item.id !== run.id)];
  return sortMobileRuns(next).slice(0, 100);
}

export function applyMobileRunFrame(run, frame) {
  if (!run || !frame?.runId || run.id !== frame.runId) return run;
  const next = { ...run };
  if (frame.event) next.events = [...(run.events || []), frame.event].slice(-MAX_VISIBLE_EVENTS);
  if (frame.status) next.status = frame.status;
  if (frame.result) next.result = frame.result;
  if (frame.error) next.error = frame.error;
  return next;
}

export function foldMobileEvents(items = []) {
  const events = [];
  const streamIndexes = new Map();
  for (const event of items) {
    if (!event?.streamId || !event.phase) {
      events.push(event);
      continue;
    }
    const index = streamIndexes.get(event.streamId);
    const current = index === undefined ? null : events[index];
    const streaming = event.phase !== "end";
    let next = { ...event, text: event.phase === "start" ? "" : String(event.text || ""), streaming };
    if (current) {
      if (event.phase === "delta") next = { ...current, ...event, text: `${current.text || ""}${event.text || ""}`, streaming: true };
      else if (event.phase === "start") next = { ...current, ...event, text: "", streaming: true };
      else next = { ...current, ...event, text: String(event.text || ""), streaming };
    }
    if (index === undefined) {
      streamIndexes.set(event.streamId, events.length);
      events.push(next);
    } else {
      events[index] = next;
    }
  }
  return events;
}

export function mobileConversationID(run) {
  return String(run?.conversationId || run?.id || "");
}

export function groupMobileConversations(items = []) {
  const groups = new Map();
  for (const run of sortMobileRuns(items)) {
    const id = mobileConversationID(run);
    if (!id) continue;
    const current = groups.get(id);
    if (current) {
      current.runs.push(run);
      if (run.status === "running") current.status = "running";
      continue;
    }
    groups.set(id, {
      id,
      workspaceId: run.workspaceId,
      workerId: run.workerId,
      runtime: run.runtime,
      title: mobileRunTitle(run.prompt),
      status: run.status,
      startedAt: run.startedAt,
      runs: [run],
    });
  }
  return [...groups.values()].map((conversation) => {
    const runs = [...conversation.runs].sort((left, right) => String(left.startedAt || "").localeCompare(String(right.startedAt || "")));
    return { ...conversation, title: mobileRunTitle(runs[0]?.prompt), runs };
  });
}
