import assert from "node:assert/strict";
import test from "node:test";
import { buildGitReview, parseUnifiedDiff } from "./gitReview.js";

const diff = `diff --git a/src/app.js b/src/app.js
index 1111111..2222222 100644
--- a/src/app.js
+++ b/src/app.js
@@ -1,3 +1,4 @@
 import React from "react";
-const oldValue = 1;
+const nextValue = 2;
+const ready = true;
 export default ready;
`;

test("parses unified diff hunks with old and new line numbers", () => {
  const [file] = parseUnifiedDiff(diff);
  assert.equal(file.path, "src/app.js");
  assert.equal(file.additions, 2);
  assert.equal(file.deletions, 1);
  assert.deepEqual(file.hunks[0].lines.map((line) => [line.kind, line.oldNumber, line.newNumber]), [
    ["context", 1, 1],
    ["delete", 2, null],
    ["add", null, 2],
    ["add", null, 3],
    ["context", 3, 4],
  ]);
});

test("combines staged, worktree and untracked changes into one review", () => {
  const review = buildGitReview({ files: [
    { path: "src/app.js", index: "M", worktree: " " },
    { path: "notes.txt", index: "?", worktree: "?" },
  ] }, diff, "", { "notes.txt": "one\ntwo\n" });
  assert.deepEqual(review.files.map((file) => file.path), ["src/app.js", "notes.txt"]);
  assert.equal(review.additions, 4);
  assert.equal(review.deletions, 1);
  assert.deepEqual(review.files[1].scopes, ["untracked"]);
});

test("keeps diff body lines that look like file headers inside the hunk", () => {
  const [file] = parseUnifiedDiff(`diff --git a/docs/config.toml b/docs/config.toml
--- a/docs/config.toml
+++ b/docs/config.toml
@@ -1,2 +1,2 @@
+++ title = "guide"
--- draft = true
 body = ""
`);
  assert.equal(file.path, "docs/config.toml");
  assert.equal(file.hunks.length, 1);
  assert.deepEqual(file.hunks[0].lines.map((line) => [line.kind, line.text]), [
    ["add", '++ title = "guide"'],
    ["delete", "-- draft = true"],
    ["context", 'body = ""'],
  ]);
});

test("reads quoted paths from diff headers", () => {
  const [file] = parseUnifiedDiff(`diff --git "a/文档/说明.md" "b/文档/说明.md"
--- "a/文档/说明.md"
+++ "b/文档/说明.md"
@@ -1 +1,2 @@
 first
+second
`);
  assert.equal(file.path, "文档/说明.md");
  assert.equal(file.additions, 1);
});
