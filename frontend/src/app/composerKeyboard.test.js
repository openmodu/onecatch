import test from "node:test";
import assert from "node:assert/strict";
import { composerSubmitMode, shouldSubmitComposer } from "./composerKeyboard.js";

test("composer submits with Enter and keeps Shift + Enter for a new line", () => {
  assert.equal(shouldSubmitComposer({ key: "Enter", nativeEvent: {} }), true);
  assert.equal(shouldSubmitComposer({ key: "Enter", shiftKey: true, nativeEvent: {} }), false);
  assert.equal(shouldSubmitComposer({ key: "Space", nativeEvent: {} }), false);
});

test("composer does not submit while an input method is composing", () => {
  assert.equal(shouldSubmitComposer({ key: "Enter", nativeEvent: { isComposing: true } }), false);
  assert.equal(shouldSubmitComposer({ key: "Enter", nativeEvent: { keyCode: 229 } }), false);
  assert.equal(shouldSubmitComposer({ key: "Enter", nativeEvent: {} }, true), false);
});

test("running composer queues by default and steers with the inverse shortcut", () => {
  assert.equal(composerSubmitMode({ key: "Enter", nativeEvent: {} }, { running: true }), "queue");
  assert.equal(composerSubmitMode({ key: "Enter", shiftKey: true, metaKey: true, nativeEvent: {} }, { running: true }), "insert");
  assert.equal(composerSubmitMode({ key: "Enter", shiftKey: true, ctrlKey: true, nativeEvent: {} }, { running: true }), "insert");
  assert.equal(composerSubmitMode({ key: "Enter", shiftKey: true, nativeEvent: {} }, { running: true }), "");
  assert.equal(composerSubmitMode({ key: "Enter", shiftKey: true, metaKey: true, nativeEvent: {} }, { running: true, defaultRunningMode: "insert" }), "queue");
});
