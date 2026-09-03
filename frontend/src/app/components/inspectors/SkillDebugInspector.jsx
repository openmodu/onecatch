import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Events } from "@wailsio/runtime";
import { Brain, ChevronDown, ChevronRight, FileDiff, History, MessageSquare, Play, Square, Terminal, Trash2, TriangleAlert, Wrench } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Textarea } from "@/components/ui/textarea";
import { SkillBinding } from "../../../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import MarkdownContent from "../MarkdownContent.jsx";
import { StatusBadge } from "../../../ui/primitives.jsx";
import { createFrameBatcher } from "../../frameBatcher.js";
import { errorMessage, formatDateTime, formatDuration, formatTokens } from "../../format.js";
import { applyDebugFrames, currentSkillSelection, SKILL_DEBUG_EVENT, SKILL_SELECTED_EVENT, subscribeSkillWorkspace } from "../../skillWorkspace.js";

// A debug run returns the same event stream a task does — reasoning, messages,
// tool calls, their output — so it deserves the same treatment: prose rendered
// as prose, machine output kept as monospace, and the run's shape visible at a
// glance instead of one undifferentiated block of text.
const EVENT_VISUALS = {
  reasoning: { icon: Brain, labelKey: "timeline.thought", prose: true, quiet: true },
  message: { icon: MessageSquare, labelKey: "timeline.agentMessage", prose: true },
  tool_use: { icon: Wrench, labelKey: "timeline.toolUse" },
  tool_result: { icon: Terminal, labelKey: "timeline.result", quiet: true },
  file_change: { icon: FileDiff, labelKey: "timeline.fileChange" },
  error: { icon: TriangleAlert, labelKey: "skill.event.error", failed: true },
};

// Long tool output is the main reason a debug transcript becomes unreadable.
// Anything past this collapses behind a disclosure that names its own size.
const COLLAPSE_LINES = 8;

// Demo mode has no Modu behind it, so the streaming path is reproduced here
// rather than stubbed out: without it the browser preview would exercise a
// code path the desktop app never takes.
const DEMO_DEBUG_SCRIPT = [
  { kind: "reasoning", text: "The prompt names the skill directly, so its body is already in context." },
  { kind: "tool_use", text: "read_file ~/.onecatch/skills/{{name}}/SKILL.md" },
  { kind: "tool_result", text: "---\nname: {{name}}\n---\n\n# {{name}}\n\n(284 bytes)" },
  { kind: "message", text: "Loaded the skill explicitly and produced this preview response.\n\n- Checked the frontmatter\n- Followed the workflow in `SKILL.md`" },
];

function streamDemoDebug(name, onFrames) {
  const events = DEMO_DEBUG_SCRIPT.map((step) => ({ ...step, text: step.text.replaceAll("{{name}}", name), at: new Date().toISOString() }));
  return new Promise((resolve) => {
    let index = 0;
    const step = () => {
      if (index >= events.length) {
        resolve({ succeeded: true, output: events[events.length - 1].text, durationMs: 1280, usage: { inputTokens: 942, outputTokens: 86 }, events });
        return;
      }
      const event = events[index];
      // Prose arrives a few words at a time so the caret has something to do.
      const words = event.text.split(" ");
      let cursor = 0;
      const grow = () => {
        cursor = Math.min(words.length, cursor + 4);
        onFrames([{ index, event: { ...event, text: words.slice(0, cursor).join(" "), streaming: cursor < words.length } }]);
        if (cursor < words.length) { window.setTimeout(grow, 90); return; }
        index += 1;
        window.setTimeout(step, 160);
      };
      grow();
    };
    window.setTimeout(step, 200);
  });
}

function demoDebugHistory(name) {
  return [
    { runId: "demo-1", skill: name, prompt: "Draft notes for v0.1.11", startedAt: new Date(Date.now() - 3_600_000).toISOString(), result: { succeeded: true, output: "Wrote five bullets from the diff.", durationMs: 2140, usage: { inputTokens: 880, outputTokens: 120 }, events: [{ kind: "message", text: "Wrote five bullets from the diff." }] } },
    { runId: "demo-2", skill: name, prompt: "Summarise the revert", startedAt: new Date(Date.now() - 86_400_000).toISOString(), result: { succeeded: false, output: "", durationMs: 640, usage: {}, events: [{ kind: "error", text: "modu: no model configured", failed: true }] } },
  ];
}

