import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const inspector = readFileSync(new URL("./components/inspectors/FileInspector.jsx", import.meta.url), "utf8");
const css = readFileSync(new URL("../index.css", import.meta.url), "utf8");

test("file editor input and highlight layers use one font metric source", () => {
  assert.doesNotMatch(inspector, /components\/ui\/textarea/);
  assert.match(inspector, /<textarea[\s\S]*?className="file-editor-textarea/);
  assert.doesNotMatch(inspector, /md:text-sm/);
  assert.match(css, /\.file-editor-highlight,\s*\.file-editor-textarea\s*\{[\s\S]*?font-size:\s*12px;[\s\S]*?line-height:\s*20px;[\s\S]*?tab-size:\s*4;/);
});
