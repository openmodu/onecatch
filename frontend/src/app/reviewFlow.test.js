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
  const statusCall = review.indexOf("const snapshot = await GitBinding.Status(workspaceID)");
  const diffCalls = review.indexOf("const [stagedDiff, worktreeDiff] = await Promise.all");
  assert.ok(statusCall >= 0);
  assert.ok(diffCalls > statusCall, "repository status must be checked before requesting diffs");
  assert.match(review, /if \(!snapshot\?\.isRepo\)/);
  assert.match(review, /GitBinding\.Diff\(workspaceID, true\)/);
  assert.match(review, /GitBinding\.Diff\(workspaceID, false\)/);
  assert.match(review, /WorkspaceBinding\.ReadWorkspaceFile\(workspaceID, file\.path\)/);
});

test("selected Agent produces prioritized line-specific findings", () => {
  assert.match(review, /<HarnessSelector value=\{reviewProfile\}/);
  assert.match(review, /GitBinding\.ReviewChanges\(\{ workspaceId: workspaceID, runtime: reviewProfile\.harness/);
  assert.match(review, /review-finding priority-\$\{finding\.priority\}/);
  assert.match(review, /finding\.file\}:\$\{finding\.startLine\}/);
  assert.match(css, /\.review-finding\.priority-0/);
  assert.match(css, /\.review-diff-line\.review-finding-line/);
});

test("review action is optically centered with the file-change disclosure", () => {
  assert.match(css, /\.conversation-file-changes > summary\s*\{[\s\S]*?display:\s*block;/);
  assert.match(css, /\.conversation-review-action\s*\{[\s\S]*?top:\s*35px;[\s\S]*?transform:\s*translateY\(-50%\);/);
});

// The review panel's grid gives its toolbar exactly one 46px row, so nothing in
// that row may wrap: a two-line title overflows the row instead of growing it,
// which is how the heading ended up stacked vertically beside the agent
// controls in a narrow inspector.
test("the review toolbar survives a narrow inspector without wrapping", () => {
  const toolbar = css.slice(css.indexOf(".review-toolbar {"), css.indexOf(".review-layout {"));
  assert.match(toolbar, /\.review-toolbar strong \{[^}]*white-space: nowrap/);
  // The title takes the free space, so the trailing icons stay right-aligned
  // even at the widths where the diff totals — which carry the auto margin —
  // are hidden.
  assert.match(toolbar, /\.review-toolbar > div:first-child \{[^}]*flex: 1 1 auto/);

  // Controls drop in priority order rather than colliding.
  assert.match(toolbar, /@container \(max-width: 470px\)[\s\S]*?div:first-child span \{ display: none/);
  assert.match(toolbar, /@container \(max-width: 400px\)[\s\S]*?review-agent-run-label \{ display: none/);
  assert.match(toolbar, /@container \(max-width: 330px\)[\s\S]*?\.review-stats \{ display: none/);

  // Each override has to come after the base rule it overrides, or the cascade
  // resolves the other way on equal specificity.
  assert.ok(css.indexOf("@container (max-width: 330px)") > css.indexOf(".review-stats {"),
    "the narrow-width overrides must follow the base .review-stats rule");

  // Hiding the label leaves the icon alone, so the button needs its own name.
  assert.match(review, /className="review-agent-run"[^>]*aria-label=\{t\("review\.runAgent"\)\}/);
  assert.match(review, /<span className="review-agent-run-label">/);
});
