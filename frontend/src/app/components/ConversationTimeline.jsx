import { lazy, memo, Suspense, useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Clipboard } from "@wailsio/runtime";
import { BookOpen, BrainCircuit, Check, ChevronDown, ChevronRight, Clock3, Copy, FilePenLine, LoaderCircle, Search, Terminal, TriangleAlert, Wrench } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { fileName, formatDateTime, formatDuration, formatMessageDateTime, formatTime, formatToolTime } from "../format.js";
import { groupRoundItems } from "../runConversation.js";
import { Action } from "../../ui/primitives.jsx";

const MarkdownContent = lazy(() => import("./MarkdownContent.jsx"));

function MessageBody({ content, streaming = false }) {
  return <Suspense fallback={<div className="markdown-content markdown-loading">{content}</div>}><MarkdownContent content={content} streaming={streaming} /></Suspense>;
}

// Keep timestamps local to the row that displays them. Hiding repeated values
// makes an individual permission or round record look like it has no time.
function createTimeLabeler() {
  return (at) => formatTime(at);
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
  const { t } = useTranslation();
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
    <time dateTime={at || undefined} title={formatDateTime(at)}>{formatMessageDateTime(at)}</time>
    <Tooltip><TooltipTrigger asChild><Button type="button" variant="ghost" size="icon-xs" className="conversation-message-copy" aria-label={copyLabel} onClick={copyMessage}>{copied ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}</Button></TooltipTrigger><TooltipContent side="top" sideOffset={6}>{copyLabel}</TooltipContent></Tooltip>
  </div>;
}

const UserMessage = memo(function UserMessage({ item }) {
  const { t } = useTranslation();
  const contentId = useId();
  const contentRef = useRef(null);
  const [expanded, setExpanded] = useState(false);
  const [overflowing, setOverflowing] = useState(false);

  useLayoutEffect(() => {
    const content = contentRef.current;
    if (!content || expanded) return undefined;
    const measure = () => setOverflowing(content.scrollHeight > content.clientHeight + 1);
    measure();
    if (typeof ResizeObserver === "undefined") {
      window.addEventListener("resize", measure);
      return () => window.removeEventListener("resize", measure);
    }
    const observer = new ResizeObserver(measure);
    observer.observe(content);
    return () => observer.disconnect();
  }, [expanded, item.text]);

  return <div className="conversation-user">
    <div className={`conversation-bubble ${overflowing ? "has-disclosure" : ""}`}>
      <div ref={contentRef} id={contentId} className={`conversation-user-message-content ${expanded ? "is-expanded" : "is-collapsed"} ${overflowing ? "has-overflow" : ""}`}>
        <MessageBody content={item.text} />
      </div>
      {overflowing && <Button type="button" variant="ghost" size="sm" className="conversation-user-disclosure" aria-expanded={expanded} aria-controls={contentId} onClick={() => setExpanded((current) => !current)}>
        <span>{t(expanded ? "timeline.showLess" : "timeline.showMore")}</span><ChevronDown className={expanded ? "is-expanded" : ""} aria-hidden="true" />
      </Button>}
    </div>
    <MessageActions at={item.at} content={item.text} align="end" />
  </div>;
});

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

function toolDuration(entry, running) {
  if (entry.kind !== "tool_use" || !entry.at) return "";
  const startedAt = new Date(entry.at).getTime();
  const finishedAt = new Date(entry.finishedAt || 0).getTime();
  if (!Number.isFinite(startedAt)) return "";
  const end = Number.isFinite(finishedAt) && finishedAt ? finishedAt : running ? Date.now() : 0;
  return end ? formatDuration(Math.max(0, end - startedAt)) : "";
}

