import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const sidebar = readFileSync(new URL("./components/Sidebar.jsx", import.meta.url), "utf8");
const css = readFileSync(new URL("../index.css", import.meta.url), "utf8");

test("collapsed sidebar and its hover reveal stay above a maximized terminal", () => {
  assert.match(sidebar, /sidebar-shell relative z-50/);
  assert.match(css, /\.terminal-dock\.is-maximized\s*\{[\s\S]*?z-index:\s*40;/);
});
