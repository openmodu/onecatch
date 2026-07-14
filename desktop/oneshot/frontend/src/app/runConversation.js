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

export function readableToolTitle(value) {
  const firstLine = String(value || "").trim().split("\n")[0];
  if (!firstLine) return "无内容";
  const shell = firstLine.match(/(?:^|\s)(?:\S*\/)?(?:zsh|bash|sh)\s+-lc\s+/);
  let command = shell ? firstLine.slice((shell.index || 0) + shell[0].length).trim() : firstLine;
  for (let index = 0; index < 2; index += 1) {
    if ((command.startsWith('"') && command.endsWith('"')) || (command.startsWith("'") && command.endsWith("'"))) command = command.slice(1, -1).trim();
  }
  const sedTarget = command.match(/\bsed\s+-n\s+['"][^'"]+['"]\s+['"]([^'"]+)['"]/);
  if (sedTarget) return `读取 ${sedTarget[1].split("/").pop()}`;
  if (/^(?:npm|pnpm|yarn|bun)\b/.test(command)) return `运行 ${command}`;
  if (/^git\s+(?:diff|status|show|log)\b/.test(command)) return `检查 ${command}`;
  if (/^(?:rg|grep)\b/.test(command)) return `搜索 ${command}`;
  if (/^find\b/.test(command)) return `查找 ${command}`;
  return command || firstLine;
}

function roundItems(events, fallbackText, fallbackError) {
  const items = [];
  const seenMessages = new Set();
  let lastTool = null;
  for (const event of [...events].sort((a, b) => a.seq - b.seq)) {
    if (!event.text || hiddenKinds.has(event.kind)) continue;
    if (assistantKinds.has(event.kind)) {
      const text = readableAgentMessage(event.text);
      if (!text || seenMessages.has(text)) continue;
      seenMessages.add(text);
      items.push({ type: "message", tone: event.kind === "error" ? "error" : "agent", text, at: event.at });
      lastTool = null;
      continue;
    }
    if (event.kind === "tool_result" && lastTool) {
      lastTool.details.push({ kind: event.kind, text: event.text, at: event.at });
      continue;
    }
    const tool = { type: "tool", kind: event.kind, title: readableToolTitle(event.text), text: event.text, at: event.at, details: [] };
    items.push(tool);
    lastTool = event.kind === "tool_use" ? tool : null;
  }
  if (!items.some((item) => item.type === "message")) {
    const text = readableAgentMessage(fallbackText || fallbackError);
    if (text) items.push({ type: "message", tone: fallbackError ? "error" : "agent", text });
  }
  return items;
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

function appliedInstructions(instructions) {
  return (instructions || []).flatMap((instruction, index) => {
    const text = String(instruction.content || "").trim();
    if (instruction.status !== "applied" || !text) return [];
    return [{ type: "user", id: `instruction-${instruction.id || index}`, text, at: instruction.appliedAt || instruction.createdAt || "" }];
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
  if (taskText) timeline.push({ type: "user", id: "task", text: taskText, at: detail.task.createdAt || detail.run?.startedAt || detail.task.updatedAt || detail.run?.updatedAt || "", sortRank: 0 });
  for (const instruction of resumedInstructions(detail?.events)) timeline.push({ ...instruction, sortRank: 0 });
  for (const instruction of appliedInstructions(detail?.instructions)) timeline.push({ ...instruction, sortRank: 0 });
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
