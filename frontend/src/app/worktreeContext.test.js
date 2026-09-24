import test from "node:test";
import assert from "node:assert/strict";
import { executionWorkspace, worktreeSelectionMode, demoTaskWorktree } from "./worktreeContext.js";

test("a task binding wins over draft selection without changing project identity", () => {
 const project = { id: "project", path: "/repo", name: "repo" };
 const worktree = { contextId: "worktree:project:a", path: "/repo-a" };
 const other = { contextId: "worktree:project:b", path: "/repo-b" };
 const context = executionWorkspace(project, { id: "task-a", worktree }, other);
 assert.equal(context.id, "task:task-a");
 assert.equal(context.path, "/repo-a");
 assert.equal(context.projectId, "project");
 assert.equal(project.path, "/repo");
 assert.equal(executionWorkspace(project, { id: "legacy" }, other), project);
 assert.equal(executionWorkspace(project, null, other).id, other.contextId);
 assert.equal(executionWorkspace(project, null, null), project);
});

test("project defaults and per-session overrides resolve without retargeting a draft", () => {
 const project = { id: "project", path: "/repo", autoWorktree: true };
 assert.equal(worktreeSelectionMode(project, null), "new");
 assert.equal(worktreeSelectionMode(project, { mode: "project" }), "project");
 assert.equal(worktreeSelectionMode({ ...project, autoWorktree: false }, { mode: "new" }), "new");
 assert.equal(worktreeSelectionMode(project, { contextId: "existing" }), "existing");
 assert.equal(executionWorkspace(project, null, { mode: "new" }), project);
 assert.equal(executionWorkspace(project, null, { mode: "project" }), project);
 const first = demoTaskWorktree(project, null, "session-one");
 const second = demoTaskWorktree(project, null, "session-two");
 assert.notEqual(first.path, second.path);
 assert.notEqual(first.branch, second.branch);
 const existing = { contextId: "existing", path: "/existing" };
 assert.equal(demoTaskWorktree(project, existing, "session-three"), existing);
 assert.equal(demoTaskWorktree(project, { mode: "project" }, "session-four"), undefined);
 assert.equal(executionWorkspace({ ...project, autoWorktree: false }, { id: "one", worktree: first }, null).path, first.path);
});
