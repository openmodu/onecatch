import { useTranslation } from "react-i18next";
import { formatTime } from "../../format.js";

function prettyPayload(value) {
  try { return JSON.stringify(JSON.parse(value), null, 2); } catch { return String(value); }
}

export default function EventInspector({ detail }) {
  const { t } = useTranslation();
  if (!detail) return <p className="inspector-placeholder">{t("inspector.eventSelect")}</p>;
  return <div className="event-inspector">{[...(detail.events || [])].reverse().map((event) => <article key={`${event.seq}-${event.type}`}><div><b>{event.seq}</b><strong>{event.type}</strong><time>{formatTime(event.at)}</time></div>{event.stepId && <span>{event.stepId}</span>}{event.payload && event.payload !== "{}" && <pre>{prettyPayload(event.payload)}</pre>}</article>)}{!detail.events?.length && <p className="inspector-placeholder">{t("inspector.eventEmpty")}</p>}</div>;
}
