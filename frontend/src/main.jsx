import React from "react";
import { createRoot } from "react-dom/client";
import "./i18n.js";
import App from "./app/App.jsx";
import { applyAppearance, readAppearance } from "./app/appearance.js";
import "./styles.css";
import "./ui/tokens.css";
import "./mirage.css";

applyAppearance(readAppearance());

createRoot(document.getElementById("root")).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
