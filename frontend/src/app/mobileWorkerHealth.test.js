import assert from "node:assert/strict";
import test from "node:test";
import { createWorkerHealthLoader, workerHealthPollKey } from "./mobileWorkerHealth.js";

function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

test("health responses and worker ordering do not restart polling", () => {
  const workers = [{ id: "a", name: "Mac", address: "old" }, { id: "b" }];
  const refreshed = [{ id: "b", lastSeenAt: 100 }, { id: "a", name: "New name", address: "new" }];
  for (const all of [false, true]) {
    assert.equal(workerHealthPollKey(workers, "a", all), workerHealthPollKey(refreshed, "a", all));
  }
});

test("health polling watches only the visible computers", () => {
  const workers = [{ id: "b" }, { id: "a" }, { id: "b" }, { id: "" }];
  assert.deepEqual(JSON.parse(workerHealthPollKey(workers, "", true)), []);
  assert.deepEqual(JSON.parse(workerHealthPollKey(workers, "a", false)), ["a"]);
  assert.deepEqual(JSON.parse(workerHealthPollKey(workers, "b", false)), ["b"]);
  assert.deepEqual(JSON.parse(workerHealthPollKey(workers, "a", true)), ["a", "b"]);
  assert.deepEqual(JSON.parse(workerHealthPollKey([...workers, { id: "c" }], "a", true)), ["a", "b", "c"]);
});

test("overlapping manual and automatic health checks share one request and publish once", async () => {
  const response = deferred();
  let checks = 0;
  const published = [];
  const load = createWorkerHealthLoader(() => { checks++; return response.promise; }, (id, value) => published.push([id, value]));
  const first = load("a");
  const second = load("a");
  assert.equal(first, second);
  await Promise.resolve();
  assert.equal(checks, 1);
  response.resolve({ latencyMs: 12 });
  assert.deepEqual(await second, { latencyMs: 12 });
  assert.deepEqual(published, [["a", { latencyMs: 12 }]]);
  await load("a");
  assert.equal(checks, 2, "a later poll must make a fresh check");
});

test("a slow health check does not block a different computer", async () => {
  const slow = deferred();
  const published = [];
  const load = createWorkerHealthLoader((id) => id === "a" ? slow.promise : { latencyMs: 5 }, (id) => published.push(id));
  const first = load("a");
  assert.deepEqual(await load("b"), { latencyMs: 5 });
  assert.deepEqual(published, ["b"]);
  slow.resolve({ latencyMs: 40 });
  await first;
  assert.deepEqual(published, ["b", "a"]);
});

test("failed health checks clear the pending request so the next poll can recover", async () => {
  let checks = 0;
  const published = [];
  const load = createWorkerHealthLoader(() => {
    if (++checks === 1) throw new Error("offline");
    return { latencyMs: 8 };
  }, (id, value) => published.push([id, value]));
  const first = load("a");
  assert.equal(load("a"), first);
  await assert.rejects(first, /offline/);
  assert.deepEqual(published, []);
  assert.deepEqual(await load("a"), { latencyMs: 8 });
  assert.equal(checks, 2);
  assert.deepEqual(published, [["a", { latencyMs: 8 }]]);
});
