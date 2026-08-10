import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const css = readFileSync(new URL("../mobile.css", import.meta.url), "utf8");
const mobileApp = readFileSync(new URL("../../../internal/app/mobile/mobile.go", import.meta.url), "utf8");

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

test("mobile form controls do not trigger iOS focus zoom", () => {
  assert.match(css, /\.mobile-app-shell input,\s*\.mobile-app-shell textarea,\s*\.mobile-app-shell select\s*\{[^}]*font-size:\s*16px/s);
  assert.match(css, /\.mobile-search-pill input\s*\{[^}]*font-size:\s*16px/s);
  assert.match(css, /\.mobile-composer textarea\s*\{[^}]*font-size:\s*16px/s);
  assert.match(css, /\.mobile-sheet input,\s*\.mobile-sheet select\s*\{[^}]*font-size:\s*16px/s);
});
