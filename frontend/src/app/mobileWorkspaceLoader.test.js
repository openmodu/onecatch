import assert from "node:assert/strict";
import test from "node:test";
import { createWorkspaceLoader } from "./mobileWorkspaceLoader.js";

function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

test("failed workspace refresh preserves the last list and can recover", async () => {
  const cache = {};
  let failure = false;
  let items = [{ id: "project" }];
  const load = createWorkspaceLoader(async () => {
    if (failure) throw new Error("offline");
    return items;
  }, (id, value) => { cache[id] = value; });
  await load("worker");
  failure = true;
  await assert.rejects(load("worker"), /offline/);
  assert.deepEqual(cache.worker, [{ id: "project" }]);
  failure = false;
  items = [];
  await load("worker");
  assert.deepEqual(cache.worker, [], "a successful empty response still removes deleted workspaces");
});

test("late workspace responses stay with their worker and cannot replace a newer refresh", async () => {
  const requests = [deferred(), deferred(), deferred()];
  let index = 0;
  const cache = {};
  const load = createWorkspaceLoader(() => requests[index++].promise, (id, value) => { cache[id] = value; });
  const oldA = load("a");
  const b = load("b");
  const newA = load("a");
  requests[2].resolve([{ id: "new-a" }]);
  await newA;
  requests[1].resolve([{ id: "b" }]);
  await b;
  requests[0].resolve([]);
  await oldA;
  assert.deepEqual(cache, { a: [{ id: "new-a" }], b: [{ id: "b" }] });
});
