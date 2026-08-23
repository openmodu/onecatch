import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const iconSource = readFileSync(new URL("./components/RuntimeHarnessIcon.jsx", import.meta.url), "utf8");
const harnessSelectorSource = readFileSync(new URL("./components/HarnessSelector.jsx", import.meta.url), "utf8");
const taskExecutorSource = readFileSync(new URL("./components/TaskExecutorSelector.jsx", import.meta.url), "utf8");
const codexIcon = readFileSync(new URL("../../public/assets/runtime/codex.svg", import.meta.url), "utf8");
const claudeCodeIcon = readFileSync(new URL("../../public/assets/runtime/claude-code.svg", import.meta.url), "utf8");
const grokIcon = readFileSync(new URL("../../public/assets/runtime/grok.svg", import.meta.url), "utf8");
const deepseekIcon = readFileSync(new URL("../../public/assets/runtime/deepseek.svg", import.meta.url), "utf8");
const piIcon = readFileSync(new URL("../../public/assets/runtime/pi.svg", import.meta.url), "utf8");

test("runtime harnesses have distinct icons", () => {
  // Marks resolve by harness id; only the differently-named files need an
  // entry, so adding a harness means dropping in one asset.
  assert.match(iconSource, /`\/assets\/runtime\/\$\{harness\}\.svg`/);
  assert.match(iconSource, /claude:\s*"\/assets\/runtime\/claude-code\.svg"/);
  assert.match(iconSource, /modu:\s*"\/assets\/runtime\/modu-code\.png"/);
  assert.match(iconSource, /dsh:\s*"\/assets\/runtime\/deepseek\.svg"/);
  assert.match(codexIcon, /<title>Codex<\/title>/);
  assert.match(claudeCodeIcon, /<title>Claude Code<\/title>/);
  assert.match(grokIcon, /<title>Grok<\/title>/);
  assert.match(deepseekIcon, /<title>DeepSeek<\/title>/);
  assert.match(piIcon, /<title>Pi<\/title>/);
});

// Single-colour marks render black inside an <img>, which cannot inherit the
// page's text colour, so a dark theme has to invert them. The DeepSeek mark is
// full colour and inverting it would wreck the brand palette.
test("single-colour runtime marks are inverted in dark themes", () => {
  const appStyles = readFileSync(new URL("../index.css", import.meta.url), "utf8");
  for (const harness of ["modu", "pi", "grok"]) {
    assert.match(appStyles, new RegExp(`runtime-harness-icon-${harness}`), `${harness} needs a dark-theme inversion`);
  }
  assert.doesNotMatch(appStyles, /runtime-harness-icon-dsh/, "the DeepSeek colour mark must not be inverted");
});

test("harness selectors show icons for the selected runtime and menu options", () => {
  assert.match(harnessSelectorSource, /RuntimeHarnessIcon harness=\{harness\.id\}/);
  assert.match(harnessSelectorSource, /RuntimeHarnessIcon harness=\{option\.value\}/);
  assert.match(taskExecutorSource, /RuntimeHarnessIcon harness=\{selectedHarness\.id\}/);
  assert.match(taskExecutorSource, /RuntimeHarnessIcon harness=\{option\.value\}/);
});
