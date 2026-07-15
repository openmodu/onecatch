import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { normalizeLanguage, translationResources } from "./i18n.js";

function sourceFiles(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return sourceFiles(path);
    return /\.(?:js|jsx)$/.test(entry.name) && !entry.name.endsWith(".test.js") ? [path] : [];
  });
}

test("normalizes system locales to the two supported languages", () => {
  assert.equal(normalizeLanguage("zh-CN"), "zh-CN");
  assert.equal(normalizeLanguage("zh-TW"), "zh-CN");
  assert.equal(normalizeLanguage("en-US"), "en");
  assert.equal(normalizeLanguage("fr-FR"), "en");
});

test("Chinese and English resources contain the same keys", () => {
  assert.deepEqual(Object.keys(translationResources.en).sort(), Object.keys(translationResources["zh-CN"]).sort());
});

test("task creation examples contain no buddy wording", () => {
  for (const resources of Object.values(translationResources)) {
    assert.doesNotMatch(resources["task.namePlaceholder"], /buddy/i);
  }
});

test("every static translation key used by the UI exists", () => {
  const files = [...sourceFiles(join(process.cwd(), "src", "app")), ...sourceFiles(join(process.cwd(), "src", "ui"))];
  const usedKeys = files.flatMap((file) => [...readFileSync(file, "utf8").matchAll(/\bt\("([^"]+)"/g)].map((match) => match[1]));
  const missing = [...new Set(usedKeys)].filter((key) => !(key in translationResources["zh-CN"]));
  assert.deepEqual(missing, []);
});

test("English UI resources do not fall back to Chinese copy", () => {
  const mixed = Object.entries(translationResources.en)
    .filter(([key, value]) => key !== "language.chinese" && /\p{Script=Han}/u.test(value))
    .map(([key]) => key);
  assert.deepEqual(mixed, []);
});
