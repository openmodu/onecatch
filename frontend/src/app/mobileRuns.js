import { shortenPath } from "./format.js";

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
  return sortMobileRuns(next);
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

// Plumbing the phone has no use for. Connecting to a worker and counting
// tokens are facts about the machinery, not about the answer, and a row for
// each one is what turned a short reply into a page of scaffolding.
// `result` carries the run's final message, which the transcript already shows
// as the last reply, so leaving it in printed the answer twice.
const TRANSCRIPT_NOISE = new Set(["started", "usage", "permission_resolved", "result"]);

export function foldMobileEvents(items = []) {
  const events = [];
  const streamIndexes = new Map();
  const resolvedPermissions = new Set(items.filter((event) => event.kind === "permission_resolved").map((event) => event.permission?.id));
  for (const event of items) {
    if (event.kind === "permission_request" && resolvedPermissions.has(event.permission?.id)) continue;
    if (TRANSCRIPT_NOISE.has(event?.kind)) continue;
    // A thought the harness never filled in is a heading with nothing under it.
    if (event?.kind === "reasoning" && !String(event.text || "").trim()) continue;
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
  return foldToolResults(events);
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
    return { ...conversation, title: runs[0]?.title || mobileRunTitle(runs[0]?.prompt), runs };
  });
}

// projectActivity condenses a project's sessions into the one line the list
// shows under its name: how much is there, and how long ago it moved.
export function projectActivity(sessions = []) {
  let latestAt = "";
  let running = false;
  for (const session of sessions) {
    if (session.status === "running") running = true;
    const at = String(session.startedAt || "");
    if (at > latestAt) latestAt = at;
  }
  return { count: sessions.length, latestAt, running };
}

// Harnesses hand a command to the user's login shell, so almost every tool
// call arrives wrapped in `/long/path/to/zsh -lc "…"`. The wrapper is identical
// every time; what the agent actually ran is inside the quotes.
export function unwrapShellCommand(value) {
  const text = String(value || "").trim();
  const match = /^\S*\/?(?:sh|bash|zsh|fish|dash)\s+-[a-zA-Z]*c\s+(['"])([\s\S]*)\1$/.exec(text);
  return match ? match[2].trim() : text;
}

// foldToolResults puts a call and what it returned on one row. Split across two
// rows, four operations filled the screen with eight lines and the reader had to
// pair them up by eye.
function foldToolResults(events) {
  const folded = [];
  // A call and its result carry the same tool-call id. Pairing by that rather
  // than by adjacency matters because the agent often narrates between the two,
  // and a result stranded from its call reads as a row about nothing.
  const callIndexes = new Map();
  for (const event of events) {
    if (event?.kind === "tool_use") {
      if (event.streamId) callIndexes.set(event.streamId, folded.length);
      folded.push(event);
      continue;
    }
    if (event?.kind === "tool_result") {
      const index = event.streamId && callIndexes.has(event.streamId)
        ? callIndexes.get(event.streamId)
        : (folded[folded.length - 1]?.kind === "tool_use" && folded[folded.length - 1].result === undefined ? folded.length - 1 : -1);
      if (index >= 0) {
        const call = folded[index];
        folded[index] = {
          ...call,
          result: String(event.text || ""),
          finishedAt: event.at || call.finishedAt,
          failed: Boolean(call.failed || event.failed),
        };
        continue;
      }
    }
    folded.push(event);
  }
  return folded;
}

// Harnesses do not agree on what a tool call looks like: Codex hands over a
// shell command, Modu hands over the tool's JSON arguments. Raw JSON in a
// one-line row is unreadable, so pull out the field that says what the call
// actually does and leave the rest to the expansion.
const ARGUMENT_KEYS = ["command", "cmd", "script", "pattern", "query", "prompt", "question", "url", "file_path", "filePath", "path", "filename", "name"];

export function describeToolArguments(value) {
  const text = String(value || "").trim();
  if (!text.startsWith("{")) return unwrapShellCommand(text);
  let parsed;
  try {
    parsed = JSON.parse(text);
  } catch {
    return text;
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return text;
  const key = ARGUMENT_KEYS.find((name) => typeof parsed[name] === "string" && parsed[name].trim());
  if (!key) {
    // An unfamiliar shape has no line worth writing; the row keeps its tool
    // name and the whole payload stays one tap away.
    return "";
  }
  const primary = unwrapShellCommand(parsed[key]);
  // A pattern on its own does not say where it was looked for.
  const where = key === "pattern" || key === "query" ? String(parsed.path || parsed.file_path || "").trim() : "";
  return where ? `${primary} · ${shortenPath(where)}` : primary;
}

// commandParts splits a shell command into the program that ran and what it was
// asked to do, so a row can lead with `rg` rather than with the path to zsh.
export function commandParts(value) {
  const command = unwrapShellCommand(value).split("\n")[0].trim();
  const match = /^([\w.\-/]+)(?:\s+([\s\S]*))?$/.exec(command);
  if (!match) return { program: "", args: command };
  const program = match[1].includes("/") ? match[1].slice(match[1].lastIndexOf("/") + 1) : match[1];
  return { program, args: (match[2] || "").trim() };
}

// The row is elided by width in CSS, which is the only thing that knows how
// much fits. These two only keep a runaway blob out of the DOM and decide
// whether opening the row would show more than it already does — a phone row
// holds roughly this much, and erring low just offers an expander nobody needs.
const MAX_ROW_TEXT = 400;
const ROW_FITS = 38;

// mobileEventSummary turns an event into the single line the transcript shows
// for it, saying what the agent actually did rather than only its category.
export function mobileEventSummary(event, label) {
  const text = String(event?.text || "").trim();
  const failed = Boolean(event?.failed) || event?.kind === "error";
  const result = String(event?.result || "").trim();
  if (event?.kind === "tool_use") {
    const { program, args } = commandParts(text);
    return { label: program || label, detail: describeToolArguments(args).slice(0, MAX_ROW_TEXT), body: text, result, expandable: Boolean(text || result), failed };
  }
  const [first = "", ...rest] = unwrapShellCommand(text).split("\n");
  return {
    label,
    detail: first.slice(0, MAX_ROW_TEXT),
    body: text,
    result: "",
    // Only worth an expander when opening it would show something the row does
    // not already say.
    expandable: Boolean(text) && (rest.length > 0 || first.length > ROW_FITS),
    failed,
  };
}

// conversationUsage adds up what a conversation has spent. The numbers ride on
// the usage events' structured fields rather than their text, which is why the
// transcript's old "usage" row opened onto nothing.
export function conversationUsage(runs = []) {
  let input = 0;
  let output = 0;
  let cached = 0;
  let context = null;
  for (const run of runs) {
    // A run still in flight has no result yet, so fall back to its latest
    // reading — the count should climb while the agent works.
    let usage = run?.result?.usage || null;
    for (const event of run?.events || []) {
      if (event?.kind !== "usage") continue;
      if (!run?.result?.usage && event.usage) usage = event.usage;
      if (event.context?.tokens) context = event.context;
    }
    if (!usage) continue;
    input += usage.inputTokens || 0;
    output += usage.outputTokens || 0;
    cached += usage.cachedInputTokens || 0;
  }
  return { input, output, cached, total: input + output, context };
}