function DebugEvent({ event, last }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const visual = EVENT_VISUALS[event.kind] || { icon: Terminal, labelKey: "skill.event.other", quiet: true };
  const Icon = visual.icon;
  const failed = visual.failed || event.failed;
  const text = String(event.text || "");
  const lines = useMemo(() => text.split("\n"), [text]);
  const collapsible = !visual.prose && lines.length > COLLAPSE_LINES;
  const shown = collapsible && !open ? lines.slice(0, COLLAPSE_LINES).join("\n") : text;

  return <li className="relative grid grid-cols-[20px_minmax(0,1fr)] gap-2.5 pb-4 last:pb-0">
    {/* One continuous rail behind the gutter icons is what makes a list of
        heterogeneous blocks read as a single run rather than stacked cards. */}
    {!last && <span className="absolute top-6 bottom-0 left-[9.5px] w-px bg-border/70" aria-hidden="true" />}
    <span className={`relative z-10 mt-0.5 grid size-5 place-items-center rounded-full ${failed ? "bg-destructive/12 text-destructive" : visual.quiet ? "bg-muted text-muted-foreground" : "bg-accent text-foreground"}`}>
      <Icon size={11} strokeWidth={2.2} aria-hidden="true" />
    </span>
    <div className="min-w-0">
      <span className={`text-[10px] font-medium ${failed ? "text-destructive" : "text-muted-foreground"}`}>{t(visual.labelKey)}</span>
      {visual.prose
        ? <MarkdownContent className={`mt-1 text-[13px] leading-[1.75] ${visual.quiet ? "text-muted-foreground" : "text-foreground"}`} content={text} streaming={Boolean(event.streaming)} />
        : <div className="mt-1 overflow-x-auto rounded-lg bg-muted/45 px-3 py-2">
          <pre className={`m-0 font-mono text-[11px] leading-[1.65] whitespace-pre-wrap ${failed ? "text-destructive" : "text-foreground/80"}`}>{shown}</pre>
        </div>}
      {collapsible && <button type="button" className="mt-1 inline-flex items-center gap-1 text-[10px] text-muted-foreground transition-colors hover:text-foreground" aria-expanded={open} onClick={() => setOpen((current) => !current)}>
        {open ? <ChevronDown size={11} aria-hidden="true" /> : <ChevronRight size={11} aria-hidden="true" />}
        {open ? t("skill.collapseOutput") : t("skill.expandOutput", { count: lines.length - COLLAPSE_LINES })}
      </button>}
    </div>
  </li>;
}

