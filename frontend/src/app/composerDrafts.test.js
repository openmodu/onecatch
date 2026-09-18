import assert from "node:assert/strict";
import test from "node:test";
import { createComposerDrafts, draftKey } from "./composerDrafts.js";

test("session drafts survive unmount and stay isolated across sessions and projects", () => {
  const store = createComposerDrafts();
  const a = draftKey("project-a", "session-a", "text");
  const b = draftKey("project-a", "session-b", "text");
  const otherProject = draftKey("project-b", "session-a", "text");
  const unmount = store.subscribe(a, () => {});
  store.set(a, "unfinished message", "");
  unmount();
  assert.equal(store.get(b, ""), "");
  assert.equal(store.get(otherProject, ""), "");
  store.set(b, "another message", "");
  assert.equal(store.get(a, ""), "unfinished message");
  assert.equal(store.get(b, ""), "another message");
});

test("new task text and attachments are restored together for their project", () => {
  const store = createComposerDrafts();
  const a = draftKey("a", "", "new-task");
  const b = draftKey("b", "", "new-task");
  const empty = { prompt: "", attachmentPaths: [] };
  store.set(a, { prompt: "draft", attachmentPaths: ["/tmp/image.png"] }, empty);
  assert.deepEqual(store.get(b, empty), empty);
  assert.deepEqual(store.get(a, empty), { prompt: "draft", attachmentPaths: ["/tmp/image.png"] });
});

test("delayed attachment completion updates the originating session only", async () => {
  const store = createComposerDrafts();
  const a = draftKey("a", "1", "attachments");
  const b = draftKey("a", "2", "attachments");
  let finish;
  const pending = new Promise((resolve) => { finish = resolve; })
    .then((path) => store.set(a, (paths) => [...paths, path], []));
  store.set(b, ["/tmp/b.png"], []);
  finish("/tmp/a.png");
  await pending;
  assert.deepEqual(store.get(a, []), ["/tmp/a.png"]);
  assert.deepEqual(store.get(b, []), ["/tmp/b.png"]);
});

test("draft edits notify only their subscribers and preserve edits made during send", () => {
  const store = createComposerDrafts();
  const a = draftKey("a", "1", "text");
  const b = draftKey("a", "2", "text");
  let changes = 0;
  store.subscribe(b, () => { changes += 1; });
  store.set(a, "sending", "");
  const submitted = store.get(a, "");
  store.set(a, "new unsent text", "");
  store.set(a, (current) => current === submitted ? "" : current, "");
  assert.equal(store.get(a, ""), "new unsent text");
  assert.equal(changes, 0);
  store.set(b, "second draft", "");
  assert.equal(changes, 1);
});
