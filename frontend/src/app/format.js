import i18n from "../i18n.js";

export function copy(value) { return JSON.parse(JSON.stringify(value)); }

export function shortID(value = "") { return value.length > 18 ? `${value.slice(0, 8)}…${value.slice(-5)}` : value; }

export function formatTime(value) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleTimeString(i18n.resolvedLanguage === "en" ? "en-US" : "zh-CN", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

export function errorMessage(error) {
  const message = String(error?.message || error || i18n.t("common.unknownError")).replace(/^Error:\s*/, "");
  const code = message.match(/^([a-z][a-z0-9_]+):/)?.[1];
  return code && i18n.exists(`error.${code}`) ? i18n.t(`error.${code}`) : message;
}

export function formatDuration(value = 0) {
  const milliseconds = Math.max(0, Number(value) || 0);
  if (milliseconds < 1000) return `${milliseconds}ms`;
  const seconds = Math.round(milliseconds / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m ${seconds % 60}s`;
}

export function formatTokens(value = 0) { return new Intl.NumberFormat(i18n.resolvedLanguage === "en" ? "en-US" : "zh-CN").format(Number(value) || 0); }

export function fileName(value = "") { return String(value).split(/[\\/]/).pop() || value; }

export function taskTitleFromPrompt(value = "", fallback = "") {
  const firstLine = String(value)
    .split(/\r?\n/)
    .map((line) => line.trim())
    .find(Boolean)
    ?.replace(/^(?:#{1,6}|[-*+]|\d+[.)])\s+/, "")
    .replace(/\s+/g, " ")
    .trim();
  if (!firstLine) return fallback;
  const characters = Array.from(firstLine);
  return characters.length > 48 ? `${characters.slice(0, 47).join("")}…` : firstLine;
}