// Debugging a skill is a conversation with it, so it lives beside the skill in
// the inspector rather than in a drawer under the document it is testing.
// State stays here because the panel is its own React tree: the page cannot
// hand it props, only tell it which skill is open.
function SkillDebugInspector({ mode, notify }) {
  const { t } = useTranslation();
  const [skillName, setSkillName] = useState(() => currentSkillSelection().name);
  const [prompt, setPrompt] = useState("");
  const [result, setResult] = useState(null);
  const [events, setEvents] = useState([]);
  const [history, setHistory] = useState([]);
  const [viewingRunID, setViewingRunID] = useState("");
  const [running, setRunning] = useState(false);
  const runRef = useRef("");
  const tokens = (result?.usage?.inputTokens || 0) + (result?.usage?.outputTokens || 0);
  const outcome = result?.stopped ? "warn" : result?.succeeded ? "good" : "danger";
  const outcomeLabel = result?.stopped ? "skill.debugStopped" : result?.succeeded ? "skill.debugPassed" : "skill.debugFailed";

  const loadHistory = useCallback(async (name) => {
    if (!name) { setHistory([]); return; }
    try {
      setHistory(mode === "demo" ? demoDebugHistory(name) : await SkillBinding.DebugHistory(name) || []);
    } catch {
      // History is a convenience beside the live run; a panel that opens
      // without it is still usable.
      setHistory([]);
    }
  }, [mode]);

  // Selecting another skill starts a clean sheet: a transcript belongs to the
  // skill that produced it.
  useEffect(() => subscribeSkillWorkspace(SKILL_SELECTED_EVENT, (event) => {
    const next = event.detail?.name || "";
    setSkillName((current) => {
      if (current === next) return current;
      setResult(null);
      setEvents([]);
      setViewingRunID("");
      return next;
    });
  }), []);

  useEffect(() => { void loadHistory(skillName); }, [loadHistory, skillName]);

  // Streamed frames arrive on a Wails channel while Debug is still awaiting.
  // They are coalesced per display frame, the same way runtime frames are.
  useEffect(() => {
    if (mode !== "wails") return undefined;
    let pending = [];
    const scheduleFrame = typeof window.requestAnimationFrame === "function" ? window.requestAnimationFrame.bind(window) : (callback) => window.setTimeout(callback, 16);
    const cancelFrame = typeof window.cancelAnimationFrame === "function" ? window.cancelAnimationFrame.bind(window) : window.clearTimeout.bind(window);
    const batcher = createFrameBatcher(() => {
      const frames = pending;
      pending = [];
      if (frames.length) setEvents((current) => applyDebugFrames(current, frames));
    }, scheduleFrame, cancelFrame);
    const off = Events.On(SKILL_DEBUG_EVENT, (event) => {
      const frame = event?.data;
      if (!frame || frame.runId !== runRef.current) return;
      pending.push(frame);
      batcher.schedule();
    });
    return () => { batcher.cancel(); off(); };
  }, [mode]);

  const run = async () => {
    if (!skillName || !prompt.trim()) { notify?.("error", t("skill.debugPromptRequired")); return; }
    const runID = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
    runRef.current = runID;
    setRunning(true);
    setResult(null);
    setEvents([]);
    setViewingRunID("");
    try {
      const value = mode === "demo"
        ? await streamDemoDebug(skillName, (frames) => setEvents((current) => applyDebugFrames(current, frames)))
        : await SkillBinding.Debug({ name: skillName, prompt: prompt.trim(), runId: runID });
      if (runRef.current !== runID) return;
      setResult(value);
      setEvents(value.events || []);
      notify?.(value.stopped ? "info" : value.succeeded ? "success" : "error", t(value.stopped ? "skill.debugStopped" : value.succeeded ? "skill.debugComplete" : "skill.debugFailed"));
    } catch (error) {
      // The streamed transcript is the only record of a failed run, so it
      // stays on screen; only the outcome is reported.
      notify?.("error", errorMessage(error));
    } finally {
      if (runRef.current === runID) runRef.current = "";
      setRunning(false);
      void loadHistory(skillName);
    }
  };

  const stop = () => {
    const runID = runRef.current;
    if (!runID) return;
    if (mode === "demo") { runRef.current = ""; setRunning(false); return; }
    void SkillBinding.StopDebug(runID).catch((error) => notify?.("error", errorMessage(error)));
  };

  const viewRun = (record) => {
    if (running) return;
    setViewingRunID(record.runId);
    setResult(record.result);
    setEvents(record.result?.events || []);
    setPrompt(record.prompt || "");
  };

  const clearHistory = async () => {
    if (!skillName) return;
    try {
      if (mode !== "demo") await SkillBinding.ClearDebugHistory(skillName);
      setHistory([]);
      setViewingRunID("");
    } catch (error) {
      notify?.("error", errorMessage(error));
    }
  };

  if (!skillName) return <p className="px-4 py-4 text-[12px] leading-relaxed text-muted-foreground">{t("skill.debugNeedsSkill")}</p>;

  return <div className="flex h-full min-h-0 flex-col">
    <div className="flex h-11 shrink-0 items-center gap-2 border-b border-border/70 px-3">
      <Play size={13} strokeWidth={2.4} className="shrink-0 text-muted-foreground" aria-hidden="true" />
      <code className="min-w-0 flex-1 truncate font-mono text-[11px] text-muted-foreground">/{skillName}</code>
      {viewingRunID && <span className="shrink-0 text-[11px] text-muted-foreground">{t("skill.viewingSavedRun")}</span>}
      {result && !running && <>
        <StatusBadge status={outcome}>{t(outcomeLabel)}</StatusBadge>
        <span className="shrink-0 text-[11px] text-muted-foreground">{formatDuration(result.durationMs)} · {t("skill.tokens", { count: formatTokens(tokens) })}</span>
      </>}
      {/* Every run is written to ~/.onecatch/skill-debug, so the panel can
          reopen an old transcript instead of asking for the run again. */}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon-xs" className="shrink-0 text-muted-foreground" disabled={history.length === 0} aria-label={t("skill.debugHistory")} title={t("skill.debugHistory")}><History aria-hidden="true" /></Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="max-w-80">
          <DropdownMenuLabel className="text-[11px] font-normal text-muted-foreground">{t("skill.debugHistoryCount", { count: history.length })}</DropdownMenuLabel>
          {history.map((record) => <DropdownMenuItem className="gap-2" key={record.runId} onSelect={() => viewRun(record)}>
            <i className={`size-1.5 shrink-0 rounded-full ${record.result?.stopped ? "bg-warning" : record.result?.succeeded ? "bg-success" : "bg-destructive"}`} aria-hidden="true" />
            <span className="min-w-0 flex-1 truncate text-[12px]">{record.prompt || t("skill.debugUntitledRun")}</span>
            <span className="shrink-0 text-[10px] text-muted-foreground">{formatDateTime(record.startedAt)}</span>
          </DropdownMenuItem>)}
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={clearHistory}><Trash2 aria-hidden="true" />{t("skill.clearDebugHistory")}</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>

    <div className="flex shrink-0 items-end gap-2 px-3 py-3">
      <Textarea
        className="max-h-28 min-h-9 flex-1 resize-none rounded-lg border-border/60 bg-background px-3 py-2 text-[12px] leading-relaxed shadow-none"
        aria-label={t("skill.testPrompt")}
        placeholder={t("skill.debugPlaceholder")}
        value={prompt}
        onChange={(event) => setPrompt(event.target.value)}
        onKeyDown={(event) => { if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) { event.preventDefault(); void run(); } }}
      />
      {running
        ? <Button size="sm" variant="outline" className="shrink-0" onClick={stop}><Square aria-hidden="true" />{t("skill.stopDebug")}</Button>
        : <Button size="sm" className="shrink-0" disabled={!prompt.trim()} onClick={() => void run()}><Play aria-hidden="true" />{t("skill.runDebug")}</Button>}
    </div>

    <div className="min-h-0 flex-1 overflow-y-auto px-3 pb-5">
      {running && events.length === 0 && <p className="text-[12px] text-muted-foreground">{t("skill.debugging")}</p>}
      {!running && !result && events.length === 0 && <p className="text-[12px] leading-relaxed text-muted-foreground">{t("skill.debugDescription")}</p>}
      {events.length > 0 && <ol className="m-0 list-none p-0">
        {events.map((event, index) => <DebugEvent key={index} event={event} last={index === events.length - 1} />)}
      </ol>}
      {result && !running && <div className="mt-4 border-t border-border/50 pt-4">
        <span className="text-[10px] font-medium text-muted-foreground">{t("skill.debugOutput")}</span>
        {result.output
          ? <MarkdownContent className="mt-1.5 text-[13px] leading-[1.75] text-foreground" content={result.output} />
          : <p className="mt-1.5 text-[12px] text-muted-foreground">{t("skill.noDebugOutput")}</p>}
        {result.sessionId && <code className="mt-3 block font-mono text-[10px] text-muted-foreground">{result.sessionId}</code>}
      </div>}
    </div>
  </div>;
}

export default memo(SkillDebugInspector);
