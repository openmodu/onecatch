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
  assert.match(inspector, /if \(scope === "task" && reviewRequest > 0\) setTab\("review"\)/, "review auto-focus belongs to the task inspector, not the skills file tree");
});

test("review combines repository state, diffs and untracked file content", () => {
  const statusCall = review.indexOf("const snapshot = await GitBinding.Status(workspaceID)");
  const diffCalls = review.indexOf("const [stagedDiff, worktreeDiff] = await Promise.all");
  assert.ok(statusCall >= 0);
  assert.ok(diffCalls > statusCall, "repository status must be checked before requesting diffs");
  assert.match(review, /if \(!snapshot\?\.isRepo\)/);
  assert.match(review, /GitBinding\.Diff\(workspaceID, true\)/);
  assert.match(review, /GitBinding\.Diff\(workspaceID, false\)/);
  assert.match(review, /WorkspaceBinding\.ReadWorkspaceFile\(workspaceID, file\.path\)/);
});

test("review stays focused on the diff without Agent review controls", () => {
  assert.doesNotMatch(review, /HarnessSelector|ReviewChanges|review-agent|review-finding/);
  assert.doesNotMatch(css, /review-agent|review-finding|has-agent-review/);
});

test("long diff lines carry their background through horizontal overflow", () => {
  assert.match(review, /className="review-diff-canvas"/);
  assert.match(css, /\.review-diff-canvas\s*\{[^}]*width:\s*max-content;[^}]*min-width:\s*100%;/s);
  assert.match(css, /\.review-file\s*\{[^}]*width:\s*100%;/s);
  assert.match(css, /\.review-diff-line\s*\{[^}]*width:\s*100%;/s);
});

test("review action is optically centered with the file-change disclosure", () => {
  assert.match(css, /\.conversation-file-changes > summary\s*\{[\s\S]*?display:\s*block;/);
  assert.match(css, /\.conversation-review-action\s*\{[\s\S]*?top:\s*35px;[\s\S]*?transform:\s*translateY\(-50%\);/);
  assert.match(css, /\.conversation-file-changes > summary\s*\{[\s\S]*?padding:\s*14px 84px 14px 16px;/, "the disclosure keeps the English Review label close to the caret without overlapping it");
  assert.match(css, /\.conversation-review-action\s*\{[\s\S]*?right:\s*16px;/, "the Review action keeps a stable trailing inset");
});

// The review panel's grid gives its toolbar exactly one 46px row, so nothing in
// that row may wrap.
test("the review toolbar survives a narrow inspector without wrapping", () => {
  const toolbar = css.slice(css.indexOf(".review-toolbar {"), css.indexOf(".review-layout {"));
  assert.match(toolbar, /\.review-toolbar strong \{[^}]*white-space: nowrap/);
  // The title takes the free space, so the trailing icons stay right-aligned
  // even at the widths where the diff totals — which carry the auto margin —
  // are hidden.
  assert.match(toolbar, /\.review-toolbar > div:first-child \{[^}]*flex: 1 1 auto/);

  // Secondary information drops before it can collide with the actions.
  assert.match(toolbar, /@container \(max-width: 470px\)[\s\S]*?div:first-child span \{ display: none/);
  assert.match(toolbar, /@container \(max-width: 330px\)[\s\S]*?\.review-stats \{ display: none/);

  // Each override has to come after the base rule it overrides, or the cascade
  // resolves the other way on equal specificity.
  assert.ok(css.indexOf("@container (max-width: 330px)") > css.indexOf(".review-stats {"),
    "the narrow-width overrides must follow the base .review-stats rule");
});
