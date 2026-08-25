import test from "node:test";
import assert from "node:assert/strict";
import {
  INSPECTOR_LAYOUT_STORAGE_KEY,
  parseInspectorDetached,
  readInspectorDetached,
  resolveInspectorCollapsed,
  writeInspectorDetached,
} from "./inspectorLayout.js";

function memoryStorage(initial) {
  const values = new Map(initial ? [[INSPECTOR_LAYOUT_STORAGE_KEY, initial]] : []);
  return {
    values,
    getItem: (key) => values.get(key) || null,
    setItem: (key, value) => values.set(key, value),
  };
}

test("the inspector starts collapsed and respects an in-session choice", () => {
  // Nothing chosen yet this session reads as collapsed. Only an explicit
  // toggle opens it, and that choice lives no longer than the window.
  assert.equal(resolveInspectorCollapsed(null), true);
  assert.equal(resolveInspectorCollapsed(undefined), true);
  assert.equal(resolveInspectorCollapsed(false), false);
  assert.equal(resolveInspectorCollapsed(true), true);
});

test("storage failures do not break the workbench", () => {
  const storage = {
    getItem: () => { throw new Error("denied"); },
    setItem: () => { throw new Error("denied"); },
  };
  assert.equal(readInspectorDetached(storage), false);
  assert.equal(writeInspectorDetached(storage, true), false);
});

test("the inspector is docked unless the record explicitly says otherwise", () => {
  assert.equal(parseInspectorDetached('{"inspectorDetached":true}'), true);
  assert.equal(parseInspectorDetached('{"inspectorDetached":false}'), false);
  assert.equal(parseInspectorDetached('{"inspectorCollapsed":true}'), false);
  assert.equal(parseInspectorDetached("not-json"), false);
  assert.equal(parseInspectorDetached(""), false);
});

test("floating the inspector round-trips through storage", () => {
  const storage = memoryStorage();
  assert.equal(writeInspectorDetached(storage, true), true);
  assert.equal(readInspectorDetached(storage), true);
  assert.equal(writeInspectorDetached(storage, false), true);
  assert.equal(readInspectorDetached(storage), false);
});

test("a collapse flag left by an older build is neither read nor destroyed", () => {
  // Older builds persisted the expanded state. It is no longer honoured — the
  // panel always starts collapsed — but writing detachment merges rather than
  // replaces, so an existing record is not rewritten behind the user's back.
  const storage = memoryStorage('{"inspectorCollapsed":false}');
  assert.equal(readInspectorDetached(storage), false);
  assert.equal(writeInspectorDetached(storage, true), true);
  const record = JSON.parse(storage.values.get(INSPECTOR_LAYOUT_STORAGE_KEY));
  assert.equal(record.inspectorDetached, true);
  assert.equal(record.inspectorCollapsed, false);
});
