const assistantKinds = new Set(["message", "result", "error"]);
const hiddenKinds = new Set(["started", "usage"]);
const jsonEscapes = { '"': '"', "\\": "\\", "/": "/", b: "\b", f: "\f", n: "\n", r: "\r", t: "\t" };
const defaultTranslate = (_key, options) => options.defaultValue;

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

export function readableToolTitle(value, translate = defaultTranslate) {
  const firstLine = String(value || "").trim().split("\n")[0];
  if (!firstLine) return translate("timeline.noContent", { defaultValue: "无内容" });
  const shell = firstLine.match(/(?:^|\s)(?:\S*\/)?(?:zsh|bash|sh)\s+-lc\s+/);
  let command = shell ? firstLine.slice((shell.index || 0) + shell[0].length).trim() : firstLine;
  for (let index = 0; index < 2; index += 1) {
    if ((command.startsWith('"') && command.endsWith('"')) || (command.startsWith("'") && command.endsWith("'"))) command = command.slice(1, -1).trim();
  }
  const sedTarget = command.match(/\bsed\s+-n\s+['"][^'"]+['"]\s+['"]([^'"]+)['"]/);
  if (sedTarget) return translate("timeline.readCommand", { name: sedTarget[1].split("/").pop(), defaultValue: `读取 ${sedTarget[1].split("/").pop()}` });
  if (/^(?:npm|pnpm|yarn|bun)\b/.test(command)) return translate("timeline.runCommand", { command, defaultValue: `运行 ${command}` });
  if (/^git\s+(?:diff|status|show|log)\b/.test(command)) return translate("timeline.inspectCommand", { command, defaultValue: `检查 ${command}` });
  if (/^(?:rg|grep)\b/.test(command)) return translate("timeline.searchCommand", { command, defaultValue: `搜索 ${command}` });
  if (/^find\b/.test(command)) return translate("timeline.findCommand", { command, defaultValue: `查找 ${command}` });
  return command || firstLine;
}

function commandToolName(value) {
  const first = String(value || "").trim().split(/\s+/, 1)[0] || "";
  return first.split(/[\\/]/).pop() || first;
}

// Codex tool calls can arrive as `bash {"command":"…","description":"…"}`.
// Keep that transport envelope out of the UI: the summary uses the human
// description while the expanded body shows the decoded command itself.
export function readableToolInvocation(value, translate = defaultTranslate) {
  const source = String(value || "").trim();
  const envelope = source.match(/^([^\s{}]+)\s+(\{[\s\S]*\})$/);
  if (envelope) {
    try {
      const payload = JSON.parse(envelope[2]);
      if (payload && typeof payload === "object" && !Array.isArray(payload)) {
        const preferred = ["command", "cmd", "path", "file_path", "query", "url", "prompt"]
          .map((key) => payload[key])
          .find((candidate) => typeof candidate === "string" && candidate.trim());
        const text = preferred?.trim() || JSON.stringify(payload, null, 2);
        const description = typeof payload.description === "string" ? payload.description.trim() : "";
        return {
          toolName: commandToolName(envelope[1]),
          title: description || readableToolTitle(text, translate),
          text,
        };
      }
    } catch {
      // Providers may stream an incomplete JSON envelope. Until it closes,
      // preserve the original text instead of presenting a misleading parse.
    }
  }
  return { toolName: commandToolName(source), title: readableToolTitle(source, translate), text: source };
}

