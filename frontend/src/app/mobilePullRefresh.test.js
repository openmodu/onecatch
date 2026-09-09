import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { PULL_THRESHOLD, pullOffset } from "./mobilePullRefresh.js";

const sourceURL = new URL("./MobileApp.jsx", import.meta.url);
const hookURL = new URL("./mobilePullRefresh.js", import.meta.url);

test("the pull lags the finger and stops short of taking over the screen", () => {
  assert.equal(pullOffset(0), 0);
  assert.equal(pullOffset(-40), 0, "scrolling up is not a pull");
  assert.equal(pullOffset(40), 20);
  assert.equal(pullOffset(400), 96, "a long drag stops at the indicator's height");
  assert.ok(pullOffset(PULL_THRESHOLD * 2) >= PULL_THRESHOLD, "the threshold has to be reachable");
});

// The shell disables WebKit's bounce, so this gesture has no platform default
// to fall back on and must not swallow the ones that do.
test("the pull only starts at the top, and never fights the back swipe", async () => {
  const hook = await readFile(hookURL, "utf8");
  assert.match(hook, /element\.scrollTop > 0\) return;/);
  assert.match(hook, /Math\.abs\(deltaX\) > Math\.abs\(deltaY\)/);
  assert.match(hook, /event\.touches\.length !== 1/);
  // Claiming the drag needs a listener the browser will let us cancel.
  assert.match(hook, /addEventListener\("touchmove", move, \{ passive: false \}\)/);
});

test("both lists refresh by pulling, and a refresh re-reads what a poll would", async () => {
  const source = await readFile(sourceURL, "utf8");
  assert.match(source, /useMobilePullToRefresh\(listRef, refreshLists, view === "projects" \|\| view === "sessions"\)/);
  const projects = source.match(/view === "projects" \? <main className="mobile-main"[^>]*>/)[0];
  assert.match(projects, /ref=\{listRef\}/);
  const sessions = source.match(/view === "sessions" \? <main className="mobile-main"[^>]*>/)[0];
  assert.match(sessions, /ref=\{listRef\}/);
  const refresh = source.match(/const refreshLists = useCallback\([\s\S]*?\}, \[loadWorkspaces, refreshWorker, selectedWorkerID\]\);/)[0];
  assert.match(refresh, /refreshWorker\(selectedWorkerID, true\)/);
  assert.match(refresh, /loadWorkspaces\(selectedWorkerID, true\)/);
  assert.match(refresh, /ListRuns\(\)/);
});
