import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const source = readFileSync(new URL("./SettingsPage.jsx", import.meta.url), "utf8");

test("places Harness between runtime and terminal in settings navigation", () => {
  assert.match(source, /\["runtime", "harness", "terminal"/);
});

test("keeps appearance separate and moves all runtime controls into Harness", () => {
  assert.match(source, /section === "runtime" && <InterfaceSettings/);
  assert.match(source, /section === "harness" && <HarnessSettings/);
  assert.doesNotMatch(source, /function RuntimeEnvironmentSettings/);
  assert.match(source, /const harnessFields = \["integration", "configSource", "configPath", "binary", "environmentAllowlist", "defaultModel", "reasoningEffort", "serviceTier", "provider"\]/);
});

test("shows every harness agent without nested tabs", () => {
  const harness = source.slice(source.indexOf("function HarnessSettings"), source.indexOf("function ExecutionSettings"));
  assert.doesNotMatch(harness, /<Tabs|TabsList|TabsTrigger|TabsContent/);
  assert.match(harness, /runtimeIds\.map/);
  assert.match(harness, /<section key=\{id\}>/);
  assert.match(harness, /settings\.integrationMode/);
  assert.match(harness, /settings\.moduConfigSource/);
  assert.match(harness, /settings\.binaryPath/);
  assert.match(harness, /settings\.defaultModel/);
});

test("uses a frameless collapsible list for harness configuration", () => {
  assert.doesNotMatch(source, /settings\.harnessAgents/);
  const harness = source.slice(source.indexOf("function HarnessSettings"), source.indexOf("function ExecutionSettings"));
  assert.match(harness, /aria-expanded=\{isExpanded\}/);
  assert.match(harness, /aria-controls=\{panelID\}/);
  assert.match(harness, /divide-y divide-border\/70 border-y/);
});
