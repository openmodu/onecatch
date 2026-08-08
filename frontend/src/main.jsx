import React from "react";
import { createRoot } from "react-dom/client";
import "./i18n.js";
import App from "./app/App.jsx";
import { applyAppearance, readAppearance } from "./app/appearance.js";
// index.css pulls in the hand-written stylesheets itself, inside @layer legacy,
// so Tailwind utilities outrank them during the migration.
import "./index.css";

applyAppearance(readAppearance());

createRoot(document.getElementById("root")).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
