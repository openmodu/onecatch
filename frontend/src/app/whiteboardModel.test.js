import assert from "node:assert/strict";
import test from "node:test";
import {
  applyWhiteboardChange,
  clampWhiteboardScale,
  createDemoWhiteboardProposal,
  createInitialWhiteboard,
  moveWhiteboardObject,
  mergeWhiteboardProposal,
  normalizeWhiteboardProposal,
  parseStoredWhiteboard,
  screenToWorld,
  serializeWhiteboardForAgent,
  zoomWhiteboardAt,
} from "./whiteboardModel.js";

test("whiteboard zoom stays anchored under the pointer", () => {
  const viewport = { x: 40, y: 30, scale: 1 };
  const anchor = { x: 240, y: 180 };
  const before = screenToWorld(anchor, viewport);
  const next = zoomWhiteboardAt(viewport, 1.5, anchor);
  assert.deepEqual(screenToWorld(anchor, next), before);
  assert.equal(clampWhiteboardScale(99), 1.65);
  assert.equal(clampWhiteboardScale(0.1), 0.55);
});

test("moving a human object preserves the rest of the board", () => {
  const board = createInitialWhiteboard();
  const original = board.objects.find((object) => object.id === "goal-cold-start");
  const moved = moveWhiteboardObject(board.objects, original.id, { x: 12, y: -8 });
  const result = moved.find((object) => object.id === original.id);
  assert.equal(result.x, original.x + 12);
  assert.equal(result.y, original.y - 8);
  assert.equal(moved.find((object) => object.id === "priority-list").x, board.objects.find((object) => object.id === "priority-list").x);
});

test("Agent proposals are normalized into reviewable changes", () => {
  const proposal = normalizeWhiteboardProposal({ summary: "ok", changes: [{ action: "unsafe", category: "confirm", x: 9999 }] });
  assert.equal(proposal.changes[0].action, "add");
  assert.equal(proposal.changes[0].x, 980);
  assert.equal(proposal.changes[0].requiresConfirmation, true);
  assert.equal(proposal.changes[0].state, "pending");
});

test("continued Agent turns refine the proposal layer without dropping other work", () => {
  const current = normalizeWhiteboardProposal(createDemoWhiteboardProposal());
  const refined = mergeWhiteboardProposal(current, {
    runtime: "codex",
    sessionId: "continued-session",
    summary: "更新优先级",
    changes: [{ ...current.changes[1], title: "任务序列（已细化）", action: "update" }],
  });
  assert.equal(refined.changes.length, current.changes.length);
  assert.equal(refined.changes.find((change) => change.id === current.changes[1].id).title, "任务序列（已细化）");
  assert.equal(refined.changes.find((change) => change.id === current.changes[0].id).title, current.changes[0].title);
  assert.equal(refined.sessionId, "continued-session");
});

test("accepting a proposal writes it into the human canvas and audit trail", () => {
  const board = createInitialWhiteboard();
  const change = normalizeWhiteboardProposal(createDemoWhiteboardProposal()).changes.find((item) => item.id === "recovery-file");
  const result = applyWhiteboardChange(board, change, "10:31");
  assert.ok(result.objects.some((object) => object.id === "accepted-recovery-file"));
  assert.ok(result.connections.some((connection) => connection.to === "accepted-recovery-file"));
  assert.ok(result.acceptedChangeIDs.includes("recovery-file"));
  assert.match(result.activity.at(-1).text, /关联文件/);
});

test("Agent context contains the current canvas and selection", () => {
  const board = createInitialWhiteboard();
  const proposal = normalizeWhiteboardProposal(createDemoWhiteboardProposal());
  proposal.changes[0].state = "ignored";
  const context = serializeWhiteboardForAgent(board, "open-question", proposal, "recovery-order");
  assert.equal(context.selectedObjectId, "open-question");
  assert.equal(context.objects.length, board.objects.length);
  assert.equal(context.canvas.width, 1120);
  assert.equal(context.review.focusedChangeId, "recovery-order");
  assert.equal(context.review.pendingChanges.length, proposal.changes.length - 1);
  assert.equal(context.review.pendingChanges.some((change) => change.id === proposal.changes[0].id), false);
});

test("stored whiteboards fail closed when the schema is stale", () => {
  assert.equal(parseStoredWhiteboard("not-json"), null);
  assert.equal(parseStoredWhiteboard(JSON.stringify({ schemaVersion: 4, objects: [], connections: [] })), null);
  const board = createInitialWhiteboard();
  assert.equal(parseStoredWhiteboard(JSON.stringify(board)).objects.length, board.objects.length);
});
