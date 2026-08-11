import assert from "node:assert/strict";
import test from "node:test";
import { normalizeTerminalPreferences, TERMINAL_THEME_IDS } from "./terminalThemes.js";

test("normalizes terminal preferences from persisted settings", () => {
  assert.deepEqual(normalizeTerminalPreferences({ shell: " zsh ", arguments: [" -l ", "", 42], theme: "midnight" }), {
    shell: "zsh", arguments: ["-l", "42"], theme: "midnight",
  });
});

test("falls back to the system terminal theme", () => {
  assert.equal(normalizeTerminalPreferences({ theme: "unknown" }).theme, "system");
  assert.deepEqual(TERMINAL_THEME_IDS, ["system", "paper", "midnight", "contrast"]);
});
