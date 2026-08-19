import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const dock = readFileSync(new URL("./components/TerminalDock.jsx", import.meta.url), "utf8");
const pane = readFileSync(new URL("./components/TerminalPane.jsx", import.meta.url), "utf8");
const css = readFileSync(new URL("../index.css", import.meta.url), "utf8");

test("terminal tabs remain mounted while inactive", () => {
  assert.match(dock, /rootTabs\.map\(\(rootTab\) =>/);
  assert.match(dock, /terminal-tab-layout \$\{active \? "active" : ""\}/);
  assert.match(dock, /<TerminalPane[^>]+active=\{active\}/);
  assert.match(css, /\.terminal-tab-layout\s*\{[\s\S]*?display:\s*none;/);
  assert.match(css, /\.terminal-tab-layout\.active\s*\{[\s\S]*?display:\s*block;/);
});

test("reactivating a terminal refits and repaints its preserved buffer", () => {
  assert.match(pane, /useEffect\(\(\) => \{\s*if \(!active\) return undefined;/);
  assert.match(pane, /fit\.fit\(\);\s*terminal\.refresh\(0, terminal\.rows - 1\);/);
});
