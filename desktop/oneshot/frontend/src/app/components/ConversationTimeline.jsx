import { memo } from "react";
import { CaretDown, CaretRight, Circle } from "@phosphor-icons/react";
import { formatTime } from "../format.js";

function ToolTimelineItem({ entry, running, stalled, time }) {
  const labels = { tool_use: "TOOL USE", tool_result: "RESULT", file_change: "FILE CHANGE", reasoning: "PROCESS" };
  const failed = Boolean(entry.failed);
  const state = failed ? "失败" : running ? "执行中" : stalled ? "未完成" : entry.kind === "reasoning" ? "过程" : "完成";
  return <details className={`conversation-tool kind-${entry.kind} ${running ? "running" : ""} ${failed ? "failed" : ""} ${stalled ? "stalled" : ""}`}>
    <summary aria-label={`${labels[entry.kind] || entry.kind}: ${entry.title}`}><span className="conversation-tool-summary"><span className="conversation-tool-caret"><CaretRight className="closed" weight="bold" /><CaretDown className="opened" weight="bold" /></span><strong title={entry.text}>{entry.title}</strong><span className="conversation-tool-state">{running && <span className="pulse" />}{state}</span><time>{time ?? formatTime(entry.at)}</time></span></summary>
    <div className="conversation-tool-body"><div><span>{entry.kind === "file_change" ? "PATH" : entry.kind === "reasoning" ? "PROCESS" : "COMMAND"}</span><pre>{entry.text}</pre></div>{entry.details.map((detail, index) => <div key={`${detail.kind}-${index}`}><span>{labels[detail.kind] || detail.kind}</span><pre>{detail.text}</pre></div>)}</div>
  </details>;
}

// The timeline can grow to hundreds of rows; poll-driven parent re-renders must
// not rebuild it unless `items`/`active` actually change, hence memo() paired
// with the memoized conversation array the parent feeds in.
function ConversationTimeline({ items, active }) {
  const rounds = items.filter((item) => item.type === "round");
  // Consecutive events often share the same second; repeating the identical
  // timestamp on every row is noise, so only the first of a same-second run
  // shows it.
  let lastTimeLabel = "";
  const timeLabel = (at) => {
    const label = formatTime(at);
    if (!label || label === lastTimeLabel) return "";
    lastTimeLabel = label;
    return label;
  };
  // A tool reports its own outcome (entry.failed) whenever the runtime answered
  // it. Only when it never answered do we fall back to the step: a step that
  // succeeded left nothing unfinished behind — Codex just does not emit a result
  // event per command — while a step that died mid-flight did.
  const isRunning = (round, entry, index) => Boolean(active) && round === rounds[rounds.length - 1] && index === round.items.length - 1 && entry.kind === "tool_use";
  const isStalled = (round, entry, running) => !entry.settled && !running && round.status !== "succeeded";
  return <div className="conversation-section">
    <div className="conversation-list">
      {items.map((item) => item.type === "user" ? <div className="conversation-user" key={item.id}>
        <div className="conversation-speaker"><span className="conversation-identity"><Circle className="conversation-event-dot user" weight="fill" aria-label="用户消息" /></span><span className="conversation-message-meta"><time>{timeLabel(item.at)}</time></span></div>
        <p>{item.text}</p>
      </div> : <article className="conversation-round" key={item.id}>
        <div className="conversation-round-body">{item.items.map((entry, index) => entry.type === "message" ? <div className={`conversation-agent ${entry.tone}`} key={`message-${index}`}><div className="conversation-speaker"><span className="conversation-identity"><Circle className="conversation-event-dot agent" weight="fill" aria-label="Agent 消息" /><strong>{item.runtime}</strong></span><span className="conversation-message-meta"><span className="conversation-round-index">第 {item.round} 轮</span><time>{timeLabel(entry.at || item.finishedAt || item.startedAt)}</time></span></div><p>{entry.text}</p></div> : <ToolTimelineItem key={`tool-${index}`} entry={entry} time={timeLabel(entry.at)} running={isRunning(item, entry, index)} stalled={isStalled(item, entry, isRunning(item, entry, index))} />)}</div>
      </article>)}
      {!items.length && <p className="muted-copy">Agent 消息会按执行轮次显示在这里。</p>}
    </div>
  </div>;
}

export default memo(ConversationTimeline);
