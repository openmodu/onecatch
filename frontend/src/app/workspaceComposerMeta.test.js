import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const app = readFileSync(new URL("./App.jsx", import.meta.url), "utf8");
const composer = readFileSync(new URL("./components/Composer.jsx", import.meta.url), "utf8");
const newTask = readFileSync(new URL("./components/NewTaskView.jsx", import.meta.url), "utf8");
const meta = readFileSync(new URL("./components/WorkspaceComposerMeta.jsx", import.meta.url), "utf8");
const styles = readFileSync(new URL("../index.css", import.meta.url), "utf8");

test("project identity and filesystem type live below both task composers", () => {
  assert.match(composer, /<\/div>\s*<WorkspaceComposerMeta mode=\{mode\} workspace=\{workspace\} onEdit=\{onEditWorkspace\} \/>\s*<\/div>/, "session metadata belongs outside the composer card");
  assert.match(newTask, /<\/div>\s*<WorkspaceComposerMeta mode=\{mode\} workspace=\{workspace\} onEdit=\{onEditWorkspace\} \/>\s*<\/div>/, "new-task metadata belongs outside the composer card");
  assert.match(meta, /className="composer-workspace-button"/);
  assert.match(meta, /workspace\.remoteFs \? "workspace\.remoteFS" : "common\.local"/);
});

test("clicking the project name edits the existing workspace instead of replacing its identity", () => {
  assert.match(meta, /onClick=\{onEdit\}/);
  assert.match(app, /setWorkspaceEditingID\(selectedWorkspace\.id\)/);
  assert.match(app, /WorkspaceBinding\.UpdateWorkspace\(\{ id: workspaceEditingID, \.\.\.payload \}\)/);
});

test("task titlebar actions stay aligned to the right after moving workspace status below", () => {
  assert.match(app, /inspectorCollapsed && <button[^>]+className=\{`no-drag ml-auto grid size-7[\s\S]{0,800}?<SquareTerminal/);
});

test("workspace metadata keeps a visible gap below both composer cards", () => {
  assert.match(styles, /\.new-task-composer-stack\s*\{[^}]*gap:\s*10px;/s);
  assert.match(styles, /\.workbench-composer-inner\s*\{[^}]*display:\s*grid;[^}]*gap:\s*10px;/s);
});
