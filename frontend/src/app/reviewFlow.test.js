import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const timeline = readFileSync(new URL("./components/ConversationTimeline.jsx", import.meta.url), "utf8");
const workbench = readFileSync(new URL("./components/TaskWorkbench.jsx", import.meta.url), "utf8");
const inspector = readFileSync(new URL("./components/inspectors/InspectorPanel.jsx", import.meta.url), "utf8");
const review = readFileSync(new URL("./components/inspectors/ReviewPanel.jsx", import.meta.url), "utf8");
const css = readFileSync(new URL("../index.css", import.meta.url), "utf8");

test("file-change cards open the inspector review flow", () => {
  assert.match(timeline, /conversation-review-action/);
  assert.match(timeline, /onReview=\{onReview\}/);
  assert.match(workbench, /onReview=\{openReview\}/);
  assert.match(workbench, /reviewRequest=\{reviewRequest\}/);
  assert.match(inspector, /if \(reviewRequest > 0\) setTab\("review"\)/);
});

test("review combines repository state, diffs and untracked file content", () => {
  assert.match(review, /GitBinding\.Status\(workspaceID\)/);
  assert.match(review, /GitBinding\.Diff\(workspaceID, true\)/);
  assert.match(review, /GitBinding\.Diff\(workspaceID, false\)/);
  assert.match(review, /WorkspaceBinding\.ReadWorkspaceFile\(workspaceID, file\.path\)/);
});

test("review action is optically centered with the file-change disclosure", () => {
  assert.match(css, /\.conversation-file-changes > summary\s*\{[\s\S]*?display:\s*block;/);
  assert.match(css, /\.conversation-review-action\s*\{[\s\S]*?top:\s*35px;[\s\S]*?transform:\s*translateY\(-50%\);/);
});
