import assert from "node:assert/strict";
import test from "node:test";
import { modelLoadLabel, modelLoadStatus } from "./modelLoad.js";

const t = (key, values) => ({
  "settings.modelQueueSize": `${values?.count} queued`,
  "settings.modelQueueEmpty": "Queue empty",
  "settings.modelMayQueue": "High load, may queue",
  "settings.modelLoadPercent": `Load ${values?.percent}%`,
})[key];

test("absent load and queue data never imply an empty queue", () => {
  for (const model of [{}, { load: null }, { load: {} }, { load: { percent: null, queueSize: null } }]) {
    assert.equal(modelLoadStatus(model), null);
    assert.equal(modelLoadLabel(model, t), "");
  }
  assert.equal(modelLoadLabel({ load: { percent: 264 } }, t), "High load, may queue · Load 264%");
});

test("reported queues and zero values are displayed without clamping load", () => {
  assert.equal(modelLoadLabel({ load: { percent: 120, queueSize: 12 } }, t), "12 queued · Load 120%");
  assert.equal(modelLoadLabel({ load: { percent: 0, queueSize: 0 } }, t), "Queue empty · Load 0%");
  assert.equal(modelLoadLabel({ load: { queueSize: 3 } }, t), "3 queued");
  assert.equal(modelLoadLabel({ load: { percent: 72 } }, t), "Load 72%");
  assert.equal(modelLoadLabel({ load: { percent: 110, queueSize: 0 } }, t), "Queue empty · Load 110%");
});
