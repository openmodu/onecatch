import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Events, Window } from "@wailsio/runtime";
import { Button } from "@/components/ui/button";
import appIcon from "../../../internal/app/desktop/assets/appicon.png";
import AuxWindowCloseButton from "./AuxWindowCloseButton.jsx";
import { auxiliaryWindowShownEvent } from "./auxiliaryWindowEvents.js";
import { appUpdateAction, appUpdatePercent, appUpdateWindowSize, shouldCheckAppUpdateOnOpen, useAppUpdate } from "./appUpdate.js";
import { errorMessage } from "./format.js";

const transferStates = new Set(["downloading", "verifying", "installing"]);
const formatBytes = (bytes = 0) => bytes < 1048576 ? `${(bytes / 1024).toFixed(0)} KB` : `${(bytes / 1048576).toFixed(1)} MB`;

export function AppUpdatePanel({ status, progress, busy, loaded = true, error = "", onCheck, onDownload, onApply, onClose, onCancel = onClose, onSizeChange }) {
  const { t } = useTranslation();
  const [notesExpanded, setNotesExpanded] = useState(false);
  const state = status?.state || "unconfigured";
  const action = appUpdateAction(status, busy);
  const checking = !loaded || state === "checking";
  const transferring = transferStates.has(state);
  const compact = checking || transferring;
  const current = state === "up-to-date";
  const percent = appUpdatePercent(progress);
  const failure = error || status?.error;
  const title = checking ? t("settings.updateState.checking")
    : current ? t("updateWindow.currentTitle")
      : state === "ready" ? t("updateWindow.readyTitle")
        : state === "available" ? t("updateWindow.availableTitle")
          : failure ? t("settings.updateState.error")
            : transferring ? t(`settings.updateState.${state}`)
              : t("updateWindow.previewTitle");
  const description = current ? t("updateWindow.currentDescription", { version: status?.currentVersion || "—" })
    : state === "ready" ? t("updateWindow.readyDescription")
      : state === "available" ? t("updateWindow.availableDescription", { version: status.availableVersion, currentVersion: status.currentVersion || "—" })
        : !status ? t("updateWindow.previewDescription")
          : state === "unconfigured" ? t("settings.updateDisabled") : "";
  const actionLabel = action === "apply" ? t("settings.restartToUpdate")
    : action === "download" ? t(state === "error" ? "updateWindow.retryDownload" : "updateWindow.download")
      : t("settings.checkForUpdates");

  useEffect(() => setNotesExpanded(false), [status?.availableVersion, compact]);
  useLayoutEffect(() => {
    const size = appUpdateWindowSize(checking ? "checking" : failure ? "error" : state, notesExpanded && Boolean(status?.notes));
    onSizeChange?.(size);
  }, [checking, failure, state, notesExpanded, status?.notes, onSizeChange]);

  return <div className={`app-update-window relative flex h-full min-h-0 select-none flex-col overflow-hidden bg-popover text-popover-foreground ${compact ? "is-progress" : "is-result"}`} onKeyDown={(event) => { if (event.key === "Escape") { event.preventDefault(); checking ? onCancel() : onClose(); } }}>
    {compact ? <header className="drag-region flex h-8 shrink-0 cursor-default items-center border-b border-border/50 px-5 text-xs font-semibold text-muted-foreground">{t("settings.appUpdate")}</header> : <div className="drag-region absolute top-0 right-0 left-0 h-6" />}
    <AuxWindowCloseButton allowMaximise={false} />
    {compact ? <main className="flex min-h-0 flex-1 gap-3.5 px-[26px] pt-2.5 pb-5">
      <img src={appIcon} alt="" aria-hidden="true" className="size-[52px] shrink-0 object-contain" />
      <div className="flex min-w-0 flex-1 flex-col pt-1">
        <h1 className="m-0 text-[13px] leading-5 font-semibold" role="status" aria-live="polite">{title}</h1>
        <div className="app-update-progress mt-3 h-2 shrink-0 overflow-hidden rounded-full bg-muted" role="progressbar" aria-label={title} aria-valuemin="0" aria-valuemax="100" aria-valuenow={state === "downloading" && progress?.total > 0 ? percent : undefined}>
          <div className={`h-full rounded-full bg-[#007aff] ${state === "downloading" && progress?.total > 0 ? "transition-[width]" : "app-update-indeterminate"}`} style={state === "downloading" && progress?.total > 0 ? { width: `${percent}%` } : undefined} />
        </div>
        {state === "downloading" && <p className="mt-1 mb-0 text-[10px] leading-3 tabular-nums text-muted-foreground">{progress?.total > 0 ? `${percent}% · ${formatBytes(progress.written)} / ${formatBytes(progress.total)}` : formatBytes(progress?.written)}</p>}
        <div className="mt-auto flex justify-end"><Button variant="secondary" size="sm" className="h-7 min-w-[100px] rounded-full border-0 text-xs font-medium shadow-none" onClick={checking ? onCancel : onClose}>{t(checking ? "common.cancel" : "common.close")}</Button></div>
      </div>
    </main> : <>
      <main className="flex min-h-0 flex-1 flex-col overflow-y-auto px-[22px] pt-[26px]">
        <img src={appIcon} alt="" aria-hidden="true" className="ml-1 size-[52px] shrink-0 object-contain" />
        <h1 className="mt-[22px] mb-0 text-[13px] leading-5 font-semibold" role="status" aria-live="polite">{title}</h1>
        {description && <p className="mt-2 mb-0 text-[13px] leading-[18px] break-words">{description}</p>}
        {status?.verificationEnabled && !status.automaticSupported && <p className="mt-2 mb-0 text-xs leading-[18px] text-muted-foreground">{t("settings.manualUpdateRequired")}</p>}
        {failure && <p className="mt-2 mb-0 select-text text-xs leading-[18px] break-words text-destructive" role="alert">{failure}</p>}
        {status?.notes && !current && <section className="mt-2">
          <button type="button" className="border-0 bg-transparent p-0 text-xs text-muted-foreground underline underline-offset-4 hover:text-foreground" aria-expanded={notesExpanded} aria-controls="update-release-notes" onClick={() => setNotesExpanded((value) => !value)}>{t(notesExpanded ? "updateWindow.hideReleaseNotes" : "updateWindow.releaseNotes")}</button>
          {notesExpanded && <p id="update-release-notes" className="mt-2 mb-0 select-text text-xs leading-[18px] whitespace-pre-wrap break-words text-muted-foreground">{status.notes}</p>}
        </section>}
      </main>
      <footer className="flex shrink-0 gap-2 px-4 pt-2.5 pb-4">
        {action && !current ? <>
          <Button variant="secondary" size="sm" className="h-7 rounded-full border-0 px-3 text-xs shadow-none" onClick={onClose}>{t("updateWindow.later")}</Button>
          <Button size="sm" className="app-update-primary bg-[#007aff] text-white hover:bg-[#006ce3] focus-visible:ring-0 focus-visible:border-transparent focus-visible:brightness-95 h-7 min-w-0 flex-1 rounded-full border-0 px-3 text-xs shadow-none" onClick={action === "download" ? onDownload : action === "apply" ? onApply : onCheck}>{actionLabel}</Button>
        </> : <Button autoFocus size="sm" className="app-update-primary bg-[#007aff] text-white hover:bg-[#006ce3] focus-visible:ring-0 focus-visible:border-transparent focus-visible:brightness-95 h-7 w-full rounded-full border-0 text-[13px] shadow-none" onClick={onClose}>{t("updateWindow.ok")}</Button>}
      </footer>
    </>}
  </div>;
}

