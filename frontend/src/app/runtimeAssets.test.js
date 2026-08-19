import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("the Codex runtime mark has a transparent background", async () => {
  const icon = await readFile(new URL("../../public/assets/runtime/codex.svg", import.meta.url), "utf8");

  assert.doesNotMatch(icon, /fill=["']#(?:fff|ffffff)["']/i);
  assert.match(icon, /fill=["']url\(#codex-gradient\)["']/);
});

test("narrow inspector toolbars preserve the window actions", async () => {
  const styles = await readFile(new URL("../index.css", import.meta.url), "utf8");
  const tabs = styles.match(/\.workbench-inspector-tabs\s*\{([^}]*)\}/)?.[1] || "";
  const tabButton = styles.match(/\.workbench-inspector-tabs button\s*\{([^}]*)\}/)?.[1] || "";
  const actions = styles.match(/\.workbench-inspector-window-actions\s*\{([^}]*)\}/)?.[1] || "";

  assert.match(tabs, /min-width:\s*0/);
  assert.match(tabs, /flex:\s*1 1 auto/);
  assert.match(tabs, /overflow-x:\s*auto/);
  assert.match(tabButton, /flex:\s*0 0 auto/);
  assert.match(actions, /flex:\s*0 0 auto/);
});
