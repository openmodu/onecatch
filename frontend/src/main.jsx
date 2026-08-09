import React from "react";
import { createRoot } from "react-dom/client";
import { Events } from "@wailsio/runtime";
import "./i18n.js";
import App from "./app/App.jsx";
import { SettingsWindow, WorkflowsWindow } from "./app/AuxiliaryWindow.jsx";
import { APPEARANCE_CHANGED_EVENT, ACCENT_STORAGE_KEY, THEME_STORAGE_KEY, applyAppearance, readAppearance } from "./app/appearance.js";
// index.css pulls in the hand-written stylesheets itself, inside @layer legacy,
// so Tailwind utilities outrank them during the migration.
import "./index.css";

applyAppearance(readAppearance());

// Every native window owns a separate document. Keep their root theme
// attributes in lockstep when Settings changes the shared preference.
const syncAppearance = () => applyAppearance(readAppearance());
window.addEventListener("storage", (event) => {
  if (event.key === THEME_STORAGE_KEY || event.key === ACCENT_STORAGE_KEY) syncAppearance();
});
Events.On(APPEARANCE_CHANGED_EVENT, syncAppearance);

const windowKind = new URLSearchParams(window.location.search).get("window");
const WindowRoot = windowKind === "settings" ? SettingsWindow : windowKind === "workflows" ? WorkflowsWindow : App;

createRoot(document.getElementById("root")).render(
  <React.StrictMode>
    <WindowRoot />
  </React.StrictMode>,
);
