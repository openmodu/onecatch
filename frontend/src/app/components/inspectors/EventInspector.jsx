import { useTranslation } from "react-i18next";
import { formatTime } from "../../format.js";

function prettyPayload(value) {
  try { return JSON.stringify(JSON.parse(value), null, 2); } catch { return String(value); }
}

export default function EventInspector({ detail }) {
  const { t } = useTranslation();
  const placeholder = "m-0 px-4 py-5 text-xs leading-relaxed text-muted-foreground";
  if (!detail) return <p className={placeholder}>{t("inspector.eventSelect")}</p>;
  return <div className="grid gap-2 p-3.5">
    {[...(detail.events || [])].reverse().map((event) => <article className="grid gap-1.5 rounded-md border bg-card p-2.5" key={`${event.seq}-${event.type}`}>
      <div className="flex items-center gap-2">
        <b className="font-mono text-[11px] text-muted-foreground">{event.seq}</b>
        <strong className="min-w-0 truncate text-xs font-medium text-foreground">{event.type}</strong>
        <time className="ml-auto shrink-0 font-mono text-[11px] text-muted-foreground">{formatTime(event.at)}</time>
      </div>
      {event.stepId && <span className="font-mono text-[11px] text-muted-foreground">{event.stepId}</span>}
      {event.payload && event.payload !== "{}" && <pre className="m-0 overflow-x-auto rounded-sm bg-muted p-2 font-mono text-[11px] leading-relaxed text-muted-foreground">{prettyPayload(event.payload)}</pre>}
    </article>)}
    {!detail.events?.length && <p className={placeholder}>{t("inspector.eventEmpty")}</p>}
  </div>;
}
