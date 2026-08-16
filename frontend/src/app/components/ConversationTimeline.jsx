import { lazy, memo, Suspense, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Clipboard } from "@wailsio/runtime";
import { BookOpen, BrainCircuit, Check, ChevronDown, ChevronRight, Clock3, Copy, FilePenLine, LoaderCircle, Search, Terminal, TriangleAlert, Wrench } from "lucide-react";
import { Button } from "@/components/ui/button";
import { fileName, formatDuration, formatTime } from "../format.js";
import { groupRoundItems } from "../runConversation.js";
import { Action } from "../../ui/primitives.jsx";

const MarkdownContent = lazy(() => import("./MarkdownContent.jsx"));

function MessageBody({ content, streaming = false }) {
  return <Suspense fallback={<div className="markdown-content markdown-loading">{content}</div>}><MarkdownContent content={content} streaming={streaming} /></Suspense>;
}

// Keep timestamps local to the row that displays them. Tool calls often finish
// within the same second, but hiding repeated values makes an individual row
// look like it has no recorded time.
function createTimeLabeler() {
  return (at) => formatTime(at);
}

function messageTime(value, language) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleTimeString(language === "en" ? "en-US" : "zh-CN", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

async function copyText(value) {
  try {
    await Clipboard.SetText(value);
  } catch (wailsError) {
    if (!navigator.clipboard?.writeText) throw wailsError;
    await navigator.clipboard.writeText(value);
  }
}

function MessageActions({ at, content, align = "start" }) {
  const { t, i18n } = useTranslation();
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return undefined;
    const timer = window.setTimeout(() => setCopied(false), 1600);
    return () => window.clearTimeout(timer);
  }, [copied]);
  const copyMessage = async () => {
    try {
      await copyText(content);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };
  const copyLabel = copied ? t("timeline.messageCopied") : t("timeline.copyMessage");
  return <div className={`conversation-message-actions ${align}`}>
    <time dateTime={at || undefined}>{messageTime(at, i18n.resolvedLanguage)}</time>
    <Button type="button" variant="ghost" size="icon-xs" className="conversation-message-copy" aria-label={copyLabel} title={copyLabel} onClick={copyMessage}>{copied ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}</Button>
  </div>;
}

function roundDuration(round) {
  const startedAt = new Date(round.startedAt || 0).getTime();
  const finishedAt = new Date(round.finishedAt || 0).getTime();
  if (!Number.isFinite(startedAt) || !startedAt) return "";
  const end = Number.isFinite(finishedAt) && finishedAt ? finishedAt : Date.now();
  return formatDuration(Math.max(0, end - startedAt));
}

function toolIcon(entry) {
  const command = `${entry.toolName || ""} ${entry.title || ""}`.toLocaleLowerCase();
  if (/\b(?:rg|grep|find)\b|(?:搜索|查找|search|find)/.test(command)) return Search;
  if (/\b(?:cat|sed|head|tail|read)\b|(?:读取|read)/.test(command)) return BookOpen;
  if (entry.kind === "reasoning") return BrainCircuit;
  if (/\b(?:go|npm|pnpm|yarn|bun|git|bash|zsh|sh)\b|(?:运行|检查|run|inspect)/.test(command)) return Terminal;
  return Wrench;
}

