import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { pairingCountdown } from "./mobileAccess.js";

const panel = readFileSync(new URL("./components/settings/MobileAccessSettings.jsx", import.meta.url), "utf8");
const settings = readFileSync(new URL("./SettingsPage.jsx", import.meta.url), "utf8");

test("a pairing code counts down and disappears once it expires", () => {
  const now = Date.UTC(2026, 8, 6, 12, 0, 0);
  assert.equal(pairingCountdown(new Date(now + 600_000).toISOString(), now), "10:00");
  assert.equal(pairingCountdown(new Date(now + 65_000).toISOString(), now), "1:05");
  assert.equal(pairingCountdown(new Date(now - 1).toISOString(), now), "");
  assert.equal(pairingCountdown("", now), "");
});

test("phone access is its own settings section and is off until asked for", () => {
  assert.match(settings, /sectionMeta = \(t\) => \[[^\]]*"mobile"/);
  assert.match(settings, /section === "mobile" && <MobileAccessSettings/);
  // Nothing here writes a settings section, so the panel must never join the
  // draft/save flow that every other section runs through.
  assert.doesNotMatch(settings, /section === "mobile".*UpdateMobile/s);
  // Reset restores a settings draft, and this section has none to restore.
  assert.match(settings.match(/!\[([^\]]+)\]\.includes\(section\)/)[1], /"mobile"/);
});

test("the panel drives the hosted worker rather than storing a preference", () => {
  assert.match(panel, /WorkerBinding\.StartHostedWorker\(\)/);
  assert.match(panel, /WorkerBinding\.StopHostedWorker\(\)/);
  assert.match(panel, /WorkerBinding\.PairHostedWorker\(\)/);
  // Pairing details only exist while the worker is up.
  assert.match(panel, /\{running && <SettingsSection/);
});
