import test from "node:test";
import assert from "node:assert/strict";
import { runtimeResumeCommand, runtimeSessionEntries } from "./sessionCommands.js";

test("builds recovery commands for supported runtimes", () => {
  assert.equal(runtimeResumeCommand("codex", "thread-123"), "codex resume thread-123");
  assert.equal(runtimeResumeCommand("claude", "session-456"), "claude --resume session-456");
  assert.equal(runtimeResumeCommand("modu", "session-789"), "modu_code --resume session-789");
  assert.equal(runtimeResumeCommand("pi", "session-pi"), "pi --session session-pi");
  assert.equal(runtimeResumeCommand("grok", "session-grok"), "grok --resume session-grok");
  assert.equal(runtimeResumeCommand("custom", "session-789"), "");
  assert.equal(runtimeResumeCommand("codex", "thread; rm -rf project"), "");
});

test("prefers the current run session and emits one entry per step", () => {
  const detail = {
    run: { sessions: { implement: "thread-current" } },
    workflow: { steps: [{ id: "implement", name: "实现", runtime: "codex" }] },
    stepRuns: [
      { stepId: "implement", sessionIdAfter: "thread-old" },
      { stepId: "implement", sessionIdBefore: "thread-old" },
    ],
  };
  assert.deepEqual(runtimeSessionEntries(detail), [{
    stepID: "implement",
    stepName: "实现",
    runtime: "codex",
    sessionID: "thread-current",
    idLabel: "thread id",
    command: "codex resume thread-current",
  }]);
});

test("falls back to the latest StepRun session for older run data", () => {
  const detail = {
    run: {},
    workflow: { steps: [{ id: "review", name: "审查", runtime: "claude" }] },
    stepRuns: [
      { stepId: "review", sessionIdAfter: "session-first" },
      { stepId: "review", sessionIdBefore: "session-latest" },
    ],
  };
  const [entry] = runtimeSessionEntries(detail);
  assert.equal(entry.sessionID, "session-latest");
  assert.equal(entry.command, "claude --resume session-latest");
});

test("shows the Pi session command in the resume card", () => {
  const detail = {
    run: { sessions: { execute: "01a02ebb-f359-7342-8a77-a679259cd80f" } },
    workflow: { steps: [{ id: "execute", name: "执行", runtime: "pi" }] },
    stepRuns: [],
  };
  const [entry] = runtimeSessionEntries(detail);
  assert.equal(entry.command, "pi --session 01a02ebb-f359-7342-8a77-a679259cd80f");
});

test("omits steps that do not have a persisted session", () => {
  const detail = { run: {}, workflow: { steps: [{ id: "pending", runtime: "codex" }] }, stepRuns: [] };
  assert.deepEqual(runtimeSessionEntries(detail), []);
});
