import assert from "node:assert/strict";
import test from "node:test";

import { formatSkillBytes, newSkillTemplate, syncStatusTone } from "./skills.js";

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
