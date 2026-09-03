import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Brain, ChevronDown, ChevronRight, FileDiff, History, MessageSquare, Play, Square, Terminal, Trash2, TriangleAlert, Wrench, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Textarea } from "@/components/ui/textarea";
import MarkdownContent from "./MarkdownContent.jsx";
import { StatusBadge } from "../../ui/primitives.jsx";
import { formatDuration, formatDateTime, formatTokens } from "../format.js";

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

export default function SkillDebugPanel({ skillName, prompt, result, events = [], history = [], viewingRunID = "", running, blocked, onPromptChange, onRun, onStop, onViewRun, onClearHistory, onClose }) {
  const { t } = useTranslation();
  const tokens = (result?.usage?.inputTokens || 0) + (result?.usage?.outputTokens || 0);
  const outcome = result?.stopped ? "warn" : result?.succeeded ? "good" : "danger";
  const outcomeLabel = result?.stopped ? "skill.debugStopped" : result?.succeeded ? "skill.debugPassed" : "skill.debugFailed";

  return <section className="flex min-h-0 shrink-0 flex-col border-t border-border/60 bg-card/45" style={{ height: "min(46%, 420px)" }} aria-label={t("skill.debugTitle")}>
    <header className="flex h-10 shrink-0 items-center gap-2 px-6">
      <Play size={13} strokeWidth={2.4} className="shrink-0 text-muted-foreground" aria-hidden="true" />
      <strong className="shrink-0 text-[12px] font-semibold text-foreground">{t("skill.debug")}</strong>
      <code className="min-w-0 truncate font-mono text-[11px] text-muted-foreground">/{skillName}</code>
      <div className="ml-auto flex shrink-0 items-center gap-2">
        {viewingRunID && <span className="text-[11px] text-muted-foreground">{t("skill.viewingSavedRun")}</span>}
        {result && !running && <>
          <StatusBadge status={outcome}>{t(outcomeLabel)}</StatusBadge>
          <span className="text-[11px] text-muted-foreground">{formatDuration(result.durationMs)} · {t("skill.tokens", { count: formatTokens(tokens) })}</span>
        </>}
        {/* Every run is written to ~/.onecatch/skill-debug, so the panel can
            reopen an old transcript instead of asking for the run again. */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-xs" className="text-muted-foreground" disabled={history.length === 0} aria-label={t("skill.debugHistory")} title={t("skill.debugHistory")}><History aria-hidden="true" /></Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="max-w-80">
            <DropdownMenuLabel className="text-[11px] font-normal text-muted-foreground">{t("skill.debugHistoryCount", { count: history.length })}</DropdownMenuLabel>
            {history.map((record) => <DropdownMenuItem className="gap-2" key={record.runId} onSelect={() => onViewRun(record)}>
              <i className={`size-1.5 shrink-0 rounded-full ${record.result?.stopped ? "bg-warning" : record.result?.succeeded ? "bg-success" : "bg-destructive"}`} aria-hidden="true" />
              <span className="min-w-0 flex-1 truncate text-[12px]">{record.prompt || t("skill.debugUntitledRun")}</span>
              <span className="shrink-0 text-[10px] text-muted-foreground">{formatDateTime(record.startedAt)}</span>
            </DropdownMenuItem>)}
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onSelect={onClearHistory}><Trash2 aria-hidden="true" />{t("skill.clearDebugHistory")}</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <Button variant="ghost" size="icon-xs" className="text-muted-foreground" aria-label={t("common.close")} onClick={onClose}><X aria-hidden="true" /></Button>
      </div>
    </header>

    <div className="flex shrink-0 items-end gap-2 px-6 pb-3">
      <Textarea
        className="max-h-24 min-h-9 flex-1 resize-none rounded-lg border-border/60 bg-background px-3 py-2 text-[12px] leading-relaxed shadow-none"
        aria-label={t("skill.testPrompt")}
        placeholder={t("skill.debugPlaceholder")}
        value={prompt}
        onChange={(event) => onPromptChange(event.target.value)}
        onKeyDown={(event) => { if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) { event.preventDefault(); onRun(); } }}
      />
      {running
        ? <Button size="sm" variant="outline" className="shrink-0" onClick={onStop}><Square aria-hidden="true" />{t("skill.stopDebug")}</Button>
        : <Button size="sm" className="shrink-0" disabled={blocked || !prompt.trim()} onClick={onRun}><Play aria-hidden="true" />{t("skill.runDebug")}</Button>}
    </div>

    <div className="min-h-0 flex-1 overflow-y-auto px-6 pb-5">
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
  </section>;
}
