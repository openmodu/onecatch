import assert from "node:assert/strict";
import test from "node:test";
import { conversationImageURL } from "./conversationImages.js";
import { mergeMobileRun } from "./mobileRuns.js";

test("image routes keep credentials out of URLs and reject executable schemes", () => {
  for (const source of ["javascript:alert(1)", "data:text/html,test", "//example.com/a.png", "https://user:password@example.com/a.png", "#anchor"]) {
    assert.equal(conversationImageURL(source, "run"), "");
  }
  assert.equal(conversationImageURL("/tmp/a & b.png", "run/1"), "/conversation-image?run=run%2F1&path=%2Ftmp%2Fa%20%26%20b.png");
  assert.equal(conversationImageURL("photo.png", ""), "");
});

test("attachment-only changes refresh cached runs and follow-up messages", () => {
  const old = { id: "run", prompt: "image", events: [{ kind: "user_message", text: "look" }] };
  const updated = { ...old, attachments: [{ storedPath: "/a.png" }] };
  assert.equal(mergeMobileRun([old], updated)[0], updated);
  const followup = { ...old, events: [{ ...old.events[0], attachments: ["/b.png"] }] };
  assert.equal(mergeMobileRun([old], followup)[0], followup);
});
