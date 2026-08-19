import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const css = readFileSync(new URL("../index.css", import.meta.url), "utf8");

test("detached inspector keeps its bottom inset inside the window", () => {
  const rule = css.match(/\.inspector-window-panel\s*\{([\s\S]*?)\}/)?.[1] || "";
  assert.match(rule, /height:\s*auto;/);
  assert.match(rule, /min-height:\s*0;/);
  assert.match(rule, /margin:\s*0 8px 8px;/);
  assert.doesNotMatch(rule, /height:\s*100%;/);
});
