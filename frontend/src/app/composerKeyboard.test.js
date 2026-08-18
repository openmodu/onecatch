import test from "node:test";
import assert from "node:assert/strict";
import { shouldSubmitComposer } from "./composerKeyboard.js";

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
