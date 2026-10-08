import assert from "node:assert/strict";
import test from "node:test";
import { loadTaskRuntimeConfiguration, watchRuntimeConfiguration } from "./taskRuntimeConfiguration.js";

const flush = () => new Promise((resolve) => setImmediate(resolve));

test("switching from missing Codex to TRAE clears the old error and ignores a late Codex rejection", async () => {
  let rejectCodex;
  const codex = new Promise((_, reject) => { rejectCodex = reject; });
  const catalog = { models: [{ model: "Doubao-Seed-Code" }] };
  const inspected = [];
  const inspect = (harness) => { inspected.push(harness); return harness === "codex" ? codex : Promise.resolve(catalog); };
  let state = { harness: "codex", loading: false, data: null, error: "Codex binary is not executable" };
  const publish = (next) => { state = next; };
  const cancel = loadTaskRuntimeConfiguration("codex", inspect, publish, String);
  await flush();
  cancel();
  loadTaskRuntimeConfiguration("trae", inspect, publish, String);
  assert.equal(state.error, "");
  assert.equal(state.harness, "trae");
  await flush();
  rejectCodex(new Error("Codex binary is not executable"));
  await flush();
  assert.deepEqual(inspected, ["codex", "trae"]);
  assert.deepEqual(state, { harness: "trae", loading: false, data: catalog, error: "" });
});

test("TRAE inspection failures are attributed to TRAE and discard old catalog data", async () => {
  let state;
  loadTaskRuntimeConfiguration("trae", () => { throw new Error("TRAE unavailable"); }, (next) => { state = next; }, (err) => err.message);
  await flush();
  assert.deepEqual(state, { harness: "trae", loading: false, data: null, error: "TRAE unavailable" });
});


test("returning to the app refreshes models and discards an older in-flight result", async () => {
  const target = new EventTarget();
  const pending = [];
  let state;
  const stop = watchRuntimeConfiguration("codex", () => new Promise((resolve) => pending.push(resolve)), (next) => { state = next; }, String, target);
  await flush();
  target.dispatchEvent(new Event("focus"));
  await flush();
  pending[1]({ models: [{ model: "new-model" }] });
  await flush();
  pending[0]({ models: [{ model: "old-model" }] });
  await flush();
  assert.equal(state.data.models[0].model, "new-model");
  stop();
  target.dispatchEvent(new Event("focus"));
  await flush();
  assert.equal(pending.length, 2);
});
