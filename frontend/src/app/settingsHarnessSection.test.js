import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const source = readFileSync(new URL("./SettingsPage.jsx", import.meta.url), "utf8");

test("places Harness between runtime and terminal in settings navigation", () => {
  assert.match(source, /\["runtime", "harness", "terminal"/);
});

test("separates runtime environment and harness agent controls", () => {
  assert.match(source, /section === "runtime"[\s\S]*?<RuntimeEnvironmentSettings/);
  assert.match(source, /section === "harness"[\s\S]*?<HarnessSettings/);
  assert.match(source, /const runtimeEnvironmentFields = \["integration", "configSource", "configPath", "binary", "environmentAllowlist"\]/);
  assert.match(source, /const harnessAgentFields = \["defaultModel", "reasoningEffort", "serviceTier", "provider"\]/);
});

test("shows every harness agent without nested tabs", () => {
  const harness = source.slice(source.indexOf("function HarnessSettings"), source.indexOf("function ExecutionSettings"));
  assert.doesNotMatch(harness, /<Tabs|TabsList|TabsTrigger|TabsContent/);
  assert.match(harness, /runtimeIds\.map/);
  assert.match(harness, /<RuntimeSettingsCard key=\{id\}/);
});
