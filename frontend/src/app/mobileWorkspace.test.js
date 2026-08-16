import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const sourceURL = new URL("./MobileApp.jsx", import.meta.url);

test("mobile Workspace management closes the API-to-page loop", async () => {
  const source = await readFile(sourceURL, "utf8");
  assert.match(source, /function WorkspaceManagerPage\(/);
  assert.match(source, /function WorkspaceEditorSheet\(/);
  assert.match(source, /MobileBinding\.PrepareWorkspace\(/);
  assert.match(source, /MobileBinding\.RemoveWorkspace\(/);
  assert.match(source, /MobileBinding\.WorkspaceGitStatus\(/);
  assert.match(source, /克隆 Git 仓库/);
  assert.match(source, /绑定已有目录/);
  assert.match(source, /同时删除远端克隆/);
});
