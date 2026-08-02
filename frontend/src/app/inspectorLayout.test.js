import test from "node:test";
import assert from "node:assert/strict";
import {
  INSPECTOR_LAYOUT_STORAGE_KEY,
  parseInspectorPreference,
  readInspectorPreference,
  resolveInspectorCollapsed,
  writeInspectorPreference,
} from "./inspectorLayout.js";

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
});
