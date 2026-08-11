import assert from "node:assert/strict";
import test from "node:test";
import { hasLostTerminalSelectionMouseUp, isAccidentalTerminalSelectionDrag, isTerminalCopyShortcut, shouldStartTerminalSelection } from "./terminalInteraction.js";

const keyEvent = (change = {}) => ({ type: "keydown", key: "c", altKey: false, ctrlKey: false, metaKey: false, shiftKey: false, ...change });

test("terminal copy keeps Ctrl+C available for interrupting macOS shells", () => {
  const macOS = { platform: "MacIntel" };
  assert.equal(isTerminalCopyShortcut(keyEvent({ metaKey: true }), macOS), true);
  assert.equal(isTerminalCopyShortcut(keyEvent({ ctrlKey: true }), macOS), false);
  assert.equal(isTerminalCopyShortcut(keyEvent({ ctrlKey: true, metaKey: true }), macOS), false);
});

test("terminal copy uses Ctrl+Shift+C outside macOS", () => {
  const windows = { platform: "Win32" };
  assert.equal(isTerminalCopyShortcut(keyEvent({ ctrlKey: true, shiftKey: true }), windows), true);
  assert.equal(isTerminalCopyShortcut(keyEvent({ ctrlKey: true }), windows), false);
  assert.equal(isTerminalCopyShortcut(keyEvent({ ctrlKey: true, shiftKey: true, altKey: true }), windows), false);
});

test("terminal selection ignores tiny distance and fast trackpad drags", () => {
  const start = { clientX: 100, clientY: 80, timeStamp: 1000 };
  assert.equal(isAccidentalTerminalSelectionDrag(start, { clientX: 103, clientY: 84, timeStamp: 1400 }), true);
  assert.equal(isAccidentalTerminalSelectionDrag(start, { clientX: 130, clientY: 100, timeStamp: 1100 }), true);
  assert.equal(isAccidentalTerminalSelectionDrag(start, { clientX: 130, clientY: 100, timeStamp: 1400 }), false);
});

test("terminal selection recovers when WebKit loses mouseup", () => {
  assert.equal(hasLostTerminalSelectionMouseUp(true, { buttons: 0 }), true);
  assert.equal(hasLostTerminalSelectionMouseUp(true, { buttons: 1 }), false);
  assert.equal(hasLostTerminalSelectionMouseUp(false, { buttons: 0 }), false);
});

test("terminal selection starts only after a deliberate held drag", () => {
  const start = { clientX: 100, clientY: 80, timeStamp: 1000 };
  assert.equal(shouldStartTerminalSelection(start, { clientX: 140, clientY: 100, timeStamp: 1100, buttons: 1 }), false);
  assert.equal(shouldStartTerminalSelection(start, { clientX: 140, clientY: 100, timeStamp: 1400, buttons: 0 }), false);
  assert.equal(shouldStartTerminalSelection(start, { clientX: 140, clientY: 100, timeStamp: 1400, buttons: 1 }), true);
});
