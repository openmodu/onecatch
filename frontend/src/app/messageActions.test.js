import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const css = readFileSync(new URL("../index.css", import.meta.url), "utf8");

test("message hover actions have no dead zone before the copy button", () => {
  assert.match(css, /\.conversation-message-actions\s*\{[\s\S]*?height:\s*24px;[\s\S]*?padding-top:\s*0;/);
  assert.match(css, /\.conversation-user \.conversation-message-actions,[\s\S]*?top:\s*100%;/);
  assert.doesNotMatch(css, /top:\s*calc\(100%\s*\+\s*3px\)/);
});
