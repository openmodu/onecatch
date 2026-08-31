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

test("markdown tables use one application-owned scrolling frame", () => {
  assert.match(markdown, /function PlainTable/);
  assert.match(markdown, /className="markdown-table-scroll"/);
  assert.match(markdown, /table: PlainTable/);
  assert.match(css, /\.markdown-table-scroll\s*\{[^}]*overflow-x:\s*auto[^}]*border:\s*1px solid var\(--border\)/s);
  assert.match(css, /\.markdown-content table\s*\{[^}]*margin:\s*0[^}]*table-layout:\s*auto/s);
});

test("markdown headings respect the configured conversation font size", () => {
  for (const level of [1, 2, 3, 4]) assert.match(markdown, new RegExp(`h${level}: PlainH${level}`));
  assert.match(css, /\.markdown-content h1,[\s\S]*?\.markdown-content h4\s*\{[^}]*font-size:\s*inherit/s);
  assert.doesNotMatch(css, /\.markdown-content h[1-4]\s*\{[^}]*font-size:\s*[\d.]+em/s, "headings must not multiply the user's chat font size");
});

test("markdown selection uses one theme-aware treatment across nested fragments", () => {
  assert.match(css, /@source "\.\.\/node_modules\/streamdown\/dist\/\*\.js"/);
  assert.match(css, /\.markdown-content::selection\s*\{/);
  assert.match(css, /background:\s*color-mix\(in oklab, var\(--primary\) 28%, transparent\)/);
  assert.doesNotMatch(css, /\.markdown-content \*::selection/, "block descendants must inherit the root highlight instead of painting full-width selection bands");
  assert.doesNotMatch(css, /\.markdown-content :is\(ul, ol, li, pre, code\)::selection/, "code and list selections must not punch transparent gaps through the shared highlight");
  assert.match(css, /text-shadow:\s*none/);
});
