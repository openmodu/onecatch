import test from "node:test";
import assert from "node:assert/strict";
import { activityTime, withWorkspaceActivity } from "./activityOrder.js";
import { workspaceResults } from "./listNavigation.js";
import { buildSidebarTaskEntries } from "./sidebarNavigation.js";
import { groupMobileConversations, mergeMobileRunSummaries } from "./mobileRuns.js";

const old = "2026-09-01T00:00:00Z";
const recent = "2026-09-14T00:00:00Z";
const latest = "2026-09-15T00:00:00Z";
test("old conversations with new activity lead on desktop and mobile", () => {
  const runs = [
    { id: "new", conversationId: "new", workspaceId: "b", startedAt: recent },
    { id: "old", conversationId: "old", workspaceId: "a", startedAt: old, updatedAt: latest },
  ];
  assert.deepEqual(buildSidebarTaskEntries([], runs).map((entry) => entry.item.id), ["old", "new"]);
  assert.deepEqual(groupMobileConversations(runs).map((item) => item.id), ["old", "new"]);
  const workspaces = [{ id: "b" }, { id: "a" }];
  assert.deepEqual(workspaceResults(workspaces, { activities: runs }).map((item) => item.id), ["a", "b"]);
  assert.deepEqual(workspaceResults(withWorkspaceActivity(workspaces, groupMobileConversations(runs))).map((item) => item.id), ["a", "b"]);
});
test("activity includes messages and queued prompts without reordering turns", () => {
  const runs = [
    { id: "later", conversationId: "c", startedAt: recent },
    { id: "earlier", conversationId: "c", startedAt: old, events: [{ at: latest }] },
  ];
  const conversation = groupMobileConversations(runs)[0];
  assert.deepEqual(conversation.runs.map((item) => item.id), ["earlier", "later"]);
  assert.equal(activityTime(conversation), Date.parse(latest));
  assert.equal(activityTime({ startedAt: "invalid", queued: [{ createdAt: latest }] }), Date.parse(latest));
  assert.equal(activityTime({ updatedAt: "invalid" }), 0);
});
test("summary-only activity changes are retained by mobile polling", () => {
  const previous = [{ id: "a", startedAt: old, updatedAt: recent }];
  const result = mergeMobileRunSummaries(previous, [{ ...previous[0], updatedAt: latest }]);
  assert.equal(result[0].updatedAt, latest);
  assert.notEqual(result[0], previous[0]);
});
