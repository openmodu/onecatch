import test from "node:test";
import assert from "node:assert/strict";
import {
  INSPECTOR_LAYOUT_STORAGE_KEY,
  parseInspectorDetached,
  parseInspectorPreference,
  readInspectorDetached,
  readInspectorPreference,
  resolveInspectorCollapsed,
  writeInspectorDetached,
  writeInspectorPreference,
} from "./inspectorLayout.js";

function memoryStorage(initial) {
  const values = new Map(initial ? [[INSPECTOR_LAYOUT_STORAGE_KEY, initial]] : []);
  return {
    values,
    getItem: (key) => values.get(key) || null,
    setItem: (key, value) => values.set(key, value),
  };
}

test("parses only a valid versioned inspector preference", () => {
  assert.equal(parseInspectorPreference('{"inspectorCollapsed":true}'), true);
  assert.equal(parseInspectorPreference('{"inspectorCollapsed":false}'), false);
  assert.equal(parseInspectorPreference('{"inspectorCollapsed":"yes"}'), null);
  assert.equal(parseInspectorPreference("not-json"), null);
  assert.equal(parseInspectorPreference(""), null);
});

test("saved user preference overrides the compact viewport default", () => {
  assert.equal(resolveInspectorCollapsed(null, true), true);
  assert.equal(resolveInspectorCollapsed(null, false), false);
  assert.equal(resolveInspectorCollapsed(false, true), false);
  assert.equal(resolveInspectorCollapsed(true, false), true);
});

test("reads and writes the inspector preference without depending on browser storage", () => {
  const values = new Map();
  const storage = {
    getItem: (key) => values.get(key) || null,
    setItem: (key, value) => values.set(key, value),
  };

  assert.equal(readInspectorPreference(storage), null);
  assert.equal(writeInspectorPreference(storage, true), true);
  assert.equal(values.get(INSPECTOR_LAYOUT_STORAGE_KEY), '{"inspectorCollapsed":true}');
  assert.equal(readInspectorPreference(storage), true);
});

test("storage failures do not break the workbench", () => {
  const storage = {
    getItem: () => { throw new Error("denied"); },
    setItem: () => { throw new Error("denied"); },
  };
  assert.equal(readInspectorPreference(storage), null);
  assert.equal(writeInspectorPreference(storage, false), false);
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

test("detaching the inspector preserves the collapse preference and vice versa", () => {
  const storage = memoryStorage();

  assert.equal(writeInspectorPreference(storage, true), true);
  assert.equal(writeInspectorDetached(storage, true), true);
  assert.equal(readInspectorPreference(storage), true);
  assert.equal(readInspectorDetached(storage), true);

  assert.equal(writeInspectorPreference(storage, false), true);
  assert.equal(readInspectorDetached(storage), true);

  assert.equal(writeInspectorDetached(storage, false), true);
  assert.equal(readInspectorPreference(storage), false);
  assert.equal(readInspectorDetached(storage), false);
});

test("a layout record written by an older build still reads", () => {
  const storage = memoryStorage('{"inspectorCollapsed":true}');
  assert.equal(readInspectorPreference(storage), true);
  assert.equal(readInspectorDetached(storage), false);
});
