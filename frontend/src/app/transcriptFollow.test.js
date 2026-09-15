import test from "node:test";
import assert from "node:assert/strict";
import { createTranscriptFollower } from "./transcriptFollow.js";

test("keyboard and queue resizing do not lose follow mode", () => {
  const element = { scrollHeight: 1200, clientHeight: 600, scrollTop: 600 };
  const follower = createTranscriptFollower(element);
  element.clientHeight = 300;
  follower.scroll(); // WebKit emits this during keyboard/composer resize.
  follower.sync();
  assert.equal(element.scrollTop, 900);
  element.scrollHeight = 1400;
  follower.sync();
  assert.equal(element.scrollTop, 1100);
});
test("sending from history follows the new message immediately", () => {
  const element = { scrollHeight: 1200, clientHeight: 600, scrollTop: 600 };
  const follower = createTranscriptFollower(element);
  element.scrollTop = 200;
  follower.scroll();
  element.scrollHeight = 1400;
  follower.sync();
  assert.equal(element.scrollTop, 200, "reading history must not jump");
  follower.sync(true); // User pressed send.
  element.scrollHeight = 1500; // Optimistic bubble commits.
  follower.sync();
  assert.equal(element.scrollTop, 900);
});
test("loading earlier history stays paused across a resize", () => {
  const element = { scrollHeight: 1200, clientHeight: 600, scrollTop: 200 };
  const follower = createTranscriptFollower(element);
  follower.pause();
  element.scrollHeight = 2000;
  element.scrollTop = 1000;
  follower.scroll();
  follower.sync();
  assert.equal(element.scrollTop, 1000);
});
