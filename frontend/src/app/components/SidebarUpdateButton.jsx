import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { CircleAlert, Download, LoaderCircle, RotateCcw } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { appUpdatePercent, openAppUpdateWindow, shouldShowSidebarUpdate, useAppUpdate } from "../appUpdate.js";
import { errorMessage } from "../format.js";
import ProgressRing from "./ProgressRing.jsx";

const activeDownloadStates = new Set(["downloading", "verifying", "installing"]);

export default function SidebarUpdateButton({ mode, notify }) {
  const { t } = useTranslation();
  const { status, progress } = useAppUpdate(mode);
  const [promptVisible, setPromptVisible] = useState(false);
  const promptedVersion = useRef("");
  const state = status?.state || "unconfigured";
  const percent = appUpdatePercent(progress);
  const available = state === "available" && Boolean(status?.availableVersion);
  const downloading = activeDownloadStates.has(state);
  const visible = shouldShowSidebarUpdate(status);

  useEffect(() => {
    if (!available || !status.automaticSupported || promptedVersion.current === status.availableVersion) return undefined;
    promptedVersion.current = status.availableVersion;
    setPromptVisible(true);
  }, [available, status?.automaticSupported, status?.availableVersion]);

  useEffect(() => {
    if (!promptVisible) return undefined;
    const timer = window.setTimeout(() => setPromptVisible(false), 7000);
    return () => window.clearTimeout(timer);
  }, [promptVisible]);

  if (!visible) return null;

  const label = available
    ? t("sidebar.updateDownload", { version: status.availableVersion })
    : state === "ready"
      ? t("sidebar.updateRestart", { version: status.availableVersion })
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
      await openAppUpdateWindow(mode);
    } catch (error) {
      notify?.("error", errorMessage(error));
    }
  };

  const icon = state === "downloading" && progress?.total > 0
    ? <span className="relative grid size-6 place-items-center"><ProgressRing ratio={percent / 100} size={22} radius={8.5} stroke={2.25} /><span className="absolute text-[8px] font-semibold leading-none tabular-nums text-foreground">{percent}</span></span>
    : downloading || state === "checking"
      ? <LoaderCircle size={17} className="animate-spin" aria-hidden="true" />
      : available ? <Download size={15} strokeWidth={2} aria-hidden="true" />
        : state === "ready" ? <RotateCcw size={14} strokeWidth={2.2} aria-hidden="true" />
          : <CircleAlert size={16} strokeWidth={2.2} aria-hidden="true" />;

  const ready = state === "ready";
  const failed = state === "error";
  return <div className="sidebar-update-control no-drag relative grid size-9 shrink-0 place-items-center">
    {promptVisible && <div className="pointer-events-none absolute right-0 bottom-[calc(100%+9px)] w-48 rounded-lg bg-popover px-3 py-2.5 text-left shadow-lg" role="status" aria-live="polite">
      <strong className="block text-xs font-semibold text-popover-foreground">{t("settings.updateAvailable", { version: status.availableVersion })}</strong>
      <span className="mt-0.5 block text-[11px] leading-snug text-muted-foreground">{t("sidebar.updatePromptAction")}</span>
    </div>}
    <Tooltip>
      <TooltipTrigger asChild>
        <button type="button" className={`sidebar-update-trigger relative grid size-9 place-items-center rounded-lg border-0 bg-transparent p-0 shadow-none transition-[color,background-color,transform] focus-visible:outline-none focus-visible:bg-sidebar-accent ${available ? "text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground active:scale-95" : ready ? "text-primary-foreground hover:bg-sidebar-accent active:scale-95" : failed ? "text-destructive hover:bg-destructive/10" : "text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"}`} aria-label={label} data-update-state={state} onClick={() => void act()}>
          {ready ? <span className="grid size-6 place-items-center rounded-[6px] bg-primary shadow-xs">{icon}</span> : icon}
        </button>
      </TooltipTrigger>
      <TooltipContent side="top" align="end" sideOffset={7}>{label}</TooltipContent>
    </Tooltip>
  </div>;
}
