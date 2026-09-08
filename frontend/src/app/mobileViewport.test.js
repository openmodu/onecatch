import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { isPinnedToBottom, viewportFrameFrom } from "./mobileViewport.js";

const css = readFileSync(new URL("../mobile.css", import.meta.url), "utf8");
const mobileApp = readFileSync(new URL("../../../internal/app/mobile/mobile.go", import.meta.url), "utf8");
const indexHTML = readFileSync(new URL("../../index.html", import.meta.url), "utf8");
const workbench = readFileSync(new URL("./MobileApp.jsx", import.meta.url), "utf8");

test("mobile shell prevents root bounce and horizontal scrolling", () => {
  assert.match(mobileApp, /DisableBounce:\s+true/);
  assert.match(css, /\.mobile-app-shell\s*\{[^}]*overflow:\s*hidden[^}]*overscroll-behavior:\s*none/s);
  assert.match(css, /\.mobile-main\s*\{[^}]*overflow-x:\s*hidden[^}]*overflow-y:\s*auto[^}]*overscroll-behavior:\s*none[^}]*touch-action:\s*pan-y/s);
});

test("empty conversations stay fixed while populated transcripts only scroll vertically", () => {
  assert.match(css, /\.mobile-conversation-shell\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\)[^}]*overflow:\s*hidden[^}]*overscroll-behavior:\s*none/s);
  assert.match(css, /\.mobile-transcript\s*\{[^}]*overflow:\s*hidden[^}]*overscroll-behavior:\s*none/s);
  assert.match(css, /\.mobile-transcript\.has-messages\s*\{[^}]*overflow-x:\s*hidden[^}]*overflow-y:\s*auto[^}]*touch-action:\s*pan-y/s);
  assert.match(css, /\.mobile-transcript\.empty\s*\{[^}]*touch-action:\s*none/s);
  assert.match(css, /\.mobile-composer-wrap\s*\{[^}]*max-width:\s*100%[^}]*overflow:\s*hidden[^}]*overscroll-behavior:\s*none/s);
});

test("the mobile shell locks page scale so a pinch cannot strand it zoomed in", () => {
  const guard = indexHTML.match(/get\("platform"\) === "mobile"\)\s*\{[\s\S]*?\n {8}\}/);
  assert.ok(guard, "index.html must special-case the mobile platform before first paint");
  assert.match(guard[0], /maximum-scale=1/);
  assert.match(guard[0], /user-scalable=no/);
  assert.match(guard[0], /viewport-fit=cover/);
  assert.match(guard[0], /gesturestart/);
  assert.match(css, /\.mobile-app-shell\s*\{[^}]*touch-action:\s*manipulation/s);
});

test("the shell follows the visible viewport while the keyboard is open", () => {
  assert.match(css, /\.mobile-app-shell\s*\{[^}]*top:\s*var\(--mobile-viewport-top\)[^}]*height:\s*var\(--mobile-viewport-height\)/s);
  assert.match(workbench, /useMobileViewportFrame\(shellRef\)/);
  assert.match(workbench, /className="mobile-app-shell" ref=\{shellRef\}/);
});

test("viewportFrameFrom tracks both keyboard shrinkage and WebKit panning", () => {
  assert.deepEqual(viewportFrameFrom({ height: 844, offsetTop: 0 }, 844), { height: 844, top: 0 });
  assert.deepEqual(viewportFrameFrom({ height: 508.4, offsetTop: 35.6 }, 844), { height: 508, top: 36 });
  assert.deepEqual(viewportFrameFrom({ height: 508, offsetTop: -3 }, 844), { height: 508, top: 0 }, "rubber-band offsets stay on-screen");
  assert.deepEqual(viewportFrameFrom(null, 844), { height: 844, top: 0 });
});

test("native iOS hides the web form navigation toolbar", () => {
  assert.match(mobileApp, /DisableInputAccessoryView:\s+true/);
});

test("streamed output follows the run only while the reader stays at the bottom", () => {
  assert.equal(isPinnedToBottom({ scrollHeight: 2000, scrollTop: 1400, clientHeight: 600 }), true);
  assert.equal(isPinnedToBottom({ scrollHeight: 2000, scrollTop: 900, clientHeight: 600 }), false);
  assert.equal(isPinnedToBottom(null), true);
});

test("destructive mobile actions ask in-app, since iOS has no window.confirm", () => {
  const code = workbench.split("\n").filter((line) => !line.trim().startsWith("//")).join("\n");
  assert.doesNotMatch(code, /window\.confirm/);
  assert.match(workbench, /function ConfirmSheet\(/);
  assert.match(workbench, /<ConfirmSheet request=\{confirmRequest\}/);
});

test("mobile form controls do not trigger iOS focus zoom", () => {
  assert.match(css, /\.mobile-app-shell input,\s*\.mobile-app-shell textarea,\s*\.mobile-app-shell select\s*\{[^}]*font-size:\s*16px/s);
  assert.match(css, /\.mobile-search-pill input\s*\{[^}]*font-size:\s*16px/s);
  assert.match(css, /\.mobile-composer textarea\s*\{[^}]*font-size:\s*16px/s);
  assert.match(css, /\.mobile-sheet input,\s*\.mobile-sheet select\s*\{[^}]*font-size:\s*16px/s);
});
