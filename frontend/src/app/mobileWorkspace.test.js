import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const sourceURL = new URL("./MobileApp.jsx", import.meta.url);

test("mobile Workspace management closes the API-to-page loop", async () => {
  const source = await readFile(sourceURL, "utf8");
  assert.match(source, /function WorkspaceManagerPage\(/);
  assert.match(source, /function WorkspaceEditorSheet\(/);
  assert.match(source, /MobileBinding\.PrepareWorkspace\(/);
  assert.match(source, /MobileBinding\.RemoveWorkspace\(/);
  assert.match(source, /MobileBinding\.WorkspaceGitStatus\(/);
  assert.match(source, /克隆 Git 仓库/);
  assert.match(source, /绑定已有目录/);
  assert.match(source, /同时删除远端克隆/);
});

// A project shared by the desktop hosting the Worker is usable from the phone
// but owned by the computer, so the phone must not offer to rename or unmap it.
test("workspaces shared by the desktop are read-only on the phone", async () => {
  const source = await readFile(sourceURL, "utf8");
  assert.match(source, /workspace\.shared && <p className="mobile-workspace-shared"/);
  assert.match(source, /\{!workspace\.shared && <Button[^>]*aria-label=\{`编辑/);
  // Pairing no longer starts with a terminal command on the remote machine.
  assert.match(source, /设置 › 手机连接/);
});

// The project list is the first screen after connecting, so it stays a name
// and one quiet line — no leading glyph, no chevron, the whole row tappable.
test("project rows carry a name and its recent activity, nothing else", async () => {
  const source = await readFile(sourceURL, "utf8");
  const row = source.match(/<button type="button" className="mobile-project-link"[\s\S]*?<\/button>/)[0];
  assert.match(row, /className="mobile-project-name"/);
  assert.match(row, /relativeTime\(latestAt\)/, "a row says when the project last moved");
  assert.doesNotMatch(row, /<Folder \/>/, "the folder glyph repeats what the page already says");
  assert.doesNotMatch(row, /<ChevronRight \/>/, "a chevron adds nothing when the row itself is the target");
});

// With more than one computer paired, switching machines belongs on the screen
// the projects are listed on, not buried in the run-settings sheet.
test("the paired machine is a switcher in the top bar", async () => {
  const source = await readFile(sourceURL, "utf8");
  assert.match(source, /function WorkerSwitchSheet\(/);
  // The machine sits on the line under the page's name, where the page says
  // where it runs — not squeezed into the action bar.
  assert.match(source, /const switchWorker = workers\.length > 1 \? \(\) => setWorkerSwitchOpen\(true\) : null;/);
  assert.match(source, /<PageHead title="项目" meta=\{meta\} onMeta=\{onMeta\} \/>/);
  // Switching scope must not double as starting a task.
  const select = source.match(/const selectWorker = \(id\) => \{[\s\S]*?\n  \};/)[0];
  assert.match(select, /setView\("projects"\)/);
  assert.doesNotMatch(select, /setView\("conversation"\)/);
  // Dots only mean something if every machine is polled.
  assert.match(source, /if \(workers\.length < 2\) return undefined;/);
});

// `files.length && <div/>` renders the number 0 when the list is empty, which
// put a stray "0" above the composer for every clean workspace.
test("a clean workspace does not leak a bare count into the composer", async () => {
  const source = await readFile(sourceURL, "utf8");
  const alert = source.match(/\{[^\n]*mobile-workspace-alert[^\n]*\}/)[0];
  assert.match(alert, /^\{Boolean\(/, "the guard ends in a number, so it has to be cast before React sees it");
});

// The reply is what the reader came for; everything around it stays quiet.
test("the transcript reads as prose, not as a stack of cards", async () => {
  const source = await readFile(sourceURL, "utf8");
  const css = await readFile(new URL("../mobile.css", sourceURL), "utf8");
  assert.doesNotMatch(source, /mobile-agent-mark/, "an avatar on every reply is a column of noise");
  assert.doesNotMatch(source, /mobile-turn-meta/, "the runtime and time already sit in the title bar");
  // Events collapse to one muted line with no border of their own.
  assert.doesNotMatch(css, /\.mobile-event-detail, \.mobile-event-line \{[^}]*border:/, "an event row is a line of text, not a box");
  assert.match(css, /\.mobile-event-detail pre \{[^}]*background: transparent/);
  // The read-only caption is said once, on the empty screen.
  assert.match(source, /Agent 在远端以只读模式运行/);
  assert.doesNotMatch(css, /\.mobile-composer-wrap > p/);
});

// Connecting to a worker and counting tokens are facts about the machinery.
// A row for each turned a two-line answer into a page of scaffolding.
test("the transcript drops plumbing events and names what a tool touched", async () => {
  const source = await readFile(sourceURL, "utf8");
  const runs = await readFile(new URL("./mobileRuns.js", sourceURL), "utf8");
  assert.match(runs, /TRANSCRIPT_NOISE = new Set\(\[[^\]]*"result"\]\)/);
  assert.doesNotMatch(source, /usage: "用量"/);
  assert.doesNotMatch(source, /started: "已连接 Worker"/);
  // A row that opens onto nothing is not a control.
  assert.match(source, /if \(!summary\.expandable\) return <div className=\{`mobile-event-line/);
  assert.match(source, /<code>\{summary\.detail\}<\/code>/);
});

// The title bar names the workspace, so the page below it should add where the
// workspace lives — and a path cut from the right loses exactly that.
test("a workspace path is shortened from its middle, not its end", async () => {
  const source = await readFile(sourceURL, "utf8");
  assert.doesNotMatch(source, /mobile-workspace-heading/, "the heading repeated the title bar");
  // The path sits under the name it describes, where the session count used to
  // repeat what the list below already shows.
  // The page states where it lives; the bar carries only actions.
  assert.match(source, /<PageHead title=\{workspaceLabel\(workspace\)\} meta=\{shortenPath\(workspace\?\.path\)\} \/>/);
  assert.doesNotMatch(source, /个会话`;/);
  assert.match(source, /\{shortenPath\(workspace\.path, 3\)\}/, "the manage card has room for one more segment");
});

// The bar's 44px targets are wider than their glyphs, so a bar and a page that
// both padded by "20px" still put the arrow 8px right of the title under it.
test("every layer shares one gutter, and the bar offsets for its targets", async () => {
  const css = await readFile(new URL("../mobile.css", sourceURL), "utf8");
  assert.match(css, /--m-gutter: 24px;/);
  assert.match(css, /--m-bar-inset: calc\(var\(--m-gutter\) - 16px\);/);
  assert.match(css, /\.mobile-topbar \{[^}]*padding: calc\(14px \+ env\(safe-area-inset-top\)\) var\(--m-bar-inset\) 0;/s);
  for (const layer of ["\\.mobile-page \\{", "\\.mobile-bottom-bar \\{", "\\.mobile-composer-wrap \\{", "\\.mobile-sheet \\{"]) {
    assert.match(css, new RegExp(`${layer}[^}]*var\\(--m-gutter\\)`, "s"), `${layer} must use the shared gutter`);
  }
});

// Removing the native appearance takes the picker's arrow with it, and a
// select with no arrow reads as a field you cannot change.
test("a picker still looks like a picker after appearance is reset", async () => {
  const css = await readFile(new URL("../mobile.css", sourceURL), "utf8");
  assert.match(css, /\.mobile-sheet select \{[^}]*background-image: url\("data:image\/svg\+xml/s);
  assert.match(css, /\.mobile-sheet select \{[^}]*padding-right: 40px/s);
});
