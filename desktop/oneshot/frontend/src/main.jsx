import React from "react";
import { createRoot } from "react-dom/client";
import "./i18n.js";
import App from "./app/App.jsx";
import "./styles.css";
import "./ui/tokens.css";
import "./mirage.css";

createRoot(document.getElementById("root")).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
