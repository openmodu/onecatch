import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const iconSource = readFileSync(new URL("./components/RuntimeHarnessIcon.jsx", import.meta.url), "utf8");
const harnessSelectorSource = readFileSync(new URL("./components/HarnessSelector.jsx", import.meta.url), "utf8");
const taskExecutorSource = readFileSync(new URL("./components/TaskExecutorSelector.jsx", import.meta.url), "utf8");
const codexIcon = readFileSync(new URL("../../public/assets/runtime/codex.svg", import.meta.url), "utf8");
const claudeCodeIcon = readFileSync(new URL("../../public/assets/runtime/claude-code.svg", import.meta.url), "utf8");

test("runtime harnesses have distinct icons", () => {
  assert.match(iconSource, /codex:\s*"\/assets\/runtime\/codex\.svg"/);
  assert.match(iconSource, /claude:\s*"\/assets\/runtime\/claude-code\.svg"/);
  assert.match(iconSource, /modu:\s*"\/assets\/runtime\/modu-code\.png"/);
  assert.match(codexIcon, /<title>Codex<\/title>/);
  assert.match(claudeCodeIcon, /<title>Claude Code<\/title>/);
});

test("harness selectors show icons for the selected runtime and menu options", () => {
  assert.match(harnessSelectorSource, /RuntimeHarnessIcon harness=\{harness\.id\}/);
  assert.match(harnessSelectorSource, /RuntimeHarnessIcon harness=\{option\.value\}/);
  assert.match(taskExecutorSource, /RuntimeHarnessIcon harness=\{selectedHarness\.id\}/);
  assert.match(taskExecutorSource, /RuntimeHarnessIcon harness=\{option\.value\}/);
});
