import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { formatSkillBytes, newSkillTemplate, syncStatusTone } from "./skills.js";

const page = readFileSync(new URL("./SkillManagerPage.jsx", import.meta.url), "utf8");

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

test("skill library uses selectable cards that adapt to available width", () => {
  assert.match(page, /function SkillCard/);
  assert.match(page, /aria-pressed=\{selected\}/);
  assert.match(page, /repeat\(auto-fill, minmax\(min\(100%, 220px\), 300px\)\)/);
  assert.doesNotMatch(page, /<SelectTrigger[^>]*skill\.library/);
});