function ToolTimelineItem({ entry, running, stalled }) {
  const { t } = useTranslation();
  const labels = { tool_use: t("timeline.toolUse"), tool_result: t("timeline.result"), file_change: t("timeline.fileChange"), reasoning: t("timeline.process") };
  const failed = Boolean(entry.failed);
  const state = failed ? t("timeline.failed") : running ? t("timeline.executing") : stalled ? t("timeline.incomplete") : entry.kind === "reasoning" ? t("timeline.process") : t("timeline.done");
  const ToolIcon = toolIcon(entry);
  const StateIcon = failed ? TriangleAlert : running ? LoaderCircle : stalled ? Clock3 : Check;
  const duration = toolDuration(entry, running);
  return <details className={`conversation-tool kind-${entry.kind} ${running ? "running" : ""} ${failed ? "failed" : ""} ${stalled ? "stalled" : ""}`}>
    <summary aria-label={`${labels[entry.kind] || entry.kind}: ${entry.title}`}><span className="conversation-tool-summary"><span className="conversation-tool-icon"><ToolIcon aria-hidden="true" /></span><span className="conversation-tool-heading"><strong title={entry.title}>{entry.title}</strong><span className="conversation-tool-meta"><time dateTime={entry.at || undefined} title={formatDateTime(entry.at)}>{formatToolTime(entry.at)}</time>{duration && <><span aria-hidden="true">·</span><span title={t("timeline.toolDuration", { duration })}>{duration}</span></>}</span></span><span className="conversation-tool-state" title={state} role="status" aria-label={state}><StateIcon className={running ? "conversation-tool-state-icon spinning" : "conversation-tool-state-icon"} aria-hidden="true" /></span><span className="conversation-tool-caret"><ChevronRight className="closed" strokeWidth={2.25} /><ChevronDown className="opened" strokeWidth={2.25} /></span></span></summary>
    <div className="conversation-tool-body"><div><span>{entry.kind === "file_change" ? t("timeline.path") : entry.kind === "reasoning" ? t("timeline.process") : t("timeline.command")}</span><pre>{entry.text}</pre></div>{entry.details.map((detail, index) => <div key={`${detail.kind}-${index}`}><span>{labels[detail.kind] || detail.kind}</span><pre>{detail.text}</pre></div>)}</div>
  </details>;
}

function PermissionTimelineItem({ entry, busy, onDecision, time }) {
  const { t } = useTranslation();
  const request = entry.request || {};
  const pending = !entry.decision;
  // Whether a decision can be remembered is the adapter's answer, folded into
  // suppressAlwaysAllow: Claude needs provider-authored rules to persist, an
  // ACP agent simply offers the option and keeps the memory itself.
  const canAlwaysAllow = pending && !request.suppressAlwaysAllow;
  const input = Object.keys(request.input || {}).length ? JSON.stringify(request.input, null, 2) : "";
  const status = pending ? t("timeline.permissionWaiting") : entry.decision === "allow" ? t("timeline.permissionAllowed") : t("timeline.permissionDenied");
  return <section className={`conversation-permission ${pending ? "pending" : entry.decision}`}>
    <div className="conversation-permission-head"><div><span>{request.displayName || request.toolName || t("timeline.permissionRequest")}</span><strong>{request.title || t("timeline.permissionTitle", { tool: request.toolName })}</strong></div><div><b>{status}</b><time>{time ?? formatTime(entry.at)}</time></div></div>
    {(request.description || request.decisionReason) && <p>{request.description || request.decisionReason}</p>}
    {input && <details><summary>{t("timeline.permissionDetails")}</summary><pre>{input}</pre></details>}
    {pending && !request.requiresUserInteraction && <div className="conversation-permission-actions"><Action size="compact" tone="danger" disabled={busy} onClick={() => onDecision?.(request.id, "deny")}>{t("timeline.permissionDeny")}</Action><Action size="compact" disabled={busy} onClick={() => onDecision?.(request.id, "allow_once")}>{t("timeline.permissionAllowOnce")}</Action>{canAlwaysAllow && <Action size="compact" tone="primary" disabled={busy} title={t("timeline.permissionAllowAlwaysHint")} onClick={() => onDecision?.(request.id, "allow_always")}>{t("timeline.permissionAllowAlways")}</Action>}</div>}
    {pending && request.requiresUserInteraction && <p className="conversation-permission-note">{t("timeline.permissionOpenClaude")}</p>}
  </section>;
}

