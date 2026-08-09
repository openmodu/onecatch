import { useTranslation } from "react-i18next";
import { Kicker } from "../../ui/primitives.jsx";
import { formatTime } from "../format.js";

export default function QueuedTaskView({ task, position }) {
  const { t } = useTranslation();
  return <div className="mx-auto max-w-lg px-6 py-10 text-center">
    <div className="mx-auto mb-4 grid size-12 place-items-center rounded-full border-2 border-info/40 bg-info/10 font-mono text-lg font-bold text-info">
      <span>{position}</span>
    </div>
    <Kicker>{t("queue.kicker")}</Kicker>
    <h3 className="mt-2 mb-1.5 text-lg font-semibold text-foreground">{t("queue.waiting")}</h3>
    <p className="m-0 text-sm leading-relaxed text-muted-foreground">{t("queue.description")}</p>
    <dl className="mt-6 grid gap-1.5 text-left">
      {[[t("queue.goal"), task.prompt],
        [t("queue.enqueuedAt"), formatTime(task.queue?.enqueuedAt || task.createdAt)],
        [t("queue.attachments"), t("common.itemsCount", { count: task.attachments?.length || 0 })]].map(([term, value]) => (
        <div className="grid grid-cols-[minmax(0,7rem)_minmax(0,1fr)] gap-3 rounded-md bg-muted/45 px-3 py-2.5" key={term}>
          <dt className="text-xs text-muted-foreground">{term}</dt>
          <dd className="m-0 text-xs leading-relaxed text-foreground">{value}</dd>
        </div>
      ))}
    </dl>
  </div>;
}
