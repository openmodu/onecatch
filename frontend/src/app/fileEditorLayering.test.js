import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const inspector = readFileSync(new URL("./components/inspectors/FileInspector.jsx", import.meta.url), "utf8");
const codeEditor = readFileSync(new URL("./components/CodeEditor.jsx", import.meta.url), "utf8");
const css = readFileSync(new URL("../index.css", import.meta.url), "utf8");

test("file editor delegates text layout and navigation to CodeMirror", () => {
  assert.match(inspector, /import CodeEditor from "\.\.\/CodeEditor\.jsx"/);
  assert.match(inspector, /<CodeEditor[\s\S]*?onDefinition=\{goToDefinition\}/);
  assert.match(inspector, /LSPBinding\.Definition\(\{[\s\S]*?content:\s*draft,[\s\S]*?position,/);
  assert.match(inspector, /LSPBinding\.Detect\(\{\s*workspaceId:\s*workspaceID,\s*path/);
  assert.doesNotMatch(inspector, /endsWith\("\.go"\)/);
  assert.match(codeEditor, /key:\s*"F12"/);
  assert.match(codeEditor, /event\.metaKey[\s\S]*?event\.ctrlKey[\s\S]*?posAtCoords/);
  assert.doesNotMatch(inspector, /<textarea|file-editor-highlight|file-editor-textarea/);
  assert.doesNotMatch(css, /\.file-editor-highlight|\.file-editor-textarea/);
});

test("file editing autosaves and keeps refresh plus tab closing controls in the expected places", () => {
  assert.match(inspector, /AUTO_SAVE_DELAY_MS/);
  assert.match(inspector, /scheduleAutoSave\(activePath\)/);
  assert.match(inspector, /AUTO_REFRESH_INTERVAL_MS/);
  assert.match(inspector, /window\.addEventListener\("focus", refreshSilently\)/);
  assert.match(inspector, /<ContextMenuTrigger asChild>/);
  assert.match(inspector, /files\.closeOthers/);
  assert.match(inspector, /files\.closeRight/);
  assert.match(inspector, /files\.closeAll/);
  assert.doesNotMatch(inspector, /<Save\b/);
  assert.doesNotMatch(inspector, /onClick=\{reloadFile\}/);
  assert.match(inspector, /aria-label=\{t\("files\.refresh"\)\}/);
});

test("CodeMirror search uses the application control styling", () => {
  assert.match(codeEditor, /"\.cm-panel\.cm-search"/);
  assert.match(codeEditor, /"\.cm-panel\.cm-search input\.cm-textfield"/);
  assert.match(codeEditor, /"\.cm-panel\.cm-search button\.cm-button"/);
  assert.match(codeEditor, /"\.cm-searchMatch-selected"/);
});