function FileChangeGroup({ entries, onReview }) {
  const { t } = useTranslation();
  const paths = [...new Set(entries.flatMap((entry) => String(entry.text || "").split("\n")).map((path) => path.trim()).filter(Boolean))];
  const label = paths.length === 1
    ? t("timeline.editedFile", { name: fileName(paths[0]) })
    : t("timeline.editedFiles", { count: paths.length });
  return <div className="conversation-file-changes-card"><details className="conversation-file-changes">
    <summary aria-label={label}><span className="conversation-file-changes-summary"><span className="conversation-file-changes-icon"><FilePenLine aria-hidden="true" /></span><span><strong>{label}</strong><small>{t("timeline.fileChange")}</small></span><span className="conversation-file-changes-caret"><ChevronRight className="closed" /><ChevronDown className="opened" /></span></span></summary>
    <div className="conversation-file-changes-list">{paths.map((path) => <div key={path}><FilePenLine aria-hidden="true" /><span title={path}>{path}</span></div>)}</div>
  </details>{onReview && <Action size="compact" tone="muted" className="conversation-review-action" onClick={onReview}>{t("review.open")}</Action>}</div>;
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
      return <ToolTimelineItem key={entry.id || `tool-${index}`} entry={entry} running={running} stalled={stalled} />;
    })}</div>
  </details>;
}

// One round of the transcript. `buildRunConversation` hands back a stable object
// reference for any round whose step has finished, so memo() short-circuits every
// finished round on a stream/poll frame — only the live round is reconciled.
const ConversationRound = memo(function ConversationRound({ round, active, permissionBusy, onPermissionDecision, onReview }) {
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
        if (block.type === "files") return <FileChangeGroup entries={block.items} onReview={onReview} key={block.id} />;
        return <ProcessGroup entries={block.items} active={Boolean(active) && lastItem === block.items[block.items.length - 1]} round={round} permissionBusy={permissionBusy} onPermissionDecision={onPermissionDecision} key={block.id} />;
      })}
      <span className="sr-only">{round.runtime} · {t("timeline.round", { count: round.round })} · <time>{timeLabel(round.finishedAt || round.startedAt)}</time></span>
    </div>
  </article>;
});

// The timeline can grow to hundreds of rows; poll-driven parent re-renders must
// not rebuild it unless `items`/`active` actually change, hence memo() paired
// with the memoized conversation array the parent feeds in.
function ConversationTimeline({ items, active, hiddenCount = 0, onLoadEarlier, permissionBusy = "", onPermissionDecision, onReview }) {
  const { t } = useTranslation();
  const [loadingEarlier, setLoadingEarlier] = useState(false);
  const rounds = items.filter((item) => item.type === "round");
  const lastRound = rounds[rounds.length - 1];
  const loadEarlier = async () => {
    setLoadingEarlier(true);
    try {
      await onLoadEarlier?.();
    } finally {
      setLoadingEarlier(false);
    }
  };
  return <div className="conversation-section">
    <div className="conversation-list">
      {hiddenCount > 0 && <div className="flex justify-center py-2">
        <Button type="button" variant="outline" size="sm" disabled={loadingEarlier} onClick={loadEarlier}>
          {loadingEarlier ? t("common.loading") : t("timeline.loadEarlier", { count: hiddenCount })}
        </Button>
      </div>}
      {items.map((item) => item.type === "user" ? <UserMessage item={item} key={item.id} /> : <ConversationRound key={item.id} round={item} active={Boolean(active) && item === lastRound} permissionBusy={permissionBusy} onPermissionDecision={onPermissionDecision} onReview={onReview} />)}
      {!items.length && <p className="muted-copy">{t("timeline.empty")}</p>}
    </div>
  </div>;
}

export default memo(ConversationTimeline);
