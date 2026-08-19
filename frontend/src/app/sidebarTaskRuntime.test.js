import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const sidebarSource = readFileSync(new URL("./components/Sidebar.jsx", import.meta.url), "utf8");
const composerSource = readFileSync(new URL("./components/Composer.jsx", import.meta.url), "utf8");

test("sidebar tasks show their locked Agent or workflow execution icon", () => {
  assert.match(sidebarSource, /function TaskExecutionIcon/);
  assert.match(sidebarSource, /RuntimeHarnessIcon harness=\{task\?\.harness \|\| "codex"\}/);
  assert.match(sidebarSource, /return <Workflow size=\{14\}/);
  assert.match(sidebarSource, /<TaskExecutionIcon task=\{task\}/);
});

test("existing conversations render the selected Agent as read only", () => {
  assert.match(composerSource, /<HarnessSelector value=\{runtimeProfile\} runtimes=\{runtimes\} readOnly agentLabel/);
  assert.doesNotMatch(composerSource, /<HarnessSelector[^>]*readOnly=\{runStatus === "running"\}/);
});
