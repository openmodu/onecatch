import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const app = readFileSync(new URL("./App.jsx", import.meta.url), "utf8");
const composer = readFileSync(new URL("./components/Composer.jsx", import.meta.url), "utf8");
const styles = readFileSync(new URL("../index.css", import.meta.url), "utf8");

test("single-Agent tasks omit the redundant titlebar status", () => {
  assert.match(app, /selectedTask\.workflowId && selectedTask\.workflowId !== directAgentWorkflowID && <StatusPill/);
});

test("single-Agent composer has one state-aware primary control", () => {
  assert.match(composer, /directAgent \? <>/);
  assert.match(composer, /className="composer-pause-action"[^>]*onClick=\{onInterrupt\}[^>]*><Pause/);
  assert.match(composer, /className="composer-send-action"/);
  assert.match(composer, /aria-label=\{t\("composer\.sendMessage"\)\}/);
  assert.match(composer, /composer-send-action[\s\S]{0,500}<ArrowUp/);
  assert.match(composer, /composer\.sendMessage/);
  assert.doesNotMatch(composer, /CircleStop|onCancel|composer\.(?:runningActions|stop|terminate)/);
  assert.doesNotMatch(app, /TaskRunBinding\.CancelRun|onCancel=\{cancelRun\}/);
});

test("session runtime profile uses the same intrinsic width as new task", () => {
  assert.match(styles, /\.workbench-runtime-profile\s*\{[^}]*width:\s*auto;[^}]*min-width:\s*138px;[^}]*max-width:\s*220px;/s);
  assert.doesNotMatch(styles, /\.workbench-runtime-profile\s*\{[^}]*min-width:\s*224px;/s);
});

test("narrow new-task flex growth does not widen the session Codex label", () => {
  assert.match(styles, /\.new-task-toolbar \.new-task-select\.executor\s*\{\s*flex:\s*1 1 210px;/);
  assert.doesNotMatch(styles, /(?:^|,)\s*\.new-task-select\.executor\s*\{\s*flex:\s*1 1 210px;/m);
});
