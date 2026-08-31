import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { CircleAlert, Download, LoaderCircle, RefreshCw, RotateCcw } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { appUpdatePercent, useAppUpdate } from "../appUpdate.js";
import { errorMessage } from "../format.js";
import ProgressRing from "./ProgressRing.jsx";

const activeDownloadStates = new Set(["downloading", "verifying", "installing"]);

export default function SidebarUpdateButton({ mode, notify }) {
  const { t } = useTranslation();
  const { status, progress, busy, check, download, apply } = useAppUpdate(mode);
  const [promptVisible, setPromptVisible] = useState(false);
  const promptedVersion = useRef("");
  const state = status?.state || "unconfigured";
  const percent = appUpdatePercent(progress);
  const available = state === "available" && Boolean(status?.availableVersion);
  const downloading = activeDownloadStates.has(state);
  const actionable = !busy && !downloading && state !== "checking" && state !== "unconfigured" && !(state === "ready" && !status?.automaticSupported);

  useEffect(() => {
    if (!available || !status.automaticSupported || promptedVersion.current === status.availableVersion) return undefined;
    promptedVersion.current = status.availableVersion;
    setPromptVisible(true);
    const timer = window.setTimeout(() => setPromptVisible(false), 7000);
    return () => window.clearTimeout(timer);
  }, [available, status?.automaticSupported, status?.availableVersion]);

  const label = available
    ? t("sidebar.updateDownload", { version: status.availableVersion })
    : state === "ready"
      ? status?.automaticSupported ? t("sidebar.updateRestart", { version: status.availableVersion }) : t("settings.manualUpdateRequired")
      : state === "downloading" && progress?.total > 0
        ? t("sidebar.updateProgress", { percent })
        : state === "verifying" ? t("settings.updateState.verifying")
          : state === "installing" ? t("settings.updateState.installing")
            : state === "checking" ? t("settings.updateState.checking")
              : state === "error" ? t("sidebar.updateRetry")
                : state === "unconfigured" ? t("settings.updateDisabled") : t("settings.checkForUpdates");

  const act = async () => {
    setPromptVisible(false);
    try {
      if (state === "ready") {
        await apply();
        return;
      }
      if (available) {
        await download();
        notify?.("success", t("settings.updateVerified"));
        return;
      }
      const next = await check();
      if (next?.state === "up-to-date") notify?.("success", t("settings.updateCurrent"));
    } catch (error) {
      notify?.("error", errorMessage(error));
    }
  };

  const icon = state === "downloading" && progress?.total > 0
    ? <span className="relative grid size-6 place-items-center"><ProgressRing ratio={percent / 100} size={22} radius={8.5} stroke={2.25} /><span className="absolute text-[8px] font-semibold leading-none tabular-nums text-foreground">{percent}</span></span>
    : downloading || state === "checking" || busy
      ? <LoaderCircle size={17} className="animate-spin" aria-hidden="true" />
      : available ? <Download size={16} strokeWidth={2.2} aria-hidden="true" />
        : state === "ready" ? <RotateCcw size={16} strokeWidth={2.2} aria-hidden="true" />
          : state === "error" ? <CircleAlert size={16} strokeWidth={2.2} aria-hidden="true" />
            : <RefreshCw size={15} strokeWidth={2.1} aria-hidden="true" />;

  const attention = available || state === "ready";
  const failed = state === "error";
  return <div className="sidebar-update-control no-drag absolute right-4 top-3 z-20">
    {promptVisible && <div className="pointer-events-none absolute right-0 bottom-[calc(100%+9px)] w-48 rounded-lg border border-border/80 bg-popover px-3 py-2.5 text-left shadow-lg" role="status" aria-live="polite">
      <strong className="block text-xs font-semibold text-popover-foreground">{t("settings.updateAvailable", { version: status.availableVersion })}</strong>
      <span className="mt-0.5 block text-[11px] leading-snug text-muted-foreground">{t("sidebar.updatePromptAction")}</span>
    </div>}
    <Tooltip>
      <TooltipTrigger asChild>
        <button type="button" className={`sidebar-update-trigger relative grid size-7 place-items-center rounded-full border-0 bg-transparent p-0 shadow-none transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 ${attention ? "bg-primary/10 text-primary hover:bg-primary/18" : failed ? "text-destructive hover:bg-destructive/10" : "text-muted-foreground hover:bg-accent hover:text-foreground"}`} aria-label={label} aria-busy={busy || downloading || state === "checking" || undefined} disabled={!actionable} data-update-state={state} onClick={() => void act()}>
          {icon}
          {available && <i className="absolute top-0 right-0 size-1.5 rounded-full bg-primary ring-2 ring-sidebar" aria-hidden="true" />}
        </button>
      </TooltipTrigger>
      <TooltipContent side="top" align="end" sideOffset={7}>{label}</TooltipContent>
    </Tooltip>
  </div>;
}
