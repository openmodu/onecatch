import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const app = readFileSync(new URL("./App.jsx", import.meta.url), "utf8");
const inspector = readFileSync(new URL("./components/inspectors/InspectorPanel.jsx", import.meta.url), "utf8");
const workbench = readFileSync(new URL("./components/TaskWorkbench.jsx", import.meta.url), "utf8");

test("add-project UI offers a remote FS source and persists its SSH target", () => {
  assert.match(app, /value: "remote", label: t\("workspace\.remoteFS"\)/);
  assert.match(app, /remoteFs: \{ host: workspaceForm\.remoteHost\.trim\(\), root: workspaceForm\.remoteRoot\.trim\(\), username: workspaceForm\.remoteUsername\.trim\(\) \}/);
  assert.match(app, /type="password"[^>]+autoComplete="current-password"/);
  assert.match(app, /\.\.\.\(workspaceForm\.remotePassword \? \{ password: workspaceForm\.remotePassword \} : \{\}\)/);
  assert.match(app, /const \{ password: _password, \.\.\.safePayload \} = payload/);
  assert.match(app, /WorkspaceBinding\.AddWorkspace\(payload\)/);
});

test("remote FS workspaces keep SFTP files but hide local-only tools", () => {
  assert.match(inspector, /!remoteFS \? \[\{ value: "git"/);
  assert.match(workbench, /workspace\?\.remoteFs \? "workspace\.remoteFS" : "common\.local"/);
  assert.match(workbench, /!workspace\?\.remoteFs && <button[^>]+workbench-terminal-toggle/);
});
