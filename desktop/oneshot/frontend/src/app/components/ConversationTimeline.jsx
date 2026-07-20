import { lazy, memo, Suspense } from "react";
import { useTranslation } from "react-i18next";
import { CaretDown, CaretRight, Circle } from "@phosphor-icons/react";
import { formatTime } from "../format.js";
import { Action } from "../../ui/primitives.jsx";

const MarkdownContent = lazy(() => import("./MarkdownContent.jsx"));

function MessageBody({ content, streaming = false }) {
  return <Suspense fallback={<div className="markdown-content markdown-loading">{content}</div>}><MarkdownContent content={content} streaming={streaming} /></Suspense>;
}

// Consecutive events often share the same second; repeating the identical
// timestamp on every row is noise. Each caller keeps its own de-duper so the
// label only shows on the first of a same-second run.
function createTimeLabeler() {
  let last = "";
  return (at) => {
    const label = formatTime(at);
    if (!label || label === last) return "";
    last = label;
    return label;
  };
}

function ToolTimelineItem({ entry, running, stalled, time }) {
  const { t } = useTranslation();
  const labels = { tool_use: t("timeline.toolUse"), tool_result: t("timeline.result"), file_change: t("timeline.fileChange"), reasoning: t("timeline.process") };
  const failed = Boolean(entry.failed);
  const state = failed ? t("timeline.failed") : running ? t("timeline.executing") : stalled ? t("timeline.incomplete") : entry.kind === "reasoning" ? t("timeline.process") : t("timeline.done");
  return <details className={`conversation-tool kind-${entry.kind} ${running ? "running" : ""} ${failed ? "failed" : ""} ${stalled ? "stalled" : ""}`}>
    <summary aria-label={`${labels[entry.kind] || entry.kind}: ${entry.title}`}><span className="conversation-tool-summary"><span className="conversation-tool-caret"><CaretRight className="closed" weight="bold" /><CaretDown className="opened" weight="bold" /></span><strong title={entry.text}>{entry.title}</strong><span className="conversation-tool-state">{running && <span className="pulse" />}{state}</span><time>{time ?? formatTime(entry.at)}</time></span></summary>
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

// One round of the transcript. `buildRunConversation` hands back a stable object
// reference for any round whose step has finished, so memo() short-circuits every
// finished round on a stream/poll frame — only the live round is reconciled.
const ConversationRound = memo(function ConversationRound({ round, active, permissionBusy, onPermissionDecision }) {
  const { t } = useTranslation();
  const timeLabel = createTimeLabeler();
  // A tool reports its own outcome (entry.failed) whenever the runtime answered
  // it. Only when it never answered do we fall back to the step: a step that
  // succeeded left nothing unfinished behind — Codex just does not emit a result
  // event per command — while a step that died mid-flight did.
  const lastIndex = round.items.length - 1;
  return <article className="conversation-round">
    <div className="conversation-round-body">{round.items.map((entry, index) => {
      if (entry.type === "message") {
        return <div className={`conversation-agent ${entry.tone}`} key={entry.id || `message-${index}`}><div className="conversation-speaker"><span className="conversation-identity"><Circle className="conversation-event-dot agent" weight="fill" aria-label={t("timeline.agentMessage")} /><strong>{round.runtime}</strong></span><span className="conversation-message-meta"><span className="conversation-round-index">{t("timeline.round", { count: round.round })}</span><time>{timeLabel(entry.at || round.finishedAt || round.startedAt)}</time></span></div><MessageBody content={entry.text} streaming={entry.streaming} /></div>;
      }
      if (entry.type === "permission") {
        return <PermissionTimelineItem key={entry.id} entry={entry} busy={permissionBusy === entry.request?.id} onDecision={onPermissionDecision} time={timeLabel(entry.at)} />;
      }
      const running = Boolean(active) && index === lastIndex && entry.kind === "tool_use";
      const stalled = !entry.settled && !running && round.status !== "succeeded";
      return <ToolTimelineItem key={entry.id || `tool-${index}`} entry={entry} time={timeLabel(entry.at)} running={running} stalled={stalled} />;
    })}</div>
  </article>;
});

// The timeline can grow to hundreds of rows; poll-driven parent re-renders must
// not rebuild it unless `items`/`active` actually change, hence memo() paired
// with the memoized conversation array the parent feeds in.
function ConversationTimeline({ items, active, permissionBusy = "", onPermissionDecision }) {
  const { t } = useTranslation();
  const rounds = items.filter((item) => item.type === "round");
  const lastRound = rounds[rounds.length - 1];
  const userTimeLabel = createTimeLabeler();
  return <div className="conversation-section">
    <div className="conversation-list">
      {items.map((item) => item.type === "user" ? <div className="conversation-user" key={item.id}>
        <div className="conversation-speaker"><span className="conversation-identity"><Circle className="conversation-event-dot user" weight="fill" aria-label={t("timeline.userMessage")} /></span><span className="conversation-message-meta"><time>{userTimeLabel(item.at)}</time></span></div>
        <MessageBody content={item.text} />
      </div> : <ConversationRound key={item.id} round={item} active={Boolean(active) && item === lastRound} permissionBusy={permissionBusy} onPermissionDecision={onPermissionDecision} />)}
      {!items.length && <p className="muted-copy">{t("timeline.empty")}</p>}
    </div>
  </div>;
}

export default memo(ConversationTimeline);
