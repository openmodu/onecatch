const assistantKinds = new Set(["message", "result", "error"]);
const hiddenKinds = new Set(["started", "usage"]);

export function readableAgentMessage(value) {
  const text = String(value || "").trim();
  if (!text) return "";
  try {
    const parsed = JSON.parse(text);
    if (parsed && typeof parsed === "object" && typeof parsed.content === "string") {
      return parsed.content.trim();
    }
  } catch {
    // Ordinary assistant prose is already the preferred display form.
  }
  return text;
}

function roundItems(events, fallbackText, fallbackError) {
  const messages = [];
  const activities = [];
  const seenMessages = new Set();
  for (const event of [...events].sort((a, b) => a.seq - b.seq)) {
    if (!event.text || hiddenKinds.has(event.kind)) continue;
    if (assistantKinds.has(event.kind)) {
      const text = readableAgentMessage(event.text);
      if (!text || seenMessages.has(text)) continue;
      seenMessages.add(text);
      messages.push({ type: "message", tone: event.kind === "error" ? "error" : "agent", text, at: event.at });
      continue;
    }
    activities.push({ kind: event.kind, text: event.text, at: event.at });
  }
  if (!messages.length) {
    const text = readableAgentMessage(fallbackText || fallbackError);
    if (text) messages.push({ type: "message", tone: fallbackError ? "error" : "agent", text });
  }
  return activities.length ? [...messages, { type: "activity", events: activities }] : messages;
}

function resumedInstructions(events) {
  return (events || []).flatMap((event, index) => {
    if (event.type !== "run.resumed") return [];
    try {
      const payload = JSON.parse(event.payload || "{}");
      const text = String(payload.instruction || "").trim();
      return text ? [{ type: "user", id: `resume-${event.seq || index}`, text, at: event.at }] : [];
    } catch {
      return [];
    }
  });
}

export function buildRunConversation(detail) {
  const workflowSteps = new Map((detail?.workflow?.steps || []).map((step) => [step.id, step]));
  const eventsByStepRun = new Map();
  for (const event of detail?.runtimeEvents || []) {
    const list = eventsByStepRun.get(event.stepRunId) || [];
    list.push(event);
    eventsByStepRun.set(event.stepRunId, list);
  }
  const timeline = [];
  const taskText = String(detail?.task?.prompt || "").trim();
  if (taskText) timeline.push({ type: "user", id: "task", text: taskText, at: detail.task.createdAt || detail.run?.startedAt || "", sortRank: 0 });
  for (const instruction of resumedInstructions(detail?.events)) timeline.push({ ...instruction, sortRank: 0 });
  for (const [index, stepRun] of (detail?.stepRuns || []).entries()) {
    const step = workflowSteps.get(stepRun.stepId) || {};
    timeline.push({
      type: "round",
      id: stepRun.id,
      round: index + 1,
      stepName: step.name || stepRun.stepId,
      runtime: step.runtime || "agent",
      status: stepRun.status,
      attempt: stepRun.attempt,
      signal: stepRun.signal,
      startedAt: stepRun.startedAt,
      finishedAt: stepRun.finishedAt,
      items: roundItems(eventsByStepRun.get(stepRun.id) || [], stepRun.content, stepRun.error),
      at: stepRun.startedAt || "",
      sortRank: 1,
    });
  }
  return timeline.sort((a, b) => {
    const timeDifference = new Date(a.at || 0).getTime() - new Date(b.at || 0).getTime();
    return timeDifference || a.sortRank - b.sortRank;
  });
}
