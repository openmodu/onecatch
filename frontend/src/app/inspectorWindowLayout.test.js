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

// Both dropdowns are full-width selects whose items are written to truncate,
// but shadcn's SelectContent defaults to an item-aligned popper that sizes
// itself to its content. A branch name beside its `origin/…` upstream measured
// 878px against a 402px trigger, which is how the branch picker escaped the
// panel it belongs to. Pinning the content to the trigger is what lets the
// truncation the items already carry actually apply.
test("select dropdowns are pinned to their trigger's width", () => {
  const primitives = readFileSync(new URL("../ui/primitives.jsx", import.meta.url), "utf8");
  const settingsControls = readFileSync(new URL("./components/settings/SettingsControls.jsx", import.meta.url), "utf8");
  for (const [name, source] of [["TUISelect", primitives], ["SettingsSelect", settingsControls]]) {
    // The width variable only exists in popper mode, so both are required.
    assert.match(source, /<SelectContent position="popper" className="w-\(--radix-select-trigger-width\)">/,
      `${name} must pin its dropdown to the trigger width`);
    // Pinning only helps because the label gives way before the meta does.
    assert.match(source, /className="min-w-0 flex-1 truncate"/, `${name} label must truncate`);
    assert.match(source, /className="max-w-\[50%\] shrink-0 truncate/, `${name} meta must stay bounded`);
  }
});
