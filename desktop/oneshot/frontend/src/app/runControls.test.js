import test from "node:test";
import assert from "node:assert/strict";
import { markResumeAccepted, resumeControlState } from "./runControls.js";

test("shows immediate feedback while the resume binding is pending", () => {
  const detail = { run: { id: "run_1", status: "paused" }, active: false };
  assert.deepEqual(resumeControlState(detail, "resume", false), { disabled: true, label: "恢复中" });
});

test("keeps an accepted resume disabled until the paused snapshot advances", () => {
  const detail = { run: { id: "run_1", status: "paused" }, active: true };
  assert.deepEqual(resumeControlState(detail, "", true), { disabled: true, label: "恢复中" });
});

test("prevents resume while an interrupted run is still stopping", () => {
  const detail = { run: { id: "run_1", status: "paused" }, active: true };
  assert.deepEqual(resumeControlState(detail, "", false), { disabled: true, label: "等待停止" });
});

test("marks only the matching inspector as active after resume is accepted", () => {
  const detail = { run: { id: "run_1", status: "paused" }, active: false };
  assert.equal(markResumeAccepted(detail, "run_2"), detail);
  assert.deepEqual(markResumeAccepted(detail, "run_1"), { ...detail, active: true });
});
