import assert from "node:assert/strict";
import test from "node:test";
import { createFrameBatcher } from "./frameBatcher.js";

test("frame batcher coalesces a burst into one display frame", () => {
  const callbacks = new Map();
  let nextHandle = 1;
  let flushes = 0;
  const batcher = createFrameBatcher(
    () => { flushes += 1; },
    (callback) => {
      const handle = nextHandle;
      nextHandle += 1;
      callbacks.set(handle, () => {
        callbacks.delete(handle);
        callback();
      });
      return handle;
    },
    (handle) => callbacks.delete(handle),
  );

  batcher.schedule();
  batcher.schedule();
  batcher.schedule();
  assert.equal(callbacks.size, 1);

  callbacks.get(1)();
  assert.equal(flushes, 1);
  assert.equal(callbacks.size, 0);

  batcher.schedule();
  assert.equal(callbacks.size, 1);
});

test("frame batcher cancels pending work during cleanup", () => {
  const callbacks = new Map();
  const batcher = createFrameBatcher(
    () => assert.fail("cancelled frame must not flush"),
    (callback) => {
      callbacks.set(7, callback);
      return 7;
    },
    (handle) => callbacks.delete(handle),
  );

  batcher.schedule();
  batcher.cancel();
  assert.equal(callbacks.size, 0);
});

test("a stalled animation frame still flushes once via the mobile timer", () => {
  const frames = new Map();
  const timers = new Map();
  let count = 0;
  const batcher = createFrameBatcher(() => count++, (fn) => { frames.set(1, fn); return 1; }, (id) => frames.delete(id), {
    schedule: (fn) => { timers.set(2, fn); return 2; }, cancel: (id) => timers.delete(id),
  });
  batcher.schedule();
  batcher.schedule();
  const lateFrame = frames.get(1);
  timers.get(2)();
  lateFrame();
  assert.equal(count, 1);
  assert.equal(frames.size + timers.size, 0);
  batcher.schedule();
  frames.get(1)();
  assert.equal(count, 2);
  assert.equal(timers.size, 0);
  batcher.schedule();
  batcher.cancel();
  assert.equal(frames.size + timers.size, 0);
});