export default function AppUpdateWindow() {
  const updater = useAppUpdate("wails");
  const { status, loaded, refresh, check, cancelCheck, download, apply } = updater;
  const [error, setError] = useState("");
  const initialCheck = useRef(false);
  const opening = useRef(false);
  const closed = useRef(false);
  const run = useCallback(async (action) => {
    setError("");
    try { await action(); } catch (failure) { setError(errorMessage(failure)); }
  }, []);
  const checkOnOpen = useCallback(async (snapshot) => {
    if (opening.current || closed.current) return;
    opening.current = true;
    try {
      await run(async () => {
        const latest = snapshot || await refresh();
        if (!closed.current && shouldCheckAppUpdateOnOpen(latest)) await check();
      });
    } finally { opening.current = false; }
  }, [refresh, check, run]);

  useEffect(() => {
    if (!loaded || initialCheck.current) return;
    initialCheck.current = true;
    void checkOnOpen(status);
  }, [loaded, status, checkOnOpen]);
  useEffect(() => Events.On(auxiliaryWindowShownEvent, (event) => {
    if (event?.data?.name === "updates") { closed.current = false; void checkOnOpen(); }
  }), [checkOnOpen]);

  const resizeWindow = useCallback(({ width, height }) => { void Window.SetSize(width, height); }, []);
  const closeWindow = () => { closed.current = true; void Window.Close(); };
  const cancel = () => { cancelCheck(); closeWindow(); };

  return <AppUpdatePanel {...updater} onSizeChange={resizeWindow} onCancel={cancel} error={error} onCheck={() => void run(check)} onDownload={() => void run(download)} onApply={() => void run(apply)} onClose={closeWindow} />;
}
