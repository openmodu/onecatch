import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const app = readFileSync(new URL("./App.jsx", import.meta.url), "utf8");
const sidebar = readFileSync(new URL("./components/Sidebar.jsx", import.meta.url), "utf8");
const terminalDock = readFileSync(new URL("./components/TerminalDock.jsx", import.meta.url), "utf8");
const inspector = readFileSync(new URL("./components/inspectors/InspectorPanel.jsx", import.meta.url), "utf8");
const workbench = readFileSync(new URL("./components/TaskWorkbench.jsx", import.meta.url), "utf8");
const workspaceMeta = readFileSync(new URL("./components/WorkspaceComposerMeta.jsx", import.meta.url), "utf8");

test("add-project UI offers a remote FS source and persists its SSH target", () => {
  assert.match(app, /value: "remote", label: t\("workspace\.remoteFS"\)/);
  assert.match(app, /remoteFs: \{ host: workspaceForm\.remoteHost\.trim\(\), root: workspaceForm\.remoteRoot\.trim\(\), username: workspaceForm\.remoteUsername\.trim\(\) \}/);
  assert.match(app, /type="password"[^>]+autoComplete="current-password"/);
  assert.match(app, /\.\.\.\(workspaceForm\.remotePassword \? \{ password: workspaceForm\.remotePassword \} : \{\}\)/);
  assert.match(app, /const \{ password: _password, \.\.\.safePayload \} = payload/);
  assert.match(app, /WorkspaceBinding\.AddWorkspace\(payload\)/);
});

test("remote FS workspaces keep SFTP files and remote Git review", () => {
  assert.match(inspector, /\{ value: "git", label: t\("inspector\.git"\)/);
  assert.match(inspector, /\{ value: "review", label: t\("review\.title"\)/);
  assert.match(inspector, /<GitInspector[^>]+remoteFS=\{remoteFS\}/);
  assert.match(workspaceMeta, /workspace\.remoteFs \? "workspace\.remoteFS" : "common\.local"/);
  assert.match(workbench, /<button[^>]+workbench-terminal-toggle/);
  assert.match(workbench, /onOpenTerminal=\{openTerminal\}/);
  assert.match(terminalDock, /workspaceId: workspace\.id/);
  assert.match(terminalDock, /!workspace\.remoteFs \? \{ shell: config\.shell, arguments: config\.arguments \} : \{\}/);
});

test("remote FS identity stays visible in the sidebar and below the composer", () => {
  assert.match(sidebar, /workspace\.remoteFs && <span[^>]+>[\s\S]+workspace\.remoteUnavailable/);
  assert.match(workspaceMeta, /composer-workspace-meta/);
  assert.match(workspaceMeta, /workspace\.name/);
  assert.match(app, /onEditWorkspace=\{editWorkspace\}/);
  assert.match(app, /inspectorCollapsed && <button/);
});

test("remote FS health gates task content and supports five-minute checks plus manual retry", () => {
  assert.match(app, /REMOTE_FS_HEALTH_INTERVAL_MS/);
  assert.match(app, /WorkspaceBinding\.GetWorkspaceStatus\(workspace\.id\)/);
  assert.match(app, /shouldAutoCheckRemoteFS\(remoteWorkspaceHealthRef\.current\[workspace\.id\]\)/);
  assert.match(sidebar, /const workspaceAvailable = !workspace\.remoteFs \|\| remoteHealth\?\.healthy === true/);
  assert.match(sidebar, /if \(!workspaceHealth\?\.\[workspace\.id\]\?\.checking\) onCheckWorkspaceHealth\(workspace\)/);
  assert.match(sidebar, /\{displayExpanded && <div className="project-task-panel/);
});

test("an empty workspace opens the task composer instead of a welcome page", () => {
  assert.match(app, /taskModal \|\| !selectedTask/);
});
