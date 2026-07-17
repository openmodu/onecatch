import { useTranslation } from "react-i18next";
import { Action, SettingsModule, StatusBadge } from "../../ui/primitives.jsx";

export default function WorkerPage({ workers, health, checkWorker, deleteWorker, openWorker }) {
  const { t } = useTranslation();
  return <div className="worker-page">
    <SettingsModule title={t("worker.title")} description={t("worker.description")} aside={<Action tone="primary" onClick={() => openWorker(null)}>{t("worker.register")}</Action>} bodyClassName="worker-list-body">
      <div className="worker-grid">{workers.map((worker) => <article className="worker-card" key={worker.id}>
        <div className="worker-card-head"><div className="worker-machine"><span><h4>{worker.name}</h4><small>{worker.id}</small></span></div><StatusBadge status={worker.enabled ? "completed" : "cancelled"} className="status-pill">{worker.enabled ? t("common.enabled") : t("common.disabled")}</StatusBadge></div>
        <code>{worker.baseUrl}</code>
        <div className="worker-health">{health[worker.id]?.checking ? t("worker.checking") : health[worker.id]?.ok ? <>{Object.entries(health[worker.id].runtimes || {}).map(([runtime, ok]) => <span key={runtime} className={ok ? "ok" : "missing"}><i />{runtime}</span>)}</> : health[worker.id]?.error || t("worker.notChecked")}</div>
        <div className="worker-actions"><Action onClick={() => checkWorker(worker)}>{t("worker.health")}</Action><Action onClick={() => openWorker(worker)}>{t("common.edit")}</Action><Action tone="danger" onClick={() => deleteWorker(worker.id)}>{t("common.delete")}</Action></div>
      </article>)}{!workers.length && <div className="empty-state"><h4>{t("worker.empty")}</h4><p>{t("worker.emptyDescription")}</p></div>}</div>
    </SettingsModule>
    <SettingsModule title={t("worker.command")} description={t("worker.networkWarning")} bodyClassName="worker-command">
      <code>ONESHOT_WORKER_TOKEN=... oneshot-worker --listen 0.0.0.0:9231 --id mac-mini --workspace workspace-id=/path/to/clone</code>
    </SettingsModule>
  </div>;
}
