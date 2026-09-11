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
