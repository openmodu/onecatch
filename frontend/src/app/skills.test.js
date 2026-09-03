import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { formatSkillBytes, newSkillTemplate, parseSkillDocument, syncStatusTone } from "./skills.js";
import { applyDebugFrames } from "./skillWorkspace.js";

const page = readFileSync(new URL("./SkillManagerPage.jsx", import.meta.url), "utf8");
const inspector = readFileSync(new URL("./components/inspectors/SkillFilesInspector.jsx", import.meta.url), "utf8");
const debugPanel = readFileSync(new URL("./components/inspectors/SkillDebugInspector.jsx", import.meta.url), "utf8");
const inspectorPanel = readFileSync(new URL("./components/inspectors/InspectorPanel.jsx", import.meta.url), "utf8");
const workbench = readFileSync(new URL("./components/TaskWorkbench.jsx", import.meta.url), "utf8");
const settings = readFileSync(new URL("./SettingsPage.jsx", import.meta.url), "utf8");
const resize = readFileSync(new URL("./columnResize.js", import.meta.url), "utf8");

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
  assert.match(page, /style=\{\{ width: `\$\{rail\.width\}px` \}\}/, "the rail is one resizable column beside the detail pane");
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
  assert.ok(inspector.indexOf('aria-label={t("skill.fileEditor")}') < inspector.indexOf('role="tree"'), "the editor column comes before the tree column");
  assert.doesNotMatch(inspector, /max-h-\[36%\]/);
  // A side-by-side tree costs editor width, so it folds away.
  assert.match(inspector, /setTreeCollapsed\(\(current\) => !current\)/);
  assert.match(inspector, /const treeVisible = !file \|\| !treeCollapsed/);
});

test("both columns are draggable, and the drag knows which side it grows", () => {
  assert.match(page, /useColumnWidth\(\{ defaultWidth: 248/);
  assert.match(inspector, /useColumnWidth\(\{ defaultWidth: 152[^)]*fromRight: true/, "a column right of its handle grows as the pointer moves left");
  assert.match(resize, /const delta = fromRight \? drag\.startX - event\.clientX : event\.clientX - drag\.startX/);
  // The same manners the workbench edge already has.
  assert.match(resize, /setPointerCapture/);
  assert.match(resize, /onDoubleClick: reset/);
  assert.match(resize, /event\.key !== "ArrowLeft" && event\.key !== "ArrowRight"/);
});

test("the inspector holds both a file editor and a debug conversation", () => {
  assert.match(inspectorPanel, /function SkillsInspector/);
  assert.match(inspectorPanel, /\{ value: "files"[\s\S]{0,120}\{ value: "debug"/, "two tabs, files first");
  // Switching tabs must not throw away an unsaved buffer or a live transcript.
  assert.match(inspectorPanel, /tab === "files" \? "h-full" : "hidden"/);
  assert.match(inspectorPanel, /tab === "debug" \? "h-full" : "hidden"/);
  // The card's actions route to a tab rather than opening a drawer of their own.
  assert.match(page, /requestSkillInspectorTab\("debug"\)/);
  assert.match(page, /requestSkillInspectorTab\("files"\)/);
  assert.doesNotMatch(page, /SkillDebugPanel|debugOpen/, "the debug drawer is gone from the document pane");
});

test("debug events render by kind, and a run streams and can be stopped", () => {
  assert.match(debugPanel, /const EVENT_VISUALS = \{/);
  assert.match(debugPanel, /reasoning: \{[^}]*prose: true/, "reasoning and messages are prose, not preformatted text");
  assert.match(debugPanel, /<MarkdownContent[^>]*content=\{result\.output\}/);
  assert.match(debugPanel, /Events\.On\(SKILL_DEBUG_EVENT/, "frames arrive while Debug is still awaiting");
  assert.match(debugPanel, /createFrameBatcher\(/, "streamed frames coalesce per display frame like runtime frames");
  assert.match(debugPanel, /frame\.runId !== runRef\.current/, "a frame from an abandoned run must not paint over the current one");
  assert.match(debugPanel, /SkillBinding\.StopDebug\(runID\)/);
  assert.match(debugPanel, /SkillBinding\.DebugHistory\(name\)/);
  assert.match(debugPanel, /streaming=\{Boolean\(event\.streaming\)\}/, "a growing message shows a caret rather than reading as final");
});

test("the skills aside opens wide enough to read both of its panels", () => {
  assert.match(workbench, /inspectorScope !== "skills" \|\| inspectorCollapsed/);
  assert.match(workbench, /setInspectorWidth\(clampInspectorWidth\(preferredReviewInspectorWidth\(workbenchWidth\)\)\)/);
});

test("sync is a library-level destination rather than a per-skill tab", () => {
  assert.match(page, /pane === "sync"/);
  assert.doesNotMatch(page, /<TabsTrigger[^>]*value="sync"/);
  // It leads the rail rather than hiding under the list it is not part of.
  const railTab = page.indexOf('<RailTab active={pane === "sync"}');
  assert.ok(railTab > 0 && railTab < page.indexOf("visibleSkills.map"), "sync sits above the skills, not in a footer");
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

test("each skill carries its own targets and its own sync action", () => {
  assert.match(page, /function SkillSyncRow/);
  assert.match(page, /SkillBinding\.SyncSkill\(skill\.name\)/, "a skill is the unit that syncs");
  assert.match(page, /const receiving = targets\.filter\(\(target\) => !target\.skills\?\.length \|\| target\.skills\.includes\(skill\.name\)\)/);
  // The picker asks per skill, but the selection is stored per target.
  assert.match(page, /const toggleSkillTarget = \(skill, target\) =>/);
  assert.match(page, /SkillBinding\.SetSyncTargetSkills\(\{ id: target\.id, skills: selection \}\)/);
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
  assert.match(page, /<DropdownMenuCheckboxItem/, "membership is picked, not typed");
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
  assert.match(page, /const effective = target\.skills\?\.length \? target\.skills : skills\.map\(\(item\) => item\.name\)/, "unticking from an implicit \"all\" writes the explicit remainder");
});
