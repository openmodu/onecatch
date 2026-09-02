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
