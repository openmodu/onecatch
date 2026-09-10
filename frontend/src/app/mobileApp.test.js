import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const sources = [new URL("./MobileApp.jsx", import.meta.url), new URL("./MobileUsageBoard.jsx", import.meta.url)];

// A component or hook that is used but never imported compiles cleanly and
// blanks the whole app at runtime — twice now. The suite cannot mount the
// shell (no DOM here), so it checks the one thing that failed: every name the
// file renders or calls as a hook has to come from somewhere.
test("every component and hook the phone renders is declared or imported", async () => {
  for (const sourceURL of sources) await assertDeclared(await readFile(sourceURL, "utf8"), sourceURL.pathname);
});

async function assertDeclared(source, where) {
  const declared = new Set();
  for (const [, name] of source.matchAll(/(?:^|\n)(?:export default |export )?function ([A-Z]\w*|use[A-Z]\w*)\s*\(/g)) declared.add(name);
  for (const [, name] of source.matchAll(/(?:const|let) ([A-Z]\w*|use[A-Z]\w*)\s*=/g)) declared.add(name);
  for (const [, names] of source.matchAll(/import\s+\{([^}]+)\}\s+from/g)) {
    for (const entry of names.split(",")) {
      const name = entry.trim().split(/\s+as\s+/).pop().trim();
      if (name) declared.add(name);
    }
  }
  for (const [, name] of source.matchAll(/import\s+([A-Za-z_$][\w$]*)\s*(?:,|from)/g)) declared.add(name);
  // A renamed destructure — `{ icon: Icon }` — declares the new name too.
  for (const [, name] of source.matchAll(/:\s*([A-Z]\w*)\s*[,}=]/g)) declared.add(name);

  const used = new Set();
  for (const [, name] of source.matchAll(/<([A-Z]\w*)/g)) used.add(name);
  for (const [, name] of source.matchAll(/\b(use[A-Z]\w*)\s*\(/g)) used.add(name);
  const missing = [...used].filter((name) => !declared.has(name)).sort();
  assert.deepEqual(missing, [], `${where}: used but never declared or imported: ${missing.join(", ")}`);
}

// The board is the desktop's, ported: the same four numbers, the same year of
// squares, the same daily list — and a range the phone can change.
test("the usage board carries what the desktop board shows", async () => {
  const source = await readFile(new URL("./MobileUsageBoard.jsx", import.meta.url), "utf8");
  assert.match(source, /buildUsageHeatmap/, "the year of activity is the point of the chart");
  assert.match(source, /combineAccountUsage/, "the combined view is what 全部 means");
  for (const metric of ["lifetimeTokens", "peakDailyTokens", "currentStreakDays", "longestStreakDays"]) {
    assert.match(source, new RegExp(`summary\\?\\.${metric}`), `${metric} is missing from the metrics`);
  }
  assert.match(source, /recentDailyUsage\(daily, new Date\(\), range\)/, "the daily list follows the chosen range");
  // Node cannot import JSX, so the ranges are read from the source itself.
  assert.match(source, /export const USAGE_RANGES = \[14, 30\];/);
  // A phone opens the year at its end; a year ago is not what the reader wants.
  assert.match(source, /element\.scrollLeft = element\.scrollWidth/);
});

test("the drawer lists the usage board next to the projects", async () => {
  const source = await readFile(new URL("./MobileApp.jsx", import.meta.url), "utf8");
  const drawer = source.slice(source.indexOf("function Sidebar("), source.indexOf("function MoreMenu("));
  assert.match(drawer, /onUsage\(\); onClose\(\);/);
  assert.match(drawer, /<Gauge \/>用量/);
});

// A long prompt folds the way the desktop's does: measured, faded, and opened
// by a button that says which way it goes.
test("a long user message folds behind a disclosure", async () => {
  const source = await readFile(new URL("./MobileApp.jsx", import.meta.url), "utf8");
  const message = source.slice(source.indexOf("function UserMessage("), source.indexOf("function AssistantMessage("));
  assert.match(message, /body\.scrollHeight > body\.clientHeight \+ 1/, "the fold is measured, not guessed from the text length");
  assert.match(message, /is-collapsed/);
  assert.match(message, /显示更多/);
  assert.match(message, /收起/);
  const css = await readFile(new URL("../mobile.css", import.meta.url), "utf8");
  assert.match(css, /\.mobile-user-message-body\.is-collapsed \{[^}]*max-height/, "nothing caps the bubble, so nothing ever overflows to measure");
  assert.match(css, /\.mobile-message-disclosure \{/);
});

// ConversationTurn is memoised so a poll or a keystroke re-renders only what
// changed. A callback rebuilt on every render silently defeats that and takes
// every Markdown tree in the conversation down with it.
test("the transcript's callbacks survive a re-render", async () => {
  const source = await readFile(new URL("./MobileApp.jsx", import.meta.url), "utf8");
  assert.match(source, /const loadEarlier = useCallback\(/, "ConversationView rebuilt onLoadEarlier on every render");
  assert.match(source, /const loadEarlierRun = useCallback\(/, "the workbench rebuilt onLoadEarlier on every render");
  assert.match(source, /const respondPermission = useCallback\(/);
  assert.match(source, /const transcriptNotice = useCallback\(/);
});

// The send has to show up before the host answers: the round trip is seconds.
test("a sent prompt renders before the host answers", async () => {
  const source = await readFile(new URL("./MobileApp.jsx", import.meta.url), "utf8");
  const start = source.slice(source.indexOf("const startRun = async"), source.indexOf("const loadEarlierRun"));
  assert.match(start, /setPendingPrompt\(\{[^}]*text[^}]*\}\);\n\s*setPrompt\(""\);/, "the bubble and the empty composer come before the await");
  assert.ok(start.indexOf("setPendingPrompt({") < start.indexOf("await MobileBinding.StartRun"), "the pending bubble waits for the host");
  assert.match(start, /setPrompt\(\(current\) => current \|\| text\)/, "a failed send has to hand the text back");
});

// A message typed while the agent is working goes to the host's queue instead
// of finding a dead send button.
test("a message sent mid-turn is queued on the host", async () => {
  const source = await readFile(new URL("./MobileApp.jsx", import.meta.url), "utf8");
  const view = source.slice(source.indexOf("function ConversationView("), source.indexOf("// Projects fold"));
  assert.match(view, /const queueable = Boolean\(running && sharedRuns\)/, "only a host-run turn can hold a queue");
  assert.match(view, /onQueue\(running\.id\)/);
  assert.match(view, /onDequeue\(running\.id, item\.id\)/, "a queued message has to be withdrawable");
  assert.match(view, /className="mobile-queued"/);
  const workbench = source.slice(source.indexOf("const queueFollowUp"), source.indexOf("const loadEarlierRun"));
  assert.match(workbench, /MobileBinding\.QueueFollowUp\(runID, text\)/);
  assert.match(workbench, /MobileBinding\.DequeueFollowUp\(runID, instructionID\)/);
});
