const assistantKinds = new Set(["message", "result", "error"]);
const hiddenKinds = new Set(["started", "usage"]);
const jsonEscapes = { '"': '"', "\\": "\\", "/": "/", b: "\b", f: "\f", n: "\n", r: "\r", t: "\t" };

function readJSONString(source, start) {
  let value = "";
  for (let index = start + 1; index < source.length; index += 1) {
    const character = source[index];
    if (character === '"') return { value, end: index + 1, closed: true };
    if (character !== "\\") {
      value += character;
      continue;
    }
    if (index + 1 >= source.length) return { value, end: source.length, closed: false };
    const escape = source[++index];
    if (Object.prototype.hasOwnProperty.call(jsonEscapes, escape)) {
      value += jsonEscapes[escape];
      continue;
    }
    if (escape !== "u" || index + 4 >= source.length) return { value, end: source.length, closed: false };
    const hex = source.slice(index + 1, index + 5);
    if (!/^[0-9a-f]{4}$/i.test(hex)) return { value, end: source.length, closed: false };
    let code = Number.parseInt(hex, 16);
    index += 4;
    if (code >= 0xd800 && code <= 0xdbff && source.slice(index + 1, index + 3) === "\\u" && /^[0-9a-f]{4}$/i.test(source.slice(index + 3, index + 7))) {
      const low = Number.parseInt(source.slice(index + 3, index + 7), 16);
      if (low >= 0xdc00 && low <= 0xdfff) {
        code = 0x10000 + ((code - 0xd800) << 10) + low - 0xdc00;
        index += 6;
      }
    }
    value += String.fromCodePoint(code);
  }
  return { value, end: source.length, closed: false };
}

function skipJSONValue(source, start) {
  if (source[start] === '"') return readJSONString(source, start).end;
  let depth = 0;
  for (let index = start; index < source.length; index += 1) {
    if (source[index] === '"') {
      index = readJSONString(source, index).end - 1;
      continue;
    }
    if (source[index] === "{" || source[index] === "[") depth += 1;
    if (source[index] === "}" || source[index] === "]") {
      if (depth === 0) return index;
      depth -= 1;
    }
    if (source[index] === "," && depth === 0) return index + 1;
  }
  return source.length;
}

// Workflow outcomes are JSON envelopes, but providers may split them at any
// byte boundary. Project the content string before the closing quote arrives so
// the UI never flashes the transport envelope or a ```json fence.
export function streamingOutcomeContent(value) {
  let source = String(value || "").trimStart();
  if (source.startsWith("```")) {
    const opening = source.match(/^```(?:json)?[\t ]*(?:\r?\n|$)/i);
    if (!opening) return source.length < 8 ? "" : null;
    source = source.slice(opening[0].length).trimStart();
  }
  if (!source.startsWith("{")) return null;

  let index = 1;
  while (index < source.length) {
    while (/[\s,]/.test(source[index] || "")) index += 1;
    if (index >= source.length || source[index] === "}") return "";
    if (source[index] !== '"') return "";
    const key = readJSONString(source, index);
    if (!key.closed) return "";
    index = key.end;
    while (/\s/.test(source[index] || "")) index += 1;
    if (source[index] !== ":") return "";
    index += 1;
    while (/\s/.test(source[index] || "")) index += 1;
    if (key.value === "content") {
      if (source[index] !== '"') return "";
      return readJSONString(source, index).value;
    }
    index = skipJSONValue(source, index);
  }
  return "";
}

export function readableAgentMessage(value, streaming = false) {
  const text = String(value || "").trim();
  if (!text) return "";
  const fenced = text.match(/^```(?:json)?[\t ]*\r?\n([\s\S]*?)\r?\n```$/i);
  const candidate = fenced ? fenced[1].trim() : text;
  try {
    const parsed = JSON.parse(candidate);
    if (parsed && typeof parsed === "object" && typeof parsed.content === "string") {
      return parsed.content.trim();
    }
  } catch {
    // Ordinary assistant prose is already the preferred display form.
  }
  if (streaming) {
    const projected = streamingOutcomeContent(value);
    if (projected !== null) return projected;
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
    if (hiddenKinds.has(event.kind)) continue;
    // A tool's outcome is its own, not its step's: a step can fail on a token
    // limit while every command in it succeeded. `failed` therefore only ever
    // comes from the runtime's per-result error flag, and `settled` records
    // whether the call ever came back at all. This is checked before the
    // empty-text guard below because a result carries those two signals even
    // when the command printed nothing — a silent success or a launch failure.
    if (event.kind === "tool_result" && lastTool) {
      if (event.text) lastTool.details.push({ kind: event.kind, text: event.text, at: event.at });
      lastTool.settled = !event.streaming;
      if (event.failed) lastTool.failed = true;
      continue;
    }
    if (!event.text) continue;
    if (assistantKinds.has(event.kind)) {
      const text = readableAgentMessage(event.text, event.streaming);
      if (!text || seenMessages.has(text)) continue;
      seenMessages.add(text);
      items.push({ type: "message", id: event.streamId || `${event.kind}-${event.seq}`, tone: event.kind === "error" ? "error" : "agent", text, streaming: Boolean(event.streaming), at: event.at });
      lastTool = null;
      continue;
    }
    const tool = { type: "tool", id: event.streamId || `${event.kind}-${event.seq}`, kind: event.kind, title: readableToolTitle(event.text), text: event.text, streaming: Boolean(event.streaming), at: event.at, details: [], failed: Boolean(event.failed), settled: event.kind !== "tool_use" };
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
