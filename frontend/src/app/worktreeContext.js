// Project identity stays stable while every directory-scoped tool follows this
// context. Tasks retain their saved binding; draft selection never retargets one.
export function executionWorkspace(project, task, draftWorktree) {
  if (!project) return project;
  if (task?.workspaceId && task.workspaceId !== project.id) return project;
  const binding = task ? task.worktree : draftWorktree;
  if (!binding?.contextId) return project;
  return { ...project, projectId: project.id, id: task ? `task:${task.id}` : binding.contextId, path: binding.path, worktree: binding };
}

export function worktreeSelectionMode(project, selection) {
  if (selection?.contextId) return "existing";
  if (selection?.mode === "project" || selection?.mode === "new") return selection.mode;
  return project?.autoWorktree ? "new" : "project";
}

export function demoTaskWorktree(project, selection, token) {
  const mode = worktreeSelectionMode(project, selection);
  if (mode === "existing") return selection;
  if (mode === "project") return undefined;
  const root = `${project.path}-sessions/${token}`;
  return { contextId: `worktree:${project.id}:${token}`, path: root, root, commonDir: `${project.path}/.git`, branch: `onecatch/${token}`, managed: true };
}
