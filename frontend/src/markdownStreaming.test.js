import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const markdown = readFileSync(new URL("./app/components/MarkdownContent.jsx", import.meta.url), "utf8");
const css = readFileSync(new URL("./index.css", import.meta.url), "utf8");

test("assistant markdown stays formatted while content is streaming", () => {
  assert.match(markdown, /import \{ Streamdown \} from "streamdown"/);
  assert.match(markdown, /mode=\{streaming \? "streaming" : "static"\}/);
  assert.match(markdown, /isAnimating=\{streaming\}/);
  assert.match(markdown, /caret=\{streaming \? "block" : undefined\}/);
  assert.doesNotMatch(markdown, /if \(streaming\)[\s\S]*markdown-plain/);
});

test("streaming markdown preserves the desktop link and image safety policy", () => {
  assert.match(markdown, /components=\{MARKDOWN_COMPONENTS\}/);
  assert.match(markdown, /skipHtml/);
  assert.match(markdown, /img: ImagePlaceholder/);
  assert.match(markdown, /a: SafeLink/);
});

test("markdown selection uses one theme-aware treatment across nested fragments", () => {
  assert.match(css, /@source "\.\.\/node_modules\/streamdown\/dist\/\*\.js"/);
  assert.match(css, /\.markdown-content::selection,\s*\.markdown-content \*::selection\s*\{/);
  assert.match(css, /background:\s*color-mix\(in oklab, var\(--primary\) 24%, var\(--background\)\)/);
  assert.match(css, /\.markdown-content :is\(ul, ol, li, pre, code\)::selection/);
  assert.match(css, /background:\s*transparent;\s*color:\s*var\(--info\)/);
  assert.match(css, /text-shadow:\s*none/);
});
