import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { formatSkillBytes, newSkillTemplate, parseSkillDocument, syncStatusTone } from "./skills.js";

const page = readFileSync(new URL("./SkillManagerPage.jsx", import.meta.url), "utf8");
const inspector = readFileSync(new URL("./components/inspectors/SkillFilesInspector.jsx", import.meta.url), "utf8");
const debugPanel = readFileSync(new URL("./components/SkillDebugPanel.jsx", import.meta.url), "utf8");

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

test("debug is one small toggle, and its events render by kind", () => {
  assert.match(page, /aria-pressed=\{debugOpen\}/);
  assert.match(page, /size="icon-sm"[^>]*aria-label=\{t\("skill\.debug"\)\}/);
  assert.match(debugPanel, /const EVENT_VISUALS = \{/);
  assert.match(debugPanel, /reasoning: \{[^}]*prose: true/, "reasoning and messages are prose, not preformatted text");
  assert.match(debugPanel, /<MarkdownContent[^>]*content=\{result\.output\}/);
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
