import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { formatSkillBytes, newSkillTemplate, parseSkillDocument, syncStatusTone } from "./skills.js";
import { applyDebugFrames } from "./skillWorkspace.js";

const page = readFileSync(new URL("./SkillManagerPage.jsx", import.meta.url), "utf8");
const inspector = readFileSync(new URL("./components/inspectors/SkillFilesInspector.jsx", import.meta.url), "utf8");
const debugPanel = readFileSync(new URL("./components/SkillDebugPanel.jsx", import.meta.url), "utf8");
const settings = readFileSync(new URL("./SettingsPage.jsx", import.meta.url), "utf8");

test("new skill template emits valid matching frontmatter", () => {
  const source = newSkillTemplate("release-notes", "Write concise\n release notes");
  assert.match(source, /^---\nname: release-notes\ndescription: Write concise release notes\n---/);
  assert.match(source, /# Release Notes/);
});

test("sync states map to semantic status tones", () => {
  assert.equal(syncStatusTone("synced"), "good");
  assert.equal(syncStatusTone("out-of-sync"), "warn");
  assert.equal(syncStatusTone("rsync-unavailable"), "danger");
  assert.equal(syncStatusTone("missing"), "accent");
});

test("skill sizes stay compact", () => {
  assert.equal(formatSkillBytes(512), "512 B");
  assert.equal(formatSkillBytes(1536), "1.5 KB");
});

test("skill library is a text-first rail beside one detail card", () => {
  assert.match(page, /function SkillRow/);
  assert.match(page, /aria-pressed=\{selected\}/);
  assert.match(page, /grid-cols-\[248px_minmax\(0,1fr\)\]/);
  // The rail replaced the card grid; cards forced a second scroll region above
  // the document and pushed the skill itself below the fold.
  assert.doesNotMatch(page, /repeat\(auto-fill, minmax/);
  assert.doesNotMatch(page, /<SelectTrigger[^>]*skill\.library/);
});

test("the detail card renders the skill instead of showing its source", () => {
  assert.match(page, /<MarkdownContent className="markdown-content[^"]*" content=\{parsed\.body\}/);
  // Editing belongs to the inspector's file editor now, so the page must not
  // grow a second, competing buffer for the same bytes.
  assert.doesNotMatch(page, /value=\{draft\}|SkillBinding\.UpdateSkill/);
});

test("the file editor drives the preview and the preview drives the editor", () => {
  assert.match(page, /subscribeSkillWorkspace\(SKILL_FILE_DRAFT_EVENT/, "keystrokes in the inspector re-render the card");
  assert.match(page, /requestSkillFile\(skillDocumentPath\(selectedName\)\)/, "the card's edit action opens SKILL.md in the inspector");
  assert.match(inspector, /publishSkillFileDraft\(\{ path: file\?\.path, content: value \}\)/);
  assert.match(inspector, /SkillBinding\.WriteFile\(\{ path: file\.path, content: draft \}\)/);
});

test("the inspector splits the tree beside the file, not above it", () => {
  // Stacked, the tree took a third of an already short panel and the editor
  // never had the rows to show a skill's body at once.
  // The file leads, its tree follows on the right.
  assert.match(inspector, /w-\[152px\] shrink-0 border-l/);
  assert.ok(inspector.indexOf('aria-label={t("skill.fileEditor")}') < inspector.indexOf('role="tree"'), "the editor column comes before the tree column");
  assert.doesNotMatch(inspector, /max-h-\[36%\]/);
  // A side-by-side tree costs editor width, so it folds away.
  assert.match(inspector, /setTreeCollapsed\(\(current\) => !current\)/);
  assert.match(inspector, /const treeVisible = !file \|\| !treeCollapsed/);
});

test("debug is one small toggle, and its events render by kind", () => {
  assert.match(page, /aria-pressed=\{debugOpen\}/);
  assert.match(page, /size="icon-sm"[^>]*aria-label=\{t\("skill\.debug"\)\}/);
  assert.match(debugPanel, /const EVENT_VISUALS = \{/);
  assert.match(debugPanel, /reasoning: \{[^}]*prose: true/, "reasoning and messages are prose, not preformatted text");
  assert.match(debugPanel, /<MarkdownContent[^>]*content=\{result\.output\}/);
});

test("a debug run streams, can be stopped, and is kept", () => {
  assert.match(page, /Events\.On\(SKILL_DEBUG_EVENT/, "frames arrive while Debug is still awaiting");
  assert.match(page, /createFrameBatcher\(/, "streamed frames coalesce per display frame like runtime frames");
  assert.match(page, /frame\.runId !== debugRunRef\.current/, "a frame from an abandoned run must not paint over the current one");
  assert.match(page, /SkillBinding\.StopDebug\(runID\)/);
  assert.match(page, /SkillBinding\.DebugHistory\(selectedName\)|SkillBinding\.DebugHistory\(name\)/);
  assert.match(debugPanel, /streaming=\{Boolean\(event\.streaming\)\}/, "a growing message shows a caret rather than reading as final");
});

test("sync is a library-level destination rather than a per-skill tab", () => {
  assert.match(page, /pane === "sync"/);
  assert.doesNotMatch(page, /<TabsTrigger[^>]*value="sync"/);
});

test("frontmatter is lifted out of the rendered body", () => {
  const { frontmatter, body } = parseSkillDocument(newSkillTemplate("release-notes", "Write release notes"));
  assert.equal(frontmatter.name, "release-notes");
  assert.equal(frontmatter.description, "Write release notes");
  assert.match(body, /^\s*# Release Notes/);
  assert.doesNotMatch(body, /^---/);
  // A file that never declared frontmatter renders whole rather than losing
  // its first paragraph to a partial match.
  assert.equal(parseSkillDocument("# Notes\n").body, "# Notes\n");
});

test("streamed debug frames replace their slot instead of appending", () => {
  // A message arriving as token deltas is one event that grows, not one event
  // per chunk.
  let events = applyDebugFrames([], [{ index: 0, event: { kind: "message", text: "Loa", streaming: true } }]);
  events = applyDebugFrames(events, [{ index: 0, event: { kind: "message", text: "Loaded the", streaming: true } }]);
  events = applyDebugFrames(events, [{ index: 1, event: { kind: "tool_use", text: "read_file" } }]);
  assert.deepEqual(events.map((event) => event.text), ["Loaded the", "read_file"]);

  // A dropped frame must not leave a hole the renderer walks into.
  const sparse = applyDebugFrames([], [{ index: 2, event: { kind: "message", text: "third" } }]);
  assert.equal(sparse.length, 3);
  assert.equal(sparse[2].text, "third");
  assert.equal(sparse[0].text, "");

  // Malformed frames are ignored rather than corrupting the transcript.
  assert.deepEqual(applyDebugFrames(events, [{ index: -1, event: { text: "x" } }, { index: 0 }]), events);
  assert.equal(applyDebugFrames(events, []), events);
});

test("sync targets are configured in settings and filled from the skills page", () => {
  // Paths and membership are two different questions, so they live where each
  // is answered: the directory in Settings, the contents beside the library.
  assert.match(settings, /function SkillSyncSettings/);
  assert.match(settings, /SkillBinding\.UpdateSyncTarget\(\{ id: target\.id, path \}\)/);
  assert.match(settings, /"skills", "terminal"/, "Skills is a settings section of its own");
  assert.doesNotMatch(settings, /SetSyncTargetSkills/, "choosing skills belongs to the skills page");
  // Adding a target is one action behind a header control, not a permanent
  // row of empty fields sitting under the real ones.
  assert.match(settings, /aside=\{<SettingsButton[^>]*onClick=\{\(\) => setAdding\(true\)\}><Plus/);
  assert.match(settings, /<Dialog open=\{adding\}/);
  assert.match(settings, /rounded-lg bg-muted\/35/, "rows carry the surface so the card is not a slab of white");

  assert.match(page, /SkillBinding\.SetSyncTargetSkills\(\{ id: target\.id, skills: selection \}\)/);
  assert.match(page, /<DropdownMenuCheckboxItem/, "a target's skills are picked, not typed");
  assert.doesNotMatch(page, /AddSyncTarget|RemoveSyncTarget|UpdateSyncTarget/, "a target's existence and directory belong to settings");

  // Both surfaces read the same demo fixture, so the browser preview cannot
  // show two different libraries.
  assert.match(settings, /import \{ demoSyncTargets \} from "\.\/skills\.js"/);
  assert.match(page, /demoSyncTargets/);
});

test("an empty selection means the whole library on both sides of the wire", () => {
  // The page collapses "everything ticked" back to no selection so the target
  // keeps following the library as skills are added.
  assert.match(page, /names\.length === skills\.length \? \[\] : names/);
  assert.match(page, /const everything = selection\.length === 0/);
});
