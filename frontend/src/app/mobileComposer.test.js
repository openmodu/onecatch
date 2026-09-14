import assert from "node:assert/strict";
import test from "node:test";
import { mobileComposerAction, enqueueMobileFollowUp } from "./mobileComposer.js";

const state = { prompt: "", sharedRuns: true, ready: true, busy: "" };
test("one action switches from send to running to queue and back", () => {
  assert.equal(mobileComposerAction(state).disabled, true);
  assert.deepEqual(mobileComposerAction({ ...state, prompt: "hello" }), { mode: "send", disabled: false });
  const pending = { text: "hello" };
  assert.deepEqual(mobileComposerAction({ ...state, pending, busy: "run" }), { mode: "running", disabled: false, interruptible: false });
  assert.deepEqual(mobileComposerAction({ ...state, pending, busy: "run", prompt: "next" }), { mode: "queue", disabled: false });
  const running = { id: "run-1" };
  assert.deepEqual(mobileComposerAction({ ...state, running }), { mode: "running", disabled: false, interruptible: true });
  assert.deepEqual(mobileComposerAction({ ...state, running, prompt: "next" }), { mode: "queue", disabled: false });
  assert.equal(mobileComposerAction({ ...state, running, prompt: "  \n " }).mode, "running");
  assert.deepEqual(mobileComposerAction({ ...state, prompt: "next" }), { mode: "send", disabled: false });
});
test("unsupported worker queues and unready workspaces cannot send", () => {
  assert.equal(mobileComposerAction({ ...state, sharedRuns: false, running: { id: "r" }, prompt: "next" }).mode, "running");
  assert.equal(mobileComposerAction({ ...state, ready: false, prompt: "hello" }).disabled, true);
  assert.equal(mobileComposerAction({ ...state, running: { id: "r" }, busy: "interrupt" }).interruptible, false);
});
test("rapid follow-ups wait for the run ID and reach the host in order", async () => {
  let resolveStart;
  const target = new Promise((resolve) => { resolveStart = resolve; });
  const sent = [];
  const send = async (id, text) => { sent.push([id, text]); return sent.map(([, text]) => ({ text })); };
  const first = enqueueMobileFollowUp(Promise.resolve(), target, "first", send);
  const second = enqueueMobileFollowUp(first, target, "second", send);
  await Promise.resolve();
  assert.deepEqual(sent, []);
  resolveStart({ id: "new-run" });
  assert.equal((await first).runID, "new-run");
  assert.equal((await second).queued.length, 2);
  assert.deepEqual(sent, [["new-run", "first"], ["new-run", "second"]]);
});
test("a failed queue request does not block the next message", async () => {
  const target = Promise.resolve({ id: "r" });
  const first = enqueueMobileFollowUp(Promise.resolve(), target, "first", async () => { throw new Error("offline"); });
  const second = enqueueMobileFollowUp(first, target, "second", async (_, text) => [{ text }]);
  await assert.rejects(first, /offline/);
  assert.deepEqual((await second).queued, [{ text: "second" }]);
  await assert.rejects(enqueueMobileFollowUp(Promise.resolve(), Promise.resolve(null), "third", () => assert.fail("must not send without a run")), /未能启动/);
});
