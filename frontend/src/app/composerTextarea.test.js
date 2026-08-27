import assert from "node:assert/strict";
import test from "node:test";
import { autosizeComposerTextarea, NEW_TASK_TEXTAREA_MIN_HEIGHT, WORKBENCH_TEXTAREA_MIN_HEIGHT } from "./composerTextarea.js";

function textarea(scrollHeight) {
  return { scrollHeight, style: {} };
}

test("composer textareas grow with content up to three times their resting height", () => {
  const element = textarea(90);
  autosizeComposerTextarea(element, NEW_TASK_TEXTAREA_MIN_HEIGHT);
  assert.equal(element.style.height, "90px");
  assert.equal(element.style.overflowY, "hidden");

  const session = textarea(130);
  autosizeComposerTextarea(session, WORKBENCH_TEXTAREA_MIN_HEIGHT);
  assert.equal(session.style.height, "130px");
  assert.equal(session.style.overflowY, "hidden");
});

test("composer textareas scroll after three heights and shrink back to rest", () => {
  const element = textarea(220);
  autosizeComposerTextarea(element, NEW_TASK_TEXTAREA_MIN_HEIGHT);
  assert.equal(element.style.height, "168px");
  assert.equal(element.style.overflowY, "auto");

  element.scrollHeight = 20;
  autosizeComposerTextarea(element, NEW_TASK_TEXTAREA_MIN_HEIGHT);
  assert.equal(element.style.height, "56px");
  assert.equal(element.style.overflowY, "hidden");
});
