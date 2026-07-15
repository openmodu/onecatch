import test from "node:test";
import assert from "node:assert/strict";
import { preserveComposerFocus } from "./composerInteraction.js";

function mouseEvent({ button = 0, targetButton = true, contained = true } = {}) {
  let prevented = false;
  const matchedButton = targetButton ? {} : null;
  return {
    event: {
      button,
      target: { closest: () => matchedButton },
      currentTarget: { contains: (candidate) => contained && candidate === matchedButton },
      preventDefault: () => { prevented = true; },
    },
    prevented: () => prevented,
  };
}

test("keeps the composer focused while a primary mouse-button click continues", () => {
  const input = mouseEvent();
  assert.equal(preserveComposerFocus(input.event), true);
  assert.equal(input.prevented(), true);
});

test("does not interfere with textarea, disabled-button, or secondary mouse-button input", () => {
  for (const input of [
    mouseEvent({ targetButton: false }),
    mouseEvent({ contained: false }),
    mouseEvent({ button: 2 }),
  ]) {
    assert.equal(preserveComposerFocus(input.event), false);
    assert.equal(input.prevented(), false);
  }
});
