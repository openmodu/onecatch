import { useCallback, useEffect, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import { UpdateBinding } from "../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";

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

// The workbench sidebar and the standalone Settings window are separate React
// roots, so updater state is reconciled from the native service and its event
// stream instead of being owned by either screen. Both surfaces consequently
// show the same release and follow the same check/download/apply transitions.
export function useAppUpdate(mode) {
  const [status, setStatus] = useState(null);
  const [progress, setProgress] = useState(null);
  const [busy, setBusy] = useState(false);
  const mounted = useRef(true);

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
    }
  }, [mode]);

  useEffect(() => {
    if (mode !== "wails") return undefined;
    void refresh();
    const off = APP_UPDATE_EVENTS.map((name) => Events.On(name, () => { void refresh(); }));
    off.push(Events.On("wails:updater:download-progress", (event) => setProgress(event?.data || null)));
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
    return UpdateBinding.Check();
  }), [mode, perform]);

  const download = useCallback(() => perform(async () => {
    if (mode !== "wails") return null;
    return UpdateBinding.Download();
  }), [mode, perform]);

  const apply = useCallback(() => perform(async () => {
    if (mode !== "wails") return null;
    return UpdateBinding.Apply();
  }), [mode, perform]);

  return { status, progress, busy, refresh, check, download, apply };
}
