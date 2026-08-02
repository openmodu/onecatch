import { useTranslation } from "react-i18next";
import { Kicker } from "../../ui/primitives.jsx";
import { formatTime } from "../format.js";

export default function QueuedTaskView({ task, position }) {
  const { t } = useTranslation();
  return <div className="queued-task-view"><div className="queue-orbit"><span>{position}</span></div><Kicker>{t("queue.kicker")}</Kicker><h3>{t("queue.waiting")}</h3><p>{t("queue.description")}</p><dl><div><dt>{t("queue.goal")}</dt><dd>{task.prompt}</dd></div><div><dt>{t("queue.enqueuedAt")}</dt><dd>{formatTime(task.queue?.enqueuedAt || task.createdAt)}</dd></div><div><dt>{t("queue.attachments")}</dt><dd>{t("common.itemsCount", { count: task.attachments?.length || 0 })}</dd></div></dl></div>;
}