function ToolTimelineItem({ entry, running, stalled, time }) {
  const { t } = useTranslation();
  const labels = { tool_use: t("timeline.toolUse"), tool_result: t("timeline.result"), file_change: t("timeline.fileChange"), reasoning: t("timeline.process") };
  const failed = Boolean(entry.failed);
  const state = failed ? t("timeline.failed") : running ? t("timeline.executing") : stalled ? t("timeline.incomplete") : entry.kind === "reasoning" ? t("timeline.process") : t("timeline.done");
  const ToolIcon = toolIcon(entry);
  const StateIcon = failed ? TriangleAlert : running ? LoaderCircle : stalled ? Clock3 : Check;
  return <details className={`conversation-tool kind-${entry.kind} ${running ? "running" : ""} ${failed ? "failed" : ""} ${stalled ? "stalled" : ""}`}>
    <summary aria-label={`${labels[entry.kind] || entry.kind}: ${entry.title}`}><span className="conversation-tool-summary"><span className="conversation-tool-icon"><ToolIcon aria-hidden="true" /></span><span className="conversation-tool-heading"><strong title={entry.title}>{entry.title}</strong></span><span className="conversation-tool-state" title={state} role="status" aria-label={state}><StateIcon className={running ? "conversation-tool-state-icon spinning" : "conversation-tool-state-icon"} aria-hidden="true" /></span><span className="conversation-tool-caret"><ChevronRight className="closed" strokeWidth={2.25} /><ChevronDown className="opened" strokeWidth={2.25} /></span><time className="sr-only">{time ?? formatTime(entry.at)}</time></span></summary>
    <div className="conversation-tool-body"><div><span>{entry.kind === "file_change" ? t("timeline.path") : entry.kind === "reasoning" ? t("timeline.process") : t("timeline.command")}</span><pre>{entry.text}</pre></div>{entry.details.map((detail, index) => <div key={`${detail.kind}-${index}`}><span>{labels[detail.kind] || detail.kind}</span><pre>{detail.text}</pre></div>)}</div>
  </details>;
}

function PermissionTimelineItem({ entry, busy, onDecision, time }) {
  const { t } = useTranslation();
  const request = entry.request || {};
  const pending = !entry.decision;
  const canAlwaysAllow = pending && !request.suppressAlwaysAllow && (request.suggestions || []).length > 0;
  const input = Object.keys(request.input || {}).length ? JSON.stringify(request.input, null, 2) : "";
  const status = pending ? t("timeline.permissionWaiting") : entry.decision === "allow" ? t("timeline.permissionAllowed") : t("timeline.permissionDenied");
  return <section className={`conversation-permission ${pending ? "pending" : entry.decision}`}>
    <div className="conversation-permission-head"><div><span>{request.displayName || request.toolName || t("timeline.permissionRequest")}</span><strong>{request.title || t("timeline.permissionTitle", { tool: request.toolName || "Claude" })}</strong></div><div><b>{status}</b><time>{time ?? formatTime(entry.at)}</time></div></div>
    {(request.description || request.decisionReason) && <p>{request.description || request.decisionReason}</p>}
    {input && <details><summary>{t("timeline.permissionDetails")}</summary><pre>{input}</pre></details>}
    {pending && !request.requiresUserInteraction && <div className="conversation-permission-actions"><Action size="compact" tone="danger" disabled={busy} onClick={() => onDecision?.(request.id, "deny")}>{t("timeline.permissionDeny")}</Action><Action size="compact" disabled={busy} onClick={() => onDecision?.(request.id, "allow_once")}>{t("timeline.permissionAllowOnce")}</Action>{canAlwaysAllow && <Action size="compact" tone="primary" disabled={busy} onClick={() => onDecision?.(request.id, "allow_always")}>{t("timeline.permissionAllowAlways")}</Action>}</div>}
    {pending && request.requiresUserInteraction && <p className="conversation-permission-note">{t("timeline.permissionOpenClaude")}</p>}
  </section>;
}

function FileChangeGroup({ entries }) {
  const { t } = useTranslation();
  const label = entries.length === 1
    ? t("timeline.editedFile", { name: fileName(entries[0].text) })
    : t("timeline.editedFiles", { count: entries.length });
  return <details className="conversation-file-changes">
    <summary aria-label={label}><span className="conversation-file-changes-summary"><span className="conversation-file-changes-icon"><FilePenLine aria-hidden="true" /></span><span><strong>{label}</strong><small>{t("timeline.fileChange")}</small></span><span className="conversation-file-changes-caret"><ChevronRight className="closed" /><ChevronDown className="opened" /></span></span></summary>
    <div className="conversation-file-changes-list">{entries.map((entry, index) => <div key={entry.id || `${entry.text}-${index}`}><FilePenLine aria-hidden="true" /><span title={entry.text}>{entry.text}</span></div>)}</div>
  </details>;
}

