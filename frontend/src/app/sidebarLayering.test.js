import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const sidebar = readFileSync(new URL("./components/Sidebar.jsx", import.meta.url), "utf8");
const css = readFileSync(new URL("../index.css", import.meta.url), "utf8");

test("collapsed sidebar and its hover reveal stay above a maximized terminal", () => {
  assert.match(sidebar, /sidebar-shell relative z-50/);
  assert.match(css, /\.terminal-dock\.is-maximized\s*\{[\s\S]*?z-index:\s*40;/);
});

test("sidebar row actions overlay labels instead of reserving label width", () => {
  assert.match(sidebar, /workspace-item[^`]*py-0 pr-2 pl-2/);
  assert.match(sidebar, /project-task-item[^`]*py-0 pr-2 pl-8/);
  assert.doesNotMatch(sidebar, /workspace-item[^`]*\bpr-14\b/);
  assert.doesNotMatch(sidebar, /project-task-item[^`]*\bpr-20\b/);
  assert.match(sidebar, /className="workspace-row-actions[^\"]*\babsolute\b[^\"]*\bbg-gradient-to-l\b/);
  assert.match(sidebar, /className="task-row-actions[^\"]*\babsolute\b[^\"]*\bbg-gradient-to-l\b/);
  assert.doesNotMatch(sidebar, /className="(?:workspace|task)-row-actions[^\"]*\b(?:rounded-md|shadow-sm|backdrop-blur-sm)\b/);
});

test("remote workspace badge follows the project name on the same baseline", () => {
  assert.match(sidebar, /inline-flex w-fit min-w-0 max-w-full items-center gap-1\.5/);
  assert.match(sidebar, /workspace\.remoteFs && <span className=\{`inline-flex h-4 shrink-0 items-center gap-1 rounded-md/);
});

test("workspace folders keep independent expanded state and cached task lists", () => {
  assert.match(sidebar, /expandedWorkspaceIDs, setExpandedWorkspaceIDs/);
  assert.match(sidebar, /const expanded = expandedWorkspaceIDs\.has\(workspace\.id\)/);
  assert.match(sidebar, /workspaceTaskCache\.current\.set\(workspaceID, \{ tasks, runs, runTotal, runHasMore \}\)/);
  assert.match(sidebar, /\{displayExpanded && <div className="project-task-panel/);
  assert.doesNotMatch(sidebar, /expandedWorkspaceID === workspace\.id/);
});
