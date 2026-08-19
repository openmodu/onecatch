import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const app = readFileSync(new URL("./App.jsx", import.meta.url), "utf8");
const composer = readFileSync(new URL("./components/Composer.jsx", import.meta.url), "utf8");

test("single-Agent tasks omit the redundant titlebar status", () => {
  assert.match(app, /selectedTask\.workflowId && selectedTask\.workflowId !== directAgentWorkflowID && <StatusPill/);
});

test("single-Agent composer has one state-aware primary control", () => {
  assert.match(composer, /directAgent \? <>/);
  assert.match(composer, /composer\.runningActions/);
  assert.match(composer, /onSelect=\{onInterrupt\}/);
  assert.match(composer, /onSelect=\{onCancel\}/);
  assert.match(composer, /composer\.sendMessage/);
});
