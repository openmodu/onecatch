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
  assert.match(markdown, /const renderedText = streaming \? text\.replace\(\/\\s\+\$\/u, ""\) : text/);
  assert.match(markdown, />\{renderedText\}<\/Streamdown>/);
  assert.doesNotMatch(markdown, /if \(streaming\)[\s\S]*markdown-plain/);
});

test("streaming caret stays compact, inline, and motion-safe", () => {
  assert.match(css, /\.markdown-content\.streaming > :last-child::after\s*\{[^}]*content:\s*"" !important[^}]*display:\s*inline-block !important[^}]*width:\s*1\.5px[^}]*height:\s*0\.88em/s);
  assert.match(css, /background:\s*color-mix\(in oklab, var\(--primary\) 72%, transparent\)/);
  assert.match(css, /@media \(prefers-reduced-motion:\s*reduce\)\s*\{[\s\S]*?\.markdown-content\.streaming > :last-child::after\s*\{[^}]*animation:\s*none/s);
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

test("code blocks have an accessible copy action outside their scrolling content", () => {
  assert.match(markdown, /pre: PlainPre/);
  assert.match(markdown, /className="markdown-code-toolbar"[\s\S]*?<Button type="button"[^>]*aria-label=\{copyLabel\}[^>]*disabled=\{copyState === "copying"\}[^>]*onClick=\{copyCode\}/);
  assert.match(markdown, /role="status" aria-live="polite"/);
  assert.match(markdown, /<\/Button>[\s\S]*?<\/div>\s*<pre \{\.\.\.props\} ref=\{preRef\}>\{content\}<\/pre>/);
  assert.match(css, /\.markdown-code-toolbar\s*\{[^}]*justify-content:\s*space-between/s);
  assert.match(css, /\.markdown-code-toolbar,\s*\.markdown-code-toolbar \*\s*\{[^}]*-webkit-user-select:\s*none[^}]*user-select:\s*none/s);
  assert.match(css, /\.markdown-code-toolbar::selection,\s*\.markdown-code-toolbar \*::selection\s*\{[^}]*background:\s*transparent/s);
  assert.match(css, /\.markdown-content pre\s*\{[^}]*overflow-x:\s*auto/s);
});

test("code toolbar shows a language label and an accessible line-wrap toggle", () => {
  assert.match(markdown, /className="markdown-code-language"[\s\S]*?<CodeXml[^>]*aria-hidden="true"/);
  assert.match(markdown, /codeLanguageFromClassName\(children\.props\.className\)/);
  assert.match(markdown, /aria-pressed=\{wrapped\} onClick=\{\(\) => setWrapped\(\(current\) => !current\)\}/);
  assert.match(markdown, /wrapped \? " is-wrapped" : ""/);
  assert.match(css, /\.markdown-content \.markdown-code-block\.is-wrapped pre code\s*\{[^}]*white-space:\s*pre-wrap[^}]*overflow-wrap:\s*anywhere/s);
});

test("only block code uses memoized syntax highlighting with theme-aware token colors", () => {
  assert.match(markdown, /const block = props\["data-block"\] !== undefined/);
  assert.match(markdown, /useMemo\(\(\) => block && typeof children === "string" \? highlightCode\(children, language\) : null, \[block, children, language\]\)/);
  assert.match(markdown, /dangerouslySetInnerHTML=\{\{ __html: highlighted\.html \}\}/);
  assert.match(css, /\.markdown-code-block \.token\.key\s*\{[^}]*color:\s*var\(--warning\)/);
  assert.match(css, /\.markdown-code-block \.token:is\([^)]*\.string[^)]*\)\s*\{[^}]*color:\s*var\(--success\)/);
});

test("code copy preserves current code text and falls back to the browser clipboard", () => {
  assert.match(markdown, /const text = preRef\.current\?\.textContent \?\? ""/);
  assert.match(markdown, /await Clipboard\.SetText\(text\)/);
  assert.match(markdown, /if \(!navigator\.clipboard\?\.writeText\) throw wailsError/);
  assert.match(markdown, /await navigator\.clipboard\.writeText\(text\)/);
  assert.match(markdown, /setCopyState\("copied"\);\s*\} catch \{\s*setCopyState\("error"\)/);
  assert.match(markdown, /window\.setTimeout\(\(\) => setCopyState\("idle"\), 2000\)/);
  assert.match(markdown, /return \(\) => window\.clearTimeout\(timer\)/);
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
