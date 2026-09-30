import { useCallback, useEffect, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import { UpdateBinding, WindowBinding } from "../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";

export function openAppUpdateWindow(mode) {
  if (mode === "wails") return WindowBinding.OpenUpdates();
  window.open("/?window=updates", "onecatch-updates", "width=400,height=146");
  return Promise.resolve();
}

export function appUpdateWindowSize(state, notesExpanded = false) {
  if (["checking", "downloading", "verifying", "installing"].includes(state)) return { width: 400, height: 146 };
  if (state === "up-to-date") return { width: 260, height: 220 };
  if (notesExpanded) return { width: 380, height: 340 };
  if (["available", "ready", "error"].includes(state)) return { width: 320, height: 260 };
  return { width: 300, height: 240 };
}

export function appUpdateAction(status, busy = false) {
  const state = status?.state;
  if (busy || !status?.verificationEnabled || ["checking", "downloading", "verifying", "installing"].includes(state)) return null;
  if (state === "ready") return status.automaticSupported ? "apply" : null;
  if ((state === "available" || state === "error") && status.availableVersion) return "download";
  return ["idle", "up-to-date", "error"].includes(state) ? "check" : null;
}

export function shouldCheckAppUpdateOnOpen(status) {
  return appUpdateAction(status) === "check";
}

export function appUpdateStateLabel(status, t) {
  const state = status?.state || "unconfigured";
  if (state === "available") return t("settings.updateAvailable", { version: status.availableVersion });
  if (state === "ready") return t("settings.updateReady", { version: status.availableVersion });
  if (state === "up-to-date") return t("settings.updateCurrent");
  if (state === "unconfigured") return t("settings.updateDisabled");
  return t(`settings.updateState.${state}`, { defaultValue: state });
}

export const APP_UPDATE_EVENTS = [
  "wails:updater:check-started",
  "wails:updater:update-available",
  "wails:updater:no-update",
  "wails:updater:download-started",
  "wails:updater:download-complete",
  "wails:updater:verifying",
  "wails:updater:installing",
  "wails:updater:update-ready",
  "wails:updater:error",
  "onecatch:update:status-changed",
];

export function appUpdatePercent(progress) {
  if (!(progress?.total > 0)) return 0;
  return Math.min(100, Math.max(0, Math.round(progress.written / progress.total * 100)));
}

const visibleSidebarUpdateStates = new Set(["downloading", "verifying", "installing", "ready"]);

export function shouldShowSidebarUpdate(status) {
  const state = status?.state;
  if (state === "available") return Boolean(status?.availableVersion);
  if (visibleSidebarUpdateStates.has(state)) return true;
  // Keep a failed download retryable, but do not turn a failed background
  // check into another permanent sidebar control.
  return state === "error" && Boolean(status?.availableVersion);
}

// The workbench sidebar and auxiliary windows are separate React
// roots, so updater state is reconciled from the native service and its event
// stream instead of being owned by a screen. All surfaces consequently
// show the same release and follow the same check/download/apply transitions.
export function useAppUpdate(mode) {
  const [status, setStatus] = useState(null);
  const [progress, setProgress] = useState(null);
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(mode !== "wails");
  const mounted = useRef(true);
  const pendingCheck = useRef(null);

  useEffect(() => {
    // React Strict Mode intentionally performs a setup/cleanup/setup cycle in
    // development. Restore the flag in setup so the second, live pass can
    // still commit the asynchronous check and download results.
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);

  const refresh = useCallback(async () => {
    if (mode !== "wails") return null;
    try {
      const next = await UpdateBinding.GetStatus();
      if (mounted.current) setStatus(next);
      return next;
    } catch {
      // The Wails bindings can be reachable a paint before the updater service
      // finishes booting. Its first status event will reconcile this state.
      return null;
    } finally {
      if (mounted.current) setLoaded(true);
    }
  }, [mode]);

  useEffect(() => {
    if (mode !== "wails") return undefined;
    void refresh();
    const off = APP_UPDATE_EVENTS.map((name) => Events.On(name, () => { void refresh(); }));
    off.push(Events.On("wails:updater:download-progress", (event) => setProgress(event?.data || null)));
    off.push(Events.On("wails:updater:download-started", () => setProgress(null)));
    return () => off.forEach((stop) => stop?.());
  }, [mode, refresh]);

  const perform = useCallback(async (action) => {
    setBusy(true);
    try {
      const next = await action();
      if (mounted.current && next) setStatus(next);
      return next;
    } catch (error) {
      if (mode === "wails") void refresh();
      throw error;
    } finally {
      if (mounted.current) setBusy(false);
    }
  }, [mode, refresh]);

  const check = useCallback(() => perform(async () => {
    if (mode !== "wails") return null;
    const operation = UpdateBinding.Check();
    pendingCheck.current = operation;
    try { return await operation; }
    catch (error) { if (pendingCheck.current !== operation) return null; throw error; }
    finally { if (pendingCheck.current === operation) pendingCheck.current = null; }
  }), [mode, perform]);

  const cancelCheck = useCallback(() => {
    const operation = pendingCheck.current;
    pendingCheck.current = null;
    void operation?.cancel();
  }, []);

  const download = useCallback(() => perform(async () => {
    if (mode !== "wails") return null;
    return UpdateBinding.Download();
  }), [mode, perform]);

  const apply = useCallback(() => perform(async () => {
    if (mode !== "wails") return null;
    return UpdateBinding.Apply();
  }), [mode, perform]);

  return { status, progress, busy, loaded, refresh, check, cancelCheck, download, apply };
}
