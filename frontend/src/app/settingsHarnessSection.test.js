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
  assert.match(source, /const harnessFields = \["enabled", "remoteFsEnabled", "integration", "configSource", "configPath", "binary", "environmentAllowlist", "defaultModel", "reasoningEffort", "serviceTier", "maxContextWindow", "provider"\]/);
});

test("shows every harness agent without nested tabs", () => {
  const harness = source.slice(source.indexOf("function HarnessSettings"), source.indexOf("function ExecutionSettings"));
  assert.doesNotMatch(harness, /<Tabs|TabsList|TabsTrigger|TabsContent/);
  // The section iterates the backend's catalog rather than a list of harness
  // ids kept here, so a new harness appears without editing this file.
  assert.match(harness, /catalog\.map/);
  assert.match(harness, /<section key=\{id\}>/);
  assert.match(harness, /settings\.integrationMode/);
  assert.match(harness, /settings\.moduConfigSource/);
  assert.match(harness, /settings\.binaryPath/);
  assert.match(harness, /settings\.defaultModel/);
  assert.match(harness, /onCheckedChange=\{\(checked\) => update\(id, "enabled", checked\)\}/);
  assert.match(harness, /disabled=\{!supportsRemoteFS \|\| !enabled\}/);
  assert.match(harness, /update\(id, "remoteFsEnabled", checked\)/);
});

test("uses a frameless collapsible list for harness configuration", () => {
  assert.doesNotMatch(source, /settings\.harnessAgents/);
  const harness = source.slice(source.indexOf("function HarnessSettings"), source.indexOf("function ExecutionSettings"));
  assert.match(harness, /<h2 id="harness-list-title"[\s\S]*?settings\.harnessList/);
  assert.match(harness, /aria-expanded=\{isExpanded\}/);
  assert.match(harness, /aria-controls=\{panelID\}/);
  assert.match(harness, /divide-y divide-border\/70 border-y/);
  assert.match(harness, /const \[expanded, setExpanded\] = useState\(\{\}\)/, "every harness must start collapsed");
});

test("native harness disclosures remain named, keyboard-accessible controls", () => {
  const harness = source.slice(source.indexOf("function HarnessSettings"), source.indexOf("function ExecutionSettings"));
  const buttons = [...harness.matchAll(/<button\b[\s\S]*?<\/button>/g)].map(([button]) => button);
  assert.equal(buttons.length, 2, "only the identity area and chevron are native disclosure buttons");
  for (const button of buttons) {
    assert.match(button, /type="button"/);
    assert.match(button, /aria-expanded=\{isExpanded\}/);
    assert.match(button, /aria-controls=\{panelID\}/);
    assert.match(button, /onClick=\{\(\) => setExpanded\(\(current\) => \(\{ \.\.\.current, \[id\]: !current\[id\] \}\)\)\}/);
    assert.match(button, /focus-visible:ring-2/);
    assert.doesNotMatch(button, /<(?:Switch|Input|Select|Textarea|Settings\w*)\b/, "form controls must not be nested inside a disclosure button");
  }
  assert.match(buttons[0], /\{meta\[id\]\.name\}/, "the identity button gets its name from the harness title");
  assert.match(buttons[1], /aria-label=\{t\(isExpanded \? "settings\.collapseHarness" : "settings\.expandHarness"/, "the icon-only button needs a localized name");
});

test("keeps the compact harness row focused on identity and availability", () => {
  const harness = source.slice(source.indexOf("function HarnessSettings"), source.indexOf("function ExecutionSettings"));
  const row = harness.slice(harness.indexOf("<div className=\"group flex"), harness.indexOf("{isExpanded &&"));
  const panel = harness.slice(harness.indexOf("{isExpanded &&"));
  assert.match(row, /meta\[id\]\.name[\s\S]*?<Badge[\s\S]*?statusText/, "version status belongs immediately after the harness name");
  assert.doesNotMatch(row, /remoteFsEnabled/, "Remote FS does not belong in the collapsed row");
  assert.match(panel, /<SettingsSwitchRow checked=\{remoteFSEnabled\}[\s\S]*?update\(id, "remoteFsEnabled", checked\)/, "Remote FS belongs inside expanded configuration");
  assert.doesNotMatch(harness, /current\.checkedAt/, "the compact version badge should not include the probe timestamp");
});

// The UI must not keep its own copy of which harnesses exist or what they can
// do. A second copy is how the picker came to offer a harness the backend then
// rejected as an invalid task.
test("harness settings derive every per-harness fact from the backend catalog", () => {
  const harness = source.slice(source.indexOf("function HarnessSettings"), source.indexOf("function ExecutionSettings"));
  for (const id of ["pi", "grok", "dsh"]) {
    assert.doesNotMatch(harness, new RegExp(`id === "${id}"`), `${id} must not be special-cased in the settings page`);
  }
  // Capabilities come from the runtime list the desktop reports.
  assert.match(harness, /harness\.efforts/);
  assert.match(harness, /harness\.providers/);
  assert.match(harness, /harness\.integrations \|\| \["cli"\]/);
  assert.match(harness, /item\.environmentHint/);
  assert.match(harness, /harness\.supportsRemoteFs/);
});

// Reasoning levels can belong to the model rather than to the harness: Grok
// offers xhigh on 4.6 but not on 4.5. Offering the harness-wide superset would
// let a user save a level the selected model rejects at run time.
test("reasoning levels narrow to the model the harness reported", () => {
  const harness = source.slice(source.indexOf("function HarnessSettings"), source.indexOf("function ExecutionSettings"));
  assert.match(harness, /selectedReportedModel\?\.efforts\?\.length \? selectedReportedModel\.efforts/);
  // Changing the model must drop a level the new one does not offer.
  assert.match(harness, /!supported\.includes\(current\) \? "" : current/);
  // One inspection path serves every harness that reports the shared shape.
  assert.match(source, /SettingsBinding\.InspectHarnessConfiguration\(id, draft\.runtimes\[id\]\)/);
});
