import assert from "node:assert/strict";
import test from "node:test";
import { EditorState } from "@codemirror/state";
import { editorOffsetAt, lspPositionAt } from "./editorPosition.js";

test("editor offsets map to zero-based UTF-16 LSP positions", () => {
  const document = EditorState.create({ doc: "alpha\n中😀x\nomega" }).doc;
  const offset = "alpha\n中😀".length;
  assert.deepEqual(lspPositionAt(document, offset), { line: 1, character: 3 });
  assert.equal(editorOffsetAt(document, { line: 1, character: 3 }), offset);
});

test("editor positions are clamped to the available document", () => {
  const document = EditorState.create({ doc: "one\ntwo" }).doc;
  assert.equal(editorOffsetAt(document, { line: 99, character: 99 }), document.length);
  assert.deepEqual(lspPositionAt(document, 999), { line: 1, character: 3 });
});
