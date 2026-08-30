import i18n from "../i18n.js";

export function copy(value) { return JSON.parse(JSON.stringify(value)); }

export function shortID(value = "") { return value.length > 18 ? `${value.slice(0, 8)}…${value.slice(-5)}` : value; }

function locale() { return i18n.resolvedLanguage === "en" ? "en-US" : "zh-CN"; }

function sameDay(a, b) { return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate(); }

function clockOf(date) { return date.toLocaleTimeString(locale(), { hour: "2-digit", minute: "2-digit", second: "2-digit" }); }

function twoDigits(value) { return String(value).padStart(2, "0"); }

function dayOf(date, now) {
  const options = date.getFullYear() === now.getFullYear()
    ? { month: "numeric", day: "numeric" }
    : { year: "numeric", month: "numeric", day: "numeric" };
  return date.toLocaleDateString(locale(), options);
}

/* A transcript outlives the day it was recorded, so a bare clock reading stops
   being an answer: "14:02" on a run you opened a week later says nothing about
   which day it ran. The date joins the reading as soon as it is no longer
   today's, and the year as soon as it is no longer this year — today's rows,
   which are the ones you read while the run is still going, stay short. */
export function formatTime(value) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  const now = new Date();
  return sameDay(date, now) ? clockOf(date) : `${dayOf(date, now)} ${clockOf(date)}`;
}

// The unabbreviated stamp, for the places that hold the full answer rather
// than the glanceable one — a title on a timestamp, an exported record.
export function formatDateTime(value) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return `${date.toLocaleDateString(locale(), { year: "numeric", month: "numeric", day: "numeric" })} ${clockOf(date)}`;
}

// A message's hover metadata must still identify the day when the transcript
// is read today; a clock alone cannot distinguish adjacent conversation days.
export function formatMessageDateTime(value) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  const now = new Date();
  const day = `${twoDigits(date.getMonth() + 1)}/${twoDigits(date.getDate())}`;
  const datedDay = date.getFullYear() === now.getFullYear() ? day : `${date.getFullYear()}/${day}`;
  return `${datedDay} ${twoDigits(date.getHours())}:${twoDigits(date.getMinutes())}`;
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