function ProcessGroup({ entries, active, round, permissionBusy, onPermissionDecision }) {
  const { t } = useTranslation();
  const timeLabel = createTimeLabeler();
  const lastEntry = entries[entries.length - 1];
  const toolCount = entries.filter((entry) => entry.type === "tool" && entry.kind === "tool_use").length;
  const duration = roundDuration(round);
  const label = active
    ? t("timeline.processing")
    : toolCount ? t("timeline.ranTools", { count: toolCount })
      : duration ? t("timeline.processedFor", { duration }) : t("timeline.processed");
  return <details className="conversation-process" open={active || undefined}>
    <summary aria-label={`${label} · ${round.runtime}`}><span>{label}</span><ChevronRight className="closed" aria-hidden="true" /><ChevronDown className="opened" aria-hidden="true" /></summary>
    <div className="conversation-process-body">{entries.map((entry, index) => {
      if (entry.type === "permission") {
        return <PermissionTimelineItem key={entry.id} entry={entry} busy={permissionBusy === entry.request?.id} onDecision={onPermissionDecision} time={timeLabel(entry.at)} />;
      }
      const running = Boolean(active) && entry === lastEntry && entry.kind === "tool_use";
      const stalled = !entry.settled && !running && round.status !== "succeeded";
      return <ToolTimelineItem key={entry.id || `tool-${index}`} entry={entry} time={timeLabel(entry.at)} running={running} stalled={stalled} />;
    })}</div>
  </details>;
}

// One round of the transcript. `buildRunConversation` hands back a stable object
// reference for any round whose step has finished, so memo() short-circuits every
// finished round on a stream/poll frame — only the live round is reconciled.
const ConversationRound = memo(function ConversationRound({ round, active, permissionBusy, onPermissionDecision }) {
  const { t } = useTranslation();
  const timeLabel = createTimeLabeler();
  const blocks = groupRoundItems(round.items);
  const lastItem = round.items[round.items.length - 1];
  return <article className="conversation-round conversation-agent">
    <div className="conversation-round-body">
      {blocks.map((block) => {
        if (block.type === "message") {
          const entry = block.item;
          return <div className={`conversation-agent-message ${entry.tone}`} key={block.id}><MessageBody content={entry.text} streaming={entry.streaming} /><MessageActions at={entry.at || round.finishedAt || round.startedAt} content={entry.text} /></div>;
        }
        if (block.type === "files") return <FileChangeGroup entries={block.items} key={block.id} />;
        return <ProcessGroup entries={block.items} active={Boolean(active) && lastItem === block.items[block.items.length - 1]} round={round} permissionBusy={permissionBusy} onPermissionDecision={onPermissionDecision} key={block.id} />;
      })}
      <span className="sr-only">{round.runtime} · {t("timeline.round", { count: round.round })} · <time>{timeLabel(round.finishedAt || round.startedAt)}</time></span>
    </div>
  </article>;
});

// The timeline can grow to hundreds of rows; poll-driven parent re-renders must
// not rebuild it unless `items`/`active` actually change, hence memo() paired
// with the memoized conversation array the parent feeds in.
function ConversationTimeline({ items, active, permissionBusy = "", onPermissionDecision }) {
  const { t } = useTranslation();
  const rounds = items.filter((item) => item.type === "round");
  const lastRound = rounds[rounds.length - 1];
  return <div className="conversation-section">
    <div className="conversation-list">
      {items.map((item) => item.type === "user" ? <div className="conversation-user" key={item.id}>
        <div className="conversation-bubble"><MessageBody content={item.text} /></div><MessageActions at={item.at} content={item.text} align="end" />
      </div> : <ConversationRound key={item.id} round={item} active={Boolean(active) && item === lastRound} permissionBusy={permissionBusy} onPermissionDecision={onPermissionDecision} />)}
      {!items.length && <p className="muted-copy">{t("timeline.empty")}</p>}
    </div>
  </div>;
}

export default memo(ConversationTimeline);