function roundItems(events, fallbackText, fallbackError, translate) {
  const items = [];
  const seenMessages = new Set();
  const permissions = new Map();
  let lastTool = null;
  // Tool calls in a parallel batch all start before any of them return, so the
  // "most recent tool_use" is not the call a result belongs to. When the
  // runtime tags events with a call id (streamId) we pair by that; otherwise we
  // fall back to the most recent tool_use for runtimes that emit no id.
  const pendingToolsById = new Map();
  for (const event of [...events].sort((a, b) => a.seq - b.seq)) {
    if (hiddenKinds.has(event.kind)) continue;
    if (event.kind === "permission_request" && event.permission?.id) {
      const permission = { type: "permission", id: `permission-${event.permission.id}`, request: event.permission, decision: "", at: event.at };
      permissions.set(event.permission.id, permission);
      items.push(permission);
      lastTool = null;
      continue;
    }
    if (event.kind === "permission_resolved" && event.permission?.id) {
      const permission = permissions.get(event.permission.id);
      if (permission) permission.decision = event.permissionDecision || event.text || "deny";
      continue;
    }
    // A tool's outcome is its own, not its step's: a step can fail on a token
    // limit while every command in it succeeded. `failed` therefore only ever
    // comes from the runtime's per-result error flag, and `settled` records
    // whether the call ever came back at all. This is checked before the
    // empty-text guard below because a result carries those two signals even
    // when the command printed nothing — a silent success or a launch failure.
    if (event.kind === "tool_result") {
      const target = (event.streamId && pendingToolsById.get(event.streamId)) || lastTool;
      if (target) {
        if (event.text) target.details.push({ kind: event.kind, text: event.text, at: event.at });
        target.settled = !event.streaming;
        if (event.failed) target.failed = true;
        if (!event.streaming && event.streamId) pendingToolsById.delete(event.streamId);
        continue;
      }
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
    const invocation = event.kind === "tool_use" ? readableToolInvocation(event.text, translate) : { toolName: "", title: readableToolTitle(event.text, translate), text: event.text };
    const tool = { type: "tool", id: event.streamId || `${event.kind}-${event.seq}`, kind: event.kind, toolName: invocation.toolName, title: invocation.title, text: invocation.text, streaming: Boolean(event.streaming), at: event.at, details: [], failed: Boolean(event.failed), settled: event.kind !== "tool_use" };
    items.push(tool);
    if (event.kind === "tool_use") {
      lastTool = tool;
      if (event.streamId) pendingToolsById.set(event.streamId, tool);
    } else {
      lastTool = null;
    }
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

// A round only changes while its step is live. This fingerprint captures every
// field roundItems reads, so a finished round is rebuilt zero times: the cache
// hands back the exact same object reference on every poll/stream frame, which
// lets the memoized round component skip it entirely and keeps streaming from
// re-reconciling the whole (potentially hundreds of rows) transcript.
function roundFingerprint(stepRun, events) {
  const parts = [stepRun.status, stepRun.attempt, stepRun.signal, events.length, stepRun.startedAt, stepRun.finishedAt];
  for (const event of events) {
    if (event.streaming || event.streamId) parts.push(`${event.seq}:${event.streamId || ""}:${event.revision || 0}:${event.text ? event.text.length : 0}:${event.streaming ? 1 : 0}`);
  }
  return parts.join("|");
}

let cachedTranslate = defaultTranslate;
const roundCache = new Map();

export function buildRunConversation(detail, translate = defaultTranslate) {
  // Cached rounds carry already-translated labels, so a language switch must
  // start from a clean slate.
  if (translate !== cachedTranslate) {
    cachedTranslate = translate;
    roundCache.clear();
  }
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
  const liveStepRuns = new Set();
  for (const [index, stepRun] of (detail?.stepRuns || []).entries()) {
    const step = workflowSteps.get(stepRun.stepId) || {};
    const events = eventsByStepRun.get(stepRun.id) || [];
    const fingerprint = `${index}|${step.name || stepRun.stepId}|${step.runtime || "agent"}|${roundFingerprint(stepRun, events)}`;
    liveStepRuns.add(stepRun.id);
    const cached = roundCache.get(stepRun.id);
    if (cached && cached.fingerprint === fingerprint) {
      timeline.push(cached.round);
      continue;
    }
    const round = {
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
      items: roundItems(events, stepRun.content, stepRun.error, translate),
      at: stepRun.startedAt || "",
      sortRank: 1,
    };
    roundCache.set(stepRun.id, { fingerprint, round });
    timeline.push(round);
  }
  // Drop cache entries for runs the caller has navigated away from so the map
  // does not grow without bound across every run the session opens.
  for (const key of roundCache.keys()) {
    if (!liveStepRuns.has(key)) roundCache.delete(key);
  }
  return timeline.sort((a, b) => {
    const timeDifference = new Date(a.at || 0).getTime() - new Date(b.at || 0).getTime();
    return timeDifference || a.sortRank - b.sortRank;
  });
}
