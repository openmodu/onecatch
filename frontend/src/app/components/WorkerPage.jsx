import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { SettingsButton, SettingsSection } from "./settings/SettingsControls.jsx";
import { workerLabel } from "../taskWorkers.js";

export default function WorkerPage({ workers, health, checkWorker, deleteWorker, openWorker }) {
  const { t } = useTranslation();
  const checkRef = useRef(checkWorker);
  checkRef.current = checkWorker;
  const pollKey = workers.filter((worker) => worker.enabled).map((worker) => worker.id).sort().join("\x00");
  useEffect(() => {
    const poll = () => workers.filter((worker) => worker.enabled).forEach((worker) => { void checkRef.current(worker); });
    poll();
    const timer = window.setInterval(poll, 15000);
    return () => window.clearInterval(timer);
  }, [pollKey]);

  return <SettingsSection title={t("worker.configuredEnvironments")} aside={<SettingsButton tone="primary" onClick={() => openWorker(null)}>{t("worker.addEnvironment")}</SettingsButton>}>
    <div className="min-w-0 divide-y divide-border/60">{workers.map((worker) => {
      const status = health[worker.id];
      const sandbox = worker.provider === "volcengine-sandbox";
      const label = !worker.enabled ? t("common.disabled") : status?.ok ? t("worker.connected") : status?.checking ? t("worker.checking") : t("worker.disconnected");
      return <div className="flex min-w-0 flex-wrap items-center gap-3 px-4 py-4" key={worker.id}>
        <div className="min-w-0 flex-1">
          <strong className="block truncate text-sm font-medium">{workerLabel(worker)}</strong>
          <span className="text-xs text-muted-foreground">{sandbox ? (worker.syncLocal ? t("worker.syncOnRun") : "volc sandbox") : t("worker.standardWorker")}</span>
        </div>
        <span className={`text-xs ${worker.enabled && status?.ok ? "text-success" : "text-muted-foreground"}`} title={status?.error || undefined}>{label}</span>
        <div className="flex shrink-0 gap-2">
          <SettingsButton compact tone="muted" disabled={status?.checking} onClick={() => checkWorker(worker)}>{t("worker.testConnection")}</SettingsButton>
          <SettingsButton compact tone="muted" onClick={() => openWorker(worker)}>{t("common.edit")}</SettingsButton>
          <SettingsButton compact tone="danger" onClick={() => deleteWorker(worker.id)}>{t("common.delete")}</SettingsButton>
        </div>
      </div>;
    })}{!workers.length && <p className="m-0 px-4 py-6 text-sm text-muted-foreground">{t("worker.noEnvironments")}</p>}</div>
  </SettingsSection>;
}
