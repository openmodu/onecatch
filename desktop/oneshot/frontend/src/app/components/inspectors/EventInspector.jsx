import { formatTime } from "../../format.js";

function prettyPayload(value) {
  try { return JSON.stringify(JSON.parse(value), null, 2); } catch { return String(value); }
}

export default function EventInspector({ detail }) {
  if (!detail) return <p className="inspector-placeholder">选择运行后显示过程事件。</p>;
  return <div className="event-inspector">{[...(detail.events || [])].reverse().map((event) => <article key={`${event.seq}-${event.type}`}><div><b>{event.seq}</b><strong>{event.type}</strong><time>{formatTime(event.at)}</time></div>{event.stepId && <span>{event.stepId}</span>}{event.payload && event.payload !== "{}" && <pre>{prettyPayload(event.payload)}</pre>}</article>)}{!detail.events?.length && <p className="inspector-placeholder">还没有过程事件。</p>}</div>;
}
