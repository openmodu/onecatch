import assert from "node:assert/strict";
import test from "node:test";
import { ACCENT_STORAGE_KEY, THEME_STORAGE_KEY, applyAppearance, normalizeAccentTheme, normalizeThemeMode, readAppearance, saveAppearance } from "./appearance.js";

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
  assert.deepEqual(readAppearance(storage({ [THEME_STORAGE_KEY]: "dark", [ACCENT_STORAGE_KEY]: "ocean" })), { theme: "dark", accent: "ocean" });
});

test("appearance preferences persist and update root attributes", () => {
  const target = storage();
  const root = { dataset: { theme: "dark" }, style: {} };
  const saved = saveAppearance({ theme: "light", accent: "violet" }, target);
  applyAppearance(saved, root);
  assert.deepEqual(saved, { theme: "light", accent: "violet" });
  assert.equal(target.values[THEME_STORAGE_KEY], "light");
  assert.equal(target.values[ACCENT_STORAGE_KEY], "violet");
  assert.deepEqual(root.dataset, { theme: "light", accent: "violet" });
  assert.equal(root.style.colorScheme, "light");

  applyAppearance({ theme: "system", accent: "forest" }, root);
  assert.deepEqual(root.dataset, { accent: "forest" });
  assert.equal(root.style.colorScheme, "light dark");
});
