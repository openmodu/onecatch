import React, { lazy, Suspense } from "react";
import { createRoot } from "react-dom/client";
import { Events } from "@wailsio/runtime";
import i18n, { LANGUAGE_CHANGED_EVENT, LANGUAGE_STORAGE_KEY, normalizeLanguage } from "./i18n.js";
import { APPEARANCE_CHANGED_EVENT, ACCENT_STORAGE_KEY, THEME_STORAGE_KEY, applyAppearance, readAppearance } from "./app/appearance.js";
import { desktopPlatform } from "./app/platform.js";
// index.css pulls in the hand-written stylesheets itself, inside @layer legacy,
// so Tailwind utilities outrank them during the migration.
import "./index.css";

applyAppearance(readAppearance());
const launchParameters = new URLSearchParams(window.location.search);
const windowKind = launchParameters.get("window");
const platform = launchParameters.get("platform") === "mobile" ? "mobile" : "desktop";
document.documentElement.dataset.platform = platform === "mobile" ? "mobile" : desktopPlatform();

// Native chrome installs a sidebar material panel (and its hairline) in every
// window, sized for a rail. The detached inspector has no rail, so retire that
// panel here — before React paints — instead of letting it show through.
if (windowKind === "inspector") {
  globalThis.webkit?.messageHandlers?.onecatchSidebar?.postMessage({ hidden: true });
}

// Every native window owns a separate document. Keep their root theme
// attributes in lockstep when Settings changes the shared preference.
const syncAppearance = (event) => applyAppearance(event?.data || readAppearance());
const syncLanguage = (event) => {
  const language = normalizeLanguage(event?.data || i18n.resolvedLanguage);
  if (i18n.resolvedLanguage !== language) void i18n.changeLanguage(language);
};
window.addEventListener("storage", (event) => {
  if (event.key === THEME_STORAGE_KEY || event.key === ACCENT_STORAGE_KEY) syncAppearance();
  if (event.key === LANGUAGE_STORAGE_KEY) syncLanguage({ data: event.newValue });
});
Events.On(APPEARANCE_CHANGED_EVENT, syncAppearance);
Events.On(LANGUAGE_CHANGED_EVENT, syncLanguage);

const WindowRoot = lazy(async () => {
  if (platform === "mobile") {
    return import("./app/MobileApp.jsx");
  }
  if (windowKind === "settings") {
    const module = await import("./app/AuxiliaryWindow.jsx");
    return { default: module.SettingsWindow };
  }
  if (windowKind === "workflows") {
    const module = await import("./app/AuxiliaryWindow.jsx");
    return { default: module.WorkflowsWindow };
  }
  if (windowKind === "inspector") {
    return import("./app/InspectorWindow.jsx");
  }
  return import("./app/App.jsx");
});

function WindowLoading() {
  return <div className="flex h-full items-center justify-center bg-background text-sm text-muted-foreground">{i18n.t("task.opening")}</div>;
}

createRoot(document.getElementById("root")).render(
  <React.StrictMode>
    <Suspense fallback={<WindowLoading />}><WindowRoot /></Suspense>
  </React.StrictMode>,
);
