import assert from "node:assert/strict";
import test from "node:test";
import { readFile } from "node:fs/promises";
import { readSelectedWorker, saveSelectedWorker, resolveSelectedWorker } from "./mobileWorkerSelection.js";

function storage() {
  const values = new Map();
  return { getItem: (key) => values.get(key), setItem: (key, value) => values.set(key, value), removeItem: (key) => values.delete(key) };
}

test("reopening restores the chosen computer even when it is not first or online", () => {
  const local = storage();
  const workers = [{ id: "first" }, { id: "chosen", online: false }];
  assert.equal(resolveSelectedWorker(workers, readSelectedWorker(local)), "first");
  saveSelectedWorker("chosen", local);
  assert.equal(resolveSelectedWorker(workers, readSelectedWorker(local)), "chosen");
  assert.equal(resolveSelectedWorker([...workers].reverse(), readSelectedWorker(local)), "chosen");
});

test("removing the selected computer saves a fallback or clears the preference", () => {
  const local = storage();
  saveSelectedWorker("removed", local);
  const next = resolveSelectedWorker([{ id: "remaining" }], readSelectedWorker(local));
  saveSelectedWorker(next, local);
  assert.equal(readSelectedWorker(local), "remaining");
  saveSelectedWorker(resolveSelectedWorker([], next), local);
  assert.equal(readSelectedWorker(local), "");
});

test("unavailable local storage does not break startup or switching", () => {
  const denied = { getItem() { throw Error("denied"); }, setItem() { throw Error("denied"); }, removeItem() { throw Error("denied"); } };
  assert.equal(readSelectedWorker(denied), "");
  assert.doesNotThrow(() => saveSelectedWorker("worker", denied));
  assert.doesNotThrow(() => saveSelectedWorker("", denied));
});

test("mobile startup restores selection and never probes the first computer unconditionally", async () => {
  const source = await readFile(new URL("./MobileApp.jsx", import.meta.url), "utf8");
  assert.match(source, /useState\(\(\) => readSelectedWorker\(\)\)/);
  assert.match(source, /saveSelectedWorker\(selectedWorkerID\)/);
  assert.doesNotMatch(source, /refreshWorker\(items\[0\]\.id/);
});
