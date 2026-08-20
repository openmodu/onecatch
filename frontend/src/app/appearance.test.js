import assert from "node:assert/strict";
import test from "node:test";
import { ACCENT_STORAGE_KEY, CHAT_FONT_SIZE_STORAGE_KEY, THEME_STORAGE_KEY, applyAppearance, normalizeAccentTheme, normalizeChatFontSize, normalizeThemeMode, readAppearance, saveAppearance } from "./appearance.js";

function storage(values = {}) {
  return {
    values: { ...values },
    getItem(key) { return this.values[key] ?? null; },
    setItem(key, value) { this.values[key] = value; },
  };
}

test("appearance preferences normalize invalid persisted values", () => {
  assert.equal(normalizeThemeMode("sepia"), "system");
  assert.equal(normalizeAccentTheme("red"), "forest");
  assert.equal(normalizeChatFontSize("huge"), "standard");
  assert.deepEqual(readAppearance(storage({ [THEME_STORAGE_KEY]: "dark", [ACCENT_STORAGE_KEY]: "ocean", [CHAT_FONT_SIZE_STORAGE_KEY]: "large" })), { theme: "dark", accent: "ocean", chatFontSize: "large" });
});

test("appearance preferences persist and update root attributes", () => {
  const target = storage();
  const root = { dataset: { theme: "dark" }, style: {} };
  const saved = saveAppearance({ theme: "light", accent: "violet", chatFontSize: "extra-large" }, target);
  applyAppearance(saved, root);
  assert.deepEqual(saved, { theme: "light", accent: "violet", chatFontSize: "extra-large" });
  assert.equal(target.values[THEME_STORAGE_KEY], "light");
  assert.equal(target.values[ACCENT_STORAGE_KEY], "violet");
  assert.equal(target.values[CHAT_FONT_SIZE_STORAGE_KEY], "extra-large");
  assert.deepEqual(root.dataset, { theme: "light", accent: "violet", chatFontSize: "extra-large" });
  assert.equal(root.style.colorScheme, "light");

  applyAppearance({ theme: "system", accent: "forest", chatFontSize: "small" }, root);
  assert.deepEqual(root.dataset, { accent: "forest", chatFontSize: "small" });
  assert.equal(root.style.colorScheme, "light dark");
});
